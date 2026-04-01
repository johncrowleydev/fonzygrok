// Package server — middleware.go provides JWT authentication middleware
// for the fonzygrok admin API. Supports both Bearer token (API/CLI)
// and HttpOnly session cookie (web dashboard).
//
// SECURITY: Generic error messages on auth failure — never reveal
// whether a token is expired vs invalid vs missing.
//
// REF: SPR-018 T-060
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/fonzygrok/fonzygrok/internal/auth"
)

// contextKey is an unexported type for context keys to prevent collisions.
type contextKey string

const (
	// claimsKey is the context key for JWT claims.
	claimsKey contextKey = "claims"

	// sessionCookieName is the name of the HttpOnly session cookie.
	sessionCookieName = "session"
)

// AuthMiddleware extracts and validates JWT from:
//  1. Authorization: Bearer <token> header (API clients / CLI)
//  2. HttpOnly cookie named "session" (web dashboard)
//
// On success: adds Claims to request context.
// On failure: returns 401 Unauthorized JSON response.
//
// DECISION: Check header first, then cookie. Header takes priority
// because API clients explicitly set it; cookie is implicit from browser.
func (a *AdminAPI) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := extractToken(r)
		if tokenStr == "" {
			a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}

		claims, err := a.jwt.ValidateToken(tokenStr)
		if err != nil {
			a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole returns middleware that checks the JWT claims for a
// specific role. Returns 403 Forbidden if the user doesn't have the role.
//
// PRECONDITION: Must be used AFTER AuthMiddleware.
func (a *AdminAPI) RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		if claims.Role != role {
			a.writeAdminError(w, http.StatusForbidden, "forbidden", "Insufficient permissions")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OptionalAuth is like AuthMiddleware but doesn't reject unauthenticated
// requests — passes nil claims in context for unauthenticated users.
func (a *AdminAPI) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := extractToken(r)
		if tokenStr != "" {
			if claims, err := a.jwt.ValidateToken(tokenStr); err == nil {
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ClaimsFromContext returns the JWT claims from the context, or nil.
func ClaimsFromContext(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(claimsKey).(*auth.Claims)
	return claims
}

// UserIDFromContext returns the user ID from the JWT claims, or "".
func UserIDFromContext(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.UserID
}

// extractToken gets the JWT string from the Authorization header or cookie.
func extractToken(r *http.Request) string {
	// 1. Check Authorization header.
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// 2. Check session cookie.
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// setSessionCookie sets the HttpOnly session cookie with the JWT.
func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false, // Set true when behind TLS in production.
	})
}

// clearSessionCookie clears the session cookie.
func clearSessionCookie(w http.ResponseWriter) {
	setSessionCookie(w, "", -1)
}
