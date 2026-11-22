package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const TokenIssuer = "wiredeck2-core-iam"

// Claims are the RS256 JWT access token claims issued by core-iam.
type Claims struct {
	Email           string `json:"email"`
	IsPlatformAdmin bool   `json:"adm"`
	jwt.RegisteredClaims
}

// JWK represents an RSA public key in RFC 7517 JWKS format.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is the response served at /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// Signer issues RS256 JWT access tokens and publishes the matching JWKS.
type Signer struct {
	keyID      string
	privateKey *rsa.PrivateKey
	accessTTL  time.Duration
}

// NewSignerFromBase64PEM decodes a base64-encoded PKCS#8 or PKCS#1 RSA private key PEM.
func NewSignerFromBase64PEM(b64PEM, keyID string, accessTTL time.Duration) (*Signer, error) {
	if keyID == "" {
		keyID = "v1"
	}
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	pemBytes, err := base64.StdEncoding.DecodeString(b64PEM)
	if err != nil {
		return nil, fmt.Errorf("decode JWT_PRIVATE_KEY_B64: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM block in JWT_PRIVATE_KEY_B64")
	}

	var rsaKey *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		var ok bool
		rsaKey, ok = k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("JWT private key is not RSA")
		}
	} else if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		rsaKey = k
	} else {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}

	return &Signer{
		keyID:      keyID,
		privateKey: rsaKey,
		accessTTL:  accessTTL,
	}, nil
}

// NewSignerFromKey creates a Signer directly from an *rsa.PrivateKey (useful in tests).
func NewSignerFromKey(key *rsa.PrivateKey, keyID string, accessTTL time.Duration) *Signer {
	if keyID == "" {
		keyID = "v1"
	}
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	return &Signer{keyID: keyID, privateKey: key, accessTTL: accessTTL}
}

// IssueAccessToken signs a short-lived RS256 JWT for the given user.
func (s *Signer) IssueAccessToken(userID uuid.UUID, email string, isPlatformAdmin bool) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(s.accessTTL)
	claims := Claims{
		Email:           email,
		IsPlatformAdmin: isPlatformAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    TokenIssuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.keyID
	signed, err := tok.SignedString(s.privateKey)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// JWKS returns the public JSON Web Key Set for token verification.
func (s *Signer) JWKS() JWKS {
	pub := &s.privateKey.PublicKey
	return JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Use: "sig",
				Alg: "RS256",
				Kid: s.keyID,
				N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
}

// PublicKey returns the RSA public key for local verification.
func (s *Signer) PublicKey() *rsa.PublicKey {
	return &s.privateKey.PublicKey
}

// Verifier validates RS256 JWTs against either a local key map or a cached remote JWKS URL.
type Verifier struct {
	jwksURL    string
	httpClient *http.Client
	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	lastFetch  time.Time
}

// NewLocalVerifier verifies tokens directly against a known key ID and RSA public key.
func NewLocalVerifier(kid string, pub *rsa.PublicKey) *Verifier {
	return &Verifier{
		keys: map[string]*rsa.PublicKey{kid: pub},
	}
}

// NewRemoteJWKSVerifier fetches and caches public keys from core-iam's JWKS endpoint.
func NewRemoteJWKSVerifier(jwksURL string) *Verifier {
	return &Verifier{
		jwksURL:    jwksURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		keys:       make(map[string]*rsa.PublicKey),
	}
}

// Verify parses and validates an RS256 JWT access token.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(tok *jwt.Token) (any, error) {
		if tok.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing alg: %v", tok.Header["alg"])
		}
		kid, _ := tok.Header["kid"].(string)
		return v.getKey(ctx, kid)
	}, jwt.WithIssuer(TokenIssuer))
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid access token: %w", err)
	}
	return claims, nil
}

func (v *Verifier) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if k, ok := v.keys[kid]; ok {
		v.mu.RUnlock()
		return k, nil
	}
	if kid == "" && len(v.keys) == 1 {
		for _, k := range v.keys {
			v.mu.RUnlock()
			return k, nil
		}
	}
	canFetch := v.jwksURL != "" && time.Since(v.lastFetch) > 10*time.Second
	v.mu.RUnlock()

	if !canFetch {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown signing key %q after refresh", kid)
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks returned %d", resp.StatusCode)
	}
	var set JWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return err
	}
	parsed := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		eVal := int(new(big.Int).SetBytes(eb).Int64())
		parsed[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nb),
			E: eVal,
		}
	}
	v.mu.Lock()
	v.keys = parsed
	v.lastFetch = time.Now()
	v.mu.Unlock()
	return nil
}
