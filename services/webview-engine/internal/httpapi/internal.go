package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"wiredeck/pkg/auth"
	"wiredeck/pkg/telemetry"
	"wiredeck/services/webview-engine/internal/engine"
)

// NewInternalRouter builds the :8081 control-plane API (never routed by Traefik).
func NewInternalRouter(svc *engine.Service, internalToken string, dbPing func(context.Context) error) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	r.Get("/healthz", telemetry.HealthzHandler)
	r.Get("/readyz", telemetry.ReadyzHandler(dbPing))
	r.Handle("/metrics", telemetry.MetricsHandler())

	r.Route("/internal", func(ir chi.Router) {
		ir.Use(auth.RequireInternalToken(internalToken))

		ir.Post("/sites/{siteId}/drain", func(w http.ResponseWriter, req *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(req, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			svc.DrainSite(siteID)
			w.WriteHeader(http.StatusNoContent)
		})

		ir.Delete("/sites/{siteId}/data", func(w http.ResponseWriter, req *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(req, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			if err := svc.DeleteSiteData(req.Context(), siteID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})

	return r
}
