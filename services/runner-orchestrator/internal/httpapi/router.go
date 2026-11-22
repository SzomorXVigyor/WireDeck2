package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/pkg/telemetry"
	"github.com/SzomorXVigyor/WireDeck2/services/runner-orchestrator/internal/orchestrator"
)

func NewRouter(svc *orchestrator.Service, internalToken string, readyCheck func(context.Context) error) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	r.Get("/healthz", telemetry.HealthzHandler)
	r.Get("/readyz", telemetry.ReadyzHandler(readyCheck))
	r.Handle("/metrics", telemetry.MetricsHandler())

	r.Group(func(ar chi.Router) {
		ar.Use(auth.RequireInternalToken(internalToken))

		ar.Get("/runners", func(w http.ResponseWriter, r *http.Request) {
			list, err := svc.ListRunners(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, list)
		})

		ar.Get("/runners/{siteId}", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			rec, err := svc.GetRunner(r.Context(), siteID)
			if err != nil {
				if errors.Is(err, orchestrator.ErrNotFound) {
					http.Error(w, "runner not found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, rec)
		})

		ar.Get("/runners/{siteId}/token", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			res, err := svc.GetRunnerAuth(r.Context(), siteID)
			if err != nil {
				if errors.Is(err, orchestrator.ErrNotFound) {
					http.Error(w, "runner not found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, res)
		})

		ar.Put("/runners/{siteId}", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			var req internalclient.EnsureRunnerRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON payload", http.StatusBadRequest)
				return
			}
			rec, err := svc.EnsureRunner(r.Context(), siteID, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, rec)
		})

		ar.Delete("/runners/{siteId}", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			if err := svc.DeleteRunner(r.Context(), siteID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		ar.Post("/runners/{siteId}/restart", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			if err := svc.RestartRunner(r.Context(), siteID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})

	return r
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
