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

	"wiredeck/pkg/auth"
	wcrypto "wiredeck/pkg/crypto"
	"wiredeck/pkg/db"
	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/k8s"
	"wiredeck/pkg/mailgopher"
	"wiredeck/pkg/telemetry"
	"wiredeck/pkg/wireguard"
	"wiredeck/services/core-iam/internal/httpapi"
	"wiredeck/services/core-iam/internal/service"
	"wiredeck/services/core-iam/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "run /readyz probe and exit")
	flag.Parse()

	internalAddr := getEnv("INTERNAL_ADDR", ":8081")
	if *healthcheck {
		os.Exit(telemetry.RunHealthcheck("http://127.0.0.1" + normalizePort(internalAddr) + "/readyz"))
	}

	telemetry.SetupLogger("core-iam", getEnv("LOG_LEVEL", "INFO"))

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

	accessTTL := parseDuration(getEnv("ACCESS_TOKEN_TTL", "15m"), 15*time.Minute)
	refreshTTL := parseDuration(getEnv("REFRESH_TOKEN_TTL", "720h"), 30*24*time.Hour)

	signer, err := auth.NewSignerFromBase64PEM(mustEnv("JWT_PRIVATE_KEY_B64"), getEnv("JWT_KEY_ID", "v1"), accessTTL)
	if err != nil {
		slog.Error("jwt_signer_init_failed", "error", err)
		os.Exit(1)
	}
	verifier := auth.NewLocalVerifier(getEnv("JWT_KEY_ID", "v1"), signer.PublicKey())

	wgCipher, err := wcrypto.NewCipherFromBase64(mustEnv("WG_CONF_ENCRYPTION_KEY"))
	if err != nil {
		slog.Error("wg_cipher_init_failed", "error", err)
		os.Exit(1)
	}

	reservedCIDRs, err := wireguard.ParseReservedCIDRs(getEnv("RESERVED_CIDRS", "172.30.248.0/24,172.30.252.0/22"))
	if err != nil {
		slog.Error("parse_reserved_cidrs_failed", "error", err)
		os.Exit(1)
	}

	internalToken := mustEnv("INTERNAL_API_TOKEN")
	iamSvc := service.NewIAMService(
		dbBundle.Bun,
		service.Config{
			PublicBaseURL:  getEnv("PUBLIC_BASE_URL", "http://localhost:5173"),
			RefreshTTL:     refreshTTL,
			ReservedCIDRs:  reservedCIDRs,
			SkipDNSCheck:   isTruthy(os.Getenv("SKIP_WG_DNS_CHECK")),
			InitAdminEmail: os.Getenv("INIT_ADMIN_EMAIL"),
			InitAdminPass:  os.Getenv("INIT_ADMIN_PASSWORD"),
		},
		signer,
		wgCipher,
		mailgopher.NewClient(os.Getenv("MAILGOPHER_URL")),
		internalclient.NewHTTPClient(os.Getenv("ORCHESTRATOR_URL"), internalToken, 20*time.Second),
		internalclient.NewHTTPClient(os.Getenv("WEBVIEW_INTERNAL_URL"), internalToken, 10*time.Second),
		internalclient.NewHTTPClient(os.Getenv("WEBVNC_INTERNAL_URL"), internalToken, 10*time.Second),
	)

	if err := iamSvc.BootstrapAdmin(ctx); err != nil {
		slog.Error("bootstrap_admin_failed", "error", err)
		os.Exit(1)
	}

	// 15s site status projection loop (LeaderElector K8s seam)
	var leader k8s.LeaderElector = k8s.NoopLeader{}
	go leader.RunAsLeader(ctx, "core-iam-status-projection", func(lctx context.Context) {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-lctx.Done():
				return
			case <-ticker.C:
				iamSvc.ProjectRunnerStatuses(lctx)
			}
		}
	})

	publicAddr := getEnv("PUBLIC_ADDR", ":8080")
	publicSrv := &http.Server{
		Addr: publicAddr,
		Handler: httpapi.NewPublicRouter(iamSvc, verifier, httpapi.PublicRouterOptions{
			EnableDocs:   isTruthy(os.Getenv("ENABLE_OPENAPI_DOCS")),
			CookieSecure: !strings.EqualFold(getEnv("COOKIE_SECURE", "true"), "false"),
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	internalSrv := &http.Server{
		Addr:              internalAddr,
		Handler:           httpapi.NewInternalRouter(iamSvc, internalToken, dbBundle.Pool.Ping),
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
	if strings.HasSuffix(raw, "d") {
		var days int
		if _, err := os.Stdout.Write(nil); err == nil {
			for _, ch := range strings.TrimSuffix(raw, "d") {
				if ch >= '0' && ch <= '9' {
					days = days*10 + int(ch-'0')
				}
			}
			if days > 0 {
				return time.Duration(days) * 24 * time.Hour
			}
		}
	}
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
