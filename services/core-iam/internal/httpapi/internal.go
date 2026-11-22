package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	"github.com/SzomorXVigyor/WireDeck2/pkg/telemetry"
	"github.com/SzomorXVigyor/WireDeck2/services/core-iam/internal/service"
)

// NewInternalRouter builds the :8081 internal control-plane router (never routed by Traefik).
func NewInternalRouter(svc *service.IAMService, internalToken string, readyCheck func(context.Context) error) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	// Unauthenticated operational probes (internal network only)
	r.Get("/healthz", telemetry.HealthzHandler)
	r.Get("/readyz", telemetry.ReadyzHandler(readyCheck))
	r.Handle("/metrics", telemetry.MetricsHandler())

	// Authenticated internal service-to-service endpoints
	r.Route("/internal", func(ir chi.Router) {
		ir.Use(auth.RequireInternalToken(internalToken))

		ir.Get("/sites/runtime-state", func(w http.ResponseWriter, r *http.Request) {
			states, err := svc.InternalRuntimeStates(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, states)
		})

		ir.Get("/sites/{siteId}/authz", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			userID, err := uuid.Parse(r.URL.Query().Get("user_id"))
			if err != nil {
				http.Error(w, "invalid user_id", http.StatusBadRequest)
				return
			}
			res, err := svc.InternalAuthz(r.Context(), siteID, userID)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					http.Error(w, "site not found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, res)
		})

		ir.Get("/runners/desired", func(w http.ResponseWriter, r *http.Request) {
			specs, err := svc.InternalDesiredRunners(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, specs)
		})

		ir.Get("/sites/{siteId}/runner-conf", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			res, err := svc.InternalRunnerConf(r.Context(), siteID)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					http.Error(w, "site not found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, res)
		})
	})

	return r
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
