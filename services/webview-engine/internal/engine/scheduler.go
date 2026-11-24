package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/ownership"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/provider"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/sitecache"
)

const (
	defaultInteractiveInterval = 1000 * time.Millisecond
	maxInteractiveInterval     = 60000 * time.Millisecond
	deviceFastPollThreshold    = 100 // Up to 100 active registers per device run at full requested rate
)

// effectiveInteractiveFloor computes the minimum safe interactive poll interval
// for a device based on how many active registers are currently demanded on it:
//   - 1..100 registers: 1000ms (honors dashboard setting down to 1000ms)
//   - >100 registers: scales up by +1000ms per additional 100 registers (capped at 60000ms)
//     so querying large register sets never overloads the remote PLC/gateway.
func effectiveInteractiveFloor(activeRegisterCount int) time.Duration {
	if activeRegisterCount <= deviceFastPollThreshold {
		return defaultInteractiveInterval
	}
	blocks := (activeRegisterCount + deviceFastPollThreshold - 1) / deviceFastPollThreshold
	floor := time.Duration(blocks) * defaultInteractiveInterval
	if floor > maxInteractiveInterval {
		return maxInteractiveInterval
	}
	return floor
}

// SampleHook lets Phase 4 History Ingester & Alarm Engine subscribe to polled samples.
type SampleHook func(sample ValueSample, mapping models.RegisterMapping, historyEntry *models.HistoryEntry)

// Scheduler implements demand-driven polling via protocol Providers:
//   - Polls a register ONLY if a dashboard is open somewhere (active WebSocket subscription
//     + grace period) OR it has an active history_entries row (at its configured pollIntervalSeconds).
//   - Primes newly subscribed registers immediately for <1s interactive startup.
//   - Delegates batch reads to the device's protocol Provider with exponential backoff (1s -> 60s).
type Scheduler struct {
	db            *bun.DB
	providers     *provider.Registry
	cache         *sitecache.Cache
	hub           *TelemetryHub
	owner         ownership.Provider
	graceDuration time.Duration
	devDirectDial bool

	mu             sync.Mutex
	devices        map[uuid.UUID]models.Device
	mappings       map[uuid.UUID]models.RegisterMapping
	historyEntries map[uuid.UUID]models.HistoryEntry // keyed by register_mapping_id
	alarmIntervals map[uuid.UUID]time.Duration       // keyed by register_mapping_id (min poll interval across active rules)
	graceUntil     map[uuid.UUID]time.Time
	graceInterval  map[uuid.UUID]time.Duration
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
	providers *provider.Registry,
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
		providers:      providers,
		cache:          cache,
		hub:            hub,
		owner:          owner,
		graceDuration:  graceDuration,
		devDirectDial:  devDirectDial,
		devices:        make(map[uuid.UUID]models.Device),
		mappings:       make(map[uuid.UUID]models.RegisterMapping),
		historyEntries: make(map[uuid.UUID]models.HistoryEntry),
		alarmIntervals: make(map[uuid.UUID]time.Duration),
		graceUntil:     make(map[uuid.UUID]time.Time),
		graceInterval:  make(map[uuid.UUID]time.Duration),
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

func (s *Scheduler) NotifySubscribed(_ uuid.UUID, mappingIDs []uuid.UUID) {
	now := time.Now()
	s.mu.Lock()
	for _, id := range mappingIDs {
		s.graceUntil[id] = now.Add(s.graceDuration)
		delete(s.lastPolled, id)
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

func (s *Scheduler) ReloadConfig(ctx context.Context) error {
	var devs []models.Device
	if err := s.db.NewSelect().Model(&devs).Where("is_active = TRUE").Scan(ctx); err != nil {
		return err
	}
	var maps []models.RegisterMapping
	if err := s.db.NewSelect().Model(&maps).Where("is_active = TRUE").Scan(ctx); err != nil {
		return err
	}
	var entries []models.HistoryEntry
	if err := s.db.NewSelect().Model(&entries).Where("is_active = TRUE").Scan(ctx); err != nil {
		return err
	}
	var alarmRules []models.AlarmRule
	if err := s.db.NewSelect().Model(&alarmRules).Where("is_active = TRUE").Scan(ctx); err != nil {
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
	histMap := make(map[uuid.UUID]models.HistoryEntry, len(entries))
	for _, he := range entries {
		if _, ok := mapMap[he.RegisterMappingID]; ok {
			histMap[he.RegisterMappingID] = he
		}
	}
	alarmMap := make(map[uuid.UUID]time.Duration)
	for _, ar := range alarmRules {
		if _, ok := mapMap[ar.RegisterMappingID]; !ok {
			continue
		}
		sec := ar.PollIntervalSeconds
		if sec < 1 {
			sec = 10
		}
		dur := time.Duration(sec) * time.Second
		if prev, exists := alarmMap[ar.RegisterMappingID]; !exists || dur < prev {
			alarmMap[ar.RegisterMappingID] = dur
		}
	}

	s.mu.Lock()
	s.devices = devMap
	s.mappings = mapMap
	s.historyEntries = histMap
	s.alarmIntervals = alarmMap
	s.mu.Unlock()
	return nil
}

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
			s.providers.ReapIdle()
		case <-s.primeCh:
			s.pollDue(ctx)
		case <-tick.C:
			s.pollDue(ctx)
		}
	}
}

func (s *Scheduler) pollDue(ctx context.Context) {
	now := time.Now()
	activeIntervals := s.hub.ActiveMappingIntervals()

	dueByDevice := make(map[uuid.UUID][]models.RegisterMapping)
	devSnapshots := make(map[uuid.UUID]models.Device)

	s.mu.Lock()
	for id, dur := range activeIntervals {
		s.graceUntil[id] = now.Add(s.graceDuration)
		s.graceInterval[id] = dur
	}

	// Count active demanded registers per device to apply overload protection above 100 entries
	activeCountByDevice := make(map[uuid.UUID]int)
	for id, m := range s.mappings {
		if !s.owner.OwnsSite(m.SiteID) {
			continue
		}
		_, isSub := activeIntervals[id]
		inGrace := now.Before(s.graceUntil[id])
		_, hasHist := s.historyEntries[id]
		_, hasAlarm := s.alarmIntervals[id]
		if isSub || inGrace || hasHist || hasAlarm {
			activeCountByDevice[m.DeviceID]++
		}
	}

	for id, m := range s.mappings {
		if !s.owner.OwnsSite(m.SiteID) {
			continue
		}

		if !s.devDirectDial {
			st, ok := s.cache.GetRuntimeState(m.SiteID)
			if !ok || !st.IsActive || !st.WebviewEnabled || st.Status == "OFFLINE" {
				continue
			}
		}

		// Demand check (D17): ONLY poll if dashboard is open (WS active/grace) OR has active history entry OR active alarm rule
		var effInterval time.Duration
		subDur, isSub := activeIntervals[id]
		inGrace := now.Before(s.graceUntil[id])
		if isSub || inGrace {
			requested := subDur
			if !isSub {
				requested = s.graceInterval[id]
			}
			if requested < defaultInteractiveInterval {
				requested = defaultInteractiveInterval
			}
			floor := effectiveInteractiveFloor(activeCountByDevice[m.DeviceID])
			if requested < floor {
				requested = floor
			}
			effInterval = requested
		}
		if he, hasHist := s.historyEntries[id]; hasHist {
			arch := time.Duration(he.PollIntervalSeconds) * time.Second
			if arch < time.Second {
				arch = time.Second
			}
			if effInterval == 0 || arch < effInterval {
				effInterval = arch
			}
		}
		if alarmDur, hasAlarm := s.alarmIntervals[id]; hasAlarm {
			if alarmDur < time.Second {
				alarmDur = time.Second
			}
			if effInterval == 0 || alarmDur < effInterval {
				effInterval = alarmDur
			}
		}

		if effInterval == 0 {
			continue
		}

		if last, ok := s.lastPolled[id]; ok && now.Sub(last) < effInterval {
			continue
		}
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

	prov, err := s.providers.Get(dev.Protocol)
	if err != nil {
		slog.Warn("provider_not_found", "device_id", dev.ID, "protocol", dev.Protocol)
		return
	}

	results, hadTransportErr := prov.ReadBatch(ctx, dev, mappings)
	now := time.Now().UTC()

	samples := make([]ValueSample, 0, len(results))
	for _, r := range results {
		if r.Err != nil {
			samples = append(samples, ValueSample{
				SiteID:    dev.SiteID,
				MappingID: r.MappingID,
				Quality:   QualityBad,
				Error:     r.Err.Error(),
				Timestamp: now,
			})
			continue
		}
		samples = append(samples, ValueSample{
			SiteID:    dev.SiteID,
			MappingID: r.MappingID,
			Value:     r.Value,
			Quality:   QualityGood,
			Timestamp: now,
		})
	}

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
	histLookup := make(map[uuid.UUID]*models.HistoryEntry, len(mappings))
	for _, m := range mappings {
		if he, ok := s.historyEntries[m.ID]; ok {
			cp := he
			histLookup[m.ID] = &cp
		}
	}
	s.mu.Unlock()

	if len(samples) > 0 {
		s.hub.Publish(samples)
		for _, sample := range samples {
			if m, ok := mappingLookup[sample.MappingID]; ok {
				for _, hook := range hooks {
					hook(sample, m, histLookup[sample.MappingID])
				}
			}
		}
	}
}
