package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/wireguard"
	"wiredeck/services/webview-engine/internal/alarm"
	"wiredeck/services/webview-engine/internal/history"
	"wiredeck/services/webview-engine/internal/models"
	"wiredeck/services/webview-engine/internal/provider"
	"wiredeck/services/webview-engine/internal/sitecache"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("entry already exists on this site")
	ErrHostOutOfSubnets = errors.New("device host IP is outside the site's allowed LAN subnets")
	ErrReadOnlyMapping  = errors.New("register mapping is read-only")
	ErrWriteForbidden   = errors.New("user does not have write permission on this site")
	ErrInvalidTicket    = errors.New("invalid or expired WebSocket ticket")
	ErrForbidden        = errors.New("site WebView access denied")
)

type Service struct {
	db            *bun.DB
	providers     *provider.Registry
	cache         *sitecache.Cache
	hub           *TelemetryHub
	scheduler     *Scheduler
	histStore     history.Store
	alarmEng      *alarm.Engine
	devDirectDial bool
}

func (s *Service) SetHistoryStore(store history.Store) {
	s.histStore = store
}

func (s *Service) SetAlarmEngine(eng *alarm.Engine) {
	s.alarmEng = eng
}

func NewService(
	db *bun.DB,
	providers *provider.Registry,
	cache *sitecache.Cache,
	hub *TelemetryHub,
	scheduler *Scheduler,
	devDirectDial bool,
) *Service {
	return &Service{
		db:            db,
		providers:     providers,
		cache:         cache,
		hub:           hub,
		scheduler:     scheduler,
		devDirectDial: devDirectDial,
	}
}

func (s *Service) reloadAsync() {
	if s.scheduler != nil || s.alarmEng != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if s.scheduler != nil {
				_ = s.scheduler.ReloadConfig(ctx)
			}
			if s.alarmEng != nil {
				_ = s.alarmEng.ReloadRules(ctx)
			}
		}()
	}
}

// --- History Entries (Per-register key + poll interval + retention) ---

func (s *Service) ListHistoryEntries(ctx context.Context, siteID uuid.UUID) ([]models.HistoryEntry, error) {
	var list []models.HistoryEntry
	err := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("created_at ASC").Scan(ctx)
	return list, err
}

func (s *Service) CreateHistoryEntry(ctx context.Context, siteID, mappingID uuid.UUID, pollSec, retentionDays int) (*models.HistoryEntry, error) {
	exists, err := s.db.NewSelect().Model((*models.RegisterMapping)(nil)).
		Where("id = ? AND site_id = ?", mappingID, siteID).Exists(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if pollSec < 1 {
		pollSec = 60
	}
	if retentionDays < 1 {
		retentionDays = 30
	}
	now := time.Now().UTC()
	he := &models.HistoryEntry{
		ID:                  uuid.New(),
		SiteID:              siteID,
		RegisterMappingID:   mappingID,
		PollIntervalSeconds: pollSec,
		RetentionDays:       retentionDays,
		IsActive:            true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if _, err := s.db.NewInsert().Model(he).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	return he, nil
}

func (s *Service) UpdateHistoryEntry(ctx context.Context, siteID, entryID uuid.UUID, pollSec *int, retentionDays *int, isActive *bool) (*models.HistoryEntry, error) {
	he := new(models.HistoryEntry)
	if err := s.db.NewSelect().Model(he).Where("id = ? AND site_id = ?", entryID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if pollSec != nil {
		he.PollIntervalSeconds = *pollSec
	}
	if retentionDays != nil {
		he.RetentionDays = *retentionDays
	}
	if isActive != nil {
		he.IsActive = *isActive
	}
	he.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(he).WherePK().Exec(ctx); err != nil {
		return nil, err
	}
	s.reloadAsync()
	return he, nil
}

func (s *Service) DeleteHistoryEntry(ctx context.Context, siteID, entryID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.HistoryEntry)(nil)).
		Where("id = ? AND site_id = ?", entryID, siteID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.reloadAsync()
	return nil
}

// --- Service-Level Fine-Grained SITE_MEMBER Write Policies ---

func (s *Service) ListMemberPolicies(ctx context.Context, siteID uuid.UUID) ([]models.MemberPolicy, error) {
	var list []models.MemberPolicy
	err := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("updated_at DESC").Scan(ctx)
	return list, err
}

func (s *Service) UpsertMemberPolicy(ctx context.Context, siteID, userID uuid.UUID, canWrite bool) (*models.MemberPolicy, error) {
	p := &models.MemberPolicy{
		ID:        uuid.New(),
		SiteID:    siteID,
		UserID:    userID,
		CanWrite:  canWrite,
		UpdatedAt: time.Now().UTC(),
	}
	_, err := s.db.NewInsert().Model(p).
		On("CONFLICT (site_id, user_id) DO UPDATE").
		Set("can_write = EXCLUDED.can_write, updated_at = EXCLUDED.updated_at").
		Returning("*").
		Exec(ctx)
	return p, err
}

func (s *Service) DeleteMemberPolicy(ctx context.Context, siteID, userID uuid.UUID) error {
	_, err := s.db.NewDelete().Model((*models.MemberPolicy)(nil)).
		Where("site_id = ? AND user_id = ?", siteID, userID).Exec(ctx)
	return err
}

func (s *Service) CanUserWrite(ctx context.Context, siteID, userID uuid.UUID, authz *internalclient.SiteAuthzResponse) (bool, error) {
	if authz == nil {
		return false, nil
	}
	if authz.IsPlatformAdmin || authz.Role == "SITE_OPERATOR" {
		return true, nil
	}
	var p models.MemberPolicy
	err := s.db.NewSelect().Model(&p).Where("site_id = ? AND user_id = ?", siteID, userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return p.CanWrite, nil
}

// --- Devices CRUD (strictly name, host, port, protocol) ---

func (s *Service) ListDevices(ctx context.Context, siteID uuid.UUID) ([]models.Device, error) {
	var list []models.Device
	err := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("name ASC").Scan(ctx)
	return list, err
}

type CreateDeviceInput struct {
	SiteID     uuid.UUID
	Name       string
	Host       string
	Port       int
	Protocol   string
	LanSubnets []string
}

func (s *Service) CreateDevice(ctx context.Context, in CreateDeviceInput) (*models.Device, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 {
		return nil, errors.New("device name must be between 1 and 100 characters")
	}
	host := strings.TrimSpace(in.Host)
	if _, err := netip.ParseAddr(host); err != nil {
		return nil, errors.New("host must be a valid IPv4 or IPv6 address")
	}
	if !s.devDirectDial && len(in.LanSubnets) > 0 && !wireguard.ContainsHost(host, in.LanSubnets) {
		return nil, ErrHostOutOfSubnets
	}
	proto := in.Protocol
	if proto == "" {
		proto = provider.ProtocolModbusTCP
	}
	if _, err := s.providers.Get(proto); err != nil {
		return nil, err
	}
	port := in.Port
	if port == 0 {
		port = 502
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("port must be between 1 and 65535")
	}
	now := time.Now().UTC()
	dev := &models.Device{
		ID:        uuid.New(),
		SiteID:    in.SiteID,
		Name:      name,
		Host:      host,
		Port:      port,
		Protocol:  proto,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.db.NewInsert().Model(dev).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	return dev, nil
}

type UpdateDeviceInput struct {
	Name       *string
	Host       *string
	Port       *int
	Protocol   *string
	IsActive   *bool
	LanSubnets []string
}

func (s *Service) UpdateDevice(ctx context.Context, siteID, deviceID uuid.UUID, in UpdateDeviceInput) (*models.Device, error) {
	dev := new(models.Device)
	if err := s.db.NewSelect().Model(dev).Where("id = ? AND site_id = ?", deviceID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 100 {
			return nil, errors.New("device name must be between 1 and 100 characters")
		}
		dev.Name = name
	}
	if in.Host != nil {
		h := strings.TrimSpace(*in.Host)
		if _, err := netip.ParseAddr(h); err != nil {
			return nil, errors.New("host must be a valid IPv4 or IPv6 address")
		}
		if !s.devDirectDial && len(in.LanSubnets) > 0 && !wireguard.ContainsHost(h, in.LanSubnets) {
			return nil, ErrHostOutOfSubnets
		}
		dev.Host = h
	}
	if in.Port != nil {
		if *in.Port < 1 || *in.Port > 65535 {
			return nil, errors.New("port must be between 1 and 65535")
		}
		dev.Port = *in.Port
	}
	if in.Protocol != nil {
		if _, err := s.providers.Get(*in.Protocol); err != nil {
			return nil, err
		}
		dev.Protocol = *in.Protocol
	}
	if in.IsActive != nil {
		dev.IsActive = *in.IsActive
	}
	dev.UpdatedAt = time.Now().UTC()

	if _, err := s.db.NewUpdate().Model(dev).
		Column("name", "host", "port", "protocol", "is_active", "updated_at").
		WherePK().Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	return dev, nil
}

func (s *Service) DeleteDevice(ctx context.Context, siteID, deviceID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.Device)(nil)).Where("id = ? AND site_id = ?", deviceID, siteID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.reloadAsync()
	return nil
}

// --- Register Mappings CRUD (dynamic protocolAttributes validated by Provider) ---

func (s *Service) ListMappings(ctx context.Context, siteID uuid.UUID) ([]models.RegisterMapping, error) {
	var list []models.RegisterMapping
	err := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("name ASC").Scan(ctx)
	return list, err
}

func (s *Service) CreateMapping(ctx context.Context, siteID, deviceID uuid.UUID, name string, rawAttrs map[string]any) (*models.RegisterMapping, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" || len(trimmedName) > 100 {
		return nil, errors.New("register name must be between 1 and 100 characters")
	}
	dev := new(models.Device)
	if err := s.db.NewSelect().Model(dev).Where("id = ? AND site_id = ?", deviceID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	prov, err := s.providers.Get(dev.Protocol)
	if err != nil {
		return nil, err
	}
	normAttrs, sum, err := prov.ValidateAttributes(rawAttrs)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	m := &models.RegisterMapping{
		ID:                 uuid.New(),
		DeviceID:           deviceID,
		SiteID:             siteID,
		Name:               trimmedName,
		ProtocolAttributes: normAttrs,
		FunctionCode:       sum.FunctionCode,
		RegisterAddress:    sum.RegisterAddress,
		RegisterCount:      sum.RegisterCount,
		DataType:           sum.DataType,
		WordOrder:          sum.WordOrder,
		AccessMode:         sum.AccessMode,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	return m, nil
}

type UpdateMappingInput struct {
	DeviceID           *uuid.UUID
	Name               *string
	ProtocolAttributes map[string]any
	IsActive           *bool
}

func (s *Service) UpdateMapping(ctx context.Context, siteID, mappingID uuid.UUID, in UpdateMappingInput) (*models.RegisterMapping, error) {
	m := new(models.RegisterMapping)
	if err := s.db.NewSelect().Model(m).Where("id = ? AND site_id = ?", mappingID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if in.DeviceID != nil {
		m.DeviceID = *in.DeviceID
	}
	dev := new(models.Device)
	if err := s.db.NewSelect().Model(dev).Where("id = ? AND site_id = ?", m.DeviceID, siteID).Scan(ctx); err != nil {
		return nil, ErrNotFound
	}
	if in.Name != nil {
		trimmedName := strings.TrimSpace(*in.Name)
		if trimmedName == "" || len(trimmedName) > 100 {
			return nil, errors.New("register name must be between 1 and 100 characters")
		}
		m.Name = trimmedName
	}
	if in.ProtocolAttributes != nil {
		prov, err := s.providers.Get(dev.Protocol)
		if err != nil {
			return nil, err
		}
		normAttrs, sum, err := prov.ValidateAttributes(in.ProtocolAttributes)
		if err != nil {
			return nil, err
		}
		m.ProtocolAttributes = normAttrs
		m.FunctionCode = sum.FunctionCode
		m.RegisterAddress = sum.RegisterAddress
		m.RegisterCount = sum.RegisterCount
		m.DataType = sum.DataType
		m.WordOrder = sum.WordOrder
		m.AccessMode = sum.AccessMode
	}
	if in.IsActive != nil {
		m.IsActive = *in.IsActive
	}
	m.UpdatedAt = time.Now().UTC()

	if _, err := s.db.NewUpdate().Model(m).
		Column("device_id", "name", "protocol_attributes", "function_code", "register_address", "register_count", "data_type", "word_order", "access_mode", "is_active", "updated_at").
		WherePK().Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	return m, nil
}

func (s *Service) DeleteMapping(ctx context.Context, siteID, mappingID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.RegisterMapping)(nil)).
		Where("id = ? AND site_id = ?", mappingID, siteID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.reloadAsync()
	return nil
}

// TestRead performs a live single-register read via the device's protocol Provider.
func (s *Service) TestRead(ctx context.Context, siteID, mappingID uuid.UUID) (*ValueSample, error) {
	m := new(models.RegisterMapping)
	if err := s.db.NewSelect().Model(m).Where("id = ? AND site_id = ?", mappingID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	dev := new(models.Device)
	if err := s.db.NewSelect().Model(dev).Where("id = ?", m.DeviceID).Scan(ctx); err != nil {
		return nil, ErrNotFound
	}
	prov, err := s.providers.Get(dev.Protocol)
	if err != nil {
		return nil, err
	}
	val, err := prov.ReadOne(ctx, *dev, *m)
	if err != nil {
		return &ValueSample{SiteID: siteID, MappingID: mappingID, Quality: QualityBad, Error: err.Error(), Timestamp: time.Now().UTC()}, nil
	}
	sample := ValueSample{SiteID: siteID, MappingID: mappingID, Value: &val, Quality: QualityGood, Timestamp: time.Now().UTC()}
	s.hub.Publish([]ValueSample{sample})
	return &sample, nil
}

// --- Dashboards & Widgets CRUD + Reorder ---

func (s *Service) ListDashboards(ctx context.Context, siteID uuid.UUID) ([]models.Dashboard, error) {
	var list []models.Dashboard
	err := s.db.NewSelect().Model(&list).
		Relation("Widgets", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("sort_order ASC", "created_at ASC")
		}).
		Where("db.site_id = ?", siteID).
		Order("db.sort_order ASC", "db.created_at ASC").
		Scan(ctx)
	return list, err
}

func (s *Service) CreateDashboard(ctx context.Context, siteID uuid.UUID, name string, isDefault bool, pollIntervalMs int, layout map[string]any) (*models.Dashboard, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" || len(trimmedName) > 100 {
		return nil, errors.New("dashboard name must be between 1 and 100 characters")
	}
	if pollIntervalMs < 1000 {
		pollIntervalMs = 1000
	} else if pollIntervalMs > 60000 {
		pollIntervalMs = 60000
	}
	if layout == nil {
		layout = map[string]any{}
	}
	cnt, _ := s.db.NewSelect().Model((*models.Dashboard)(nil)).Where("site_id = ?", siteID).Count(ctx)
	now := time.Now().UTC()
	d := &models.Dashboard{
		ID:             uuid.New(),
		SiteID:         siteID,
		Name:           trimmedName,
		Layout:         layout,
		IsDefault:      isDefault,
		SortOrder:      cnt,
		PollIntervalMs: pollIntervalMs,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if isDefault {
			if _, err := tx.NewUpdate().Model((*models.Dashboard)(nil)).
				Set("is_default = FALSE, updated_at = ?", now).
				Where("site_id = ? AND is_default = TRUE", siteID).
				Exec(ctx); err != nil {
				return err
			}
		}
		_, err := tx.NewInsert().Model(d).Exec(ctx)
		return err
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return d, nil
}

func (s *Service) UpdateDashboard(ctx context.Context, siteID, dashboardID uuid.UUID, name *string, isDefault *bool, sortOrder *int, pollIntervalMs *int, layout map[string]any) (*models.Dashboard, error) {
	d := new(models.Dashboard)
	if err := s.db.NewSelect().Model(d).Where("id = ? AND site_id = ?", dashboardID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if name != nil {
		trimmedName := strings.TrimSpace(*name)
		if trimmedName == "" || len(trimmedName) > 100 {
			return nil, errors.New("dashboard name must be between 1 and 100 characters")
		}
		d.Name = trimmedName
	}
	if isDefault != nil {
		d.IsDefault = *isDefault
	}
	if sortOrder != nil {
		d.SortOrder = *sortOrder
	}
	if pollIntervalMs != nil {
		v := *pollIntervalMs
		if v < 1000 {
			v = 1000
		} else if v > 60000 {
			v = 60000
		}
		d.PollIntervalMs = v
	}
	if layout != nil {
		d.Layout = layout
	}
	now := time.Now().UTC()
	d.UpdatedAt = now
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if isDefault != nil && *isDefault {
			if _, err := tx.NewUpdate().Model((*models.Dashboard)(nil)).
				Set("is_default = FALSE, updated_at = ?", now).
				Where("site_id = ? AND id <> ? AND is_default = TRUE", siteID, dashboardID).
				Exec(ctx); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model(d).
			Column("name", "layout", "is_default", "sort_order", "poll_interval_ms", "updated_at").
			WherePK().Exec(ctx)
		return err
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return d, nil
}

func (s *Service) ReorderDashboards(ctx context.Context, siteID uuid.UUID, orderedIDs []uuid.UUID) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now().UTC()
		for idx, id := range orderedIDs {
			if _, err := tx.NewUpdate().Model((*models.Dashboard)(nil)).
				Set("sort_order = ?, updated_at = ?", idx, now).
				Where("id = ? AND site_id = ?", id, siteID).
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) DeleteDashboard(ctx context.Context, siteID, dashboardID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.Dashboard)(nil)).
		Where("id = ? AND site_id = ?", dashboardID, siteID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func validateWidgetConfig(w *models.Widget) error {
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" || len(w.Name) > 100 {
		return errors.New("tile title must be between 1 and 100 characters")
	}
	if w.WidgetType != "graph" || w.Extra == nil {
		return nil
	}
	if rawSeries, ok := w.Extra["series"]; ok && rawSeries != nil {
		list, ok := rawSeries.([]any)
		if !ok {
			return errors.New("graph series must be a list of registers")
		}
		if len(list) == 0 || len(list) > 5 {
			return errors.New("graph tile supports between 1 and 5 registers")
		}
		seen := make(map[string]struct{}, len(list))
		for _, item := range list {
			entry, ok := item.(map[string]any)
			if !ok {
				return errors.New("invalid graph series entry")
			}
			rawID, _ := entry["registerMappingId"].(string)
			rawID = strings.TrimSpace(rawID)
			if _, err := uuid.Parse(rawID); err != nil {
				return errors.New("each graph series must reference a valid register")
			}
			if _, dup := seen[rawID]; dup {
				return errors.New("duplicate registers are not allowed in the same graph tile")
			}
			seen[rawID] = struct{}{}
			if lbl, ok := entry["label"].(string); ok && len(strings.TrimSpace(lbl)) > 64 {
				return errors.New("graph series label must be 64 characters or fewer")
			}
			if u, ok := entry["unit"].(string); ok && len(strings.TrimSpace(u)) > 32 {
				return errors.New("graph series unit must be 32 characters or fewer")
			}
			if rawPrec, ok := entry["precision"]; ok && rawPrec != nil {
				if p, ok := rawPrec.(float64); !ok || math.Trunc(p) != p || p < -5 || p > 5 {
					return errors.New("graph series precision must be an integer between -5 and 5")
				}
			}
		}
	}
	if rawRange, ok := w.Extra["timeRange"].(string); ok && strings.TrimSpace(rawRange) != "" {
		switch strings.TrimSpace(rawRange) {
		case "5m", "15m", "1h", "6h", "24h", "7d":
		default:
			return errors.New("graph timeRange must be one of 5m, 15m, 1h, 6h, 24h, 7d")
		}
	}
	yMin, hasMin := w.Extra["yMin"].(float64)
	yMax, hasMax := w.Extra["yMax"].(float64)
	if hasMin && (math.IsNaN(yMin) || math.IsInf(yMin, 0)) {
		return errors.New("graph Y-axis min must be a finite number")
	}
	if hasMax && (math.IsNaN(yMax) || math.IsInf(yMax, 0)) {
		return errors.New("graph Y-axis max must be a finite number")
	}
	if hasMin && hasMax && yMin >= yMax {
		return errors.New("graph Y-axis min must be less than max")
	}
	return nil
}

func (s *Service) CreateWidget(ctx context.Context, siteID, dashboardID uuid.UUID, w *models.Widget) (*models.Widget, error) {
	exists, err := s.db.NewSelect().Model((*models.Dashboard)(nil)).
		Where("id = ? AND site_id = ?", dashboardID, siteID).Exists(ctx)
	if err != nil || !exists {
		return nil, ErrNotFound
	}
	cnt, _ := s.db.NewSelect().Model((*models.Widget)(nil)).Where("dashboard_id = ?", dashboardID).Count(ctx)
	w.ID = uuid.New()
	w.DashboardID = dashboardID
	if err := validateWidgetConfig(w); err != nil {
		return nil, err
	}
	if w.SortOrder == 0 {
		w.SortOrder = cnt
	}
	if w.Style == nil {
		w.Style = map[string]any{}
	}
	if w.Extra == nil {
		w.Extra = map[string]any{}
	}
	w.CreatedAt = time.Now().UTC()
	w.UpdatedAt = w.CreatedAt
	if _, err := s.db.NewInsert().Model(w).Exec(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *Service) UpdateWidget(ctx context.Context, siteID, widgetID uuid.UUID, patch func(*models.Widget)) (*models.Widget, error) {
	w := new(models.Widget)
	err := s.db.NewSelect().Model(w).
		Join("JOIN dashboards AS d ON d.id = w.dashboard_id").
		Where("w.id = ? AND d.site_id = ?", widgetID, siteID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	patch(w)
	if err := validateWidgetConfig(w); err != nil {
		return nil, err
	}
	w.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(w).
		Column("name", "widget_type", "register_mapping_id", "sort_order", "style", "extra", "updated_at").
		WherePK().Exec(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *Service) ReorderWidgets(ctx context.Context, siteID, dashboardID uuid.UUID, orderedIDs []uuid.UUID) error {
	exists, err := s.db.NewSelect().Model((*models.Dashboard)(nil)).
		Where("id = ? AND site_id = ?", dashboardID, siteID).Exists(ctx)
	if err != nil || !exists {
		return ErrNotFound
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now().UTC()
		for idx, id := range orderedIDs {
			if _, err := tx.NewUpdate().Model((*models.Widget)(nil)).
				Set("sort_order = ?, updated_at = ?", idx, now).
				Where("id = ? AND dashboard_id = ?", id, dashboardID).
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) DeleteWidget(ctx context.Context, siteID, widgetID uuid.UUID) error {
	w := new(models.Widget)
	err := s.db.NewSelect().Model(w).
		Join("JOIN dashboards AS d ON d.id = w.dashboard_id").
		Where("w.id = ? AND d.site_id = ?", widgetID, siteID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	_, err = s.db.NewDelete().Model((*models.Widget)(nil)).Where("id = ?", widgetID).Exec(ctx)
	return err
}

// --- REST Register Write (D15, D16) with optional server-side Button pulse (1 -> triggerTimeMs -> 0) ---

func (s *Service) WriteRegister(ctx context.Context, siteID, userID uuid.UUID, authz *internalclient.SiteAuthzResponse, mappingID uuid.UUID, value float64, pulseMs int) error {
	canWrite, err := s.CanUserWrite(ctx, siteID, userID, authz)
	if err != nil {
		return err
	}
	if !canWrite {
		slog.Warn("register_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "result", "forbidden")
		return ErrWriteForbidden
	}

	m := new(models.RegisterMapping)
	if err := s.db.NewSelect().Model(m).Where("id = ? AND site_id = ? AND is_active = TRUE", mappingID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if m.AccessMode == "READ" {
		return ErrReadOnlyMapping
	}

	dev := new(models.Device)
	if err := s.db.NewSelect().Model(dev).Where("id = ? AND is_active = TRUE", m.DeviceID).Scan(ctx); err != nil {
		return ErrNotFound
	}
	prov, err := s.providers.Get(dev.Protocol)
	if err != nil {
		return err
	}

	if err := prov.WriteOne(ctx, *dev, *m, value); err != nil {
		slog.Warn("register_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "result", "error", "error", err.Error())
		return fmt.Errorf("write device failed: %w", err)
	}

	slog.Info("register_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "pulse_ms", pulseMs, "result", "ok")

	vCopy := value
	s.hub.Publish([]ValueSample{{
		SiteID:    siteID,
		MappingID: mappingID,
		Value:     &vCopy,
		Quality:   QualityGood,
		Timestamp: time.Now().UTC(),
	}})

	// Server-side guaranteed pulse reset (writes 0 back after triggerTimeMs even if tab closes)
	if pulseMs > 0 {
		if pulseMs > 60000 {
			pulseMs = 60000
		}
		devCopy := *dev
		mapCopy := *m
		go func() {
			timer := time.NewTimer(time.Duration(pulseMs) * time.Millisecond)
			defer timer.Stop()
			<-timer.C
			rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := prov.WriteOne(rctx, devCopy, mapCopy, 0); err != nil {
				slog.Warn("register_pulse_reset_failed", "site_id", siteID, "mapping_id", mappingID, "error", err.Error())
				return
			}
			slog.Info("register_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", 0, "result", "pulse_reset_ok")
			zero := 0.0
			s.hub.Publish([]ValueSample{{
				SiteID:    siteID,
				MappingID: mappingID,
				Value:     &zero,
				Quality:   QualityGood,
				Timestamp: time.Now().UTC(),
			}})
		}()
	}

	return nil
}

// --- WebSocket Ticket Prep (Pre-upgrade Auth) ---

func (s *Service) IssueWSTicket(siteID, userID uuid.UUID) (string, time.Time, error) {
	return s.hub.IssueTicket(siteID, userID)
}

func (s *Service) PrepareWS(ctx context.Context, rawTicket string) (WSTicket, error) {
	ticket, err := s.hub.ConsumeTicket(rawTicket)
	if err != nil {
		return WSTicket{}, ErrInvalidTicket
	}
	authz, err := s.cache.CheckAuthz(ctx, ticket.SiteID, ticket.UserID)
	if err != nil || !authz.Allowed || !authz.WebviewEnabled || (!s.devDirectDial && !authz.IsActive) {
		return WSTicket{}, ErrForbidden
	}
	return ticket, nil
}

// --- Lifecycle Drain & Site Deletion (D28) ---

func (s *Service) DrainSite(siteID uuid.UUID) {
	s.providers.DrainSite(siteID)
	s.hub.DisconnectSite(siteID)
}

func (s *Service) DeleteSiteData(ctx context.Context, siteID uuid.UUID) error {
	s.DrainSite(siteID)
	if s.alarmEng != nil {
		s.alarmEng.PurgeSiteMemory(siteID)
	}
	if s.histStore != nil {
		if err := s.histStore.DeleteSiteHistory(ctx, siteID); err != nil {
			slog.Warn("delete_site_tsdb_history_failed", "site_id", siteID, "error", err)
		}
	}
	_, _ = s.db.NewDelete().Model((*models.AlarmPendingState)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.AlarmEvent)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.AlarmRule)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.HistoryEntry)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.Dashboard)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.Device)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.MemberPolicy)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	s.reloadAsync()
	return nil
}

// --- History Query & Per-Entry Retention (Phase 4) ---

type HistoryKeyInfo struct {
	EntryID             uuid.UUID `json:"entryId"`
	RegisterMappingID   uuid.UUID `json:"registerMappingId"`
	DeviceID            uuid.UUID `json:"deviceId"`
	DeviceName          string    `json:"deviceName"`
	Protocol            string    `json:"protocol"`
	RegisterName        string    `json:"registerName"`
	DataType            string    `json:"dataType"`
	Unit                string    `json:"unit"`
	Precision           int       `json:"precision"`
	PollIntervalSeconds int       `json:"pollIntervalSeconds"`
	RetentionDays       int       `json:"retentionDays"`
	IsActive            bool      `json:"isActive"`
}

type HistorySeries struct {
	RegisterMappingID uuid.UUID              `json:"registerMappingId"`
	DeviceName        string                 `json:"deviceName"`
	RegisterName      string                 `json:"registerName"`
	Unit              string                 `json:"unit"`
	Precision         int                    `json:"precision"`
	Points            []history.HistoryPoint `json:"points"`
}

type HistoryQueryResponse struct {
	Resolution string          `json:"resolution"`
	From       time.Time       `json:"from"`
	To         time.Time       `json:"to"`
	Series     []HistorySeries `json:"series"`
}

func (s *Service) ListHistoryKeys(ctx context.Context, siteID uuid.UUID) ([]HistoryKeyInfo, error) {
	entries, err := s.ListHistoryEntries(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []HistoryKeyInfo{}, nil
	}
	mappings, err := s.ListMappings(ctx, siteID)
	if err != nil {
		return nil, err
	}
	devices, err := s.ListDevices(ctx, siteID)
	if err != nil {
		return nil, err
	}

	devByID := make(map[uuid.UUID]models.Device, len(devices))
	for _, d := range devices {
		devByID[d.ID] = d
	}
	mapByID := make(map[uuid.UUID]models.RegisterMapping, len(mappings))
	for _, m := range mappings {
		mapByID[m.ID] = m
	}

	out := make([]HistoryKeyInfo, 0, len(entries))
	for _, he := range entries {
		m, ok := mapByID[he.RegisterMappingID]
		if !ok {
			continue
		}
		dev := devByID[m.DeviceID]
		unit := ""
		prec := 0
		if u, ok := m.ProtocolAttributes["unit"].(string); ok {
			unit = strings.TrimSpace(u)
		}
		if p, ok := m.ProtocolAttributes["precision"].(float64); ok {
			prec = int(p)
		} else if p, ok := m.ProtocolAttributes["precision"].(int); ok {
			prec = p
		}
		if prec < -5 {
			prec = -5
		} else if prec > 5 {
			prec = 5
		}
		out = append(out, HistoryKeyInfo{
			EntryID:             he.ID,
			RegisterMappingID:   m.ID,
			DeviceID:            m.DeviceID,
			DeviceName:          dev.Name,
			Protocol:            dev.Protocol,
			RegisterName:        m.Name,
			DataType:            m.DataType,
			Unit:                unit,
			Precision:           prec,
			PollIntervalSeconds: he.PollIntervalSeconds,
			RetentionDays:       he.RetentionDays,
			IsActive:            he.IsActive,
		})
	}
	return out, nil
}

func (s *Service) QueryHistory(ctx context.Context, siteID uuid.UUID, mappingIDs []uuid.UUID, from, to time.Time) (*HistoryQueryResponse, error) {
	if s.histStore == nil {
		return nil, history.ErrHistoryUnavailable
	}
	keys, err := s.ListHistoryKeys(ctx, siteID)
	if err != nil {
		return nil, err
	}
	keyByMapping := make(map[uuid.UUID]HistoryKeyInfo, len(keys))
	for _, k := range keys {
		keyByMapping[k.RegisterMappingID] = k
	}

	// Only allow querying mappings that belong to this site
	validIDs := make([]uuid.UUID, 0, len(mappingIDs))
	seen := make(map[uuid.UUID]struct{}, len(mappingIDs))
	for _, id := range mappingIDs {
		if _, dup := seen[id]; dup {
			continue
		}
		if _, ok := keyByMapping[id]; ok {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	res, err := s.histStore.Query(ctx, siteID, validIDs, from, to)
	if err != nil {
		return nil, err
	}

	series := make([]HistorySeries, 0, len(validIDs))
	for _, id := range validIDs {
		k := keyByMapping[id]
		pts := res.Series[id]
		if pts == nil {
			pts = []history.HistoryPoint{}
		}
		series = append(series, HistorySeries{
			RegisterMappingID: id,
			DeviceName:        k.DeviceName,
			RegisterName:      k.RegisterName,
			Unit:              k.Unit,
			Precision:         k.Precision,
			Points:            pts,
		})
	}
	return &HistoryQueryResponse{
		Resolution: res.Resolution,
		From:       from.UTC(),
		To:         to.UTC(),
		Series:     series,
	}, nil
}

func (s *Service) PruneExpiredHistory(ctx context.Context) {
	if s.histStore == nil || s.db == nil {
		return
	}
	var entries []models.HistoryEntry
	if err := s.db.NewSelect().Model(&entries).Where("retention_days < 30").Scan(ctx); err != nil {
		slog.Warn("history_prune_list_failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, he := range entries {
		cutoff := now.Add(-time.Duration(he.RetentionDays) * 24 * time.Hour)
		if err := s.histStore.PruneBefore(ctx, he.RegisterMappingID, cutoff); err != nil {
			slog.Warn("history_prune_entry_failed", "mapping_id", he.RegisterMappingID, "error", err)
		}
	}
}
