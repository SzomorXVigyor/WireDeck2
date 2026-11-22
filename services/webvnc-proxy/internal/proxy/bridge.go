package proxy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/uptrace/bun"

	wcrypto "github.com/SzomorXVigyor/WireDeck2/pkg/crypto"
	"github.com/SzomorXVigyor/WireDeck2/pkg/wireguard"
	"github.com/SzomorXVigyor/WireDeck2/pkg/wirerelay"
	pb "github.com/SzomorXVigyor/WireDeck2/pkg/wirerelay/v1"
	"github.com/SzomorXVigyor/WireDeck2/services/webvnc-proxy/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/webvnc-proxy/internal/rfb"
	"github.com/SzomorXVigyor/WireDeck2/services/webvnc-proxy/internal/sitecache"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("target name already exists on this site")
	ErrHostOutOfSubnets = errors.New("target host IP is outside the site's allowed LAN subnets")
	ErrInvalidTicket    = errors.New("invalid or expired WebSocket ticket")
	ErrForbidden        = errors.New("site VNC access denied")
)

type Config struct {
	IdleTimeout   time.Duration
	MaxSession    time.Duration
	TicketTTL     time.Duration
	DevDirectDial bool // Dev-only: bypass WireRelay gRPC and dial TCP directly
}

type Ticket struct {
	SiteID    uuid.UUID
	TargetID  uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

type activeSession struct {
	cancel func(reason string)
	done   chan struct{}
}

type Service struct {
	db     *bun.DB
	cipher *wcrypto.Cipher
	cache  *sitecache.Cache
	cfg    Config

	ticketMu sync.Mutex
	tickets  map[string]Ticket

	sessMu   sync.Mutex
	sessions map[uuid.UUID]map[uuid.UUID]*activeSession // siteID -> sessionID -> handle
}

func NewService(db *bun.DB, cipher *wcrypto.Cipher, cache *sitecache.Cache, cfg Config) *Service {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 15 * time.Minute
	}
	if cfg.MaxSession <= 0 {
		cfg.MaxSession = 8 * time.Hour
	}
	if cfg.TicketTTL <= 0 {
		cfg.TicketTTL = 30 * time.Second
	}
	return &Service{
		db:       db,
		cipher:   cipher,
		cache:    cache,
		cfg:      cfg,
		tickets:  make(map[string]Ticket),
		sessions: make(map[uuid.UUID]map[uuid.UUID]*activeSession),
	}
}

// --- Target CRUD ---

func (s *Service) ListTargets(ctx context.Context, siteID uuid.UUID, activeOnly bool) ([]models.VncTarget, error) {
	var list []models.VncTarget
	q := s.db.NewSelect().Model(&list).Where("site_id = ?", siteID).Order("name ASC")
	if activeOnly {
		q = q.Where("is_active = TRUE")
	}
	return list, q.Scan(ctx)
}

type CreateTargetInput struct {
	SiteID     uuid.UUID
	Name       string
	Host       string
	Port       int
	Password   string
	LanSubnets []string
}

func (s *Service) CreateTarget(ctx context.Context, in CreateTargetInput) (*models.VncTarget, error) {
	if !s.cfg.DevDirectDial && len(in.LanSubnets) > 0 && !wireguard.ContainsHost(in.Host, in.LanSubnets) {
		return nil, ErrHostOutOfSubnets
	}
	port := in.Port
	if port == 0 {
		port = 5900
	}
	var encPass []byte
	if in.Password != "" {
		var err error
		encPass, err = s.cipher.Encrypt([]byte(in.Password))
		if err != nil {
			return nil, fmt.Errorf("encrypt vnc password: %w", err)
		}
	}

	t := &models.VncTarget{
		ID:                uuid.New(),
		SiteID:            in.SiteID,
		Name:              strings.TrimSpace(in.Name),
		Host:              strings.TrimSpace(in.Host),
		Port:              port,
		PasswordEncrypted: encPass,
		IsActive:          true,
	}
	if _, err := s.db.NewInsert().Model(t).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return t, nil
}

type UpdateTargetInput struct {
	SiteID        uuid.UUID
	TargetID      uuid.UUID
	Name          *string
	Host          *string
	Port          *int
	Password      *string
	ClearPassword bool
	IsActive      *bool
	LanSubnets    []string
}

func (s *Service) UpdateTarget(ctx context.Context, in UpdateTargetInput) (*models.VncTarget, error) {
	t := new(models.VncTarget)
	if err := s.db.NewSelect().Model(t).Where("id = ? AND site_id = ?", in.TargetID, in.SiteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if in.Name != nil {
		t.Name = strings.TrimSpace(*in.Name)
	}
	if in.Host != nil {
		h := strings.TrimSpace(*in.Host)
		if !s.cfg.DevDirectDial && len(in.LanSubnets) > 0 && !wireguard.ContainsHost(h, in.LanSubnets) {
			return nil, ErrHostOutOfSubnets
		}
		t.Host = h
	}
	if in.Port != nil {
		t.Port = *in.Port
	}
	if in.ClearPassword {
		t.PasswordEncrypted = nil
	} else if in.Password != nil && *in.Password != "" {
		enc, err := s.cipher.Encrypt([]byte(*in.Password))
		if err != nil {
			return nil, err
		}
		t.PasswordEncrypted = enc
	}
	if in.IsActive != nil {
		t.IsActive = *in.IsActive
	}
	t.UpdatedAt = time.Now().UTC()

	_, err := s.db.NewUpdate().Model(t).
		Column("name", "host", "port", "password_encrypted", "is_active", "updated_at").
		WherePK().Exec(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return t, nil
}

func (s *Service) DeleteTarget(ctx context.Context, siteID, targetID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.VncTarget)(nil)).
		Where("id = ? AND site_id = ?", targetID, siteID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListSessions(ctx context.Context, siteID uuid.UUID, limit int) ([]models.VncSession, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []models.VncSession
	err := s.db.NewSelect().Model(&list).
		Relation("Target").
		Where("vs.site_id = ?", siteID).
		Order("vs.started_at DESC").
		Limit(limit).
		Scan(ctx)
	return list, err
}

// --- Single-Use WS Ticket ---

func (s *Service) IssueTicket(ctx context.Context, siteID, targetID, userID uuid.UUID) (string, time.Time, error) {
	exists, err := s.db.NewSelect().Model((*models.VncTarget)(nil)).
		Where("id = ? AND site_id = ? AND is_active = TRUE", targetID, siteID).
		Exists(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if !exists {
		return "", time.Time{}, ErrNotFound
	}

	raw, _, err := wcrypto.GenerateToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().UTC().Add(s.cfg.TicketTTL)

	s.ticketMu.Lock()
	now := time.Now().UTC()
	for k, v := range s.tickets {
		if now.After(v.ExpiresAt) {
			delete(s.tickets, k)
		}
	}
	s.tickets[raw] = Ticket{
		SiteID:    siteID,
		TargetID:  targetID,
		UserID:    userID,
		ExpiresAt: exp,
	}
	s.ticketMu.Unlock()

	return raw, exp, nil
}

func (s *Service) consumeTicket(raw string) (Ticket, error) {
	s.ticketMu.Lock()
	t, ok := s.tickets[raw]
	if ok {
		delete(s.tickets, raw)
	}
	s.ticketMu.Unlock()
	if !ok || time.Now().UTC().After(t.ExpiresAt) {
		return Ticket{}, ErrInvalidTicket
	}
	return t, nil
}

// --- Drain & Site Deletion ---

// DrainSite terminates all active VNC sessions on siteID and waits up to timeout for teardown.
func (s *Service) DrainSite(ctx context.Context, siteID uuid.UUID, reason string) {
	s.sessMu.Lock()
	m := s.sessions[siteID]
	handles := make([]*activeSession, 0, len(m))
	for _, h := range m {
		handles = append(handles, h)
	}
	s.sessMu.Unlock()

	for _, h := range handles {
		h.cancel(reason)
	}
	for _, h := range handles {
		select {
		case <-h.done:
		case <-ctx.Done():
			return
		}
	}
}

// DeleteSiteData drains active sessions and purges all VNC targets & sessions for siteID.
func (s *Service) DeleteSiteData(ctx context.Context, siteID uuid.UUID) error {
	s.DrainSite(ctx, siteID, "site_deleted")
	_, err := s.db.NewDelete().Model((*models.VncTarget)(nil)).Where("site_id = ?", siteID).Exec(ctx)
	return err
}

// --- Full-Duplex WebSocket <-> WireRelay Tunnel Bridge ---

// PreparedSession holds validated session state resolved before upgrading HTTP to WebSocket.
type PreparedSession struct {
	Ticket    Ticket
	Target    *models.VncTarget
	PlainPass string
}

// PrepareStream atomically consumes the single-use WS ticket, re-verifies site
// authorization, loads the active VNC target, and decrypts any saved VNC password
// before the HTTP handler upgrades the connection to WebSocket.
func (s *Service) PrepareStream(ctx context.Context, rawTicket string) (*PreparedSession, error) {
	ticket, err := s.consumeTicket(rawTicket)
	if err != nil {
		return nil, ErrInvalidTicket
	}

	// Re-verify membership & vncEnabled at connection time
	authz, err := s.cache.CheckAuthz(ctx, ticket.SiteID, ticket.UserID)
	if err != nil || !authz.Allowed || !authz.VncEnabled || (!s.cfg.DevDirectDial && !authz.IsActive) {
		return nil, ErrForbidden
	}

	target := new(models.VncTarget)
	if err := s.db.NewSelect().Model(target).
		Where("id = ? AND site_id = ? AND is_active = TRUE", ticket.TargetID, ticket.SiteID).
		Scan(ctx); err != nil {
		return nil, ErrNotFound
	}

	var plainPass string
	if len(target.PasswordEncrypted) > 0 {
		dec, err := s.cipher.Decrypt(target.PasswordEncrypted)
		if err != nil {
			slog.Error("decrypt_vnc_password_failed", "target_id", target.ID, "error", err)
			return nil, fmt.Errorf("decrypt target credential: %w", err)
		}
		plainPass = string(dec)
	}

	return &PreparedSession{
		Ticket:    ticket,
		Target:    target,
		PlainPass: plainPass,
	}, nil
}

func (s *Service) HandleStream(ctx context.Context, ws *websocket.Conn, prep *PreparedSession) {
	ticket := prep.Ticket
	target := prep.Target
	plainPass := prep.PlainPass

	// Session context bounded by max session duration and drain/disconnect signals
	sessCtx, baseCancel := context.WithTimeout(ctx, s.cfg.MaxSession)
	var reasonMu sync.Mutex
	closeReason := "client_closed"
	setReason := func(r string) {
		reasonMu.Lock()
		if closeReason == "client_closed" {
			closeReason = r
		}
		reasonMu.Unlock()
	}
	cancelWithReason := func(r string) {
		setReason(r)
		baseCancel()
	}
	defer baseCancel()

	// Wrap WebSocket as an io.ReadWriter using coder/websocket's binary net.Conn adapter
	wsConn := websocket.NetConn(sessCtx, ws, websocket.MessageBinary)
	defer wsConn.Close()

	// Dial remote VNC host via WireRelay gRPC TunnelStream (or direct TCP in dev mode)
	targetRW, closeTarget, err := s.openTargetStream(sessCtx, ticket.SiteID, target.Host, target.Port)
	if err != nil {
		slog.Warn("vnc_target_dial_failed", "site_id", ticket.SiteID, "target_id", target.ID, "error", err)
		rfb.WriteBrowserFailure(wsConn, false, fmt.Sprintf("Failed to reach VNC target (%s:%d): %v", target.Host, target.Port, err))
		_ = ws.Close(websocket.StatusTryAgainLater, "failed to reach VNC target")
		return
	}
	defer closeTarget()

	// Dual-mode RFB authentication:
	//   - Saved password configured on edge point -> 100% server-side RFB 3.8 DES auth (D25),
	//     saved password never leaves the backend.
	//   - No saved password -> transparent RFB passthrough so noVNC prompts the user
	//     at connection time if the VNC server requests SecurityType 2, or connects directly for None.
	if plainPass != "" {
		if err := rfb.ProxyHandshake(wsConn, targetRW, plainPass); err != nil {
			slog.Warn("rfb_handshake_failed", "site_id", ticket.SiteID, "target_id", target.ID, "error", err)
			_ = ws.Close(websocket.StatusPolicyViolation, "VNC authentication failed")
			return
		}
	}

	// Persist VncSession open row
	sessID := uuid.New()
	sessRow := &models.VncSession{
		ID:        sessID,
		SiteID:    ticket.SiteID,
		TargetID:  target.ID,
		UserID:    ticket.UserID,
		StartedAt: time.Now().UTC(),
	}
	if _, err := s.db.NewInsert().Model(sessRow).Exec(context.Background()); err != nil {
		slog.Warn("insert_vnc_session_failed", "error", err)
	}

	doneCh := make(chan struct{})
	s.registerSession(ticket.SiteID, sessID, &activeSession{cancel: cancelWithReason, done: doneCh})
	defer func() {
		s.unregisterSession(ticket.SiteID, sessID)
		close(doneCh)
	}()

	var bytesIn, bytesOut atomic.Int64
	lastActivity := atomic.Int64{}
	lastActivity.Store(time.Now().UnixNano())

	// Idle timeout watchdog (D14)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sessCtx.Done():
				return
			case <-ticker.C:
				idle := time.Since(time.Unix(0, lastActivity.Load()))
				if idle >= s.cfg.IdleTimeout {
					cancelWithReason("idle_timeout")
					return
				}
			}
		}
	}()

	// Tear down pipes immediately when sessCtx ends
	go func() {
		<-sessCtx.Done()
		if errors.Is(sessCtx.Err(), context.DeadlineExceeded) {
			setReason("max_duration")
		}
		closeTarget()
		_ = wsConn.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// Browser -> Target
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := wsConn.Read(buf)
			if n > 0 {
				bytesIn.Add(int64(n))
				lastActivity.Store(time.Now().UnixNano())
				if _, werr := targetRW.Write(buf[:n]); werr != nil {
					cancelWithReason("target_write_error")
					return
				}
			}
			if err != nil {
				cancelWithReason("client_closed")
				return
			}
		}
	}()

	// Target -> Browser
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := targetRW.Read(buf)
			if n > 0 {
				bytesOut.Add(int64(n))
				lastActivity.Store(time.Now().UnixNano())
				if _, werr := wsConn.Write(buf[:n]); werr != nil {
					cancelWithReason("client_write_error")
					return
				}
			}
			if err != nil {
				cancelWithReason("target_closed")
				return
			}
		}
	}()

	wg.Wait()

	now := time.Now().UTC()
	reasonMu.Lock()
	finalReason := closeReason
	reasonMu.Unlock()

	_, err = s.db.NewUpdate().Model((*models.VncSession)(nil)).
		Set("ended_at = ?, bytes_in = ?, bytes_out = ?, close_reason = ?",
			now, bytesIn.Load(), bytesOut.Load(), finalReason).
		Where("id = ?", sessID).
		Exec(context.Background())
	if err != nil {
		slog.Warn("update_vnc_session_failed", "session_id", sessID, "error", err)
	}
	slog.Info("vnc_session_closed",
		"site_id", ticket.SiteID,
		"target_id", target.ID,
		"user_id", ticket.UserID,
		"bytes_in", bytesIn.Load(),
		"bytes_out", bytesOut.Load(),
		"reason", finalReason,
	)
}

func (s *Service) registerSession(siteID, sessID uuid.UUID, h *activeSession) {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if s.sessions[siteID] == nil {
		s.sessions[siteID] = make(map[uuid.UUID]*activeSession)
	}
	s.sessions[siteID][sessID] = h
}

func (s *Service) unregisterSession(siteID, sessID uuid.UUID) {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if m := s.sessions[siteID]; m != nil {
		delete(m, sessID)
		if len(m) == 0 {
			delete(s.sessions, siteID)
		}
	}
}

// openTargetStream dials targetHost:targetPort through the site's WireRelay runner gRPC TunnelStream
// (or directly via TCP when DevDirectDial is enabled for local development).
func (s *Service) openTargetStream(ctx context.Context, siteID uuid.UUID, host string, port int) (io.ReadWriter, func(), error) {
	if s.cfg.DevDirectDial {
		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return nil, nil, err
		}
		return conn, func() { _ = conn.Close() }, nil
	}

	runnerAuth, err := s.cache.GetRunnerAuth(ctx, siteID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch runner auth: %w", err)
	}
	grpcConn, client, err := wirerelay.Dial(runnerAuth.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("dial runner %s: %w", runnerAuth.Address, err)
	}

	streamCtx, streamCancel := context.WithCancel(wirerelay.WithAuthToken(ctx, runnerAuth.AuthToken))
	stream, err := client.TunnelStream(streamCtx)
	if err != nil {
		streamCancel()
		_ = grpcConn.Close()
		return nil, nil, fmt.Errorf("open TunnelStream: %w", err)
	}

	if err := stream.Send(&pb.TunnelFrame{
		Frame: &pb.TunnelFrame_Header{
			Header: &pb.TunnelHeader{
				TargetHost: host,
				TargetPort: uint32(port),
				Protocol:   pb.TransportProtocol_TRANSPORT_PROTOCOL_TCP,
				TimeoutMs:  5000,
			},
		},
	}); err != nil {
		streamCancel()
		_ = grpcConn.Close()
		return nil, nil, fmt.Errorf("send TunnelHeader: %w", err)
	}

	ackFrame, err := stream.Recv()
	if err != nil {
		streamCancel()
		_ = grpcConn.Close()
		return nil, nil, fmt.Errorf("recv TunnelAck: %w", err)
	}
	if ack := ackFrame.GetAck(); ack == nil || !ack.GetSuccess() {
		msg := "tunnel rejected"
		if ack != nil && ack.GetErrorMessage() != "" {
			msg = ack.GetErrorMessage()
		}
		streamCancel()
		_ = grpcConn.Close()
		return nil, nil, errors.New(msg)
	}

	adapter := &tunnelReadWriter{stream: stream}
	closer := func() {
		_ = stream.Send(&pb.TunnelFrame{Frame: &pb.TunnelFrame_Close{Close: &pb.TunnelClose{Reason: "proxy_closed"}}})
		_ = stream.CloseSend()
		streamCancel()
		_ = grpcConn.Close()
	}
	return adapter, closer, nil
}

type tunnelReadWriter struct {
	stream pb.WireRelayService_TunnelStreamClient
	rem    []byte
}

func (t *tunnelReadWriter) Read(p []byte) (int, error) {
	if len(t.rem) > 0 {
		n := copy(p, t.rem)
		t.rem = t.rem[n:]
		return n, nil
	}
	for {
		frame, err := t.stream.Recv()
		if err != nil {
			return 0, err
		}
		if frame.GetClose() != nil {
			return 0, io.EOF
		}
		payload := frame.GetPayload()
		if len(payload) == 0 {
			continue
		}
		n := copy(p, payload)
		if n < len(payload) {
			t.rem = append([]byte(nil), payload[n:]...)
		}
		return n, nil
	}
}

func (t *tunnelReadWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	if err := t.stream.Send(&pb.TunnelFrame{
		Frame: &pb.TunnelFrame_Payload{Payload: cp},
	}); err != nil {
		return 0, err
	}
	return len(p), nil
}
