package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJWTSignAndVerify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer := NewSignerFromKey(key, "k1", 5*time.Minute)
	uid := uuid.New()

	tok, _, err := signer.IssueAccessToken(uid, "admin@example.com", true)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	// Local verify
	localV := NewLocalVerifier("k1", signer.PublicKey())
	claims, err := localV.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("local Verify: %v", err)
	}
	if claims.Subject != uid.String() || !claims.IsPlatformAdmin {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	// Remote JWKS verify
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.JWKS())
	}))
	defer ts.Close()

	remoteV := NewRemoteJWKSVerifier(ts.URL)
	claims2, err := remoteV.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("remote JWKS Verify: %v", err)
	}
	if claims2.Email != "admin@example.com" {
		t.Fatalf("unexpected remote email: %s", claims2.Email)
	}
}
