package httpapi

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"wiredeck/pkg/auth"
	"wiredeck/services/core-iam/internal/service"
)

func TestOpenAPISchemaAndAdminSeparation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer := auth.NewSignerFromKey(key, "test-k1", 5*time.Minute)
	verifier := auth.NewLocalVerifier("test-k1", signer.PublicKey())
	svc := service.NewIAMService(nil, service.Config{}, signer, nil, nil, nil, nil, nil)

	handler := NewPublicRouter(svc, verifier, PublicRouterOptions{EnableDocs: true, CookieSecure: false})

	// 1. OpenAPI 3.1 JSON is served and includes Auth, User, Site Admin, and Platform Admin routes
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/iam/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from openapi.json, got %d: %s", rec.Code, rec.Body.String())
	}
	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("openapi.json invalid JSON: %v", err)
	}
	paths, _ := spec["paths"].(map[string]any)
	for _, wantPath := range []string{
		"/api/v1/iam/auth/login",
		"/api/v1/iam/auth/verify-token",
		"/api/v1/iam/auth/accept-invite",
		"/api/v1/iam/auth/reset-password",
		"/api/v1/iam/me/change-password",
		"/api/v1/iam/sites/{siteId}/operator/members/invite",
		"/api/v1/iam/admin/sites/{siteId}/conf",
		"/api/v1/iam/admin/users",
		"/api/v1/iam/admin/sites",
	} {
		if _, ok := paths[wantPath]; !ok {
			t.Fatalf("expected OpenAPI spec to contain path %s", wantPath)
		}
	}
	// WireGuard conf rotation must NEVER be exposed under site-operator routes
	if _, leaked := paths["/api/v1/iam/sites/{siteId}/admin/conf"]; leaked {
		t.Fatal("WireGuard conf route must exist strictly under /api/v1/iam/admin/sites/{siteId}/conf")
	}

	// 2. Strict schema validation rejects invalid email & unknown fields with 422
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/iam/auth/login",
		bytes.NewBufferString(`{"email":"not-an-email","password":"123","unexpectedField":true}`))
	badReq.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, badReq)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on schema violation, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Non-admin JWT is blocked with 403 on Platform Admin router group
	userTok, _, err := signer.IssueAccessToken(uuid.New(), "operator@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/iam/admin/users", nil)
	adminReq.Header.Set("Authorization", "Bearer "+userTok)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, adminReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin calling /admin/users, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Unauthenticated request to /api/v1/iam/me is rejected with 401
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/iam/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /me, got %d", rec.Code)
	}

	// 5. Public JWKS endpoint returns valid RSA key
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/iam/.well-known/jwks.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from jwks.json, got %d", rec.Code)
	}

	// 6. Verify-token returns valid=false cleanly for empty/short tokens and 422 for bad token type
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/iam/auth/verify-token",
		bytes.NewBufferString(`{"token":"short","type":"INVITE"}`))
	verifyReq.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, verifyReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from verify-token, got %d: %s", rec.Code, rec.Body.String())
	}
	var verifyBody struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &verifyBody); err != nil {
		t.Fatalf("verify-token invalid JSON: %v", err)
	}
	if verifyBody.Valid || verifyBody.Reason != "INVALID" {
		t.Fatalf("expected valid=false reason=INVALID, got %+v", verifyBody)
	}

	badVerifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/iam/auth/verify-token",
		bytes.NewBufferString(`{"token":"abc","type":"UNKNOWN"}`))
	badVerifyReq.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, badVerifyReq)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid verify-token type, got %d: %s", rec.Code, rec.Body.String())
	}
}
