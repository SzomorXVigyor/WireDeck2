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

	"wiredeck/pkg/api"
	wcrypto "wiredeck/pkg/crypto"
	"wiredeck/pkg/db"
	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/k8s"
	"wiredeck/pkg/telemetry"
	"wiredeck/services/runner-orchestrator/internal/backend"
	"wiredeck/services/runner-orchestrator/internal/httpapi"
	"wiredeck/services/runner-orchestrator/internal/orchestrator"
	"wiredeck/services/runner-orchestrator/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "run /readyz probe and exit")
	flag.Parse()

	internalAddr := getEnv("INTERNAL_ADDR", ":8081")
	if *healthcheck {
		// Probe localhost on the healthcheck port or bound control IP
		os.Exit(telemetry.RunHealthcheck("http://127.0.0.1:8082/readyz"))
	}

	telemetry.SetupLogger("runner-orchestrator", getEnv("LOG_LEVEL", "INFO"))

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

	tokenCipher, err := wcrypto.NewCipherFromBase64(mustEnv("RUNNER_TOKEN_ENCRYPTION_KEY"))
	if err != nil {
		slog.Error("token_cipher_init_failed", "error", err)
		os.Exit(1)
	}

	dockerBackend, err := backend.NewDockerBackend(backend.DockerConfig{
		NetworkName: getEnv("RUNNER_NETWORK", "wiredeck-runners"),
		ConfDir:     getEnv("RUNNER_CONF_DIR", "/var/lib/wiredeck2/runners"),
	})
	if err != nil {
		slog.Error("docker_backend_init_failed", "error", err)
		os.Exit(1)
	}
	defer dockerBackend.Close()

	runnerImage := mustEnv("RUNNER_IMAGE")
	pullCtx, pullCancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := dockerBackend.EnsureImage(pullCtx, runnerImage); err != nil {
		slog.Warn("startup_runner_image_pull_failed", "image", runnerImage, "error", err)
	}
	pullCancel()

	internalToken := mustEnv("INTERNAL_API_TOKEN")
	orchSvc := orchestrator.NewService(
		dbBundle.Bun,
		dockerBackend,
		orchestrator.Config{
			RunnerImage: runnerImage,
		},
		tokenCipher,
		internalclient.NewHTTPClient(mustEnv("CORE_IAM_INTERNAL_URL"), internalToken, 15*time.Second),
		internalclient.NewHTTPClient(os.Getenv("WEBVIEW_INTERNAL_URL"), internalToken, 10*time.Second),
		internalclient.NewHTTPClient(os.Getenv("WEBVNC_INTERNAL_URL"), internalToken, 10*time.Second),
	)
	defer orchSvc.Close()

	reconcileInterval := parseDuration(getEnv("RECONCILE_INTERVAL", "60s"), 60*time.Second)
	healthInterval := parseDuration(getEnv("HEALTH_INTERVAL", "30s"), 30*time.Second)

	var leader k8s.LeaderElector = k8s.NoopLeader{}

	// 60s reconciliation loop (runs immediately on boot for tmpfs recovery - D22)
	go leader.RunAsLeader(ctx, "runner-orchestrator-reconcile", func(lctx context.Context) {
		orchSvc.ReconcileOnce(lctx)
		ticker := time.NewTicker(reconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-lctx.Done():
				return
			case <-ticker.C:
				orchSvc.ReconcileOnce(lctx)
			}
		}
	})

	// 30s health polling loop
	go leader.RunAsLeader(ctx, "runner-orchestrator-health", func(lctx context.Context) {
		ticker := time.NewTicker(healthInterval)
		defer ticker.Stop()
		for {
			select {
			case <-lctx.Done():
				return
			case <-ticker.C:
				orchSvc.CheckHealthOnce(lctx)
			}
		}
	})

	// Bind internal API strictly on wiredeck-control subnet (D20/D21) so runners on wiredeck-runners cannot reach it
	lis, err := api.ListenOnSubnet(internalAddr, os.Getenv("BIND_SUBNET"))
	if err != nil {
		slog.Error("bind_control_subnet_failed", "error", err)
		os.Exit(1)
	}

	router := httpapi.NewRouter(orchSvc, internalToken, dbBundle.Pool.Ping)
	srv := &http.Server{Handler: router, ReadHeaderTimeout: 10 * time.Second}

	// Localhost-only healthcheck probe server so Docker HEALTHCHECK works regardless of BIND_SUBNET IP
	probeMux := http.NewServeMux()
	probeMux.HandleFunc("/readyz", telemetry.ReadyzHandler(dbBundle.Pool.Ping))
	probeSrv := &http.Server{Addr: "127.0.0.1:8082", Handler: probeMux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = probeSrv.ListenAndServe() }()

	go func() {
		slog.Info("orchestrator_listening", "addr", lis.Addr().String())
		if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
			slog.Error("server_error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting_down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	_ = probeSrv.Shutdown(shutCtx)
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

func parseDuration(raw string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return fallback
}
