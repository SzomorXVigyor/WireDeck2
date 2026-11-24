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

	"wiredeck/pkg/auth"
	"wiredeck/pkg/db"
	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/mailgopher"
	"wiredeck/pkg/telemetry"
	"wiredeck/services/webview-engine/internal/alarm"
	"wiredeck/services/webview-engine/internal/engine"
	"wiredeck/services/webview-engine/internal/history"
	"wiredeck/services/webview-engine/internal/httpapi"
	"wiredeck/services/webview-engine/internal/models"
	"wiredeck/services/webview-engine/internal/ownership"
	"wiredeck/services/webview-engine/internal/provider"
	"wiredeck/services/webview-engine/internal/sitecache"
	"wiredeck/services/webview-engine/migrations"
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
	var alarmEng *alarm.Engine

	cache := sitecache.New(coreIAMClient, orchClient, func(siteID uuid.UUID, prev, cur internalclient.SiteRuntimeState) {
		if svc != nil && ((prev.IsActive && !cur.IsActive) || (prev.WebviewEnabled && !cur.WebviewEnabled)) {
			slog.Info("runtime_state_draining_webview", "site_id", siteID, "is_active", cur.IsActive, "webview_enabled", cur.WebviewEnabled)
			svc.DrainSite(siteID)
		}
		if prev.Status != "OFFLINE" && cur.Status == "OFFLINE" {
			if hub != nil {
				slog.Info("site_offline_marking_telemetry_stale", "site_id", siteID)
				hub.MarkSiteStale(siteID, "site tunnel offline")
			}
			if alarmEng != nil {
				fctx, fcancel := context.WithTimeout(context.Background(), 5*time.Second)
				alarmEng.FreezeSiteOffline(fctx, siteID)
				fcancel()
			}
		}
	})

	providers := provider.NewRegistry(
		provider.NewModbusTCPProvider(cache.GetRunnerAuth, devDirectDial, idleTeardown),
	)
	defer providers.Close()

	hub = engine.NewTelemetryHub(30*time.Second, func(siteID uuid.UUID, mappingIDs []uuid.UUID) {
		if scheduler != nil {
			scheduler.NotifySubscribed(siteID, mappingIDs)
		}
	})

	var owner ownership.Provider = ownership.SingleReplicaOwnership{}
	scheduler = engine.NewScheduler(dbBundle.Bun, providers, cache, hub, owner, graceDur, devDirectDial)
	svc = engine.NewService(dbBundle.Bun, providers, cache, hub, scheduler, devDirectDial)

	// Alarm engine (latch/re-arm state machine + MailGopher notifications + WS push)
	var mailer alarm.Mailer
	if mgURL := strings.TrimSpace(os.Getenv("MAILGOPHER_URL")); mgURL != "" {
		mailer = mailgopher.NewClient(mgURL)
		slog.Info("alarm_mailgopher_enabled", "url", mgURL)
	} else {
		slog.Warn("alarm_mailgopher_disabled_missing_MAILGOPHER_URL")
	}
	alarmEng = alarm.NewEngine(
		alarm.NewBunRepository(dbBundle.Bun),
		mailer,
		cache.SiteName,
		func(siteID uuid.UUID, evt models.AlarmEvent) {
			hub.PublishAlarmEvent(siteID, svc.ToAlarmEventView(evt))
		},
	)
	if err := alarmEng.Start(ctx); err != nil {
		slog.Error("alarm_engine_start_failed", "error", err)
		os.Exit(1)
	}
	svc.SetAlarmEngine(alarmEng)
	scheduler.AddSampleHook(func(sample engine.ValueSample, mapping models.RegisterMapping, _ *models.HistoryEntry) {
		alarmEng.EvaluateSample(ctx, mapping, sample.Quality, sample.Value, sample.Timestamp)
	})

	// Optional TimescaleDB history store + batched COPY ingester
	var tsdbStore history.Store
	var histIngester *history.Ingester
	if tsdbURL := strings.TrimSpace(os.Getenv("TSDB_DATABASE_URL")); tsdbURL != "" {
		st, err := history.NewTimescaleStore(ctx, tsdbURL)
		if err != nil {
			slog.Error("tsdb_connect_failed", "error", err)
			os.Exit(1)
		}
		tsdbStore = st
		svc.SetHistoryStore(tsdbStore)

		flushMs := time.Duration(parseIntEnv("HISTORY_FLUSH_MS", 1000)) * time.Millisecond
		flushRows := parseIntEnv("HISTORY_FLUSH_ROWS", 500)
		histIngester = history.NewIngester(tsdbStore, flushMs, flushRows)

		scheduler.AddSampleHook(func(sample engine.ValueSample, _ models.RegisterMapping, he *models.HistoryEntry) {
			if he == nil || !he.IsActive || sample.Quality != engine.QualityGood || sample.Value == nil {
				return
			}
			histIngester.Record(sample.SiteID, sample.MappingID, sample.Timestamp, *sample.Value, he.PollIntervalSeconds)
		})

		go func() {
			pruneTick := time.NewTicker(time.Hour)
			defer pruneTick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-pruneTick.C:
					svc.PruneExpiredHistory(ctx)
				}
			}
		}()
		slog.Info("tsdb_history_enabled", "flush_ms", flushMs.Milliseconds(), "flush_rows", flushRows)
	} else {
		slog.Warn("tsdb_history_disabled_missing_TSDB_DATABASE_URL")
	}

	readyCheck := func(rctx context.Context) error {
		if err := dbBundle.Pool.Ping(rctx); err != nil {
			return err
		}
		if tsdbStore != nil {
			return tsdbStore.Ping(rctx)
		}
		return nil
	}

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
		Handler:           httpapi.NewInternalRouter(svc, internalToken, readyCheck),
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
	if alarmEng != nil {
		alarmEng.Shutdown(shutCtx)
	}
	if histIngester != nil {
		histIngester.Shutdown(shutCtx)
	}
	if tsdbStore != nil {
		tsdbStore.Close()
	}
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
