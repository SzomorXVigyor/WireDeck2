package internalclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SiteAuthzResponse is returned by core-iam GET /internal/sites/{siteId}/authz?user_id=.
type SiteAuthzResponse struct {
	Allowed         bool     `json:"allowed"`
	Role            string   `json:"role"` // SITE_OPERATOR | SITE_MEMBER (empty if platform admin w/o explicit row -> SITE_OPERATOR)
	IsPlatformAdmin bool     `json:"isPlatformAdmin"`
	IsActive        bool     `json:"isActive"`
	Status          string   `json:"status"`
	VncEnabled      bool     `json:"vncEnabled"`
	WebviewEnabled  bool     `json:"webviewEnabled"`
	LanSubnets      []string `json:"lanSubnets"`
}

// SiteRuntimeState is returned by core-iam GET /internal/sites/runtime-state.
type SiteRuntimeState struct {
	SiteID         uuid.UUID `json:"siteId"`
	IsActive       bool      `json:"isActive"`
	Status         string    `json:"status"`
	VncEnabled     bool      `json:"vncEnabled"`
	WebviewEnabled bool      `json:"webviewEnabled"`
	LanSubnets     []string  `json:"lanSubnets"`
}

// DesiredRunnerSpec is returned by core-iam GET /internal/runners/desired.
type DesiredRunnerSpec struct {
	SiteID          uuid.UUID `json:"siteId"`
	DesiredState    string    `json:"desiredState"` // RUNNING | STOPPED
	ConfFingerprint string    `json:"confFingerprint"`
}

// RunnerConfResponse is returned by core-iam GET /internal/sites/{siteId}/runner-conf.
type RunnerConfResponse struct {
	SiteID          uuid.UUID `json:"siteId"`
	Conf            string    `json:"conf"`
	ConfFingerprint string    `json:"confFingerprint"`
}

// EnsureRunnerRequest is sent to runner-orchestrator PUT /runners/{siteId}.
type EnsureRunnerRequest struct {
	Conf            string `json:"conf"`
	ConfFingerprint string `json:"confFingerprint"`
	DesiredState    string `json:"desiredState"` // RUNNING | STOPPED
}

// RunnerRecord represents a runner's observed status returned by runner-orchestrator.
type RunnerRecord struct {
	ID                  uuid.UUID  `json:"id"`
	SiteID              uuid.UUID  `json:"siteId"`
	ContainerName       string     `json:"containerName"`
	ContainerID         string     `json:"containerId,omitempty"`
	Image               string     `json:"image"`
	ConfFingerprint     string     `json:"confFingerprint,omitempty"`
	DesiredState        string     `json:"desiredState"`
	ObservedState       string     `json:"observedState,omitempty"`
	Status              string     `json:"status"`
	HandshakeState      string     `json:"handshakeState"`
	HandshakeAgeSeconds *int64     `json:"handshakeAgeSeconds,omitempty"`
	LastHandshakeAt     *time.Time `json:"lastHandshakeAt,omitempty"`
	LastCheckedAt       *time.Time `json:"lastCheckedAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	LastReconciledAt    *time.Time `json:"lastReconciledAt,omitempty"`
}

// RunnerAuthResponse is returned by runner-orchestrator GET /runners/{siteId}/token
// so webview-engine and webvnc-proxy can authenticate to site-runner-<site_id>:9000.
type RunnerAuthResponse struct {
	SiteID    uuid.UUID `json:"siteId"`
	Address   string    `json:"address"`
	AuthToken string    `json:"authToken"`
}

// HTTPClient is a minimal helper for authenticated internal :8081 service calls.
type HTTPClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewHTTPClient(baseURL, token string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Client:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) DoJSON(ctx context.Context, method, path string, in any, out any) (int, error) {
	if c.BaseURL == "" {
		return 0, fmt.Errorf("internal client base URL is empty")
	}
	var bodyReader io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, fmt.Errorf("internal %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response from %s %s: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}
