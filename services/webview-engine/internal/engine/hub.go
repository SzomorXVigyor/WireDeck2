package engine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	wcrypto "wiredeck/pkg/crypto"
	"wiredeck/pkg/valkey"
)

const (
	QualityGood  = "GOOD"
	QualityStale = "STALE"
	QualityBad   = "BAD"
)

// ValueSample represents a live telemetry reading pushed to subscribers and Phase 4 consumers.
type ValueSample struct {
	SiteID    uuid.UUID `json:"siteId"`
	MappingID uuid.UUID `json:"mappingId"`
	Value     *float64  `json:"value"`
	Quality   string    `json:"quality"` // GOOD | STALE | BAD
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// WSTicket binds a single-use 30s WebSocket handshake ticket to a user and site.
type WSTicket struct {
	UserID    uuid.UUID
	SiteID    uuid.UUID
	ExpiresAt time.Time
}

type wsClientMsg struct {
	Type           string      `json:"type"` // subscribe | unsubscribe | ping
	MappingIDs     []uuid.UUID `json:"mappingIds,omitempty"`
	PollIntervalMs int         `json:"pollIntervalMs,omitempty"`
}

type wsServerMsg struct {
	Type    string        `json:"type"` // snapshot | value_update | alarm_event | pong | error
	Samples []ValueSample `json:"samples,omitempty"`
	Alarm   any           `json:"alarm,omitempty"`
	Message string        `json:"message,omitempty"`
}

type wsClient struct {
	id             uuid.UUID
	userID         uuid.UUID
	siteID         uuid.UUID
	connectedAt    time.Time
	conn           *websocket.Conn
	sendCh         chan []byte
	cancel         context.CancelFunc
	mu             sync.Mutex
	subs           map[uuid.UUID]struct{}
	pollIntervalMs int
}

// TelemetryHub manages single-use WS tickets, multiplexed browser WebSocket clients,
// subscription counts (driving demand-driven polling), and live ValueSample fan-out.
type TelemetryHub struct {
	ticketTTL   time.Duration
	onSubChange func(siteID uuid.UUID, mappingIDs []uuid.UUID)
	vk          *valkey.Client

	ticketMu sync.Mutex
	tickets  map[string]WSTicket

	mu          sync.RWMutex
	clients     map[uuid.UUID]*wsClient
	mappingSubs map[uuid.UUID]map[uuid.UUID]*wsClient // mappingID -> clientID -> client
	latest      map[uuid.UUID]ValueSample             // mappingID -> last known sample
}

// SetValkey enables Valkey-backed single-use WS tickets and warm latest-value mirroring.
func (h *TelemetryHub) SetValkey(vk *valkey.Client) {
	h.vk = vk
}

type SiteClientSummary struct {
	Clients            int
	SubscribedMappings int
	Sessions           []valkey.WebviewActiveSession
}

// SiteClientStats returns active WebSocket client counts, active sessions, and unique subscribed mapping counts per site.
func (h *TelemetryHub) SiteClientStats() map[uuid.UUID]SiteClientSummary {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[uuid.UUID]SiteClientSummary)
	uniqueMappings := make(map[uuid.UUID]map[uuid.UUID]struct{})
	for _, c := range h.clients {
		c.mu.Lock()
		subCount := len(c.subs)
		pollMs := c.pollIntervalMs
		for mid := range c.subs {
			if uniqueMappings[c.siteID] == nil {
				uniqueMappings[c.siteID] = make(map[uuid.UUID]struct{})
			}
			uniqueMappings[c.siteID][mid] = struct{}{}
		}
		c.mu.Unlock()

		st := out[c.siteID]
		st.Clients++
		st.Sessions = append(st.Sessions, valkey.WebviewActiveSession{
			ClientID:        c.id,
			UserID:          c.userID,
			ConnectedAt:     c.connectedAt,
			SubscribedCount: subCount,
			PollIntervalMs:  pollMs,
		})
		out[c.siteID] = st
	}
	for sid, m := range uniqueMappings {
		st := out[sid]
		st.SubscribedMappings = len(m)
		out[sid] = st
	}
	return out
}

func NewTelemetryHub(ticketTTL time.Duration, onSubChange func(uuid.UUID, []uuid.UUID)) *TelemetryHub {
	if ticketTTL <= 0 {
		ticketTTL = 30 * time.Second
	}
	return &TelemetryHub{
		ticketTTL:   ticketTTL,
		onSubChange: onSubChange,
		tickets:     make(map[string]WSTicket),
		clients:     make(map[uuid.UUID]*wsClient),
		mappingSubs: make(map[uuid.UUID]map[uuid.UUID]*wsClient),
		latest:      make(map[uuid.UUID]ValueSample),
	}
}

func (h *TelemetryHub) IssueTicket(siteID, userID uuid.UUID) (string, time.Time, error) {
	raw, _, err := wcrypto.GenerateToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().UTC().Add(h.ticketTTL)
	ticket := WSTicket{UserID: userID, SiteID: siteID, ExpiresAt: exp}

	if h.vk != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := h.vk.SetJSON(ctx, valkey.KeyWebviewWSTicket(raw), ticket, h.ticketTTL)
		cancel()
		if err != nil {
			return "", time.Time{}, err
		}
		return raw, exp, nil
	}

	now := time.Now().UTC()
	h.ticketMu.Lock()
	for k, v := range h.tickets {
		if now.After(v.ExpiresAt) {
			delete(h.tickets, k)
		}
	}
	h.tickets[raw] = ticket
	h.ticketMu.Unlock()
	return raw, exp, nil
}

func (h *TelemetryHub) ConsumeTicket(raw string) (WSTicket, error) {
	if h.vk != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		t, ok, err := valkey.ConsumeJSON[WSTicket](ctx, h.vk, valkey.KeyWebviewWSTicket(raw))
		cancel()
		if err != nil || !ok || time.Now().UTC().After(t.ExpiresAt) {
			return WSTicket{}, errors.New("invalid or expired WebSocket ticket")
		}
		return t, nil
	}

	h.ticketMu.Lock()
	t, ok := h.tickets[raw]
	if ok {
		delete(h.tickets, raw)
	}
	h.ticketMu.Unlock()
	if !ok || time.Now().UTC().After(t.ExpiresAt) {
		return WSTicket{}, errors.New("invalid or expired WebSocket ticket")
	}
	return t, nil
}

// ActiveMappingIntervals returns the fastest requested interactive poll interval
// across all connected browser tabs subscribed to each mapping ID.
func (h *TelemetryHub) ActiveMappingIntervals() map[uuid.UUID]time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[uuid.UUID]time.Duration, len(h.mappingSubs))
	for id, m := range h.mappingSubs {
		if len(m) == 0 {
			continue
		}
		var bestMs int
		for _, c := range m {
			ms := c.pollIntervalMs
			if ms < 1000 {
				ms = 1000
			} else if ms > 60000 {
				ms = 60000
			}
			if bestMs == 0 || ms < bestMs {
				bestMs = ms
			}
		}
		out[id] = time.Duration(bestMs) * time.Millisecond
	}
	return out
}

// Publish stores the latest sample and fans it out to subscribed WebSocket clients.
func (h *TelemetryHub) Publish(samples []ValueSample) {
	if len(samples) == 0 {
		return
	}

	// Group samples per client so each WS frame batches updates cleanly
	perClient := make(map[*wsClient][]ValueSample)

	h.mu.Lock()
	for _, s := range samples {
		h.latest[s.MappingID] = s
		for _, c := range h.mappingSubs[s.MappingID] {
			perClient[c] = append(perClient[c], s)
		}
	}
	h.mu.Unlock()

	// Mirror latest readings to Valkey asynchronously (non-blocking) for warm snapshots across restarts.
	if h.vk != nil {
		bySite := make(map[uuid.UUID]map[string]any)
		for _, s := range samples {
			if bySite[s.SiteID] == nil {
				bySite[s.SiteID] = make(map[string]any)
			}
			bySite[s.SiteID][s.MappingID.String()] = s
		}
		go func(siteBatches map[uuid.UUID]map[string]any) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			for sid, fields := range siteBatches {
				_ = h.vk.HSetManyJSON(ctx, valkey.KeyWebviewLatest(sid), fields, valkey.TTLWebviewLatest)
			}
		}(bySite)
	}

	for c, batch := range perClient {
		raw, err := json.Marshal(wsServerMsg{Type: "value_update", Samples: batch})
		if err != nil {
			continue
		}
		select {
		case c.sendCh <- raw:
		default:
			// Drop oldest burst if slow consumer rather than blocking poll loop
		}
	}
}

// MarkSiteStale marks all cached samples for a site as STALE and notifies live subscribers (D12).
func (h *TelemetryHub) MarkSiteStale(siteID uuid.UUID, reason string) {
	now := time.Now().UTC()
	var updates []ValueSample

	h.mu.RLock()
	for _, s := range h.latest {
		if s.SiteID == siteID && s.Quality != QualityStale {
			cp := s
			cp.Quality = QualityStale
			cp.Error = reason
			cp.Timestamp = now
			updates = append(updates, cp)
		}
	}
	h.mu.RUnlock()

	if len(updates) > 0 {
		h.Publish(updates)
	}
}

// PublishAlarmEvent fans out a live alarm state change to all WebSocket clients connected to siteID.
func (h *TelemetryHub) PublishAlarmEvent(siteID uuid.UUID, alarmPayload any) {
	raw, err := json.Marshal(wsServerMsg{Type: "alarm_event", Alarm: alarmPayload})
	if err != nil {
		return
	}
	h.mu.RLock()
	var targets []*wsClient
	for _, c := range h.clients {
		if c.siteID == siteID {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.sendCh <- raw:
		default:
		}
	}
}

// DisconnectSite drops all active WebSocket clients bound to siteID.
func (h *TelemetryHub) DisconnectSite(siteID uuid.UUID) {
	h.mu.RLock()
	var targets []*wsClient
	for _, c := range h.clients {
		if c.siteID == siteID {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		c.cancel()
	}
}

// ServeConn runs the read/write pumps for an authenticated WebSocket client until close.
func (h *TelemetryHub) ServeConn(ctx context.Context, conn *websocket.Conn, ticket WSTicket) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := &wsClient{
		id:          uuid.New(),
		userID:      ticket.UserID,
		siteID:      ticket.SiteID,
		connectedAt: time.Now().UTC(),
		conn:        conn,
		sendCh:      make(chan []byte, 64),
		cancel:      cancel,
		subs:        make(map[uuid.UUID]struct{}),
	}

	h.mu.Lock()
	h.clients[client.id] = client
	h.mu.Unlock()

	defer func() {
		h.removeClient(client)
		_ = conn.Close(websocket.StatusNormalClosure, "closed")
	}()

	// Writer goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-cctx.Done():
				return
			case msg := <-client.sendCh:
				wctx, wcancel := context.WithTimeout(cctx, 5*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Reader loop (handles subscribe, unsubscribe, ping)
	for {
		_, data, err := conn.Read(cctx)
		if err != nil {
			break
		}
		var msg wsClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "ping":
			if raw, err := json.Marshal(wsServerMsg{Type: "pong"}); err == nil {
				select {
				case client.sendCh <- raw:
				default:
				}
			}
		case "subscribe":
			h.subscribeClient(client, msg.MappingIDs, msg.PollIntervalMs)
		case "unsubscribe":
			h.unsubscribeClient(client, msg.MappingIDs)
		}
	}

	cancel()
	wg.Wait()
	slog.Debug("webview_ws_disconnected", "site_id", ticket.SiteID, "user_id", ticket.UserID)
}

func (h *TelemetryHub) subscribeClient(c *wsClient, ids []uuid.UUID, pollIntervalMs int) {
	if len(ids) == 0 {
		return
	}
	if pollIntervalMs < 1000 {
		pollIntervalMs = 1000
	} else if pollIntervalMs > 60000 {
		pollIntervalMs = 60000
	}

	var snapshot []ValueSample
	var newlyActive []uuid.UUID
	var missingFromLocal bool

	c.mu.Lock()
	h.mu.Lock()
	c.pollIntervalMs = pollIntervalMs
	for _, id := range ids {
		c.subs[id] = struct{}{}
		m := h.mappingSubs[id]
		if m == nil {
			m = make(map[uuid.UUID]*wsClient)
			h.mappingSubs[id] = m
		}
		if len(m) == 0 {
			newlyActive = append(newlyActive, id)
		}
		m[c.id] = c
		if s, ok := h.latest[id]; ok {
			snapshot = append(snapshot, s)
		} else {
			missingFromLocal = true
		}
	}
	h.mu.Unlock()
	c.mu.Unlock()

	// Warm-hydrate any missing latest samples from Valkey (e.g. immediately after service restart).
	if missingFromLocal && h.vk != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		vkLatest, err := valkey.HGetAllJSON[ValueSample](ctx, h.vk, valkey.KeyWebviewLatest(c.siteID))
		cancel()
		if err == nil && len(vkLatest) > 0 {
			h.mu.Lock()
			for _, id := range ids {
				if _, ok := h.latest[id]; ok {
					continue
				}
				if s, ok := vkLatest[id.String()]; ok {
					h.latest[id] = s
					snapshot = append(snapshot, s)
				}
			}
			h.mu.Unlock()
		}
	}

	if len(snapshot) > 0 {
		if raw, err := json.Marshal(wsServerMsg{Type: "snapshot", Samples: snapshot}); err == nil {
			select {
			case c.sendCh <- raw:
			default:
			}
		}
	}

	if len(newlyActive) > 0 && h.onSubChange != nil {
		h.onSubChange(c.siteID, newlyActive)
	}
}

func (h *TelemetryHub) unsubscribeClient(c *wsClient, ids []uuid.UUID) {
	c.mu.Lock()
	h.mu.Lock()
	if len(ids) == 0 {
		for id := range c.subs {
			if m := h.mappingSubs[id]; m != nil {
				delete(m, c.id)
				if len(m) == 0 {
					delete(h.mappingSubs, id)
				}
			}
		}
		clear(c.subs)
	} else {
		for _, id := range ids {
			delete(c.subs, id)
			if m := h.mappingSubs[id]; m != nil {
				delete(m, c.id)
				if len(m) == 0 {
					delete(h.mappingSubs, id)
				}
			}
		}
	}
	h.mu.Unlock()
	c.mu.Unlock()
}

func (h *TelemetryHub) removeClient(c *wsClient) {
	h.unsubscribeClient(c, nil)
	h.mu.Lock()
	delete(h.clients, c.id)
	h.mu.Unlock()
}
