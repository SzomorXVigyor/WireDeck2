package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"wiredeck/pkg/auth"
	wcrypto "wiredeck/pkg/crypto"
	"wiredeck/pkg/db"
	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/telemetry"
	"wiredeck/pkg/valkey"
	"wiredeck/services/webvnc-proxy/internal/httpapi"
	"wiredeck/services/webvnc-proxy/internal/proxy"
	"wiredeck/services/webvnc-proxy/internal/sitecache"
	"wiredeck/services/webvnc-proxy/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "run /readyz probe and exit")
	flag.Parse()

	internalAddr := getEnv("INTERNAL_ADDR", ":8081")
	if *healthcheck {
		os.Exit(telemetry.RunHealthcheck("http://127.0.0.1" + normalizePort(internalAddr) + "/readyz"))
	}

	telemetry.SetupLogger("webvnc-proxy", getEnv("LOG_LEVEL", "INFO"))

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

	var vkClient *valkey.Client
	if vkAddr := strings.TrimSpace(os.Getenv("VALKEY_ADDR")); vkAddr != "" {
		vkClient, err = valkey.Connect(ctx, vkAddr, os.Getenv("VALKEY_PASSWORD"))
		if err != nil {
			slog.Error("valkey_connect_failed", "addr", vkAddr, "error", err)
			os.Exit(1)
		}
		defer vkClient.Close()
		slog.Info("valkey_connected", "addr", vkAddr)
	}

	passCipher, err := wcrypto.NewCipherFromBase64(mustEnv("VNC_PASSWORD_ENCRYPTION_KEY"))
	if err != nil {
		slog.Error("vnc_cipher_init_failed", "error", err)
		os.Exit(1)
	}

	internalToken := mustEnv("INTERNAL_API_TOKEN")
	coreIAMClient := internalclient.NewHTTPClient(mustEnv("CORE_IAM_INTERNAL_URL"), internalToken, 10*time.Second)
	orchClient := internalclient.NewHTTPClient(os.Getenv("ORCHESTRATOR_URL"), internalToken, 10*time.Second)

	var vncSvc *proxy.Service
	cache := sitecache.New(coreIAMClient, orchClient, func(siteID uuid.UUID, prev, cur internalclient.SiteRuntimeState) {
		if vncSvc != nil && ((prev.VncEnabled && !cur.VncEnabled) || (prev.IsActive && !cur.IsActive)) {
			slog.Info("disconnecting_vnc_sessions_on_site_change", "site_id", siteID)
			go vncSvc.DrainSite(context.Background(), siteID, "site_disabled")
		}
	})

	cache.SetValkey(vkClient)

	vncSvc = proxy.NewService(dbBundle.Bun, passCipher, cache, proxy.Config{
		IdleTimeout:   parseDuration(getEnv("VNC_IDLE_TIMEOUT", "15m"), 15*time.Minute),
		MaxSession:    parseDuration(getEnv("VNC_MAX_SESSION", "8h"), 8*time.Hour),
		DevDirectDial: isTruthy(os.Getenv("DEV_DIRECT_DIAL")),
	})
	vncSvc.SetValkey(vkClient)

	readyCheck := func(rctx context.Context) error {
		if err := dbBundle.Pool.Ping(rctx); err != nil {
			return err
		}
		if vkClient != nil {
			return vkClient.Ping(rctx)
		}
		return nil
	}

	// 15s runtime-state poll from core-iam:8081
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

	verifier := auth.NewRemoteJWKSVerifier(mustEnv("JWKS_URL"))
	publicAddr := getEnv("PUBLIC_ADDR", ":8080")

	publicSrv := &http.Server{
		Addr:              publicAddr,
		Handler:           httpapi.NewPublicRouter(vncSvc, verifier, cache, isTruthy(os.Getenv("ENABLE_OPENAPI_DOCS"))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	internalSrv := &http.Server{
		Addr:              internalAddr,
		Handler:           httpapi.NewInternalRouter(vncSvc, internalToken, readyCheck),
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

func parseDuration(raw string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
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
