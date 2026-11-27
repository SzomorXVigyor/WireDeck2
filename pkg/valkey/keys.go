package valkey

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Default TTLs for ephemeral runtime data and shared caches stored in Valkey.
const (
	TTLRunnerRuntime   = 2 * time.Minute
	TTLAuthzCache      = 30 * time.Second
	TTLUserSitesCache  = 15 * time.Second
	TTLSitesRuntime    = 30 * time.Second
	TTLWSTicket        = 30 * time.Second
	TTLWebviewLatest   = 10 * time.Minute
	TTLWebviewRuntime  = 60 * time.Second
	TTLVncActive       = 60 * time.Second
	TTLAlarmPending    = 25 * time.Hour // Exceeds max hold_seconds (86400s = 24h)
)

// Key builders (centralized keyspace prevents cross-service collisions).
func KeyRunnerRuntime(siteID uuid.UUID) string {
	return "wd:runner:runtime:" + siteID.String()
}

func KeyAuthz(siteID, userID uuid.UUID) string {
	return fmt.Sprintf("wd:cache:authz:%s:%s", siteID.String(), userID.String())
}

func PrefixAuthzSite(siteID uuid.UUID) string {
	return "wd:cache:authz:" + siteID.String() + ":"
}

func PrefixAuthzAll() string {
	return "wd:cache:authz:"
}

func KeyUserSites(userID uuid.UUID) string {
	return "wd:cache:user_sites:" + userID.String()
}

func PrefixUserSitesAll() string {
	return "wd:cache:user_sites:"
}

func KeySitesRuntime() string {
	return "wd:cache:sites_runtime"
}

func KeyWebviewWSTicket(token string) string {
	return "wd:ws:webview:ticket:" + token
}

func KeyVncWSTicket(token string) string {
	return "wd:ws:vnc:ticket:" + token
}

func KeyWebviewLatest(siteID uuid.UUID) string {
	return "wd:webview:latest:" + siteID.String()
}

func KeyWebviewRuntime(siteID uuid.UUID) string {
	return "wd:webview:runtime:" + siteID.String()
}

func KeyVncActiveSessions(siteID uuid.UUID) string {
	return "wd:vnc:active:" + siteID.String()
}

func KeyAlarmPending() string {
	return "wd:alarm:pending"
}

// --- Shared Ephemeral Runtime Models ---

// RunnerPeerStats captures per-WireGuard-peer cumulative transfer counters from WireRelay GetStats.
type RunnerPeerStats struct {
	PublicKey string `json:"publicKey"`
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
}

// RunnerStats captures live WireRelay tunnel throughput, stream load, and error counters.
type RunnerStats struct {
	Interface            string            `json:"interface,omitempty"`
	RxBytes              uint64            `json:"rxBytes"`
	TxBytes              uint64            `json:"txBytes"`
	RxBytesPerSec        float64           `json:"rxBytesPerSec"`
	TxBytesPerSec        float64           `json:"txBytesPerSec"`
	UptimeSeconds        int64             `json:"uptimeSeconds"`
	ActiveStreams        int64             `json:"activeStreams"`
	MaxStreams           int64             `json:"maxStreams"`
	DialErrorsTotal      uint64            `json:"dialErrorsTotal"`
	StreamsRejectedTotal uint64            `json:"streamsRejectedTotal"`
	Peers                []RunnerPeerStats `json:"peers,omitempty"`
	CollectedAt          time.Time         `json:"collectedAt"`
}

// RunnerRuntime stores all ephemeral runtime status, handshake health, and live stats for a site runner.
// Persisted exclusively in Valkey (never written periodically to PostgreSQL).
type RunnerRuntime struct {
	SiteID              uuid.UUID    `json:"siteId"`
	ContainerID         string       `json:"containerId,omitempty"`
	ObservedState       string       `json:"observedState,omitempty"`
	Configured          bool         `json:"configured"`
	Status              string       `json:"status"` // ONLINE | DEGRADED | OFFLINE | UNKNOWN
	HandshakeState      string       `json:"handshakeState"`
	HandshakeAgeSeconds *int64       `json:"handshakeAgeSeconds,omitempty"`
	LastHandshakeAt     *time.Time   `json:"lastHandshakeAt,omitempty"`
	LastCheckedAt       *time.Time   `json:"lastCheckedAt,omitempty"`
	LastReconciledAt    *time.Time   `json:"lastReconciledAt,omitempty"`
	LastError           string       `json:"lastError,omitempty"`
	Stats               *RunnerStats `json:"stats,omitempty"`
}

// VncActiveSession represents a live in-flight VNC proxy session stored in Valkey.
type VncActiveSession struct {
	SessionID      uuid.UUID `json:"sessionId"`
	SiteID         uuid.UUID `json:"siteId"`
	TargetID       uuid.UUID `json:"targetId"`
	TargetName     string    `json:"targetName"`
	UserID         uuid.UUID `json:"userId"`
	StartedAt      time.Time `json:"startedAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	BytesIn        int64     `json:"bytesIn"`
	BytesOut       int64     `json:"bytesOut"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// DeviceRuntimeState represents live poller backoff / health status for one WebView device.
type DeviceRuntimeState struct {
	DeviceID     uuid.UUID  `json:"deviceId"`
	Name         string     `json:"name"`
	Healthy      bool       `json:"healthy"`
	Failures     int        `json:"failures"`
	NextRetryAt  *time.Time `json:"nextRetryAt,omitempty"`
	InFlight     bool       `json:"inFlight"`
	LastPolledAt *time.Time `json:"lastPolledAt,omitempty"`
}

// WebviewActiveSession represents a connected WebView browser WebSocket client stored in Valkey.
type WebviewActiveSession struct {
	ClientID        uuid.UUID `json:"clientId"`
	UserID          uuid.UUID `json:"userId"`
	UserEmail       string    `json:"userEmail,omitempty"`
	ConnectedAt     time.Time `json:"connectedAt"`
	SubscribedCount int       `json:"subscribedCount"`
	PollIntervalMs  int       `json:"pollIntervalMs"`
}

// CachedSample mirrors a cached latest WebView telemetry sample stored in wd:webview:latest:<siteId>.
type CachedSample struct {
	SiteID    uuid.UUID `json:"siteId"`
	MappingID uuid.UUID `json:"mappingId"`
	Value     *float64  `json:"value"`
	Quality   string    `json:"quality"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// WebviewRuntime stores ephemeral WebView poller and WebSocket client telemetry for a site.
type WebviewRuntime struct {
	SiteID             uuid.UUID              `json:"siteId"`
	ActiveWSClients    int                    `json:"activeWsClients"`
	SubscribedMappings int                    `json:"subscribedMappings"`
	Sessions           []WebviewActiveSession `json:"sessions,omitempty"`
	Devices            []DeviceRuntimeState   `json:"devices,omitempty"`
	UpdatedAt          time.Time              `json:"updatedAt"`
}

// AlarmPendingRecord stores transient alarm hold-timer state in Valkey while awaiting hold_seconds.
type AlarmPendingRecord struct {
	RuleID    uuid.UUID `json:"ruleId"`
	SiteID    uuid.UUID `json:"siteId"`
	SinceAt   time.Time `json:"sinceAt"`
	LastValue float64   `json:"lastValue"`
}
