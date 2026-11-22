package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/engine"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/sitecache"
)

func TestWebViewOpenAPI_RoleSeparationAndPreUpgradeWSAuth(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer := auth.NewSignerFromKey(key, "k1", 5*time.Minute)
	verifier := auth.NewLocalVerifier("k1", signer.PublicKey())

	iamMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(internalclient.SiteAuthzResponse{
			Allowed:        true,
			Role:           "SITE_MEMBER",
			IsActive:       true,
			WebviewEnabled: true,
			LanSubnets:     []string{"172.21.0.0/24"},
		})
	}))
	defer iamMock.Close()

	cache := sitecache.New(internalclient.NewHTTPClient(iamMock.URL, "tok", 5*time.Second), nil, nil)
	hub := engine.NewTelemetryHub(30*time.Second, nil)
	svc := engine.NewService(nil, nil, cache, hub, nil, false)
	handler := NewPublicRouter(svc, hub, verifier, cache, true)

	// 1. OpenAPI 3.1 spec served
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/webview/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 openapi.json, got %d", rec.Code)
	}

	// 2. SITE_MEMBER must get 403 Forbidden on Site Operator route
	tok, _, _ := signer.IssueAccessToken(uuid.New(), "member@example.com", false)
	siteID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/webview/sites/"+siteID+"/operator/devices", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for SITE_MEMBER on operator route, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Unauthenticated / invalid WebSocket ticket must be rejected with HTTP 401 before upgrade
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/webview/stream", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing WS ticket, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/webview/stream?ticket=bogus", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid WS ticket, got %d", rec.Code)
	}
}
