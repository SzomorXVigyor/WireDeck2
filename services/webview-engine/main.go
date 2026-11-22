package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	"github.com/SzomorXVigyor/WireDeck2/pkg/db"
	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/pkg/telemetry"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/engine"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/httpapi"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/modbus"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/ownership"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/sitecache"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "run /readyz probe and exit")
	flag.Parse()

	internalAddr := getEnv("INTERNAL_ADDR", ":8081")
	if *healthcheck {
		os.Exit(telemetry.RunHealthcheck("http://127.0.0.1" + normalizePort(internalAddr) + "/readyz"))
	}

	telemetry.SetupLogger("webview-engine", getEnv("LOG_LEVEL", "INFO"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbBundle, err := db.Connect(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		slog.Error("database_connect_failed", "error", err)
		os.Exit(1)
	}
	defer dbBundle.Close()

	if err := dbBundle.RunMigrations(ctx, migrations.FS, "."); err != nil {
		slog.Error("database_migrations_failed", "error", err)
		os.Exit(1)
	}

	internalToken := mustEnv("INTERNAL_API_TOKEN")
	coreIAMClient := internalclient.NewHTTPClient(mustEnv("CORE_IAM_INTERNAL_URL"), internalToken, 10*time.Second)
	orchClient := internalclient.NewHTTPClient(os.Getenv("ORCHESTRATOR_URL"), internalToken, 10*time.Second)

	devDirectDial := isTruthy(os.Getenv("DEV_DIRECT_DIAL"))
	graceDur := time.Duration(parseIntEnv("INTERACTIVE_GRACE_MS", 30000)) * time.Millisecond
	idleTeardown := time.Duration(parseIntEnv("DEVICE_IDLE_TEARDOWN_MS", 300000)) * time.Millisecond

	var svc *engine.Service
	var hub *engine.TelemetryHub
	var scheduler *engine.Scheduler

	cache := sitecache.New(coreIAMClient, orchClient, func(siteID uuid.UUID, prev, cur internalclient.SiteRuntimeState) {
		if svc != nil && ((prev.IsActive && !cur.IsActive) || (prev.WebviewEnabled && !cur.WebviewEnabled)) {
			slog.Info("runtime_state_draining_webview", "site_id", siteID, "is_active", cur.IsActive, "webview_enabled", cur.WebviewEnabled)
			svc.DrainSite(siteID)
		}
		if hub != nil && prev.Status != "OFFLINE" && cur.Status == "OFFLINE" {
			slog.Info("site_offline_marking_telemetry_stale", "site_id", siteID)
			hub.MarkSiteStale(siteID, "site tunnel offline")
		}
	})

	pool := modbus.NewDevicePool(cache.GetRunnerAuth, devDirectDial, idleTeardown)
	defer pool.Close()

	hub = engine.NewTelemetryHub(30*time.Second, func(siteID uuid.UUID, mappingIDs []uuid.UUID) {
		if scheduler != nil {
			scheduler.NotifySubscribed(siteID, mappingIDs)
		}
	})

	var owner ownership.Provider = ownership.SingleReplicaOwnership{}
	scheduler = engine.NewScheduler(dbBundle.Bun, pool, cache, hub, owner, graceDur, devDirectDial)
	svc = engine.NewService(dbBundle.Bun, pool, cache, hub, scheduler, devDirectDial)

	// 15s runtime-state poll loop (D28)
	go func() {
		cache.SyncRuntimeStates(ctx)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cache.SyncRuntimeStates(ctx)
			}
		}
	}()

	// Demand-driven Modbus scheduler
	go scheduler.Run(ctx)

	verifier := auth.NewRemoteJWKSVerifier(mustEnv("JWKS_URL"))

	publicAddr := getEnv("PUBLIC_ADDR", ":8080")
	publicSrv := &http.Server{
		Addr:              publicAddr,
		Handler:           httpapi.NewPublicRouter(svc, hub, verifier, cache, isTruthy(os.Getenv("ENABLE_OPENAPI_DOCS"))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	internalSrv := &http.Server{
		Addr:              internalAddr,
		Handler:           httpapi.NewInternalRouter(svc, internalToken, dbBundle.Pool.Ping),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("public_server_listening", "addr", publicAddr)
		if err := publicSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("public_server_error", "error", err)
			stop()
		}
	}()

	go func() {
		slog.Info("internal_server_listening", "addr", internalAddr)
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("internal_server_error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting_down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = publicSrv.Shutdown(shutCtx)
	_ = internalSrv.Shutdown(shutCtx)
	slog.Info("stopped_cleanly")
}

func getEnv(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		slog.Error("missing_required_env", "key", k)
		os.Exit(1)
	}
	return v
}

func isTruthy(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return s == "true" || s == "1" || s == "yes"
}

func parseIntEnv(k string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func normalizePort(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[idx:]
	}
	return ":8081"
}
