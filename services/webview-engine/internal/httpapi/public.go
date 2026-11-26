package httpapi

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"wiredeck/pkg/api"
	"wiredeck/pkg/auth"
	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/telemetry"
	"wiredeck/pkg/version"
	"wiredeck/services/webview-engine/internal/engine"
	"wiredeck/services/webview-engine/internal/history"
	"wiredeck/services/webview-engine/internal/models"
	"wiredeck/services/webview-engine/internal/sitecache"
)

type authzCtxKey struct{}

// --- DTOs ---

type DeviceDTO struct {
	ID        uuid.UUID `json:"id"`
	SiteID    uuid.UUID `json:"siteId"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

func toDeviceDTO(d models.Device) DeviceDTO {
	return DeviceDTO{
		ID:        d.ID,
		SiteID:    d.SiteID,
		Name:      d.Name,
		Host:      d.Host,
		Port:      d.Port,
		Protocol:  d.Protocol,
		IsActive:  d.IsActive,
		CreatedAt: d.CreatedAt,
	}
}

type MappingDTO struct {
	ID                 uuid.UUID      `json:"id"`
	DeviceID           uuid.UUID      `json:"deviceId"`
	SiteID             uuid.UUID      `json:"siteId"`
	Name               string         `json:"name"`
	ProtocolAttributes map[string]any `json:"protocolAttributes"`
	IsActive           bool           `json:"isActive"`
}

func toMappingDTO(m models.RegisterMapping) MappingDTO {
	attrs := m.ProtocolAttributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return MappingDTO{
		ID:                 m.ID,
		DeviceID:           m.DeviceID,
		SiteID:             m.SiteID,
		Name:               m.Name,
		ProtocolAttributes: attrs,
		IsActive:           m.IsActive,
	}
}

type HistoryEntryDTO struct {
	ID                  uuid.UUID `json:"id"`
	SiteID              uuid.UUID `json:"siteId"`
	RegisterMappingID   uuid.UUID `json:"registerMappingId"`
	PollIntervalSeconds int       `json:"pollIntervalSeconds"`
	RetentionDays       int       `json:"retentionDays"`
	IsActive            bool      `json:"isActive"`
	CreatedAt           time.Time `json:"createdAt"`
}

func toHistoryEntryDTO(he models.HistoryEntry) HistoryEntryDTO {
	return HistoryEntryDTO{
		ID:                  he.ID,
		SiteID:              he.SiteID,
		RegisterMappingID:   he.RegisterMappingID,
		PollIntervalSeconds: he.PollIntervalSeconds,
		RetentionDays:       he.RetentionDays,
		IsActive:            he.IsActive,
		CreatedAt:           he.CreatedAt,
	}
}

type WidgetDTO struct {
	ID                uuid.UUID      `json:"id"`
	DashboardID       uuid.UUID      `json:"dashboardId"`
	RegisterMappingID *uuid.UUID     `json:"registerMappingId,omitempty"`
	Name              string         `json:"name"`
	WidgetType        string         `json:"widgetType"`
	SortOrder         int            `json:"sortOrder"`
	Style             map[string]any `json:"style"`
	Extra             map[string]any `json:"extra"`
}

func toWidgetDTO(w models.Widget) WidgetDTO {
	st := w.Style
	if st == nil {
		st = map[string]any{}
	}
	ex := w.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	return WidgetDTO{
		ID:                w.ID,
		DashboardID:       w.DashboardID,
		RegisterMappingID: w.RegisterMappingID,
		Name:              w.Name,
		WidgetType:        w.WidgetType,
		SortOrder:         w.SortOrder,
		Style:             st,
		Extra:             ex,
	}
}

type DashboardDTO struct {
	ID             uuid.UUID      `json:"id"`
	SiteID         uuid.UUID      `json:"siteId"`
	Name           string         `json:"name"`
	Layout         map[string]any `json:"layout"`
	IsDefault      bool           `json:"isDefault"`
	SortOrder      int            `json:"sortOrder"`
	PollIntervalMs int            `json:"pollIntervalMs"`
	Widgets        []WidgetDTO    `json:"widgets"`
}

func toDashboardDTO(d models.Dashboard) DashboardDTO {
	ws := make([]WidgetDTO, len(d.Widgets))
	for i, w := range d.Widgets {
		ws[i] = toWidgetDTO(w)
	}
	ly := d.Layout
	if ly == nil {
		ly = map[string]any{}
	}
	pollMs := d.PollIntervalMs
	if pollMs < 1000 {
		pollMs = 1000
	}
	return DashboardDTO{
		ID:             d.ID,
		SiteID:         d.SiteID,
		Name:           d.Name,
		Layout:         ly,
		IsDefault:      d.IsDefault,
		SortOrder:      d.SortOrder,
		PollIntervalMs: pollMs,
		Widgets:        ws,
	}
}

type MemberPolicyDTO struct {
	UserID    uuid.UUID `json:"userId"`
	CanWrite  bool      `json:"canWrite"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewPublicRouter builds the Huma v2 public router (:8080) with strict
// Site Member vs Site Operator separation and pre-upgrade authenticated WebSocket streaming.
func NewPublicRouter(svc *engine.Service, hub *engine.TelemetryHub, verifier *auth.Verifier, cache *sitecache.Cache, enableDocs bool) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	// Multiplexed telemetry WebSocket authenticated via single-use 30s ticket BEFORE upgrade.
	r.Get("/api/v1/webview/stream", func(w http.ResponseWriter, req *http.Request) {
		rawTicket := req.URL.Query().Get("ticket")
		if rawTicket == "" {
			http.Error(w, "missing ticket", http.StatusUnauthorized)
			return
		}
		ticket, err := svc.PrepareWS(req.Context(), rawTicket)
		if err != nil {
			if errors.Is(err, engine.ErrInvalidTicket) {
				http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
			} else {
				http.Error(w, "site WebView access denied", http.StatusForbidden)
			}
			return
		}
		ws, err := websocket.Accept(w, req, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		hub.ServeConn(req.Context(), ws, ticket)
	})

	// Authenticated CSV history export endpoint (raw HTTP response for file download)
	r.Get("/api/v1/webview/sites/{siteId}/history/export.csv", func(w http.ResponseWriter, req *http.Request) {
		rawAuth := req.Header.Get("Authorization")
		if len(rawAuth) < 8 || rawAuth[:7] != "Bearer " {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		claims, err := verifier.Verify(req.Context(), rawAuth[7:])
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		siteID, err := uuid.Parse(chi.URLParam(req, "siteId"))
		if err != nil {
			http.Error(w, "invalid siteId", http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			http.Error(w, "invalid token subject", http.StatusUnauthorized)
			return
		}
		authz, err := cache.CheckAuthz(req.Context(), siteID, userID)
		if err != nil || !authz.Allowed || !authz.WebviewEnabled {
			http.Error(w, "WebView access denied for this site", http.StatusForbidden)
			return
		}

		mappingIDs, from, to, err := parseHistoryQueryParams(req.URL.Query().Get("mappingIds"), req.URL.Query().Get("from"), req.URL.Query().Get("to"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := svc.QueryHistory(req.Context(), siteID, mappingIDs, from, to)
		if err != nil {
			if errors.Is(err, history.ErrHistoryUnavailable) {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="history-%s.csv"`, siteID.String()[:8]))
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"timestamp", "device", "register", "unit", "value", "min", "max", "samples"})
		for _, ser := range res.Series {
			factor := math.Pow10(-ser.Precision)
			decimals := ser.Precision
			if decimals < 0 {
				decimals = 0
			}
			fmtScaled := func(v *float64) string {
				if v == nil {
					return ""
				}
				return strconv.FormatFloat(*v*factor, 'f', decimals, 64)
			}
			for _, pt := range ser.Points {
				_ = cw.Write([]string{
					pt.Time.UTC().Format(time.RFC3339),
					ser.DeviceName,
					ser.RegisterName,
					ser.Unit,
					fmtScaled(pt.Value),
					fmtScaled(pt.Min),
					fmtScaled(pt.Max),
					strconv.FormatInt(pt.Count, 10),
				})
			}
		}
		cw.Flush()
	})

	humaAPI := api.NewHumaAPI(r, "WireDeck WebView Engine API", version.Version, "/api/v1/webview", enableDocs)

	type SiteIDInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
	}

	// =========================================================================
	// 0. Authenticated User Cross-Site Alarm Feed (/api/v1/webview/alarms*)
	// =========================================================================
	userGroup := huma.NewGroup(humaAPI)
	userGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier))

	type GlobalAlarmsInput struct {
		SiteID       string `query:"siteId" doc:"Optional site UUID filter"`
		State        string `query:"state" doc:"ACTIVE | CLEARED | ALL"`
		Severity     string `query:"severity" doc:"INFO | WARNING | CRITICAL"`
		Acknowledged string `query:"acknowledged" doc:"true | false"`
		Limit        int    `query:"limit" minimum:"0" maximum:"1000"`
		Offset       int    `query:"offset" minimum:"0"`
	}
	type AlarmEventsOutput struct {
		Body struct {
			Events  []engine.AlarmEventView `json:"events"`
			Summary engine.AlarmSummary     `json:"summary"`
		}
	}
	huma.Register(userGroup, huma.Operation{
		OperationID: "webviewListGlobalAlarms",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/alarms",
		Summary:     "List alarm events across all sites accessible to the current user",
		Tags:        []string{"WebView Alarms"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *GlobalAlarmsInput) (*AlarmEventsOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			return nil, huma.Error401Unauthorized("invalid token subject")
		}
		var siteFilter *uuid.UUID
		if s := strings.TrimSpace(in.SiteID); s != "" {
			parsed, err := uuid.Parse(s)
			if err != nil {
				return nil, huma.Error400BadRequest("invalid siteId")
			}
			siteFilter = &parsed
		}
		events, summary, err := svc.ListGlobalAlarmEvents(ctx, userID, engine.AlarmEventFilter{
			SiteID:       siteFilter,
			State:        in.State,
			Severity:     in.Severity,
			Acknowledged: in.Acknowledged,
			Limit:        in.Limit,
			Offset:       in.Offset,
		})
		if err != nil {
			return nil, mapError(err)
		}
		out := &AlarmEventsOutput{}
		out.Body.Events = events
		out.Body.Summary = summary
		return out, nil
	})

	type GlobalAlarmSummaryOutput struct {
		Body struct {
			Summary engine.AlarmSummary `json:"summary"`
			BySite  map[string]int      `json:"bySite"`
		}
	}
	huma.Register(userGroup, huma.Operation{
		OperationID: "webviewGlobalAlarmSummary",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/alarms/summary",
		Summary:     "Lightweight active alarm counts globally and per site for sidebar badges",
		Tags:        []string{"WebView Alarms"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, _ *struct{}) (*GlobalAlarmSummaryOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			return nil, huma.Error401Unauthorized("invalid token subject")
		}
		sum, bySite, err := svc.GetGlobalAlarmSummary(ctx, userID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &GlobalAlarmSummaryOutput{}
		out.Body.Summary = sum
		out.Body.BySite = bySite
		return out, nil
	})

	// =========================================================================
	// 1. Site Member Endpoints (/api/v1/webview/sites/{siteId}/*)
	// =========================================================================
	memberGroup := huma.NewGroup(humaAPI)
	memberGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteAuthzMiddleware(humaAPI, cache, false))

	type WorkspaceOutput struct {
		Body struct {
			CanWrite   bool           `json:"canWrite"`
			Dashboards []DashboardDTO `json:"dashboards"`
			Mappings   []MappingDTO   `json:"mappings"`
			Devices    []DeviceDTO    `json:"devices"`
		}
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewGetWorkspace",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/workspace",
		Summary:     "Fetch site dashboards, widgets, mappings, and write capability (Site Member)",
		Tags:        []string{"WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*WorkspaceOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		authz, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)

		dashboards, err := svc.ListDashboards(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		mappings, err := svc.ListMappings(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		devices, err := svc.ListDevices(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		userID, _ := uuid.Parse(claims.Subject)
		canWrite, err := svc.CanUserWrite(ctx, in.SiteID, userID, authz)
		if err != nil {
			return nil, mapError(err)
		}

		out := &WorkspaceOutput{}
		out.Body.CanWrite = canWrite
		out.Body.Dashboards = make([]DashboardDTO, len(dashboards))
		for i, d := range dashboards {
			out.Body.Dashboards[i] = toDashboardDTO(d)
		}
		out.Body.Mappings = make([]MappingDTO, len(mappings))
		for i, m := range mappings {
			out.Body.Mappings[i] = toMappingDTO(m)
		}
		out.Body.Devices = make([]DeviceDTO, len(devices))
		for i, d := range devices {
			out.Body.Devices[i] = toDeviceDTO(d)
		}
		return out, nil
	})

	type WriteInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			MappingID uuid.UUID `json:"mappingId" format:"uuid"`
			Value     float64   `json:"value"`
			PulseMs   int       `json:"pulseMs,omitempty" minimum:"0" maximum:"60000"`
		}
	}
	type WriteOutput struct {
		Body struct {
			Status string `json:"status"`
		}
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewWriteRegister",
		Method:      http.MethodPost,
		Path:        "/api/v1/webview/sites/{siteId}/write",
		Summary:     "Write a value (or timed pulse) to a writable register via its protocol provider",
		Tags:        []string{"WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *WriteInput) (*WriteOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		authz, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)
		userID, _ := uuid.Parse(claims.Subject)
		if err := svc.WriteRegister(ctx, in.SiteID, userID, authz, in.Body.MappingID, in.Body.Value, in.Body.PulseMs); err != nil {
			return nil, mapError(err)
		}
		out := &WriteOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	type WSTicketOutput struct {
		Body struct {
			Ticket    string    `json:"ticket"`
			ExpiresAt time.Time `json:"expiresAt"`
		}
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewCreateWSTicket",
		Method:      http.MethodPost,
		Path:        "/api/v1/webview/sites/{siteId}/ws-ticket",
		Summary:     "Mint a single-use 30s WebSocket ticket for live WebView telemetry",
		Tags:        []string{"WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*WSTicketOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		userID, _ := uuid.Parse(claims.Subject)
		tok, exp, err := svc.IssueWSTicket(in.SiteID, userID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &WSTicketOutput{}
		out.Body.Ticket = tok
		out.Body.ExpiresAt = exp
		return out, nil
	})

	// --- History Query & Configured Keys (Site Member+) ---
	type HistoryKeysOutput struct {
		Body struct {
			Keys []engine.HistoryKeyInfo `json:"keys"`
		}
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewListHistoryKeys",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/history/keys",
		Summary:     "List register keys enabled for history archival on a site (Site Member)",
		Tags:        []string{"WebView History"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*HistoryKeysOutput, error) {
		keys, err := svc.ListHistoryKeys(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &HistoryKeysOutput{}
		out.Body.Keys = keys
		return out, nil
	})

	type QueryHistoryInput struct {
		SiteID     uuid.UUID `path:"siteId" format:"uuid"`
		MappingIDs string    `query:"mappingIds" doc:"Comma-separated registerMappingId UUIDs"`
		From       string    `query:"from" doc:"Start timestamp (RFC3339)"`
		To         string    `query:"to" doc:"End timestamp (RFC3339)"`
	}
	type QueryHistoryOutput struct {
		Body engine.HistoryQueryResponse
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewQueryHistory",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/history",
		Summary:     "Query TimescaleDB register history with automatic resolution selection (raw / 1m / 5m / 1h / 6h)",
		Tags:        []string{"WebView History"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *QueryHistoryInput) (*QueryHistoryOutput, error) {
		ids, from, to, err := parseHistoryQueryParams(in.MappingIDs, in.From, in.To)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		res, err := svc.QueryHistory(ctx, in.SiteID, ids, from, to)
		if err != nil {
			return nil, mapError(err)
		}
		return &QueryHistoryOutput{Body: *res}, nil
	})

	// --- Site Alarms Feed & Acknowledge (Any Site Member+) ---
	type SiteAlarmsInput struct {
		SiteID       uuid.UUID `path:"siteId" format:"uuid"`
		State        string    `query:"state" doc:"ACTIVE | CLEARED | ALL"`
		Severity     string    `query:"severity" doc:"INFO | WARNING | CRITICAL"`
		Acknowledged string    `query:"acknowledged" doc:"true | false"`
		Limit        int       `query:"limit" minimum:"0" maximum:"1000"`
		Offset       int       `query:"offset" minimum:"0"`
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewListSiteAlarms",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/alarms",
		Summary:     "List alarm events and summary for a specific site (Site Member)",
		Tags:        []string{"WebView Alarms"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteAlarmsInput) (*AlarmEventsOutput, error) {
		events, summary, err := svc.ListSiteAlarmEvents(ctx, in.SiteID, engine.AlarmEventFilter{
			State:        in.State,
			Severity:     in.Severity,
			Acknowledged: in.Acknowledged,
			Limit:        in.Limit,
			Offset:       in.Offset,
		})
		if err != nil {
			return nil, mapError(err)
		}
		out := &AlarmEventsOutput{}
		out.Body.Events = events
		out.Body.Summary = summary
		return out, nil
	})

	type AckAlarmInput struct {
		SiteID  uuid.UUID `path:"siteId" format:"uuid"`
		EventID uuid.UUID `path:"eventId" format:"uuid"`
		Body    struct {
			Note string `json:"note,omitempty" maxLength:"500"`
		}
	}
	type AlarmEventOutput struct {
		Body engine.AlarmEventView
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "webviewAckAlarm",
		Method:      http.MethodPost,
		Path:        "/api/v1/webview/sites/{siteId}/alarms/{eventId}/ack",
		Summary:     "Acknowledge an alarm event (any authenticated site member; cosmetic, preserves latch)",
		Tags:        []string{"WebView Alarms"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *AckAlarmInput) (*AlarmEventOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			return nil, huma.Error401Unauthorized("invalid token subject")
		}
		ev, err := svc.AcknowledgeAlarmEvent(ctx, in.SiteID, in.EventID, userID, claims.Email, in.Body.Note)
		if err != nil {
			return nil, mapError(err)
		}
		return &AlarmEventOutput{Body: *ev}, nil
	})

	// =========================================================================
	// 2. Site Operator Endpoints (/api/v1/webview/sites/{siteId}/operator/*)
	// =========================================================================
	opGroup := huma.NewGroup(humaAPI)
	opGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteAuthzMiddleware(humaAPI, cache, true))

	// --- Devices (name, host, port, protocol only) ---
	type ListDevicesOutput struct {
		Body struct {
			Devices []DeviceDTO `json:"devices"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListDevices",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/devices",
		Summary:     "List devices for a site (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListDevicesOutput, error) {
		list, err := svc.ListDevices(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListDevicesOutput{}
		out.Body.Devices = make([]DeviceDTO, len(list))
		for i, d := range list {
			out.Body.Devices[i] = toDeviceDTO(d)
		}
		return out, nil
	})

	type CreateDeviceInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			Name     string `json:"name" minLength:"1" maxLength:"100"`
			Host     string `json:"host" minLength:"3" maxLength:"64"`
			Port     int    `json:"port,omitempty" minimum:"1" maximum:"65535"`
			Protocol string `json:"protocol,omitempty" enum:"MODBUS_TCP"`
		}
	}
	type DeviceOutput struct {
		Body DeviceDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateDevice",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/devices",
		Summary:       "Create a device (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateDeviceInput) (*DeviceOutput, error) {
		authz, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)
		var subnets []string
		if authz != nil {
			subnets = authz.LanSubnets
		}
		d, err := svc.CreateDevice(ctx, engine.CreateDeviceInput{
			SiteID:     in.SiteID,
			Name:       in.Body.Name,
			Host:       in.Body.Host,
			Port:       in.Body.Port,
			Protocol:   in.Body.Protocol,
			LanSubnets: subnets,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &DeviceOutput{Body: toDeviceDTO(*d)}, nil
	})

	type UpdateDeviceInput struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		DeviceID uuid.UUID `path:"deviceId" format:"uuid"`
		Body     struct {
			Name     *string `json:"name,omitempty" minLength:"1" maxLength:"100"`
			Host     *string `json:"host,omitempty" minLength:"3" maxLength:"64"`
			Port     *int    `json:"port,omitempty" minimum:"1" maximum:"65535"`
			Protocol *string `json:"protocol,omitempty" enum:"MODBUS_TCP"`
			IsActive *bool   `json:"isActive,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateDevice",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/devices/{deviceId}",
		Summary:     "Update a device (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateDeviceInput) (*DeviceOutput, error) {
		authz, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)
		var subnets []string
		if authz != nil {
			subnets = authz.LanSubnets
		}
		d, err := svc.UpdateDevice(ctx, in.SiteID, in.DeviceID, engine.UpdateDeviceInput{
			Name:       in.Body.Name,
			Host:       in.Body.Host,
			Port:       in.Body.Port,
			Protocol:   in.Body.Protocol,
			IsActive:   in.Body.IsActive,
			LanSubnets: subnets,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &DeviceOutput{Body: toDeviceDTO(*d)}, nil
	})

	type DeviceIDInput struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		DeviceID uuid.UUID `path:"deviceId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteDevice",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/devices/{deviceId}",
		Summary:       "Delete a device and its register mappings (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *DeviceIDInput) (*struct{}, error) {
		if err := svc.DeleteDevice(ctx, in.SiteID, in.DeviceID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// --- Register Mappings (dynamic protocolAttributes) ---
	type ListMappingsOutput struct {
		Body struct {
			Mappings []MappingDTO `json:"mappings"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListMappings",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/mappings",
		Summary:     "List register mappings for a site (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListMappingsOutput, error) {
		list, err := svc.ListMappings(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListMappingsOutput{}
		out.Body.Mappings = make([]MappingDTO, len(list))
		for i, m := range list {
			out.Body.Mappings[i] = toMappingDTO(m)
		}
		return out, nil
	})

	type CreateMappingInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			DeviceID           uuid.UUID      `json:"deviceId" format:"uuid"`
			Name               string         `json:"name" minLength:"1" maxLength:"100"`
			ProtocolAttributes map[string]any `json:"protocolAttributes"`
		}
	}
	type MappingOutput struct {
		Body MappingDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateMapping",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/mappings",
		Summary:       "Create a register mapping with protocol-specific attributes (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateMappingInput) (*MappingOutput, error) {
		m, err := svc.CreateMapping(ctx, in.SiteID, in.Body.DeviceID, in.Body.Name, in.Body.ProtocolAttributes)
		if err != nil {
			return nil, mapError(err)
		}
		return &MappingOutput{Body: toMappingDTO(*m)}, nil
	})

	type UpdateMappingInput struct {
		SiteID    uuid.UUID `path:"siteId" format:"uuid"`
		MappingID uuid.UUID `path:"mappingId" format:"uuid"`
		Body      struct {
			DeviceID           *uuid.UUID     `json:"deviceId,omitempty" format:"uuid"`
			Name               *string        `json:"name,omitempty" minLength:"1" maxLength:"100"`
			ProtocolAttributes map[string]any `json:"protocolAttributes,omitempty"`
			IsActive           *bool          `json:"isActive,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateMapping",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/mappings/{mappingId}",
		Summary:     "Update a register mapping (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateMappingInput) (*MappingOutput, error) {
		m, err := svc.UpdateMapping(ctx, in.SiteID, in.MappingID, engine.UpdateMappingInput{
			DeviceID:           in.Body.DeviceID,
			Name:               in.Body.Name,
			ProtocolAttributes: in.Body.ProtocolAttributes,
			IsActive:           in.Body.IsActive,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &MappingOutput{Body: toMappingDTO(*m)}, nil
	})

	type MappingIDInput struct {
		SiteID    uuid.UUID `path:"siteId" format:"uuid"`
		MappingID uuid.UUID `path:"mappingId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteMapping",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/mappings/{mappingId}",
		Summary:       "Delete a register mapping (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *MappingIDInput) (*struct{}, error) {
		if err := svc.DeleteMapping(ctx, in.SiteID, in.MappingID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type TestReadOutput struct {
		Body engine.ValueSample
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorTestReadMapping",
		Method:      http.MethodPost,
		Path:        "/api/v1/webview/sites/{siteId}/operator/mappings/{mappingId}/test-read",
		Summary:     "Perform a live test read of a register mapping via its provider (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *MappingIDInput) (*TestReadOutput, error) {
		sample, err := svc.TestRead(ctx, in.SiteID, in.MappingID)
		if err != nil {
			return nil, mapError(err)
		}
		return &TestReadOutput{Body: *sample}, nil
	})

	// --- History Entries (selected keys + poll interval + retention) ---
	type ListHistoryEntriesOutput struct {
		Body struct {
			Entries []HistoryEntryDTO `json:"entries"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListHistoryEntries",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/history-entries",
		Summary:     "List selected register history storage entries (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListHistoryEntriesOutput, error) {
		list, err := svc.ListHistoryEntries(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListHistoryEntriesOutput{}
		out.Body.Entries = make([]HistoryEntryDTO, len(list))
		for i, he := range list {
			out.Body.Entries[i] = toHistoryEntryDTO(he)
		}
		return out, nil
	})

	type CreateHistoryEntryInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			RegisterMappingID   uuid.UUID `json:"registerMappingId" format:"uuid"`
			PollIntervalSeconds int       `json:"pollIntervalSeconds" minimum:"1" maximum:"86400"`
			RetentionDays       int       `json:"retentionDays" minimum:"1" maximum:"365"`
		}
	}
	type HistoryEntryOutput struct {
		Body HistoryEntryDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateHistoryEntry",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/history-entries",
		Summary:       "Add a register to scheduled history archival (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateHistoryEntryInput) (*HistoryEntryOutput, error) {
		he, err := svc.CreateHistoryEntry(ctx, in.SiteID, in.Body.RegisterMappingID, in.Body.PollIntervalSeconds, in.Body.RetentionDays)
		if err != nil {
			return nil, mapError(err)
		}
		return &HistoryEntryOutput{Body: toHistoryEntryDTO(*he)}, nil
	})

	type UpdateHistoryEntryInput struct {
		SiteID  uuid.UUID `path:"siteId" format:"uuid"`
		EntryID uuid.UUID `path:"entryId" format:"uuid"`
		Body    struct {
			PollIntervalSeconds *int  `json:"pollIntervalSeconds,omitempty" minimum:"1" maximum:"86400"`
			RetentionDays       *int  `json:"retentionDays,omitempty" minimum:"1" maximum:"365"`
			IsActive            *bool `json:"isActive,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateHistoryEntry",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/history-entries/{entryId}",
		Summary:     "Update poll interval, retention, or active state of a history entry (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateHistoryEntryInput) (*HistoryEntryOutput, error) {
		he, err := svc.UpdateHistoryEntry(ctx, in.SiteID, in.EntryID, in.Body.PollIntervalSeconds, in.Body.RetentionDays, in.Body.IsActive)
		if err != nil {
			return nil, mapError(err)
		}
		return &HistoryEntryOutput{Body: toHistoryEntryDTO(*he)}, nil
	})

	type HistoryEntryIDInput struct {
		SiteID  uuid.UUID `path:"siteId" format:"uuid"`
		EntryID uuid.UUID `path:"entryId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteHistoryEntry",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/history-entries/{entryId}",
		Summary:       "Remove a register from scheduled history archival (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *HistoryEntryIDInput) (*struct{}, error) {
		if err := svc.DeleteHistoryEntry(ctx, in.SiteID, in.EntryID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// --- Dashboards & Widgets (CRUD + Reorder) ---
	type ListDashboardsOutput struct {
		Body struct {
			Dashboards []DashboardDTO `json:"dashboards"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListDashboards",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/dashboards",
		Summary:     "List dashboards and widgets for a site (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListDashboardsOutput, error) {
		list, err := svc.ListDashboards(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListDashboardsOutput{}
		out.Body.Dashboards = make([]DashboardDTO, len(list))
		for i, d := range list {
			out.Body.Dashboards[i] = toDashboardDTO(d)
		}
		return out, nil
	})

	type CreateDashboardInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			Name           string         `json:"name" minLength:"1" maxLength:"100"`
			IsDefault      bool           `json:"isDefault,omitempty"`
			PollIntervalMs int            `json:"pollIntervalMs,omitempty" minimum:"1000" maximum:"60000"`
			Layout         map[string]any `json:"layout,omitempty"`
		}
	}
	type DashboardOutput struct {
		Body DashboardDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateDashboard",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/dashboards",
		Summary:       "Create a WebView dashboard page (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateDashboardInput) (*DashboardOutput, error) {
		d, err := svc.CreateDashboard(ctx, in.SiteID, in.Body.Name, in.Body.IsDefault, in.Body.PollIntervalMs, in.Body.Layout)
		if err != nil {
			return nil, mapError(err)
		}
		return &DashboardOutput{Body: toDashboardDTO(*d)}, nil
	})

	type UpdateDashboardInput struct {
		SiteID      uuid.UUID `path:"siteId" format:"uuid"`
		DashboardID uuid.UUID `path:"dashboardId" format:"uuid"`
		Body        struct {
			Name           *string        `json:"name,omitempty" minLength:"1" maxLength:"100"`
			IsDefault      *bool          `json:"isDefault,omitempty"`
			SortOrder      *int           `json:"sortOrder,omitempty"`
			PollIntervalMs *int           `json:"pollIntervalMs,omitempty" minimum:"1000" maximum:"60000"`
			Layout         map[string]any `json:"layout,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateDashboard",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/dashboards/{dashboardId}",
		Summary:     "Update a WebView dashboard page (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateDashboardInput) (*DashboardOutput, error) {
		d, err := svc.UpdateDashboard(ctx, in.SiteID, in.DashboardID, in.Body.Name, in.Body.IsDefault, in.Body.SortOrder, in.Body.PollIntervalMs, in.Body.Layout)
		if err != nil {
			return nil, mapError(err)
		}
		return &DashboardOutput{Body: toDashboardDTO(*d)}, nil
	})

	type ReorderDashboardsInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			DashboardIDs []uuid.UUID `json:"dashboardIds" minItems:"1"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorReorderDashboards",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/dashboards/reorder",
		Summary:       "Persist display order for site dashboards (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *ReorderDashboardsInput) (*struct{}, error) {
		if err := svc.ReorderDashboards(ctx, in.SiteID, in.Body.DashboardIDs); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type DashboardIDInput struct {
		SiteID      uuid.UUID `path:"siteId" format:"uuid"`
		DashboardID uuid.UUID `path:"dashboardId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteDashboard",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/dashboards/{dashboardId}",
		Summary:       "Delete a WebView dashboard page and its widgets (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *DashboardIDInput) (*struct{}, error) {
		if err := svc.DeleteDashboard(ctx, in.SiteID, in.DashboardID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type CreateWidgetInput struct {
		SiteID      uuid.UUID `path:"siteId" format:"uuid"`
		DashboardID uuid.UUID `path:"dashboardId" format:"uuid"`
		Body        struct {
			Name              string         `json:"name" minLength:"1" maxLength:"100"`
			WidgetType        string         `json:"widgetType" enum:"lamp,display,switch,button,number_input"`
			RegisterMappingID *uuid.UUID     `json:"registerMappingId,omitempty" format:"uuid"`
			SortOrder         int            `json:"sortOrder,omitempty"`
			Style             map[string]any `json:"style,omitempty"`
			Extra             map[string]any `json:"extra,omitempty"`
		}
	}
	type WidgetOutput struct {
		Body WidgetDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateWidget",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/dashboards/{dashboardId}/widgets",
		Summary:       "Add a widget to a dashboard (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateWidgetInput) (*WidgetOutput, error) {
		w, err := svc.CreateWidget(ctx, in.SiteID, in.DashboardID, &models.Widget{
			RegisterMappingID: in.Body.RegisterMappingID,
			Name:              in.Body.Name,
			WidgetType:        in.Body.WidgetType,
			SortOrder:         in.Body.SortOrder,
			Style:             in.Body.Style,
			Extra:             in.Body.Extra,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &WidgetOutput{Body: toWidgetDTO(*w)}, nil
	})

	type ReorderWidgetsInput struct {
		SiteID      uuid.UUID `path:"siteId" format:"uuid"`
		DashboardID uuid.UUID `path:"dashboardId" format:"uuid"`
		Body        struct {
			WidgetIDs []uuid.UUID `json:"widgetIds" minItems:"1"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorReorderWidgets",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/dashboards/{dashboardId}/widgets/reorder",
		Summary:       "Persist display order for widgets on a dashboard (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *ReorderWidgetsInput) (*struct{}, error) {
		if err := svc.ReorderWidgets(ctx, in.SiteID, in.DashboardID, in.Body.WidgetIDs); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type UpdateWidgetInput struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		WidgetID uuid.UUID `path:"widgetId" format:"uuid"`
		Body     struct {
			Name              *string        `json:"name,omitempty" minLength:"1" maxLength:"100"`
			WidgetType        *string        `json:"widgetType,omitempty" enum:"lamp,display,switch,button,number_input"`
			RegisterMappingID *uuid.UUID     `json:"registerMappingId,omitempty" format:"uuid"`
			SortOrder         *int           `json:"sortOrder,omitempty"`
			Style             map[string]any `json:"style,omitempty"`
			Extra             map[string]any `json:"extra,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateWidget",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/widgets/{widgetId}",
		Summary:     "Update a widget (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateWidgetInput) (*WidgetOutput, error) {
		w, err := svc.UpdateWidget(ctx, in.SiteID, in.WidgetID, func(cur *models.Widget) {
			if in.Body.Name != nil {
				cur.Name = strings.TrimSpace(*in.Body.Name)
			}
			if in.Body.WidgetType != nil {
				cur.WidgetType = *in.Body.WidgetType
			}
			if in.Body.RegisterMappingID != nil {
				cur.RegisterMappingID = in.Body.RegisterMappingID
			}
			if in.Body.SortOrder != nil {
				cur.SortOrder = *in.Body.SortOrder
			}
			if in.Body.Style != nil {
				cur.Style = in.Body.Style
			}
			if in.Body.Extra != nil {
				cur.Extra = in.Body.Extra
			}
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &WidgetOutput{Body: toWidgetDTO(*w)}, nil
	})

	type WidgetIDInput struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		WidgetID uuid.UUID `path:"widgetId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteWidget",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/widgets/{widgetId}",
		Summary:       "Delete a widget (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *WidgetIDInput) (*struct{}, error) {
		if err := svc.DeleteWidget(ctx, in.SiteID, in.WidgetID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// --- Service-Level Fine-Grained Member Write Policies ---
	type ListPoliciesOutput struct {
		Body struct {
			Policies []MemberPolicyDTO `json:"policies"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListMemberPolicies",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/member-policies",
		Summary:     "List service-level SITE_MEMBER register write policies (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListPoliciesOutput, error) {
		list, err := svc.ListMemberPolicies(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListPoliciesOutput{}
		out.Body.Policies = make([]MemberPolicyDTO, len(list))
		for i, p := range list {
			out.Body.Policies[i] = MemberPolicyDTO{
				UserID:    p.UserID,
				CanWrite:  p.CanWrite,
				UpdatedAt: p.UpdatedAt,
			}
		}
		return out, nil
	})

	type UpsertPolicyInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			UserID   uuid.UUID `json:"userId" format:"uuid"`
			CanWrite bool      `json:"canWrite"`
		}
	}
	type PolicyOutput struct {
		Body MemberPolicyDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpsertMemberPolicy",
		Method:      http.MethodPut,
		Path:        "/api/v1/webview/sites/{siteId}/operator/member-policies",
		Summary:     "Grant or revoke register write permission for a SITE_MEMBER (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpsertPolicyInput) (*PolicyOutput, error) {
		p, err := svc.UpsertMemberPolicy(ctx, in.SiteID, in.Body.UserID, in.Body.CanWrite)
		if err != nil {
			return nil, mapError(err)
		}
		return &PolicyOutput{Body: MemberPolicyDTO{
			UserID:    p.UserID,
			CanWrite:  p.CanWrite,
			UpdatedAt: p.UpdatedAt,
		}}, nil
	})

	type PolicyUserInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		UserID uuid.UUID `path:"userId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteMemberPolicy",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/member-policies/{userId}",
		Summary:       "Remove explicit member write policy override (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *PolicyUserInput) (*struct{}, error) {
		if err := svc.DeleteMemberPolicy(ctx, in.SiteID, in.UserID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// --- Alarm Rules Management (Site Operator) ---
	type ListAlarmRulesOutput struct {
		Body struct {
			Rules []engine.AlarmRuleView `json:"rules"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListAlarmRules",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/alarm-rules",
		Summary:     "List alarm rules and live latch state for a site (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListAlarmRulesOutput, error) {
		rules, err := svc.ListAlarmRules(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListAlarmRulesOutput{}
		out.Body.Rules = rules
		return out, nil
	})

	type CreateAlarmRuleReq struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			RegisterMappingID   uuid.UUID `json:"registerMappingId" format:"uuid"`
			Name                string    `json:"name" minLength:"1" maxLength:"120"`
			Condition           string    `json:"condition" enum:"GT,GTE,LT,LTE,EQ,NEQ"`
			Threshold           float64   `json:"threshold"`
			ClearDeadband       float64   `json:"clearDeadband,omitempty" minimum:"0"`
			HoldSeconds         int       `json:"holdSeconds,omitempty" minimum:"0" maximum:"86400"`
			PollIntervalSeconds int       `json:"pollIntervalSeconds,omitempty" minimum:"1" maximum:"3600"`
			Severity            string    `json:"severity,omitempty" enum:"INFO,WARNING,CRITICAL"`
			NotifyEmails        []string  `json:"notifyEmails,omitempty" maxItems:"50"`
			NotifyOnClear       bool      `json:"notifyOnClear,omitempty"`
			IsActive            *bool     `json:"isActive,omitempty"`
		}
	}
	type AlarmRuleOutput struct {
		Body engine.AlarmRuleView
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateAlarmRule",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/alarm-rules",
		Summary:       "Create a latch/re-arm alarm rule evaluated in scaled engineering units (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateAlarmRuleReq) (*AlarmRuleOutput, error) {
		view, err := svc.CreateAlarmRule(ctx, in.SiteID, engine.CreateAlarmRuleInput{
			RegisterMappingID:   in.Body.RegisterMappingID,
			Name:                in.Body.Name,
			Condition:           in.Body.Condition,
			Threshold:           in.Body.Threshold,
			ClearDeadband:       in.Body.ClearDeadband,
			HoldSeconds:         in.Body.HoldSeconds,
			PollIntervalSeconds: in.Body.PollIntervalSeconds,
			Severity:            in.Body.Severity,
			NotifyEmails:        in.Body.NotifyEmails,
			NotifyOnClear:       in.Body.NotifyOnClear,
			IsActive:            in.Body.IsActive,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &AlarmRuleOutput{Body: *view}, nil
	})

	type UpdateAlarmRuleReq struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		RuleID uuid.UUID `path:"ruleId" format:"uuid"`
		Body   struct {
			RegisterMappingID   *uuid.UUID `json:"registerMappingId,omitempty" format:"uuid"`
			Name                *string    `json:"name,omitempty" minLength:"1" maxLength:"120"`
			Condition           *string    `json:"condition,omitempty" enum:"GT,GTE,LT,LTE,EQ,NEQ"`
			Threshold           *float64   `json:"threshold,omitempty"`
			ClearDeadband       *float64   `json:"clearDeadband,omitempty" minimum:"0"`
			HoldSeconds         *int       `json:"holdSeconds,omitempty" minimum:"0" maximum:"86400"`
			PollIntervalSeconds *int       `json:"pollIntervalSeconds,omitempty" minimum:"1" maximum:"3600"`
			Severity            *string    `json:"severity,omitempty" enum:"INFO,WARNING,CRITICAL"`
			NotifyEmails        *[]string  `json:"notifyEmails,omitempty" maxItems:"50"`
			NotifyOnClear       *bool      `json:"notifyOnClear,omitempty"`
			IsActive            *bool      `json:"isActive,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateAlarmRule",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/alarm-rules/{ruleId}",
		Summary:     "Update an alarm rule (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateAlarmRuleReq) (*AlarmRuleOutput, error) {
		view, err := svc.UpdateAlarmRule(ctx, in.SiteID, in.RuleID, engine.UpdateAlarmRuleInput{
			RegisterMappingID:   in.Body.RegisterMappingID,
			Name:                in.Body.Name,
			Condition:           in.Body.Condition,
			Threshold:           in.Body.Threshold,
			ClearDeadband:       in.Body.ClearDeadband,
			HoldSeconds:         in.Body.HoldSeconds,
			PollIntervalSeconds: in.Body.PollIntervalSeconds,
			Severity:            in.Body.Severity,
			NotifyEmails:        in.Body.NotifyEmails,
			NotifyOnClear:       in.Body.NotifyOnClear,
			IsActive:            in.Body.IsActive,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &AlarmRuleOutput{Body: *view}, nil
	})

	type AlarmRuleIDInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		RuleID uuid.UUID `path:"ruleId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorDeleteAlarmRule",
		Method:        http.MethodDelete,
		Path:          "/api/v1/webview/sites/{siteId}/operator/alarm-rules/{ruleId}",
		Summary:       "Delete an alarm rule and auto-clear any open latched event (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *AlarmRuleIDInput) (*struct{}, error) {
		if err := svc.DeleteAlarmRule(ctx, in.SiteID, in.RuleID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	return r
}

func humaAuthMiddleware(humaAPI huma.API, verifier *auth.Verifier) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		raw := ctx.Header("Authorization")
		if len(raw) < 8 || raw[:7] != "Bearer " {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := verifier.Verify(ctx.Context(), raw[7:])
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		next(huma.WithContext(ctx, auth.WithClaims(ctx.Context(), claims)))
	}
}

func humaSiteAuthzMiddleware(humaAPI huma.API, cache *sitecache.Cache, requireOperator bool) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := auth.ClaimsFromContext(ctx.Context())
		if !ok {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "authentication required")
			return
		}
		siteID, err := uuid.Parse(ctx.Param("siteId"))
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusBadRequest, "invalid siteId")
			return
		}
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "invalid token subject")
			return
		}
		authz, err := cache.CheckAuthz(ctx.Context(), siteID, userID)
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusBadGateway, "failed to verify site authorization")
			return
		}
		if !authz.Allowed || !authz.WebviewEnabled {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "WebView access denied for this site")
			return
		}
		if requireOperator && !authz.IsPlatformAdmin && authz.Role != "SITE_OPERATOR" {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "site operator role required")
			return
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), authzCtxKey{}, authz)))
	}
}

func parseHistoryQueryParams(rawIDs, rawFrom, rawTo string) ([]uuid.UUID, time.Time, time.Time, error) {
	now := time.Now().UTC()
	to := now
	if strings.TrimSpace(rawTo) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(rawTo))
		if err != nil {
			return nil, time.Time{}, time.Time{}, fmt.Errorf("invalid 'to' timestamp (RFC3339 required)")
		}
		to = parsed.UTC()
	}
	from := to.Add(-1 * time.Hour)
	if strings.TrimSpace(rawFrom) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(rawFrom))
		if err != nil {
			return nil, time.Time{}, time.Time{}, fmt.Errorf("invalid 'from' timestamp (RFC3339 required)")
		}
		from = parsed.UTC()
	}
	if !to.After(from) {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("'to' timestamp must be after 'from'")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("requested range cannot exceed 366 days")
	}

	var ids []uuid.UUID
	for _, part := range strings.Split(rawIDs, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		u, err := uuid.Parse(p)
		if err != nil {
			return nil, time.Time{}, time.Time{}, fmt.Errorf("invalid mappingId %q", p)
		}
		ids = append(ids, u)
		if len(ids) > 20 {
			return nil, time.Time{}, time.Time{}, fmt.Errorf("at most 20 series may be queried at once")
		}
	}
	return ids, from, to, nil
}

func mapError(err error) error {
	switch {
	case errors.Is(err, history.ErrHistoryUnavailable):
		return huma.Error503ServiceUnavailable(err.Error())
	case errors.Is(err, engine.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, engine.ErrConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, engine.ErrHostOutOfSubnets), errors.Is(err, engine.ErrReadOnlyMapping):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, engine.ErrWriteForbidden), errors.Is(err, engine.ErrForbidden):
		return huma.Error403Forbidden(err.Error())
	default:
		return huma.Error400BadRequest(err.Error())
	}
}
