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

	"wiredeck/pkg/auth"
	"wiredeck/pkg/internalclient"
	"wiredeck/services/webvnc-proxy/internal/proxy"
	"wiredeck/services/webvnc-proxy/internal/sitecache"
)

func TestWebVNCOpenAPIAndRoleSeparation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer := auth.NewSignerFromKey(key, "k1", 5*time.Minute)
	verifier := auth.NewLocalVerifier("k1", signer.PublicKey())

	// Mock core-iam internal authz returning SITE_MEMBER (not SITE_OPERATOR)
	iamMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(internalclient.SiteAuthzResponse{
			Allowed:    true,
			Role:       "SITE_MEMBER",
			VncEnabled: true,
			LanSubnets: []string{"192.168.10.0/24"},
		})
	}))
	defer iamMock.Close()

	cache := sitecache.New(internalclient.NewHTTPClient(iamMock.URL, "tok", 5*time.Second), nil, nil)
	svc := proxy.NewService(nil, nil, cache, proxy.Config{})
	handler := NewPublicRouter(svc, verifier, cache, true)

	// 1. OpenAPI 3.1 spec served
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/vnc/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 openapi.json, got %d", rec.Code)
	}

	// 2. SITE_MEMBER must get 403 Forbidden on Site Operator endpoint
	tok, _, _ := signer.IssueAccessToken(uuid.New(), "member@example.com", false)
	siteID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/vnc/sites/"+siteID+"/operator/targets", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for SITE_MEMBER on operator route, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Unauthenticated / invalid WebSocket ticket must be rejected with HTTP 401 before upgrade
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/vnc/stream", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing WS ticket, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/vnc/stream?ticket=bogus-ticket", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid WS ticket, got %d", rec.Code)
	}
}
