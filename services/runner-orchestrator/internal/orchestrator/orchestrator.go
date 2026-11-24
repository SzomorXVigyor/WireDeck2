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

	wcrypto "wiredeck/pkg/crypto"
	"wiredeck/pkg/internalclient"
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
	backend     backend.RunnerBackend
	cfg         Config
	tokenCipher *wcrypto.Cipher
	coreIAM     *internalclient.HTTPClient
	webview     *internalclient.HTTPClient
	webvnc      *internalclient.HTTPClient

	connMu sync.Mutex
	conns  map[uuid.UUID]*connEntry
}

func NewService(
	db *bun.DB,
	be backend.RunnerBackend,
	cfg Config,
	tokenCipher *wcrypto.Cipher,
	coreIAM *internalclient.HTTPClient,
	webview *internalclient.HTTPClient,
	webvnc *internalclient.HTTPClient,
) *Service {
	return &Service{
		db:          db,
		backend:     be,
		cfg:         cfg,
		tokenCipher: tokenCipher,
		coreIAM:     coreIAM,
		webview:     webview,
		webvnc:      webvnc,
		conns:       make(map[uuid.UUID]*connEntry),
	}
}

func (s *Service) Close() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	for id, c := range s.conns {
		_ = c.conn.Close()
		delete(s.conns, id)
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
	var lastErr *string
	now := time.Now().UTC()

	if desired == "STOPPED" {
		if err := s.backend.Stop(ctx, siteID.String()); err != nil {
			msg := err.Error()
			lastErr = &msg
		}
		observed, _ = s.backend.Get(ctx, siteID.String())
	} else {
		observed, err = s.backend.Ensure(ctx, backend.RunnerSpec{
			SiteID:          siteID.String(),
			Image:           s.cfg.RunnerImage,
			Conf:            []byte(req.Conf),
			ConfFingerprint: req.ConfFingerprint,
			AuthToken:       plainToken,
			DisableIPv6:     true,
		})
		if err != nil {
			msg := err.Error()
			lastErr = &msg
		}
	}

	name := backend.ContainerName(siteID.String())
	fp := req.ConfFingerprint
	obsState := observed.ObservedState
	var cid *string
	if observed.RuntimeID != "" {
		idCopy := observed.RuntimeID
		cid = &idCopy
	}

	status := "UNKNOWN"
	if desired == "STOPPED" {
		status = "OFFLINE"
	} else if !isNew {
		status = existing.Status
	}

	rec := &models.Runner{
		SiteID:             siteID,
		ContainerName:      name,
		ContainerID:        cid,
		Image:              s.cfg.RunnerImage,
		ConfFingerprint:    &fp,
		AuthTokenEncrypted: encToken,
		DesiredState:       desired,
		ObservedState:      &obsState,
		Status:             status,
		HandshakeState:     "UNSPECIFIED",
		LastError:          lastErr,
		LastReconciledAt:   &now,
		UpdatedAt:          now,
	}
	if !isNew {
		rec.ID = existing.ID
		rec.HandshakeState = existing.HandshakeState
		rec.HandshakeAgeSeconds = existing.HandshakeAgeSeconds
		rec.LastHandshakeAt = existing.LastHandshakeAt
		rec.LastCheckedAt = existing.LastCheckedAt
		_, err = s.db.NewUpdate().Model(rec).
			Column("container_id", "image", "conf_fingerprint", "auth_token_encrypted",
				"desired_state", "observed_state", "status", "last_error", "last_reconciled_at", "updated_at").
			WherePK().Exec(ctx)
	} else {
		rec.ID = uuid.New()
		_, err = s.db.NewInsert().Model(rec).Exec(ctx)
	}
	if err != nil {
		return nil, err
	}

	dto := toDTO(*rec)
	if lastErr != nil {
		return &dto, fmt.Errorf("runner ensure failed: %s", *lastErr)
	}
	return &dto, nil
}

func (s *Service) DeleteRunner(ctx context.Context, siteID uuid.UUID) error {
	s.drainDownstream(ctx, siteID)
	s.dropGRPCConn(siteID)
	if err := s.backend.Remove(ctx, siteID.String()); err != nil {
		slog.Warn("backend_remove_runner_failed", "site_id", siteID, "error", err)
	}
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
	dto := toDTO(*r)
	return &dto, nil
}

func (s *Service) ListRunners(ctx context.Context) ([]internalclient.RunnerRecord, error) {
	var rows []models.Runner
	if err := s.db.NewSelect().Model(&rows).Order("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]internalclient.RunnerRecord, len(rows))
	for i, r := range rows {
		out[i] = toDTO(r)
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
// pulling missing tmpfs configs from core-iam on host reboot (D22) and healing drift.
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
			continue
		}

		// Desired RUNNING: heal if missing, stopped, tmpfs conf wiped on reboot, fingerprint/image drifted
		needsHeal := obs.ObservedState != "running" ||
			!obs.ConfExists ||
			obs.ConfFingerprint != spec.ConfFingerprint ||
			obs.Image != s.cfg.RunnerImage

		if needsHeal {
			slog.Info("reconcile_healing_runner",
				"site_id", spec.SiteID,
				"observed", obs.ObservedState,
				"conf_exists", obs.ConfExists,
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

// CheckHealthOnce polls GetHealth concurrently over persistent gRPC channels across all RUNNING runners
// and persists RunnerHealthEvent rows ONLY on status transitions.
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
	var agePtr *int64
	var lastHSPtr *time.Time
	var errMsg *string

	if err != nil {
		msg := err.Error()
		errMsg = &msg
	} else {
		callCtx, cancel := context.WithTimeout(wirerelay.WithAuthToken(ctx, tok), 5*time.Second)
		resp, callErr := client.GetHealth(callCtx, &pb.HealthRequest{})
		cancel()
		if callErr != nil {
			msg := callErr.Error()
			errMsg = &msg
		} else {
			newStatus = mapTunnelStatus(resp.GetStatus())
			newHandshakeState = resp.GetHandshakeState().String()
			age := resp.GetHandshakeAgeSeconds()
			if age >= 0 {
				agePtr = &age
				hs := now.Add(-time.Duration(age) * time.Second)
				lastHSPtr = &hs
			}
			if resp.GetErrorMessage() != "" {
				msg := resp.GetErrorMessage()
				errMsg = &msg
			}
		}
	}

	_, err = s.db.NewUpdate().Model((*models.Runner)(nil)).
		Set("status = ?", newStatus).
		Set("handshake_state = ?", newHandshakeState).
		Set("handshake_age_seconds = ?", agePtr).
		Set("last_handshake_at = COALESCE(?, last_handshake_at)", lastHSPtr).
		Set("last_checked_at = ?", now).
		Set("last_error = ?", errMsg).
		Set("updated_at = ?", now).
		Where("id = ?", r.ID).
		Exec(ctx)
	if err != nil {
		slog.Warn("health_update_failed", "site_id", r.SiteID, "error", err)
		return
	}

	// Write RunnerHealthEvent ONLY on status transition
	if r.Status != newStatus {
		ev := &models.RunnerHealthEvent{
			SiteID:     r.SiteID,
			FromStatus: r.Status,
			ToStatus:   newStatus,
			OccurredAt: now,
			Detail:     errMsg,
		}
		if _, err := s.db.NewInsert().Model(ev).Exec(ctx); err != nil {
			slog.Warn("health_event_insert_failed", "site_id", r.SiteID, "error", err)
		}
		slog.Info("runner_status_transition",
			"site_id", r.SiteID,
			"from", r.Status,
			"to", newStatus,
			"handshake_state", newHandshakeState,
		)
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

func toDTO(r models.Runner) internalclient.RunnerRecord {
	rec := internalclient.RunnerRecord{
		ID:                  r.ID,
		SiteID:              r.SiteID,
		ContainerName:       r.ContainerName,
		Image:               r.Image,
		DesiredState:        r.DesiredState,
		Status:              r.Status,
		HandshakeState:      r.HandshakeState,
		HandshakeAgeSeconds: r.HandshakeAgeSeconds,
		LastHandshakeAt:     r.LastHandshakeAt,
		LastCheckedAt:       r.LastCheckedAt,
		LastReconciledAt:    r.LastReconciledAt,
	}
	if r.ContainerID != nil {
		rec.ContainerID = *r.ContainerID
	}
	if r.ConfFingerprint != nil {
		rec.ConfFingerprint = *r.ConfFingerprint
	}
	if r.ObservedState != nil {
		rec.ObservedState = *r.ObservedState
	}
	if r.LastError != nil {
		rec.LastError = *r.LastError
	}
	return rec
}
