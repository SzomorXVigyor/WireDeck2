package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/modbus"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/ownership"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/sitecache"
)

// SampleHook lets Phase 4 History Ingester & Alarm Engine subscribe to every polled sample.
type SampleHook func(sample ValueSample, mapping models.RegisterMapping)

// Scheduler implements demand-driven Modbus polling (D6, D17, D26):
//   - Polls only mappings with active demand (interactive WS + 30s grace, or archival when historyEnabled).
//   - Never polls access_mode == "WRITE".
//   - Primes newly subscribed mappings immediately for <1s interactive startup.
//   - Coalesces contiguous reads per device and applies exponential backoff (1s -> 60s) on failures.
type Scheduler struct {
	db            *bun.DB
	pool          *modbus.DevicePool
	cache         *sitecache.Cache
	hub           *TelemetryHub
	owner         ownership.Provider
	graceDuration time.Duration
	devDirectDial bool

	mu             sync.Mutex
	devices        map[uuid.UUID]models.Device
	mappings       map[uuid.UUID]models.RegisterMapping
	siteHistory    map[uuid.UUID]bool
	graceUntil     map[uuid.UUID]time.Time
	lastPolled     map[uuid.UUID]time.Time
	deviceBackoff  map[uuid.UUID]backoffState
	deviceInFlight map[uuid.UUID]bool
	hooks          []SampleHook

	primeCh chan struct{}
}

type backoffState struct {
	failures  int
	nextRetry time.Time
}

func NewScheduler(
	db *bun.DB,
	pool *modbus.DevicePool,
	cache *sitecache.Cache,
	hub *TelemetryHub,
	owner ownership.Provider,
	graceDuration time.Duration,
	devDirectDial bool,
) *Scheduler {
	if graceDuration <= 0 {
		graceDuration = 30 * time.Second
	}
	return &Scheduler{
		db:             db,
		pool:           pool,
		cache:          cache,
		hub:            hub,
		owner:          owner,
		graceDuration:  graceDuration,
		devDirectDial:  devDirectDial,
		devices:        make(map[uuid.UUID]models.Device),
		mappings:       make(map[uuid.UUID]models.RegisterMapping),
		siteHistory:    make(map[uuid.UUID]bool),
		graceUntil:     make(map[uuid.UUID]time.Time),
		lastPolled:     make(map[uuid.UUID]time.Time),
		deviceBackoff:  make(map[uuid.UUID]backoffState),
		deviceInFlight: make(map[uuid.UUID]bool),
		primeCh:        make(chan struct{}, 1),
	}
}

func (s *Scheduler) AddSampleHook(h SampleHook) {
	s.mu.Lock()
	s.hooks = append(s.hooks, h)
	s.mu.Unlock()
}

// NotifySubscribed triggers immediate prime-on-subscribe polling for newly watched mappings.
func (s *Scheduler) NotifySubscribed(_ uuid.UUID, mappingIDs []uuid.UUID) {
	now := time.Now()
	s.mu.Lock()
	for _, id := range mappingIDs {
		s.graceUntil[id] = now.Add(s.graceDuration)
		delete(s.lastPolled, id) // Force immediate read on next tick
	}
	s.mu.Unlock()
	s.TriggerImmediate()
}

func (s *Scheduler) TriggerImmediate() {
	select {
	case s.primeCh <- struct{}{}:
	default:
	}
}

// ReloadConfig refreshes active devices, register mappings, and per-site history flags from OLTP DB.
func (s *Scheduler) ReloadConfig(ctx context.Context) error {
	var devs []models.Device
	if err := s.db.NewSelect().Model(&devs).Where("is_active = TRUE").Scan(ctx); err != nil {
		return err
	}
	var maps []models.RegisterMapping
	if err := s.db.NewSelect().Model(&maps).Where("is_active = TRUE").Scan(ctx); err != nil {
		return err
	}
	var settings []models.SiteWebviewSettings
	if err := s.db.NewSelect().Model(&settings).Scan(ctx); err != nil {
		return err
	}

	devMap := make(map[uuid.UUID]models.Device, len(devs))
	for _, d := range devs {
		devMap[d.ID] = d
	}
	mapMap := make(map[uuid.UUID]models.RegisterMapping, len(maps))
	for _, m := range maps {
		if _, ok := devMap[m.DeviceID]; ok {
			mapMap[m.ID] = m
		}
	}
	histMap := make(map[uuid.UUID]bool, len(settings))
	for _, st := range settings {
		histMap[st.SiteID] = st.HistoryEnabled
	}

	s.mu.Lock()
	s.devices = devMap
	s.mappings = mapMap
	s.siteHistory = histMap
	s.mu.Unlock()
	return nil
}

// Run starts the demand-driven scheduler loop until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	_ = s.ReloadConfig(ctx)

	tick := time.NewTicker(250 * time.Millisecond)
	reloadTick := time.NewTicker(30 * time.Second)
	reapTick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	defer reloadTick.Stop()
	defer reapTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadTick.C:
			if err := s.ReloadConfig(ctx); err != nil {
				slog.Warn("scheduler_reload_failed", "error", err)
			}
		case <-reapTick.C:
			s.pool.ReapIdle()
		case <-s.primeCh:
			s.pollDue(ctx)
		case <-tick.C:
			s.pollDue(ctx)
		}
	}
}

func (s *Scheduler) pollDue(ctx context.Context) {
	now := time.Now()
	activeSubs := s.hub.ActiveMappingIDs()

	// Determine due mappings per device
	dueByDevice := make(map[uuid.UUID][]models.RegisterMapping)
	devSnapshots := make(map[uuid.UUID]models.Device)

	s.mu.Lock()
	// Extend grace windows for actively subscribed mappings
	for id := range activeSubs {
		s.graceUntil[id] = now.Add(s.graceDuration)
	}

	for id, m := range s.mappings {
		// D17: Never poll write-only registers
		if m.AccessMode == "WRITE" {
			continue
		}
		if !s.owner.OwnsSite(m.SiteID) {
			continue
		}

		// Skip disabled or offline sites (unless DEV_DIRECT_DIAL)
		if !s.devDirectDial {
			st, ok := s.cache.GetRuntimeState(m.SiteID)
			if !ok || !st.IsActive || !st.WebviewEnabled || st.Status == "OFFLINE" {
				continue
			}
		}

		// Compute effective demand interval = min(interactive, archival)
		var effInterval time.Duration
		_, isSub := activeSubs[id]
		inGrace := now.Before(s.graceUntil[id])
		if isSub || inGrace {
			effInterval = time.Duration(m.InteractivePollIntervalMs) * time.Millisecond
			if effInterval < 250*time.Millisecond {
				effInterval = 250 * time.Millisecond
			}
		}
		if s.siteHistory[m.SiteID] {
			arch := time.Duration(m.ArchivalPollIntervalMs) * time.Millisecond
			if arch < time.Second {
				arch = time.Second
			}
			if effInterval == 0 || arch < effInterval {
				effInterval = arch
			}
		}

		// Zero demand -> never poll!
		if effInterval == 0 {
			continue
		}

		if last, ok := s.lastPolled[id]; ok && now.Sub(last) < effInterval {
			continue
		}

		// Respect device backoff & in-flight guard
		if s.deviceInFlight[m.DeviceID] {
			continue
		}
		if bo, ok := s.deviceBackoff[m.DeviceID]; ok && now.Before(bo.nextRetry) {
			continue
		}

		dev, ok := s.devices[m.DeviceID]
		if !ok {
			continue
		}
		dueByDevice[m.DeviceID] = append(dueByDevice[m.DeviceID], m)
		devSnapshots[m.DeviceID] = dev
	}

	for devID, list := range dueByDevice {
		s.deviceInFlight[devID] = true
		for _, m := range list {
			s.lastPolled[m.ID] = now
		}
	}
	s.mu.Unlock()

	for devID, list := range dueByDevice {
		dev := devSnapshots[devID]
		go s.pollDevice(ctx, dev, list)
	}
}

func (s *Scheduler) pollDevice(ctx context.Context, dev models.Device, mappings []models.RegisterMapping) {
	defer func() {
		s.mu.Lock()
		delete(s.deviceInFlight, dev.ID)
		s.mu.Unlock()
	}()

	batches := modbus.CoalesceReads(mappings)
	now := time.Now().UTC()
	var samples []ValueSample
	var hadTransportErr bool

	for _, batch := range batches {
		reqPDU := modbus.BuildReadPDU(batch.FunctionCode, batch.StartAddress, batch.Quantity)
		respPDU, err := s.pool.Exchange(ctx, dev, reqPDU)
		if err != nil {
			hadTransportErr = true
			for _, m := range batch.Mappings {
				samples = append(samples, ValueSample{
					SiteID:    m.SiteID,
					MappingID: m.ID,
					Quality:   QualityBad,
					Error:     err.Error(),
					Timestamp: now,
				})
			}
			continue
		}
		payload, err := modbus.ParseReadResponsePDU(batch.FunctionCode, respPDU)
		if err != nil {
			for _, m := range batch.Mappings {
				samples = append(samples, ValueSample{
					SiteID:    m.SiteID,
					MappingID: m.ID,
					Quality:   QualityBad,
					Error:     err.Error(),
					Timestamp: now,
				})
			}
			continue
		}

		for _, m := range batch.Mappings {
			offset := m.RegisterAddress - int(batch.StartAddress)
			val, decErr := modbus.DecodeValue(m, payload, offset)
			if decErr != nil {
				samples = append(samples, ValueSample{
					SiteID:    m.SiteID,
					MappingID: m.ID,
					Quality:   QualityBad,
					Error:     decErr.Error(),
					Timestamp: now,
				})
				continue
			}
			vCopy := val
			samples = append(samples, ValueSample{
				SiteID:    m.SiteID,
				MappingID: m.ID,
				Value:     &vCopy,
				Quality:   QualityGood,
				Timestamp: now,
			})
		}
	}

	// Update per-device exponential backoff (1s -> 60s)
	s.mu.Lock()
	if hadTransportErr {
		bo := s.deviceBackoff[dev.ID]
		bo.failures++
		delay := time.Second << min(bo.failures-1, 6)
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
		bo.nextRetry = time.Now().Add(delay)
		s.deviceBackoff[dev.ID] = bo
	} else {
		delete(s.deviceBackoff, dev.ID)
	}
	hooks := append([]SampleHook(nil), s.hooks...)
	mappingLookup := make(map[uuid.UUID]models.RegisterMapping, len(mappings))
	for _, m := range mappings {
		mappingLookup[m.ID] = m
	}
	s.mu.Unlock()

	if len(samples) > 0 {
		s.hub.Publish(samples)
		for _, sample := range samples {
			if m, ok := mappingLookup[sample.MappingID]; ok {
				for _, hook := range hooks {
					hook(sample, m)
				}
			}
		}
	}
}
