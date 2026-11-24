package httpapi

import (
	"context"
	"errors"
	"net/http"
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
	"wiredeck/services/webvnc-proxy/internal/models"
	"wiredeck/services/webvnc-proxy/internal/proxy"
	"wiredeck/services/webvnc-proxy/internal/sitecache"
)

type authzCtxKey struct{}

type VncTargetDTO struct {
	ID          uuid.UUID `json:"id"`
	SiteID      uuid.UUID `json:"siteId"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	HasPassword bool      `json:"hasPassword"`
	IsActive    bool      `json:"isActive"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toTargetDTO(t models.VncTarget) VncTargetDTO {
	return VncTargetDTO{
		ID:          t.ID,
		SiteID:      t.SiteID,
		Name:        t.Name,
		Host:        t.Host,
		Port:        t.Port,
		HasPassword: len(t.PasswordEncrypted) > 0,
		IsActive:    t.IsActive,
		CreatedAt:   t.CreatedAt,
	}
}

type VncSessionDTO struct {
	ID          uuid.UUID  `json:"id"`
	TargetID    uuid.UUID  `json:"targetId"`
	TargetName  string     `json:"targetName"`
	UserID      uuid.UUID  `json:"userId"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	BytesIn     int64      `json:"bytesIn"`
	BytesOut    int64      `json:"bytesOut"`
	CloseReason string     `json:"closeReason,omitempty"`
}

// NewPublicRouter builds the Huma v2 public router for webvnc-proxy (:8080) with strict
// Site Member vs Site Operator separation and raw binary WebSocket streaming.
func NewPublicRouter(svc *proxy.Service, verifier *auth.Verifier, cache *sitecache.Cache, enableDocs bool) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	// WebSocket binary RFB bridge authenticated via single-use 30s ticket (never JWT in URL).
	// Validate and consume the ticket BEFORE upgrading so unauthenticated requests receive HTTP 401.
	r.Get("/api/v1/vnc/stream", func(w http.ResponseWriter, req *http.Request) {
		ticket := req.URL.Query().Get("ticket")
		if ticket == "" {
			http.Error(w, "missing ticket", http.StatusUnauthorized)
			return
		}
		prep, err := svc.PrepareStream(req.Context(), ticket)
		if err != nil {
			switch {
			case errors.Is(err, proxy.ErrInvalidTicket):
				http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
			case errors.Is(err, proxy.ErrForbidden):
				http.Error(w, "site VNC access denied", http.StatusForbidden)
			case errors.Is(err, proxy.ErrNotFound):
				http.Error(w, "VNC target not found", http.StatusNotFound)
			default:
				http.Error(w, "failed to prepare VNC session", http.StatusInternalServerError)
			}
			return
		}
		ws, err := websocket.Accept(w, req, &websocket.AcceptOptions{
			Subprotocols:       []string{"binary"},
			InsecureSkipVerify: true, // Traefik / Vite dev proxy origin handling
		})
		if err != nil {
			return
		}
		svc.HandleStream(req.Context(), ws, prep)
	})

	humaAPI := api.NewHumaAPI(r, "WireDeck WebVNC Proxy API", version.Version, "/api/v1/vnc", enableDocs)

	// =========================================================================
	// 1. Site Member Endpoints (/api/v1/vnc/sites/{siteId}/*)
	// =========================================================================
	memberGroup := huma.NewGroup(humaAPI)
	memberGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteAuthzMiddleware(humaAPI, cache, false))

	type SiteIDInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
	}
	type ListTargetsOutput struct {
		Body struct {
			Targets []VncTargetDTO `json:"targets"`
		}
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "vncListActiveTargets",
		Method:      http.MethodGet,
		Path:        "/api/v1/vnc/sites/{siteId}/targets",
		Summary:     "List active VNC edge points for a site (Site Member)",
		Tags:        []string{"VNC"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListTargetsOutput, error) {
		list, err := svc.ListTargets(ctx, in.SiteID, true)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListTargetsOutput{}
		out.Body.Targets = make([]VncTargetDTO, len(list))
		for i, t := range list {
			out.Body.Targets[i] = toTargetDTO(t)
		}
		return out, nil
	})

	type IssueTicketInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			TargetID uuid.UUID `json:"targetId" format:"uuid" required:"true"`
		}
	}
	type IssueTicketOutput struct {
		Body struct {
			Ticket    string    `json:"ticket"`
			ExpiresAt time.Time `json:"expiresAt"`
		}
	}
	huma.Register(memberGroup, huma.Operation{
		OperationID: "vncIssueWsTicket",
		Method:      http.MethodPost,
		Path:        "/api/v1/vnc/sites/{siteId}/ws-ticket",
		Summary:     "Issue a single-use 30s WebSocket ticket for a VNC target (Site Member)",
		Tags:        []string{"VNC"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *IssueTicketInput) (*IssueTicketOutput, error) {
		uid, _ := auth.UserIDFromContext(ctx)
		tok, exp, err := svc.IssueTicket(ctx, in.SiteID, in.Body.TargetID, uid)
		if err != nil {
			return nil, mapError(err)
		}
		out := &IssueTicketOutput{}
		out.Body.Ticket = tok
		out.Body.ExpiresAt = exp
		return out, nil
	})

	// =========================================================================
	// 2. Site Operator Endpoints (/api/v1/vnc/sites/{siteId}/operator/*)
	// =========================================================================
	opGroup := huma.NewGroup(humaAPI)
	opGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteAuthzMiddleware(humaAPI, cache, true))

	huma.Register(opGroup, huma.Operation{
		OperationID: "vncOperatorListTargets",
		Method:      http.MethodGet,
		Path:        "/api/v1/vnc/sites/{siteId}/operator/targets",
		Summary:     "List all VNC edge points including inactive (Site Operator)",
		Tags:        []string{"Site Operator"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListTargetsOutput, error) {
		list, err := svc.ListTargets(ctx, in.SiteID, false)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListTargetsOutput{}
		out.Body.Targets = make([]VncTargetDTO, len(list))
		for i, t := range list {
			out.Body.Targets[i] = toTargetDTO(t)
		}
		return out, nil
	})

	type CreateTargetReq struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			Name     string `json:"name" minLength:"1" maxLength:"100" required:"true"`
			Host     string `json:"host" format:"ipv4" required:"true"`
			Port     int    `json:"port,omitempty" minimum:"1" maximum:"65535"`
			Password string `json:"password,omitempty" maxLength:"128"`
		}
	}
	type TargetOutput struct {
		Body VncTargetDTO
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "vncOperatorCreateTarget",
		Method:        http.MethodPost,
		Path:          "/api/v1/vnc/sites/{siteId}/operator/targets",
		Summary:       "Create a new VNC edge point on this site (Site Operator)",
		Tags:          []string{"Site Operator"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *CreateTargetReq) (*TargetOutput, error) {
		az := authzFromCtx(ctx)
		t, err := svc.CreateTarget(ctx, proxy.CreateTargetInput{
			SiteID:     in.SiteID,
			Name:       in.Body.Name,
			Host:       in.Body.Host,
			Port:       in.Body.Port,
			Password:   in.Body.Password,
			LanSubnets: az.LanSubnets,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &TargetOutput{Body: toTargetDTO(*t)}, nil
	})

	type UpdateTargetReq struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		TargetID uuid.UUID `path:"targetId" format:"uuid"`
		Body     struct {
			Name          *string `json:"name,omitempty" minLength:"1" maxLength:"100"`
			Host          *string `json:"host,omitempty" format:"ipv4"`
			Port          *int    `json:"port,omitempty" minimum:"1" maximum:"65535"`
			Password      *string `json:"password,omitempty" maxLength:"128"`
			ClearPassword bool    `json:"clearPassword,omitempty"`
			IsActive      *bool   `json:"isActive,omitempty"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "vncOperatorUpdateTarget",
		Method:      http.MethodPatch,
		Path:        "/api/v1/vnc/sites/{siteId}/operator/targets/{targetId}",
		Summary:     "Update a VNC edge point or rotate its server-side password (Site Operator)",
		Tags:        []string{"Site Operator"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateTargetReq) (*TargetOutput, error) {
		az := authzFromCtx(ctx)
		t, err := svc.UpdateTarget(ctx, proxy.UpdateTargetInput{
			SiteID:        in.SiteID,
			TargetID:      in.TargetID,
			Name:          in.Body.Name,
			Host:          in.Body.Host,
			Port:          in.Body.Port,
			Password:      in.Body.Password,
			ClearPassword: in.Body.ClearPassword,
			IsActive:      in.Body.IsActive,
			LanSubnets:    az.LanSubnets,
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &TargetOutput{Body: toTargetDTO(*t)}, nil
	})

	type TargetIDInput struct {
		SiteID   uuid.UUID `path:"siteId" format:"uuid"`
		TargetID uuid.UUID `path:"targetId" format:"uuid"`
	}
	huma.Register(opGroup, huma.Operation{
		OperationID:   "vncOperatorDeleteTarget",
		Method:        http.MethodDelete,
		Path:          "/api/v1/vnc/sites/{siteId}/operator/targets/{targetId}",
		Summary:       "Delete a VNC edge point (Site Operator)",
		Tags:          []string{"Site Operator"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *TargetIDInput) (*struct{}, error) {
		if err := svc.DeleteTarget(ctx, in.SiteID, in.TargetID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type ListSessionsOutput struct {
		Body struct {
			Sessions []VncSessionDTO `json:"sessions"`
		}
	}
	huma.Register(opGroup, huma.Operation{
		OperationID: "vncOperatorListSessions",
		Method:      http.MethodGet,
		Path:        "/api/v1/vnc/sites/{siteId}/operator/sessions",
		Summary:     "View recent and active VNC sessions on this site (Site Operator)",
		Tags:        []string{"Site Operator"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListSessionsOutput, error) {
		rows, err := svc.ListSessions(ctx, in.SiteID, 50)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListSessionsOutput{}
		out.Body.Sessions = make([]VncSessionDTO, 0, len(rows))
		for _, s := range rows {
			tName := ""
			if s.Target != nil {
				tName = s.Target.Name
			}
			reason := ""
			if s.CloseReason != nil {
				reason = *s.CloseReason
			}
			out.Body.Sessions = append(out.Body.Sessions, VncSessionDTO{
				ID:          s.ID,
				TargetID:    s.TargetID,
				TargetName:  tName,
				UserID:      s.UserID,
				StartedAt:   s.StartedAt,
				EndedAt:     s.EndedAt,
				BytesIn:     s.BytesIn,
				BytesOut:    s.BytesOut,
				CloseReason: reason,
			})
		}
		return out, nil
	})

	return r
}

func authzFromCtx(ctx context.Context) *internalclient.SiteAuthzResponse {
	az, _ := ctx.Value(authzCtxKey{}).(*internalclient.SiteAuthzResponse)
	return az
}

func humaAuthMiddleware(humaAPI huma.API, verifier *auth.Verifier) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		h := ctx.Header("Authorization")
		if len(h) <= 7 || (h[:7] != "Bearer " && h[:7] != "bearer ") {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "missing bearer access token")
			return
		}
		claims, err := verifier.Verify(ctx.Context(), h[7:])
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "invalid or expired access token")
			return
		}
		next(huma.WithContext(ctx, auth.WithClaims(ctx.Context(), claims)))
	}
}

func humaSiteAuthzMiddleware(humaAPI huma.API, cache *sitecache.Cache, requireOperator bool) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		uid, ok := auth.UserIDFromContext(ctx.Context())
		if !ok {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "unauthenticated")
			return
		}
		siteID, err := uuid.Parse(ctx.Param("siteId"))
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusBadRequest, "invalid siteId")
			return
		}
		az, err := cache.CheckAuthz(ctx.Context(), siteID, uid)
		if err != nil || !az.Allowed {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "site access denied")
			return
		}
		if !az.VncEnabled {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "WebVNC is disabled for this site")
			return
		}
		if requireOperator && !az.IsPlatformAdmin && az.Role != "SITE_OPERATOR" {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "site operator privileges required")
			return
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), authzCtxKey{}, az)))
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, proxy.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, proxy.ErrConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, proxy.ErrHostOutOfSubnets):
		return huma.Error422UnprocessableEntity(err.Error())
	default:
		return huma.Error422UnprocessableEntity(err.Error())
	}
}
