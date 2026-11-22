package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/pkg/wireguard"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/modbus"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/sitecache"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("name already exists on this site")
	ErrHostOutOfSubnets = errors.New("device host IP is outside the site's allowed LAN subnets")
	ErrReadOnlyMapping  = errors.New("register mapping is read-only")
	ErrWriteForbidden   = errors.New("user does not have write permission on this site")
	ErrInvalidTicket    = errors.New("invalid or expired WebSocket ticket")
	ErrForbidden        = errors.New("site WebView access denied")
)

type Service struct {
	db            *bun.DB
	pool          *modbus.DevicePool
	cache         *sitecache.Cache
	hub           *TelemetryHub
	scheduler     *Scheduler
	devDirectDial bool
}

func NewService(
	db *bun.DB,
	pool *modbus.DevicePool,
	cache *sitecache.Cache,
	hub *TelemetryHub,
	scheduler *Scheduler,
	devDirectDial bool,
) *Service {
	return &Service{
		db:            db,
		pool:          pool,
		cache:         cache,
		hub:           hub,
		scheduler:     scheduler,
		devDirectDial: devDirectDial,
	}
}

func (s *Service) reloadAsync() {
	if s.scheduler != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.scheduler.ReloadConfig(ctx)
		}()
	}
}

// --- Site WebView Settings (History lives inside WebView) ---

func (s *Service) GetSiteSettings(ctx context.Context, siteID uuid.UUID) (models.SiteWebviewSettings, error) {
	st := models.SiteWebviewSettings{SiteID: siteID, HistoryEnabled: false}
	err := s.db.NewSelect().Model(&st).Where("site_id = ?", siteID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	return st, err
}

func (s *Service) UpdateSiteSettings(ctx context.Context, siteID uuid.UUID, historyEnabled bool) (models.SiteWebviewSettings, error) {
	st := models.SiteWebviewSettings{
		SiteID:         siteID,
		HistoryEnabled: historyEnabled,
		UpdatedAt:      time.Now().UTC(),
	}
	_, err := s.db.NewInsert().Model(&st).
		On("CONFLICT (site_id) DO UPDATE").
		Set("history_enabled = EXCLUDED.history_enabled, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return st, err
	}
	s.reloadAsync()
	return st, nil
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

// --- Devices CRUD ---

func (s *Service) ListDevices(ctx context.Context, siteID uuid.UUID) ([]models.Device, error) {
	var list []models.Device
	err := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("name ASC").Scan(ctx)
	return list, err
}

type CreateDeviceInput struct {
	SiteID                uuid.UUID
	Name                  string
	Host                  string
	Port                  int
	Protocol              string
	UnitID                int
	MaxConcurrentRequests int
	RequestTimeoutMs      int
	LanSubnets            []string
}

func (s *Service) CreateDevice(ctx context.Context, in CreateDeviceInput) (*models.Device, error) {
	host := strings.TrimSpace(in.Host)
	if !s.devDirectDial && len(in.LanSubnets) > 0 && !wireguard.ContainsHost(host, in.LanSubnets) {
		return nil, ErrHostOutOfSubnets
	}
	port := in.Port
	if port == 0 {
		port = 502
	}
	proto := in.Protocol
	if proto == "" {
		proto = "MODBUS_TCP"
	}
	maxConc := in.MaxConcurrentRequests
	if maxConc <= 0 {
		maxConc = 1
	}
	timeout := in.RequestTimeoutMs
	if timeout <= 0 {
		timeout = 2000
	}
	dev := &models.Device{
		ID:                    uuid.New(),
		SiteID:                in.SiteID,
		Name:                  strings.TrimSpace(in.Name),
		Host:                  host,
		Port:                  port,
		Protocol:              proto,
		UnitID:                in.UnitID,
		MaxConcurrentRequests: maxConc,
		RequestTimeoutMs:      timeout,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
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
	Name                  *string
	Host                  *string
	Port                  *int
	Protocol              *string
	UnitID                *int
	MaxConcurrentRequests *int
	RequestTimeoutMs      *int
	IsActive              *bool
	LanSubnets            []string
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
		dev.Name = strings.TrimSpace(*in.Name)
	}
	if in.Host != nil {
		h := strings.TrimSpace(*in.Host)
		if !s.devDirectDial && len(in.LanSubnets) > 0 && !wireguard.ContainsHost(h, in.LanSubnets) {
			return nil, ErrHostOutOfSubnets
		}
		dev.Host = h
	}
	if in.Port != nil {
		dev.Port = *in.Port
	}
	if in.Protocol != nil {
		dev.Protocol = *in.Protocol
	}
	if in.UnitID != nil {
		dev.UnitID = *in.UnitID
	}
	if in.MaxConcurrentRequests != nil {
		dev.MaxConcurrentRequests = *in.MaxConcurrentRequests
	}
	if in.RequestTimeoutMs != nil {
		dev.RequestTimeoutMs = *in.RequestTimeoutMs
	}
	if in.IsActive != nil {
		dev.IsActive = *in.IsActive
	}
	dev.UpdatedAt = time.Now().UTC()

	if _, err := s.db.NewUpdate().Model(dev).WherePK().Exec(ctx); err != nil {
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

// --- Register Mappings CRUD + Live Test Read ---

func (s *Service) ListMappings(ctx context.Context, siteID uuid.UUID) ([]models.RegisterMapping, error) {
	var list []models.RegisterMapping
	err := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("name ASC").Scan(ctx)
	return list, err
}

func (s *Service) CreateMapping(ctx context.Context, m *models.RegisterMapping) (*models.RegisterMapping, error) {
	// Verify device belongs to site
	exists, err := s.db.NewSelect().Model((*models.Device)(nil)).
		Where("id = ? AND site_id = ?", m.DeviceID, m.SiteID).Exists(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}

	m.ID = uuid.New()
	m.Name = strings.TrimSpace(m.Name)
	m.RegisterCount = modbus.RegisterCountForType(m.DataType)
	if m.WordOrder == "" {
		m.WordOrder = "BIG"
	}
	if m.ByteOrder == "" {
		m.ByteOrder = "BIG"
	}
	if m.Scale == 0 {
		m.Scale = 1
	}
	if m.AccessMode == "" {
		m.AccessMode = "READ"
	}
	if m.InteractivePollIntervalMs <= 0 {
		m.InteractivePollIntervalMs = 1000
	}
	if m.ArchivalPollIntervalMs <= 0 {
		m.ArchivalPollIntervalMs = 60000
	}
	if m.MaxIntervalSeconds <= 0 {
		m.MaxIntervalSeconds = 300
	}
	m.IsActive = true
	m.CreatedAt = time.Now().UTC()
	m.UpdatedAt = m.CreatedAt

	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	return m, nil
}

func (s *Service) UpdateMapping(ctx context.Context, siteID, mappingID uuid.UUID, patch func(*models.RegisterMapping)) (*models.RegisterMapping, error) {
	m := new(models.RegisterMapping)
	if err := s.db.NewSelect().Model(m).Where("id = ? AND site_id = ?", mappingID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	patch(m)
	m.RegisterCount = modbus.RegisterCountForType(m.DataType)
	m.UpdatedAt = time.Now().UTC()

	if _, err := s.db.NewUpdate().Model(m).WherePK().Exec(ctx); err != nil {
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

// TestRead performs a live single-register read over the site's WireRelay tunnel.
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
	fc, err := modbus.ReadFunctionByte(m.FunctionCode)
	if err != nil {
		return nil, err
	}
	qty := uint16(modbus.RegisterCountForType(m.DataType))
	if modbus.IsBitFunction(fc) {
		qty = 1
	}
	respPDU, err := s.pool.Exchange(ctx, *dev, modbus.BuildReadPDU(fc, uint16(m.RegisterAddress), qty))
	if err != nil {
		return &ValueSample{SiteID: siteID, MappingID: mappingID, Quality: QualityBad, Error: err.Error(), Timestamp: time.Now().UTC()}, nil
	}
	payload, err := modbus.ParseReadResponsePDU(fc, respPDU)
	if err != nil {
		return &ValueSample{SiteID: siteID, MappingID: mappingID, Quality: QualityBad, Error: err.Error(), Timestamp: time.Now().UTC()}, nil
	}
	val, err := modbus.DecodeValue(*m, payload, 0)
	if err != nil {
		return &ValueSample{SiteID: siteID, MappingID: mappingID, Quality: QualityBad, Error: err.Error(), Timestamp: time.Now().UTC()}, nil
	}
	sample := ValueSample{SiteID: siteID, MappingID: mappingID, Value: &val, Quality: QualityGood, Timestamp: time.Now().UTC()}
	s.hub.Publish([]ValueSample{sample})
	return &sample, nil
}

// --- Dashboards & Widgets CRUD ---

func (s *Service) ListDashboards(ctx context.Context, siteID uuid.UUID) ([]models.Dashboard, error) {
	var list []models.Dashboard
	err := s.db.NewSelect().Model(&list).
		Relation("Widgets", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("sort_order ASC", "created_at ASC")
		}).
		Where("db.site_id = ?", siteID).
		Order("db.is_default DESC", "db.name ASC").
		Scan(ctx)
	return list, err
}

func (s *Service) CreateDashboard(ctx context.Context, siteID uuid.UUID, name string, isDefault bool) (*models.Dashboard, error) {
	d := &models.Dashboard{
		ID:        uuid.New(),
		SiteID:    siteID,
		Name:      strings.TrimSpace(name),
		Layout:    map[string]any{},
		IsDefault: isDefault,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if _, err := s.db.NewInsert().Model(d).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return d, nil
}

func (s *Service) UpdateDashboard(ctx context.Context, siteID, dashboardID uuid.UUID, name *string, isDefault *bool) (*models.Dashboard, error) {
	d := new(models.Dashboard)
	if err := s.db.NewSelect().Model(d).Where("id = ? AND site_id = ?", dashboardID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if name != nil {
		d.Name = strings.TrimSpace(*name)
	}
	if isDefault != nil {
		d.IsDefault = *isDefault
	}
	d.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(d).WherePK().Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return d, nil
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

func (s *Service) CreateWidget(ctx context.Context, siteID, dashboardID uuid.UUID, w *models.Widget) (*models.Widget, error) {
	exists, err := s.db.NewSelect().Model((*models.Dashboard)(nil)).
		Where("id = ? AND site_id = ?", dashboardID, siteID).Exists(ctx)
	if err != nil || !exists {
		return nil, ErrNotFound
	}
	w.ID = uuid.New()
	w.DashboardID = dashboardID
	w.Name = strings.TrimSpace(w.Name)
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
	w.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(w).WherePK().Exec(ctx); err != nil {
		return nil, err
	}
	return w, nil
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

// --- REST Register Write (D15, D16) ---

func (s *Service) WriteRegister(ctx context.Context, siteID, userID uuid.UUID, authz *internalclient.SiteAuthzResponse, mappingID uuid.UUID, value float64) error {
	canWrite, err := s.CanUserWrite(ctx, siteID, userID, authz)
	if err != nil {
		return err
	}
	if !canWrite {
		slog.Warn("modbus_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "result", "forbidden")
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

	fc, reqPDU, err := modbus.BuildWritePDU(*m, value)
	if err != nil {
		return err
	}
	respPDU, err := s.pool.Exchange(ctx, *dev, reqPDU)
	if err != nil {
		slog.Warn("modbus_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "result", "error", "error", err.Error())
		return fmt.Errorf("write device failed: %w", err)
	}
	if err := modbus.ParseWriteResponsePDU(fc, respPDU); err != nil {
		slog.Warn("modbus_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "result", "exception", "error", err.Error())
		return err
	}

	// Structured JSON audit log (D16)
	slog.Info("modbus_write_audit", "user_id", userID, "site_id", siteID, "mapping_id", mappingID, "value", value, "result", "ok")

	// Immediately reflect written value to live subscribers for crisp UI feedback
	if m.AccessMode == "READ_WRITE" {
		vCopy := value
		s.hub.Publish([]ValueSample{{
			SiteID:    siteID,
			MappingID: mappingID,
			Value:     &vCopy,
			Quality:   QualityGood,
			Timestamp: time.Now().UTC(),
		}})
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
	s.pool.DrainSite(siteID)
	s.hub.DisconnectSite(siteID)
}

func (s *Service) DeleteSiteData(ctx context.Context, siteID uuid.UUID) error {
	s.DrainSite(siteID)
	_, _ = s.db.NewDelete().Model((*models.Dashboard)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.Device)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.MemberPolicy)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	_, _ = s.db.NewDelete().Model((*models.SiteWebviewSettings)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	s.reloadAsync()
	return nil
}
