package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/api"
	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/pkg/telemetry"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/engine"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/sitecache"
)

type authzCtxKey struct{}

// --- DTOs ---

type DeviceDTO struct {
	ID                    uuid.UUID `json:"id"`
	SiteID                uuid.UUID `json:"siteId"`
	Name                  string    `json:"name"`
	Host                  string    `json:"host"`
	Port                  int       `json:"port"`
	Protocol              string    `json:"protocol"`
	UnitID                int       `json:"unitId"`
	MaxConcurrentRequests int       `json:"maxConcurrentRequests"`
	RequestTimeoutMs      int       `json:"requestTimeoutMs"`
	IsActive              bool      `json:"isActive"`
	CreatedAt             time.Time `json:"createdAt"`
}

func toDeviceDTO(d models.Device) DeviceDTO {
	return DeviceDTO{
		ID:                    d.ID,
		SiteID:                d.SiteID,
		Name:                  d.Name,
		Host:                  d.Host,
		Port:                  d.Port,
		Protocol:              d.Protocol,
		UnitID:                d.UnitID,
		MaxConcurrentRequests: d.MaxConcurrentRequests,
		RequestTimeoutMs:      d.RequestTimeoutMs,
		IsActive:              d.IsActive,
		CreatedAt:             d.CreatedAt,
	}
}

type MappingDTO struct {
	ID                        uuid.UUID `json:"id"`
	DeviceID                  uuid.UUID `json:"deviceId"`
	SiteID                    uuid.UUID `json:"siteId"`
	Name                      string    `json:"name"`
	FunctionCode              string    `json:"functionCode"`
	RegisterAddress           int       `json:"registerAddress"`
	RegisterCount             int       `json:"registerCount"`
	DataType                  string    `json:"dataType"`
	WordOrder                 string    `json:"wordOrder"`
	ByteOrder                 string    `json:"byteOrder"`
	Scale                     float64   `json:"scale"`
	Offset                    float64   `json:"offset"`
	Unit                      string    `json:"unit"`
	AccessMode                string    `json:"accessMode"`
	WriteFunctionCode         string    `json:"writeFunctionCode"`
	InteractivePollIntervalMs int       `json:"interactivePollIntervalMs"`
	ArchivalPollIntervalMs    int       `json:"archivalPollIntervalMs"`
	Deadband                  float64   `json:"deadband"`
	MaxIntervalSeconds        int       `json:"maxIntervalSeconds"`
	IsActive                  bool      `json:"isActive"`
}

func toMappingDTO(m models.RegisterMapping) MappingDTO {
	return MappingDTO{
		ID:                        m.ID,
		DeviceID:                  m.DeviceID,
		SiteID:                    m.SiteID,
		Name:                      m.Name,
		FunctionCode:              m.FunctionCode,
		RegisterAddress:           m.RegisterAddress,
		RegisterCount:             m.RegisterCount,
		DataType:                  m.DataType,
		WordOrder:                 m.WordOrder,
		ByteOrder:                 m.ByteOrder,
		Scale:                     m.Scale,
		Offset:                    m.OffsetVal,
		Unit:                      m.Unit,
		AccessMode:                m.AccessMode,
		WriteFunctionCode:         m.WriteFunctionCode,
		InteractivePollIntervalMs: m.InteractivePollIntervalMs,
		ArchivalPollIntervalMs:    m.ArchivalPollIntervalMs,
		Deadband:                  m.Deadband,
		MaxIntervalSeconds:        m.MaxIntervalSeconds,
		IsActive:                  m.IsActive,
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
	ID        uuid.UUID      `json:"id"`
	SiteID    uuid.UUID      `json:"siteId"`
	Name      string         `json:"name"`
	Layout    map[string]any `json:"layout"`
	IsDefault bool           `json:"isDefault"`
	Widgets   []WidgetDTO    `json:"widgets"`
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
	return DashboardDTO{
		ID:        d.ID,
		SiteID:    d.SiteID,
		Name:      d.Name,
		Layout:    ly,
		IsDefault: d.IsDefault,
		Widgets:   ws,
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
			InsecureSkipVerify: true, // Traefik / Vite dev proxy origin handling
		})
		if err != nil {
			return
		}
		hub.ServeConn(req.Context(), ws, ticket)
	})

	humaAPI := api.NewHumaAPI(r, "WireDeck2 WebView Engine API", "2.2.0", "/api/v1/webview", enableDocs)

	type SiteIDInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
	}

	// =========================================================================
	// 1. Site Member Endpoints (/api/v1/webview/sites/{siteId}/*)
	// =========================================================================
	memberGroup := huma.NewGroup(humaAPI)
	memberGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteAuthzMiddleware(humaAPI, cache, false))

	type WorkspaceOutput struct {
		Body struct {
			HistoryEnabled bool           `json:"historyEnabled"`
			CanWrite       bool           `json:"canWrite"`
			Dashboards     []DashboardDTO `json:"dashboards"`
			Mappings       []MappingDTO   `json:"mappings"`
			Devices        []DeviceDTO    `json:"devices"`
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
		settings, err := svc.GetSiteSettings(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		userID, _ := uuid.Parse(claims.Subject)
		canWrite, err := svc.CanUserWrite(ctx, in.SiteID, userID, authz)
		if err != nil {
			return nil, mapError(err)
		}

		out := &WorkspaceOutput{}
		out.Body.HistoryEnabled = settings.HistoryEnabled
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
		Summary:     "Write a value to a writable Modbus register mapping (enforces member write policy)",
		Tags:        []string{"WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *WriteInput) (*WriteOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		authz, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)
		userID, _ := uuid.Parse(claims.Subject)
		if err := svc.WriteRegister(ctx, in.SiteID, userID, authz, in.Body.MappingID, in.Body.Value); err != nil {
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

	// =========================================================================
	// 2. Site Operator Endpoints (/api/v1/webview/sites/{siteId}/operator/*)
	// =========================================================================
	opGroup := huma.NewGroup(humaAPI)
	opGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteAuthzMiddleware(humaAPI, cache, true))

	// --- Devices ---
	type ListDevicesOutput struct {
		Body struct {
			Devices []DeviceDTO `json:"devices"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorListDevices",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/devices",
		Summary:     "List Modbus devices for a site (Site Operator)",
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
			Name                  string `json:"name" minLength:"1" maxLength:"100"`
			Host                  string `json:"host" minLength:"3" maxLength:"64"`
			Port                  int    `json:"port,omitempty" minimum:"1" maximum:"65535"`
			Protocol              string `json:"protocol,omitempty" enum:"MODBUS_TCP,MODBUS_UDP"`
			UnitID                int    `json:"unitId,omitempty" minimum:"0" maximum:"255"`
			MaxConcurrentRequests int    `json:"maxConcurrentRequests,omitempty" minimum:"1" maximum:"8"`
			RequestTimeoutMs      int    `json:"requestTimeoutMs,omitempty" minimum:"200" maximum:"30000"`
		}
	}
	type DeviceOutput struct {
		Body DeviceDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateDevice",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/devices",
		Summary:       "Create a Modbus device (Site Operator)",
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
			SiteID:                in.SiteID,
			Name:                  in.Body.Name,
			Host:                  in.Body.Host,
			Port:                  in.Body.Port,
			Protocol:              in.Body.Protocol,
			UnitID:                in.Body.UnitID,
			MaxConcurrentRequests: in.Body.MaxConcurrentRequests,
			RequestTimeoutMs:      in.Body.RequestTimeoutMs,
			LanSubnets:            subnets,
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
			Name                  *string `json:"name,omitempty" minLength:"1" maxLength:"100"`
			Host                  *string `json:"host,omitempty" minLength:"3" maxLength:"64"`
			Port                  *int    `json:"port,omitempty" minimum:"1" maximum:"65535"`
			Protocol              *string `json:"protocol,omitempty" enum:"MODBUS_TCP,MODBUS_UDP"`
			UnitID                *int    `json:"unitId,omitempty" minimum:"0" maximum:"255"`
			MaxConcurrentRequests *int    `json:"maxConcurrentRequests,omitempty" minimum:"1" maximum:"8"`
			RequestTimeoutMs      *int    `json:"requestTimeoutMs,omitempty" minimum:"200" maximum:"30000"`
			IsActive              *bool   `json:"isActive,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateDevice",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/devices/{deviceId}",
		Summary:     "Update a Modbus device (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateDeviceInput) (*DeviceOutput, error) {
		authz, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)
		var subnets []string
		if authz != nil {
			subnets = authz.LanSubnets
		}
		d, err := svc.UpdateDevice(ctx, in.SiteID, in.DeviceID, engine.UpdateDeviceInput{
			Name:                  in.Body.Name,
			Host:                  in.Body.Host,
			Port:                  in.Body.Port,
			Protocol:              in.Body.Protocol,
			UnitID:                in.Body.UnitID,
			MaxConcurrentRequests: in.Body.MaxConcurrentRequests,
			RequestTimeoutMs:      in.Body.RequestTimeoutMs,
			IsActive:              in.Body.IsActive,
			LanSubnets:            subnets,
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
		Summary:       "Delete a Modbus device and its register mappings (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *DeviceIDInput) (*struct{}, error) {
		if err := svc.DeleteDevice(ctx, in.SiteID, in.DeviceID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// --- Register Mappings ---
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
			DeviceID                  uuid.UUID `json:"deviceId" format:"uuid"`
			Name                      string    `json:"name" minLength:"1" maxLength:"100"`
			FunctionCode              string    `json:"functionCode" enum:"READ_COIL,READ_DISCRETE,READ_HOLDING,READ_INPUT"`
			RegisterAddress           int       `json:"registerAddress" minimum:"0" maximum:"65535"`
			DataType                  string    `json:"dataType" enum:"BOOL,UINT16,INT16,UINT32,INT32,FLOAT32"`
			WordOrder                 string    `json:"wordOrder,omitempty" enum:"BIG,LITTLE"`
			ByteOrder                 string    `json:"byteOrder,omitempty" enum:"BIG,LITTLE"`
			Scale                     float64   `json:"scale,omitempty"`
			Offset                    float64   `json:"offset,omitempty"`
			Unit                      string    `json:"unit,omitempty" maxLength:"24"`
			AccessMode                string    `json:"accessMode,omitempty" enum:"READ,WRITE,READ_WRITE"`
			WriteFunctionCode         string    `json:"writeFunctionCode,omitempty" enum:",WRITE_COIL,WRITE_REGISTER,WRITE_MULTIPLE_REGISTERS"`
			InteractivePollIntervalMs int       `json:"interactivePollIntervalMs,omitempty" minimum:"250" maximum:"60000"`
			ArchivalPollIntervalMs    int       `json:"archivalPollIntervalMs,omitempty" minimum:"1000" maximum:"3600000"`
			Deadband                  float64   `json:"deadband,omitempty" minimum:"0"`
			MaxIntervalSeconds        int       `json:"maxIntervalSeconds,omitempty" minimum:"10" maximum:"86400"`
		}
	}
	type MappingOutput struct {
		Body MappingDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "webviewOperatorCreateMapping",
		Method:        http.MethodPost,
		Path:          "/api/v1/webview/sites/{siteId}/operator/mappings",
		Summary:       "Create a Modbus register mapping (Site Operator)",
		Tags:          []string{"Site Operator - WebView"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *CreateMappingInput) (*MappingOutput, error) {
		m, err := svc.CreateMapping(ctx, &models.RegisterMapping{
			DeviceID:                  in.Body.DeviceID,
			SiteID:                    in.SiteID,
			Name:                      in.Body.Name,
			FunctionCode:              in.Body.FunctionCode,
			RegisterAddress:           in.Body.RegisterAddress,
			DataType:                  in.Body.DataType,
			WordOrder:                 in.Body.WordOrder,
			ByteOrder:                 in.Body.ByteOrder,
			Scale:                     in.Body.Scale,
			OffsetVal:                 in.Body.Offset,
			Unit:                      in.Body.Unit,
			AccessMode:                in.Body.AccessMode,
			WriteFunctionCode:         in.Body.WriteFunctionCode,
			InteractivePollIntervalMs: in.Body.InteractivePollIntervalMs,
			ArchivalPollIntervalMs:    in.Body.ArchivalPollIntervalMs,
			Deadband:                  in.Body.Deadband,
			MaxIntervalSeconds:        in.Body.MaxIntervalSeconds,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &MappingOutput{Body: toMappingDTO(*m)}, nil
	})

	type UpdateMappingInput struct {
		SiteID    uuid.UUID `path:"siteId" format:"uuid"`
		MappingID uuid.UUID `path:"mappingId" format:"uuid"`
		Body      struct {
			Name                      *string  `json:"name,omitempty" minLength:"1" maxLength:"100"`
			FunctionCode              *string  `json:"functionCode,omitempty" enum:"READ_COIL,READ_DISCRETE,READ_HOLDING,READ_INPUT"`
			RegisterAddress           *int     `json:"registerAddress,omitempty" minimum:"0" maximum:"65535"`
			DataType                  *string  `json:"dataType,omitempty" enum:"BOOL,UINT16,INT16,UINT32,INT32,FLOAT32"`
			WordOrder                 *string  `json:"wordOrder,omitempty" enum:"BIG,LITTLE"`
			ByteOrder                 *string  `json:"byteOrder,omitempty" enum:"BIG,LITTLE"`
			Scale                     *float64 `json:"scale,omitempty"`
			Offset                    *float64 `json:"offset,omitempty"`
			Unit                      *string  `json:"unit,omitempty" maxLength:"24"`
			AccessMode                *string  `json:"accessMode,omitempty" enum:"READ,WRITE,READ_WRITE"`
			WriteFunctionCode         *string  `json:"writeFunctionCode,omitempty" enum:",WRITE_COIL,WRITE_REGISTER,WRITE_MULTIPLE_REGISTERS"`
			InteractivePollIntervalMs *int     `json:"interactivePollIntervalMs,omitempty" minimum:"250" maximum:"60000"`
			ArchivalPollIntervalMs    *int     `json:"archivalPollIntervalMs,omitempty" minimum:"1000" maximum:"3600000"`
			Deadband                  *float64 `json:"deadband,omitempty" minimum:"0"`
			MaxIntervalSeconds        *int     `json:"maxIntervalSeconds,omitempty" minimum:"10" maximum:"86400"`
			IsActive                  *bool    `json:"isActive,omitempty"`
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
		m, err := svc.UpdateMapping(ctx, in.SiteID, in.MappingID, func(cur *models.RegisterMapping) {
			if in.Body.Name != nil {
				cur.Name = strings.TrimSpace(*in.Body.Name)
			}
			if in.Body.FunctionCode != nil {
				cur.FunctionCode = *in.Body.FunctionCode
			}
			if in.Body.RegisterAddress != nil {
				cur.RegisterAddress = *in.Body.RegisterAddress
			}
			if in.Body.DataType != nil {
				cur.DataType = *in.Body.DataType
			}
			if in.Body.WordOrder != nil {
				cur.WordOrder = *in.Body.WordOrder
			}
			if in.Body.ByteOrder != nil {
				cur.ByteOrder = *in.Body.ByteOrder
			}
			if in.Body.Scale != nil && *in.Body.Scale != 0 {
				cur.Scale = *in.Body.Scale
			}
			if in.Body.Offset != nil {
				cur.OffsetVal = *in.Body.Offset
			}
			if in.Body.Unit != nil {
				cur.Unit = *in.Body.Unit
			}
			if in.Body.AccessMode != nil {
				cur.AccessMode = *in.Body.AccessMode
			}
			if in.Body.WriteFunctionCode != nil {
				cur.WriteFunctionCode = *in.Body.WriteFunctionCode
			}
			if in.Body.InteractivePollIntervalMs != nil {
				cur.InteractivePollIntervalMs = *in.Body.InteractivePollIntervalMs
			}
			if in.Body.ArchivalPollIntervalMs != nil {
				cur.ArchivalPollIntervalMs = *in.Body.ArchivalPollIntervalMs
			}
			if in.Body.Deadband != nil {
				cur.Deadband = *in.Body.Deadband
			}
			if in.Body.MaxIntervalSeconds != nil {
				cur.MaxIntervalSeconds = *in.Body.MaxIntervalSeconds
			}
			if in.Body.IsActive != nil {
				cur.IsActive = *in.Body.IsActive
			}
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
		Summary:     "Perform a live test read of a register mapping over the site tunnel (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *MappingIDInput) (*TestReadOutput, error) {
		sample, err := svc.TestRead(ctx, in.SiteID, in.MappingID)
		if err != nil {
			return nil, mapError(err)
		}
		return &TestReadOutput{Body: *sample}, nil
	})

	// --- Dashboards & Widgets ---
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
			Name      string `json:"name" minLength:"1" maxLength:"100"`
			IsDefault bool   `json:"isDefault,omitempty"`
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
		d, err := svc.CreateDashboard(ctx, in.SiteID, in.Body.Name, in.Body.IsDefault)
		if err != nil {
			return nil, mapError(err)
		}
		return &DashboardOutput{Body: toDashboardDTO(*d)}, nil
	})

	type UpdateDashboardInput struct {
		SiteID      uuid.UUID `path:"siteId" format:"uuid"`
		DashboardID uuid.UUID `path:"dashboardId" format:"uuid"`
		Body        struct {
			Name      *string `json:"name,omitempty" minLength:"1" maxLength:"100"`
			IsDefault *bool   `json:"isDefault,omitempty"`
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
		d, err := svc.UpdateDashboard(ctx, in.SiteID, in.DashboardID, in.Body.Name, in.Body.IsDefault)
		if err != nil {
			return nil, mapError(err)
		}
		return &DashboardOutput{Body: toDashboardDTO(*d)}, nil
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
			WidgetType        string         `json:"widgetType" enum:"lamp,display,switch,button,number_input,chart"`
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

	type UpdateWidgetInput struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		WidgetID uuid.UUID `path:"widgetId" format:"uuid"`
		Body     struct {
			Name              *string        `json:"name,omitempty" minLength:"1" maxLength:"100"`
			WidgetType        *string        `json:"widgetType,omitempty" enum:"lamp,display,switch,button,number_input,chart"`
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

	// --- WebView History Settings (inside WebView, D13) ---
	type SettingsOutput struct {
		Body struct {
			SiteID         uuid.UUID `json:"siteId"`
			HistoryEnabled bool      `json:"historyEnabled"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorGetSettings",
		Method:      http.MethodGet,
		Path:        "/api/v1/webview/sites/{siteId}/operator/settings",
		Summary:     "Get WebView site settings including history storage toggle (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*SettingsOutput, error) {
		st, err := svc.GetSiteSettings(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &SettingsOutput{}
		out.Body.SiteID = st.SiteID
		out.Body.HistoryEnabled = st.HistoryEnabled
		return out, nil
	})

	type UpdateSettingsInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			HistoryEnabled bool `json:"historyEnabled"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "webviewOperatorUpdateSettings",
		Method:      http.MethodPatch,
		Path:        "/api/v1/webview/sites/{siteId}/operator/settings",
		Summary:     "Toggle WebView history storage for this site (Site Operator)",
		Tags:        []string{"Site Operator - WebView"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateSettingsInput) (*SettingsOutput, error) {
		st, err := svc.UpdateSiteSettings(ctx, in.SiteID, in.Body.HistoryEnabled)
		if err != nil {
			return nil, mapError(err)
		}
		out := &SettingsOutput{}
		out.Body.SiteID = st.SiteID
		out.Body.HistoryEnabled = st.HistoryEnabled
		return out, nil
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

func mapError(err error) error {
	switch {
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
