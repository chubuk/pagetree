package pagetree

import (
	"context"
	"net/http"
	"strings"
)

// contextKey for authenticated API key.
type contextKey int

const apiKeyContextKey contextKey = 0

// KeyFromContext returns the authenticated API key from request context.
func KeyFromContext(ctx context.Context) *APIKey {
	k, _ := ctx.Value(apiKeyContextKey).(*APIKey)
	return k
}

// KeyAuthMW returns middleware that authenticates Bearer ptk_* tokens.
// Fail-closed: no key, revoked, expired, or bad policy → 401/403.
func (e *Engine) KeyAuthMW(resource string, write bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}
			rawKey := strings.TrimPrefix(auth, "Bearer ")
			key, err := e.keystore.Authenticate(rawKey)
			if err != nil {
				http.Error(w, `{"error":"invalid or revoked key"}`, http.StatusUnauthorized)
				return
			}
			// Policy check
			if !CheckPolicy(key.Policy, resource, write) {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			// Touch for audit
			e.keystore.TouchKey(key.ID)
			// Inject key into context
			ctx := context.WithValue(r.Context(), apiKeyContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
