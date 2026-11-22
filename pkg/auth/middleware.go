package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type ctxKey string

const claimsCtxKey ctxKey = "wiredeck2_auth_claims"

// ClaimsFromContext retrieves verified JWT claims from the request context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsCtxKey).(*Claims)
	return c, ok && c != nil
}

// UserIDFromContext parses the subject UUID from verified claims.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	c, ok := ClaimsFromContext(ctx)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// WithClaims attaches claims to a context (also handy for unit tests).
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsCtxKey, c)
}

func extractBearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// RequireAuth validates the RS256 Bearer JWT and stores Claims in request context.
func RequireAuth(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r)
			if raw == "" {
				writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "missing bearer access token")
				return
			}
			claims, err := v.Verify(r.Context(), raw)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid or expired access token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

// RequirePlatformAdmin enforces that the authenticated caller is a Platform Admin.
// Must be mounted after RequireAuth on /api/v1/*/admin/* router groups.
func RequirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
			return
		}
		if !claims.IsPlatformAdmin {
			writeJSONError(w, http.StatusForbidden, "Forbidden", "platform administrator privileges required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireInternalToken enforces the shared Bearer secret on :8081 internal endpoints.
func RequireInternalToken(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				writeJSONError(w, http.StatusInternalServerError, "Misconfigured", "internal API token not configured")
				return
			}
			got := extractBearer(r)
			if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid internal API token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"title":"` + title + `","status":` + http.StatusText(status)[0:0] + strconvItoa(status) + `,"detail":"` + detail + `"}`))
}

func strconvItoa(n int) string {
	switch n {
	case 401:
		return "401"
	case 403:
		return "403"
	case 500:
		return "500"
	default:
		return "400"
	}
}
