package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	wcrypto "wiredeck/pkg/crypto"
	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/valkey"
	"wiredeck/pkg/wirerelay"
	pb "wiredeck/pkg/wirerelay/v1"
	"wiredeck/services/runner-orchestrator/internal/backend"
	"wiredeck/services/runner-orchestrator/internal/models"
)

var ErrNotFound = errors.New("runner not found")

type Config struct {
	RunnerImage string
}

type connEntry struct {
	conn   *grpc.ClientConn
	client pb.WireRelayServiceClient
}

type Service struct {
	db          *bun.DB
	vk          *valkey.Client
	backend     backend.RunnerBackend
	cfg         Config
	tokenCipher *wcrypto.Cipher
	coreIAM     *internalclient.HTTPClient
	webview     *internalclient.HTTPClient
	webvnc      *internalclient.HTTPClient

	connMu sync.Mutex
	conns  map[uuid.UUID]*connEntry

	cfgLockMu sync.Mutex
	cfgLocks  map[uuid.UUID]*sync.Mutex

	rtMu    sync.RWMutex
	runtime map[uuid.UUID]valkey.RunnerRuntime
}

func NewService(
	db *bun.DB,
	vk *valkey.Client,
	be backend.RunnerBackend,
	cfg Config,
	tokenCipher *wcrypto.Cipher,
	coreIAM *internalclient.HTTPClient,
	webview *internalclient.HTTPClient,
	webvnc *internalclient.HTTPClient,
) *Service {
	return &Service{
		db:          db,
		vk:          vk,
		backend:     be,
		cfg:         cfg,
		tokenCipher: tokenCipher,
		coreIAM:     coreIAM,
		webview:     webview,
		webvnc:      webvnc,
		conns:       make(map[uuid.UUID]*connEntry),
		cfgLocks:    make(map[uuid.UUID]*sync.Mutex),
		runtime:     make(map[uuid.UUID]valkey.RunnerRuntime),
	}
}

func (s *Service) lockSiteConfig(siteID uuid.UUID) func() {
	s.cfgLockMu.Lock()
	mu, ok := s.cfgLocks[siteID]
	if !ok {
		mu = &sync.Mutex{}
		s.cfgLocks[siteID] = mu
	}
	s.cfgLockMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (s *Service) Close() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	for id, c := range s.conns {
		_ = c.conn.Close()
		delete(s.conns, id)
	}
}

func (s *Service) saveRuntime(ctx context.Context, rt valkey.RunnerRuntime) {
	s.rtMu.Lock()
	s.runtime[rt.SiteID] = rt
	s.rtMu.Unlock()

	if s.vk != nil {
		if err := s.vk.SetJSON(ctx, valkey.KeyRunnerRuntime(rt.SiteID), rt, valkey.TTLRunnerRuntime); err != nil {
			slog.Warn("valkey_save_runner_runtime_failed", "site_id", rt.SiteID, "error", err)
		}
	}
}

func (s *Service) getRuntime(ctx context.Context, siteID uuid.UUID) (valkey.RunnerRuntime, bool) {
	if s.vk != nil {
		if rt, ok, err := valkey.GetJSON[valkey.RunnerRuntime](ctx, s.vk, valkey.KeyRunnerRuntime(siteID)); err == nil && ok {
			return rt, true
		}
	}
	s.rtMu.RLock()
	rt, ok := s.runtime[siteID]
	s.rtMu.RUnlock()
	return rt, ok
}

func (s *Service) getRuntimesBatch(ctx context.Context, siteIDs []uuid.UUID) map[uuid.UUID]valkey.RunnerRuntime {
	out := make(map[uuid.UUID]valkey.RunnerRuntime, len(siteIDs))
	if len(siteIDs) == 0 {
		return out
	}

	if s.vk != nil {
		keys := make([]string, len(siteIDs))
		for i, id := range siteIDs {
			keys[i] = valkey.KeyRunnerRuntime(id)
		}
		if byKey, err := valkey.MGetJSON[valkey.RunnerRuntime](ctx, s.vk, keys); err == nil {
			for i, id := range siteIDs {
				if rt, ok := byKey[keys[i]]; ok {
					out[id] = rt
				}
			}
		} else {
			slog.Warn("valkey_mget_runner_runtime_failed", "error", err)
		}
	}

	// Fill any misses from local in-memory copy
	s.rtMu.RLock()
	for _, id := range siteIDs {
		if _, ok := out[id]; !ok {
			if rt, ok := s.runtime[id]; ok {
				out[id] = rt
			}
		}
	}
	s.rtMu.RUnlock()
	return out
}

func (s *Service) deleteRuntime(ctx context.Context, siteID uuid.UUID) {
	s.rtMu.Lock()
	delete(s.runtime, siteID)
	s.rtMu.Unlock()

	if s.vk != nil {
		_ = s.vk.Del(ctx, valkey.KeyRunnerRuntime(siteID), valkey.KeySitesRuntime())
	}
}

func (s *Service) EnsureRunner(ctx context.Context, siteID uuid.UUID, req internalclient.EnsureRunnerRequest) (*internalclient.RunnerRecord, error) {
	desired := req.DesiredState
	if desired != "STOPPED" {
		desired = "RUNNING"
	}

	existing := new(models.Runner)
	err := s.db.NewSelect().Model(existing).Where("site_id = ?", siteID).Scan(ctx)
	isNew := errors.Is(err, sql.ErrNoRows)
	if err != nil && !isNew {
		return nil, err
	}

	prevRT, hasPrevRT := s.getRuntime(ctx, siteID)

	var plainToken string
	var encToken []byte
	if !isNew && len(existing.AuthTokenEncrypted) > 0 {
		dec, decErr := s.tokenCipher.Decrypt(existing.AuthTokenEncrypted)
		if decErr == nil {
			plainToken = string(dec)
			encToken = existing.AuthTokenEncrypted
		}
	}
	if plainToken == "" {
		raw, _, err := wcrypto.GenerateToken(32)
		if err != nil {
			return nil, err
		}
		enc, err := s.tokenCipher.Encrypt([]byte(raw))
		if err != nil {
			return nil, err
		}
		plainToken = raw
		encToken = enc
	}

	// Drain downstream streams before rotating a running container on conf or image change.
	if !isNew && existing.DesiredState == "RUNNING" {
		fpChanged := existing.ConfFingerprint != nil && *existing.ConfFingerprint != req.ConfFingerprint
		imgChanged := existing.Image != s.cfg.RunnerImage
		stopping := desired == "STOPPED"
		if fpChanged || imgChanged || stopping {
			s.drainDownstream(ctx, siteID)
			s.dropGRPCConn(siteID)
		}
	}

	var observed backend.RunnerState
	var lastErr string
	configured := false
	now := time.Now().UTC()

	if desired == "STOPPED" {
		if err := s.backend.Stop(ctx, siteID.String()); err != nil {
			lastErr = err.Error()
		}
		observed, _ = s.backend.Get(ctx, siteID.String())
	} else {
		var prevID string
		if !isNew && existing.ContainerID != nil {
			prevID = *existing.ContainerID
		}
		observed, err = s.backend.Ensure(ctx, backend.RunnerSpec{
			SiteID:          siteID.String(),
			Image:           s.cfg.RunnerImage,
			ConfFingerprint: req.ConfFingerprint,
			AuthToken:       plainToken,
			DisableIPv6:     true,
		})
		if err != nil {
			lastErr = err.Error()
		} else {
			if observed.RuntimeID != prevID || !hasPrevRT || prevRT.ObservedState != "running" {
				s.dropGRPCConn(siteID)
			}
			if cfgErr := s.ensureConfigured(ctx, siteID, req.Conf, plainToken, true); cfgErr != nil {
				lastErr = cfgErr.Error()
			} else {
				configured = true
			}
		}
	}

	name := backend.ContainerName(siteID.String())
	fp := req.ConfFingerprint
	var cid *string
	if observed.RuntimeID != "" {
		idCopy := observed.RuntimeID
		cid = &idCopy
	}

	rec := &models.Runner{
		SiteID:             siteID,
		ContainerName:      name,
		ContainerID:        cid,
		Image:              s.cfg.RunnerImage,
		ConfFingerprint:    &fp,
		AuthTokenEncrypted: encToken,
		DesiredState:       desired,
		UpdatedAt:          now,
	}
	if !isNew {
		rec.ID = existing.ID
		_, err = s.db.NewUpdate().Model(rec).
			Column("container_id", "image", "conf_fingerprint", "auth_token_encrypted", "desired_state", "updated_at").
			WherePK().Exec(ctx)
	} else {
		rec.ID = uuid.New()
		_, err = s.db.NewInsert().Model(rec).Exec(ctx)
	}
	if err != nil {
		return nil, err
	}

	rt := valkey.RunnerRuntime{
		SiteID:           siteID,
		ContainerID:      observed.RuntimeID,
		ObservedState:    observed.ObservedState,
		Configured:       configured,
		Status:           "UNKNOWN",
		HandshakeState:   "UNSPECIFIED",
		LastError:        lastErr,
		LastReconciledAt: &now,
	}
	if desired == "STOPPED" {
		rt.Status = "OFFLINE"
		rt.Configured = false
	} else if hasPrevRT {
		if prevRT.Status != "" {
			rt.Status = prevRT.Status
		}
		if prevRT.HandshakeState != "" {
			rt.HandshakeState = prevRT.HandshakeState
		}
		rt.HandshakeAgeSeconds = prevRT.HandshakeAgeSeconds
		rt.LastHandshakeAt = prevRT.LastHandshakeAt
		rt.LastCheckedAt = prevRT.LastCheckedAt
		rt.Stats = prevRT.Stats
	}
	s.saveRuntime(ctx, rt)
	if s.vk != nil {
		_ = s.vk.Del(ctx, valkey.KeySitesRuntime())
	}

	dto := toDTO(*rec, &rt)
	if lastErr != "" {
		return &dto, fmt.Errorf("runner ensure failed: %s", lastErr)
	}
	return &dto, nil
}

func (s *Service) DeleteRunner(ctx context.Context, siteID uuid.UUID) error {
	s.drainDownstream(ctx, siteID)
	s.dropGRPCConn(siteID)
	if err := s.backend.Remove(ctx, siteID.String()); err != nil {
		slog.Warn("backend_remove_runner_failed", "site_id", siteID, "error", err)
	}
	s.deleteRuntime(ctx, siteID)
	_, err := s.db.NewDelete().Model((*models.Runner)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	return err
}

func (s *Service) RestartRunner(ctx context.Context, siteID uuid.UUID) error {
	var confResp internalclient.RunnerConfResponse
	if _, err := s.coreIAM.DoJSON(ctx, http.MethodGet, "/internal/sites/"+siteID.String()+"/runner-conf", nil, &confResp); err != nil {
		return fmt.Errorf("fetch runner conf from core-iam: %w", err)
	}
	s.drainDownstream(ctx, siteID)
	s.dropGRPCConn(siteID)
	if err := s.backend.Stop(ctx, siteID.String()); err != nil {
		slog.Warn("stop_before_restart_failed", "site_id", siteID, "error", err)
	}
	_, err := s.EnsureRunner(ctx, siteID, internalclient.EnsureRunnerRequest{
		Conf:            confResp.Conf,
		ConfFingerprint: confResp.ConfFingerprint,
		DesiredState:    "RUNNING",
	})
	return err
}

func (s *Service) GetRunner(ctx context.Context, siteID uuid.UUID) (*internalclient.RunnerRecord, error) {
	r := new(models.Runner)
	if err := s.db.NewSelect().Model(r).Where("site_id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var rtPtr *valkey.RunnerRuntime
	if rt, ok := s.getRuntime(ctx, siteID); ok {
		rtPtr = &rt
	}
	dto := toDTO(*r, rtPtr)
	return &dto, nil
}

func (s *Service) ListRunners(ctx context.Context) ([]internalclient.RunnerRecord, error) {
	var rows []models.Runner
	if err := s.db.NewSelect().Model(&rows).Order("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	siteIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		siteIDs[i] = r.SiteID
	}
	rtMap := s.getRuntimesBatch(ctx, siteIDs)

	out := make([]internalclient.RunnerRecord, len(rows))
	for i, r := range rows {
		var rtPtr *valkey.RunnerRuntime
		if rt, ok := rtMap[r.SiteID]; ok {
			rtCopy := rt
			rtPtr = &rtCopy
		}
		out[i] = toDTO(r, rtPtr)
	}
	return out, nil
}

func (s *Service) GetRunnerAuth(ctx context.Context, siteID uuid.UUID) (*internalclient.RunnerAuthResponse, error) {
	r := new(models.Runner)
	if err := s.db.NewSelect().Model(r).Where("site_id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	tok := ""
	if len(r.AuthTokenEncrypted) > 0 {
		dec, err := s.tokenCipher.Decrypt(r.AuthTokenEncrypted)
		if err != nil {
			return nil, err
		}
		tok = string(dec)
	}
	return &internalclient.RunnerAuthResponse{
		SiteID:    siteID,
		Address:   wirerelay.RunnerAddress(siteID.String()),
		AuthToken: tok,
	}, nil
}

// ReconcileOnce diffs desired site runner state from core-iam against observed Docker state,
// pushing WireGuard configs over authenticated gRPC after container restarts/reboots and healing drift.
func (s *Service) ReconcileOnce(ctx context.Context) {
	if s.coreIAM == nil || s.coreIAM.BaseURL == "" {
		return
	}
	var desiredList []internalclient.DesiredRunnerSpec
	if _, err := s.coreIAM.DoJSON(ctx, http.MethodGet, "/internal/runners/desired", nil, &desiredList); err != nil {
		slog.Warn("reconcile_fetch_desired_failed", "error", err)
		return
	}

	desiredBySite := make(map[uuid.UUID]internalclient.DesiredRunnerSpec, len(desiredList))
	for _, spec := range desiredList {
		desiredBySite[spec.SiteID] = spec
		obs, err := s.backend.Get(ctx, spec.SiteID.String())
		if err != nil {
			slog.Warn("reconcile_inspect_failed", "site_id", spec.SiteID, "error", err)
			continue
		}

		if spec.DesiredState == "STOPPED" {
			if obs.ObservedState == "running" {
				slog.Info("reconcile_stopping_inactive_runner", "site_id", spec.SiteID)
				s.drainDownstream(ctx, spec.SiteID)
				s.dropGRPCConn(spec.SiteID)
				_ = s.backend.Stop(ctx, spec.SiteID.String())
			}
			now := time.Now().UTC()
			s.saveRuntime(ctx, valkey.RunnerRuntime{
				SiteID:           spec.SiteID,
				ContainerID:      obs.RuntimeID,
				ObservedState:    obs.ObservedState,
				Configured:       false,
				Status:           "OFFLINE",
				HandshakeState:   "UNSPECIFIED",
				LastReconciledAt: &now,
			})
			continue
		}

		// Desired RUNNING: heal if missing, stopped, legacy bind-mount mode, fingerprint/image drifted, or unconfigured
		needsHeal := obs.ObservedState != "running" ||
			obs.ConfFingerprint != spec.ConfFingerprint ||
			obs.Image != s.cfg.RunnerImage ||
			obs.ConfigMode != backend.ConfigModeGRPC

		configured := true
		if !needsHeal {
			var known bool
			configured, known = s.isRunnerConfigured(ctx, spec.SiteID)
			if !known || !configured {
				needsHeal = true
			}
		}

		if needsHeal {
			slog.Info("reconcile_healing_runner",
				"site_id", spec.SiteID,
				"observed", obs.ObservedState,
				"config_mode", obs.ConfigMode,
				"configured", configured,
			)
			var confResp internalclient.RunnerConfResponse
			if _, err := s.coreIAM.DoJSON(ctx, http.MethodGet, "/internal/sites/"+spec.SiteID.String()+"/runner-conf", nil, &confResp); err != nil {
				slog.Warn("reconcile_fetch_conf_failed", "site_id", spec.SiteID, "error", err)
				continue
			}
			if _, err := s.EnsureRunner(ctx, spec.SiteID, internalclient.EnsureRunnerRequest{
				Conf:            confResp.Conf,
				ConfFingerprint: confResp.ConfFingerprint,
				DesiredState:    "RUNNING",
			}); err != nil {
				slog.Warn("reconcile_ensure_failed", "site_id", spec.SiteID, "error", err)
			}
		}
	}

	// Garbage-collect orphaned containers whose site no longer exists in core-iam
	observedAll, err := s.backend.List(ctx)
	if err != nil {
		return
	}
	for _, obs := range observedAll {
		sid, err := uuid.Parse(obs.SiteID)
		if err != nil {
			continue
		}
		if _, ok := desiredBySite[sid]; !ok {
			slog.Info("reconcile_removing_orphaned_runner", "site_id", sid)
			_ = s.DeleteRunner(ctx, sid)
		}
	}
}

// CheckHealthOnce polls GetHealth and GetStats concurrently over persistent gRPC channels across all RUNNING runners,
// publishes live runtime snapshots to Valkey, and persists RunnerHealthEvent rows ONLY on status transitions.
func (s *Service) CheckHealthOnce(ctx context.Context) {
	var runners []models.Runner
	if err := s.db.NewSelect().Model(&runners).Where("desired_state = ?", "RUNNING").Scan(ctx); err != nil {
		slog.Warn("health_loop_list_failed", "error", err)
		return
	}

	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for _, r := range runners {
		wg.Add(1)
		sem <- struct{}{}
		go func(runner models.Runner) {
			defer wg.Done()
			defer func() { <-sem }()
			s.checkOneRunner(ctx, runner)
		}(r)
	}
	wg.Wait()
}

func (s *Service) checkOneRunner(ctx context.Context, r models.Runner) {
	prevRT, hasPrev := s.getRuntime(ctx, r.SiteID)
	prevStatus := "UNKNOWN"
	if hasPrev && prevRT.Status != "" {
		prevStatus = prevRT.Status
	}

	tok := ""
	if len(r.AuthTokenEncrypted) > 0 {
		if dec, err := s.tokenCipher.Decrypt(r.AuthTokenEncrypted); err == nil {
			tok = string(dec)
		}
	}

	client, err := s.getGRPCClient(r.SiteID)
	now := time.Now().UTC()
	newStatus := "OFFLINE"
	newHandshakeState := "INTERFACE_ERROR"
	configured := false
	observedState := prevRT.ObservedState
	var agePtr *int64
	lastHSPtr := prevRT.LastHandshakeAt
	var errMsg string
	var stats *valkey.RunnerStats

	if err != nil {
		errMsg = err.Error()
	} else {
		callCtx, cancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, tok), 5*time.Second)
		resp, callErr := client.GetHealth(callCtx, &pb.HealthRequest{})
		cancel()

		// Fast-path recovery: if the runner restarted on its own and lost its in-memory WG config,
		// fetch from core-iam and push it over gRPC Configure immediately instead of waiting for reconcile.
		if callErr == nil && !resp.GetConfigured() && s.coreIAM != nil && s.coreIAM.BaseURL != "" {
			slog.Info("health_detected_unconfigured_runner", "site_id", r.SiteID)
			var confResp internalclient.RunnerConfResponse
			if _, fetchErr := s.coreIAM.DoJSON(ctx, http.MethodGet, "/internal/sites/"+r.SiteID.String()+"/runner-conf", nil, &confResp); fetchErr != nil {
				slog.Warn("health_reconfigure_fetch_failed", "site_id", r.SiteID, "error", fetchErr)
			} else if cfgErr := s.ensureConfigured(ctx, r.SiteID, confResp.Conf, tok, true); cfgErr != nil {
				slog.Warn("health_reconfigure_failed", "site_id", r.SiteID, "error", cfgErr)
			} else {
				reCtx, reCancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, tok), 5*time.Second)
				resp, callErr = client.GetHealth(reCtx, &pb.HealthRequest{})
				reCancel()
			}
		}

		if callErr != nil {
			errMsg = callErr.Error()
		} else {
			observedState = "running"
			configured = resp.GetConfigured()
			newStatus = mapTunnelStatus(resp.GetStatus())
			newHandshakeState = resp.GetHandshakeState().String()
			age := resp.GetHandshakeAgeSeconds()
			if age >= 0 {
				agePtr = &age
				hs := now.Add(-time.Duration(age) * time.Second)
				lastHSPtr = &hs
			}
			if resp.GetErrorMessage() != "" {
				errMsg = resp.GetErrorMessage()
			}

			// Collect live traffic and runtime counters via GetStats (best-effort; never fails health check).
			statsCtx, statsCancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, tok), 4*time.Second)
			stResp, stErr := client.GetStats(statsCtx, &pb.StatsRequest{})
			statsCancel()
			if stErr == nil {
				stats = buildRunnerStats(stResp, prevRT.Stats, now)
			} else if status.Code(stErr) != codes.Unimplemented {
				slog.Debug("runner_get_stats_failed", "site_id", r.SiteID, "error", stErr)
			}
		}
	}

	cid := ""
	if r.ContainerID != nil {
		cid = *r.ContainerID
	}
	rt := valkey.RunnerRuntime{
		SiteID:              r.SiteID,
		ContainerID:         cid,
		ObservedState:       observedState,
		Configured:          configured,
		Status:              newStatus,
		HandshakeState:      newHandshakeState,
		HandshakeAgeSeconds: agePtr,
		LastHandshakeAt:     lastHSPtr,
		LastCheckedAt:       &now,
		LastReconciledAt:    prevRT.LastReconciledAt,
		LastError:           errMsg,
		Stats:               stats,
	}
	s.saveRuntime(ctx, rt)

	// Write RunnerHealthEvent to Postgres ONLY on status transition
	if prevStatus != newStatus {
		var detailPtr *string
		if errMsg != "" {
			detailCopy := errMsg
			detailPtr = &detailCopy
		}
		ev := &models.RunnerHealthEvent{
			SiteID:     r.SiteID,
			FromStatus: prevStatus,
			ToStatus:   newStatus,
			OccurredAt: now,
			Detail:     detailPtr,
		}
		if _, err := s.db.NewInsert().Model(ev).Exec(ctx); err != nil {
			slog.Warn("health_event_insert_failed", "site_id", r.SiteID, "error", err)
		}
		// Invalidate cached site runtime snapshot so downstream services observe transitions quickly.
		if s.vk != nil {
			_ = s.vk.Del(ctx, valkey.KeySitesRuntime())
		}
		slog.Info("runner_status_transition",
			"site_id", r.SiteID,
			"from", prevStatus,
			"to", newStatus,
			"handshake_state", newHandshakeState,
		)
	}
}

func buildRunnerStats(resp *pb.StatsResponse, prev *valkey.RunnerStats, now time.Time) *valkey.RunnerStats {
	if resp == nil {
		return nil
	}
	peers := make([]valkey.RunnerPeerStats, 0, len(resp.GetPeers()))
	for _, p := range resp.GetPeers() {
		peers = append(peers, valkey.RunnerPeerStats{
			PublicKey: p.GetPublicKey(),
			RxBytes:   p.GetRxBytes(),
			TxBytes:   p.GetTxBytes(),
		})
	}
	var rxRate, txRate float64
	if prev != nil && !prev.CollectedAt.IsZero() {
		dt := now.Sub(prev.CollectedAt).Seconds()
		if dt > 0.5 {
			if resp.GetRxBytes() >= prev.RxBytes {
				rxRate = float64(resp.GetRxBytes()-prev.RxBytes) / dt
			}
			if resp.GetTxBytes() >= prev.TxBytes {
				txRate = float64(resp.GetTxBytes()-prev.TxBytes) / dt
			}
		}
	}
	return &valkey.RunnerStats{
		Interface:            resp.GetInterface(),
		RxBytes:              resp.GetRxBytes(),
		TxBytes:              resp.GetTxBytes(),
		RxBytesPerSec:        rxRate,
		TxBytesPerSec:        txRate,
		UptimeSeconds:        resp.GetUptimeSeconds(),
		ActiveStreams:        int64(resp.GetActiveStreams()),
		MaxStreams:           int64(resp.GetMaxStreams()),
		DialErrorsTotal:      resp.GetDialErrorsTotal(),
		StreamsRejectedTotal: resp.GetStreamsRejectedTotal(),
		Peers:                peers,
		CollectedAt:          now,
	}
}

func (s *Service) isRunnerConfigured(ctx context.Context, siteID uuid.UUID) (bool, bool) {
	r := new(models.Runner)
	if err := s.db.NewSelect().Model(r).Where("site_id = ?", siteID).Scan(ctx); err != nil {
		return false, false
	}
	tok := ""
	if len(r.AuthTokenEncrypted) > 0 {
		dec, err := s.tokenCipher.Decrypt(r.AuthTokenEncrypted)
		if err != nil {
			return false, false
		}
		tok = string(dec)
	}
	client, err := s.getGRPCClient(siteID)
	if err != nil {
		return false, false
	}
	callCtx, cancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, tok), 3*time.Second)
	resp, err := client.GetHealth(callCtx, &pb.HealthRequest{})
	cancel()
	if err != nil {
		return false, false
	}
	return resp.GetConfigured(), true
}

// ensureConfigured waits for the runner's gRPC server to accept connections and pushes
// the WireGuard config via the authenticated Configure RPC if not already configured.
func (s *Service) ensureConfigured(ctx context.Context, siteID uuid.UUID, rawConf string, authToken string, disableIPv6 bool) error {
	unlock := s.lockSiteConfig(siteID)
	defer unlock()

	deadline := time.Now().Add(20 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	var lastErr error
	for {
		client, err := s.getGRPCClient(siteID)
		if err != nil {
			lastErr = err
		} else {
			hCtx, hCancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, authToken), 3*time.Second)
			hResp, hErr := client.GetHealth(hCtx, &pb.HealthRequest{})
			hCancel()

			if hErr == nil {
				if hResp.GetConfigured() {
					return nil
				}
				cCtx, cCancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, authToken), 10*time.Second)
				cResp, cErr := client.Configure(cCtx, &pb.ConfigureRequest{
					Config:      rawConf,
					DisableIpv6: disableIPv6,
				})
				cCancel()

				if cErr == nil {
					if cResp.GetApplied() {
						slog.Info("runner_configured_via_grpc", "site_id", siteID, "interface", cResp.GetInterface())
						return nil
					}
					return fmt.Errorf("wirerelay configure rejected: %s", cResp.GetErrorMessage())
				}

				switch status.Code(cErr) {
				case codes.FailedPrecondition:
					vCtx, vCancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, authToken), 3*time.Second)
					vResp, vErr := client.GetHealth(vCtx, &pb.HealthRequest{})
					vCancel()
					if vErr == nil && vResp.GetConfigured() {
						return nil
					}
					return fmt.Errorf("wirerelay configure precondition failed: %w", cErr)
				case codes.InvalidArgument, codes.Unauthenticated, codes.PermissionDenied, codes.Unimplemented:
					return fmt.Errorf("wirerelay configure failed: %w", cErr)
				default:
					lastErr = cErr
				}
			} else {
				switch status.Code(hErr) {
				case codes.Unauthenticated, codes.PermissionDenied:
					return fmt.Errorf("wirerelay health auth failed: %w", hErr)
				default:
					lastErr = hErr
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out configuring runner %s via gRPC: %w", siteID, lastErr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (s *Service) getGRPCClient(siteID uuid.UUID) (pb.WireRelayServiceClient, error) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if entry, ok := s.conns[siteID]; ok {
		return entry.client, nil
	}
	conn, client, err := wirerelay.Dial(wirerelay.RunnerAddress(siteID.String()))
	if err != nil {
		return nil, err
	}
	s.conns[siteID] = &connEntry{conn: conn, client: client}
	return client, nil
}

func (s *Service) dropGRPCConn(siteID uuid.UUID) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if entry, ok := s.conns[siteID]; ok {
		_ = entry.conn.Close()
		delete(s.conns, siteID)
	}
}

func (s *Service) drainDownstream(ctx context.Context, siteID uuid.UUID) {
	drainCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for name, client := range map[string]*internalclient.HTTPClient{
		"webview-engine": s.webview,
		"webvnc-proxy":   s.webvnc,
	} {
		if client == nil || client.BaseURL == "" {
			continue
		}
		wg.Add(1)
		go func(n string, c *internalclient.HTTPClient) {
			defer wg.Done()
			if _, err := c.DoJSON(drainCtx, http.MethodPost, "/internal/sites/"+siteID.String()+"/drain", nil, nil); err != nil {
				slog.Debug("downstream_drain_skipped_or_failed", "target", n, "site_id", siteID, "error", err)
			}
		}(name, client)
	}
	wg.Wait()
}

func mapTunnelStatus(st pb.TunnelStatus) string {
	switch st {
	case pb.TunnelStatus_TUNNEL_STATUS_CONNECTED:
		return "ONLINE"
	case pb.TunnelStatus_TUNNEL_STATUS_DEGRADED:
		return "DEGRADED"
	default:
		return "OFFLINE"
	}
}

func toDTO(r models.Runner, rt *valkey.RunnerRuntime) internalclient.RunnerRecord {
	statusStr := "UNKNOWN"
	if r.DesiredState == "STOPPED" {
		statusStr = "OFFLINE"
	}
	rec := internalclient.RunnerRecord{
		ID:             r.ID,
		SiteID:         r.SiteID,
		ContainerName:  r.ContainerName,
		Image:          r.Image,
		DesiredState:   r.DesiredState,
		Status:         statusStr,
		HandshakeState: "UNSPECIFIED",
	}
	if r.ContainerID != nil {
		rec.ContainerID = *r.ContainerID
	}
	if r.ConfFingerprint != nil {
		rec.ConfFingerprint = *r.ConfFingerprint
	}
	if rt != nil {
		if rt.ContainerID != "" {
			rec.ContainerID = rt.ContainerID
		}
		rec.ObservedState = rt.ObservedState
		rec.Configured = rt.Configured
		if rt.Status != "" {
			rec.Status = rt.Status
		}
		if rt.HandshakeState != "" {
			rec.HandshakeState = rt.HandshakeState
		}
		rec.HandshakeAgeSeconds = rt.HandshakeAgeSeconds
		rec.LastHandshakeAt = rt.LastHandshakeAt
		rec.LastCheckedAt = rt.LastCheckedAt
		rec.LastError = rt.LastError
		rec.LastReconciledAt = rt.LastReconciledAt
		rec.Stats = rt.Stats
	}
	return rec
}
