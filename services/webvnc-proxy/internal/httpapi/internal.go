package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"wiredeck/pkg/auth"
	"wiredeck/pkg/telemetry"
	"wiredeck/services/webvnc-proxy/internal/proxy"
)

func NewInternalRouter(svc *proxy.Service, internalToken string, readyCheck func(context.Context) error) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	r.Get("/healthz", telemetry.HealthzHandler)
	r.Get("/readyz", telemetry.ReadyzHandler(readyCheck))
	r.Handle("/metrics", telemetry.MetricsHandler())

	r.Route("/internal", func(ir chi.Router) {
		ir.Use(auth.RequireInternalToken(internalToken))

		ir.Post("/sites/{siteId}/drain", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			svc.DrainSite(r.Context(), siteID, "runner_drain")
			w.WriteHeader(http.StatusNoContent)
		})

		ir.Delete("/sites/{siteId}/data", func(w http.ResponseWriter, r *http.Request) {
			siteID, err := uuid.Parse(chi.URLParam(r, "siteId"))
			if err != nil {
				http.Error(w, "invalid siteId", http.StatusBadRequest)
				return
			}
			if err := svc.DeleteSiteData(r.Context(), siteID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})

	return r
}
