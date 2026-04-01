// Package server — dashboard.go provides the server-rendered web dashboard
// for user self-service: login, registration, token management, and admin.
//
// Uses Go html/template + HTMX. All assets embedded via embed.FS.
// No npm, no build step, no external JS framework.
//
// THREAD SAFETY: Safe for concurrent use — templates are parsed once at init.
//
// REF: SPR-019 T-069, T-071–T-076
package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/store"
)

// contextKey is a private type used for context value keys to avoid collisions.
type contextKey string

const (
	// claimsKey is the context key for JWT claims.
	claimsKey contextKey = "claims"

	// sessionCookieName is the cookie name for JWT sessions.
	sessionCookieName = "session"

	// flashCookieName is the cookie name for flash messages.
	flashCookieName = "flash"
)

// usernamePattern validates usernames: 3-32 chars, alphanumeric + underscore + hyphen.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$`)

// Dashboard provides the web dashboard UI.
//
// DEPENDENCIES: store.Store for data, auth.JWTManager for sessions,
// TunnelManager for live tunnel data.
type Dashboard struct {
	store   *store.Store
	jwt     *auth.JWTManager
	tunnels *TunnelManager
	logger  *slog.Logger
	pages   map[string]*template.Template // page name → layout+page template
	partial *template.Template
}

// templateFuncMap returns the shared template function map.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}
}

// NewDashboard creates a Dashboard and parses all embedded templates.
//
// FAILURE MODE: Panics if templates fail to parse — this is a programming error.
func NewDashboard(st *store.Store, jwt *auth.JWTManager, tunnels *TunnelManager, logger *slog.Logger) *Dashboard {
	d := &Dashboard{
		store:   st,
		jwt:     jwt,
		tunnels: tunnels,
		logger:  logger,
		pages:   make(map[string]*template.Template),
	}

	funcMap := templateFuncMap()
	layoutFile := "dashboard_assets/templates/layout.html"

	// Parse each page template paired with the layout so each gets
	// its own "content" block without collisions.
	pageFiles := []string{
		"dashboard_assets/templates/login.html",
		"dashboard_assets/templates/register.html",
		"dashboard_assets/templates/dashboard.html",
		"dashboard_assets/templates/admin_users.html",
		"dashboard_assets/templates/admin_invites.html",
	}
	for _, pf := range pageFiles {
		// Extract short name: "login.html", "register.html", etc.
		parts := strings.Split(pf, "/")
		name := parts[len(parts)-1]
		d.pages[name] = template.Must(
			template.New("").Funcs(funcMap).ParseFS(dashboardFS, layoutFile, pf),
		)
	}

	// Parse partial templates (HTMX fragments — no layout).
	d.partial = template.Must(
		template.New("").Funcs(funcMap).ParseFS(dashboardFS,
			"dashboard_assets/templates/partials/token_form.html",
			"dashboard_assets/templates/partials/token_reveal.html",
			"dashboard_assets/templates/partials/tunnel_list.html",
			"dashboard_assets/templates/partials/invite_reveal.html",
		),
	)

	return d
}


// RegisterRoutes registers all dashboard routes on the given mux.
// Called from AdminAPI to share the :9090 listener.
func (d *Dashboard) RegisterRoutes(mux *http.ServeMux) {
	// Static files (CSS, JS).
	staticFS, _ := fs.Sub(dashboardFS, "dashboard_assets/static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Public routes.
	mux.HandleFunc("/", d.handleRoot)
	mux.HandleFunc("/login", d.handleLogin)
	mux.HandleFunc("/register", d.handleRegister)
	mux.HandleFunc("/logout", d.handleLogout)

	// Authenticated routes.
	mux.HandleFunc("/dashboard", d.requireAuth(d.handleDashboard))
	mux.HandleFunc("/dashboard/tokens/new", d.requireAuth(d.handleTokenForm))
	mux.HandleFunc("/dashboard/tokens/", d.requireAuth(d.handleTokenAction))
	mux.HandleFunc("/dashboard/tokens", d.requireAuth(d.handleTokenCreate))
	mux.HandleFunc("/dashboard/tunnels", d.requireAuth(d.handleTunnelList))

	// Admin routes.
	mux.HandleFunc("/admin/users", d.requireAuth(d.requireAdmin(d.handleAdminUsers)))
	mux.HandleFunc("/admin/invite-codes", d.requireAuth(d.requireAdmin(d.handleAdminInvites)))
	mux.HandleFunc("/admin/invite-codes/new", d.requireAuth(d.requireAdmin(d.handleInviteCreate)))
}

// ── Page Data ────────────────────────────────────────────────────────────────

// pageData holds data passed to all templates.
type pageData struct {
	Title      string
	Nav        string
	Claims     *auth.Claims
	Flash      string
	FlashType  string
	Error      string

	// Form fields (preserved on error).
	FormUsername   string
	FormEmail     string
	FormInviteCode string

	// Dashboard data.
	Tokens      []store.Token
	Tunnels     []tunnelView
	Users       []store.User
	InviteCodes []store.InviteCode
}

// tunnelView is the dashboard representation of an active tunnel.
type tunnelView struct {
	Name        string
	PublicURL   string
	LocalPort   int
	ConnectedAt string
}

// ── Middleware ────────────────────────────────────────────────────────────────

// requireAuth wraps a handler that requires authentication.
// Redirects to /login if no valid session.
func (d *Dashboard) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := d.claimsFromRequest(r)
		if claims == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin wraps a handler that requires admin role.
// Returns 403 if user is not admin.
func (d *Dashboard) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFromCtx(r.Context())
		if claims == nil || claims.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// claimsFromRequest extracts JWT claims from the session cookie.
func (d *Dashboard) claimsFromRequest(r *http.Request) *auth.Claims {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	claims, err := d.jwt.ValidateToken(cookie.Value)
	if err != nil {
		return nil
	}
	return claims
}

// claimsFromCtx retrieves claims from request context.
func claimsFromCtx(ctx context.Context) *auth.Claims {
	v := ctx.Value(claimsKey)
	if v == nil {
		return nil
	}
	claims, ok := v.(*auth.Claims)
	if !ok {
		return nil
	}
	return claims
}

// ── Route Handlers ───────────────────────────────────────────────────────────

// handleRoot redirects to /dashboard if authenticated, /login otherwise.
func (d *Dashboard) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	claims := d.claimsFromRequest(r)
	if claims != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleLogin serves GET /login and POST /login.
func (d *Dashboard) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// If already logged in, redirect.
		if claims := d.claimsFromRequest(r); claims != nil {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		d.renderPage(w, "login.html", pageData{Title: "Login"})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	// Try username first, then email.
	user, err := d.store.GetUserByUsername(username)
	if err != nil {
		user, err = d.store.GetUserByEmail(username)
	}
	if err != nil || user == nil {
		d.renderPage(w, "login.html", pageData{
			Title:        "Login",
			Error:        "Invalid credentials",
			FormUsername: username,
		})
		return
	}

	if err := auth.VerifyPassword(password, user.PasswordHash); err != nil {
		d.renderPage(w, "login.html", pageData{
			Title:        "Login",
			Error:        "Invalid credentials",
			FormUsername: username,
		})
		return
	}

	if !user.IsActive {
		d.renderPage(w, "login.html", pageData{
			Title: "Login",
			Error: "Account is disabled",
		})
		return
	}

	// Update last login.
	_ = d.store.UpdateLastLogin(user.ID)

	// Issue JWT and set cookie.
	d.setSessionCookie(w, user)
	d.setFlash(w, "success", "Welcome back, "+user.Username+"!")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// handleRegister serves GET /register and POST /register.
func (d *Dashboard) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		d.renderPage(w, "register.html", pageData{Title: "Register"})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	inviteCode := strings.ToUpper(strings.TrimSpace(r.FormValue("invite_code")))

	formData := pageData{
		Title:          "Register",
		FormUsername:   username,
		FormEmail:      email,
		FormInviteCode: inviteCode,
	}

	// Validate username.
	if !usernamePattern.MatchString(username) {
		formData.Error = "Username must be 3–32 characters, alphanumeric, underscores, or hyphens"
		d.renderPage(w, "register.html", formData)
		return
	}

	// Validate email (basic check).
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		formData.Error = "Invalid email address"
		d.renderPage(w, "register.html", formData)
		return
	}

	// Validate password.
	if err := auth.ValidatePasswordStrength(password); err != nil {
		formData.Error = err.Error()
		d.renderPage(w, "register.html", formData)
		return
	}

	// Validate invite code.
	ic, err := d.store.ValidateInviteCode(inviteCode)
	if err != nil {
		formData.Error = "Invalid or already used invite code"
		d.renderPage(w, "register.html", formData)
		return
	}

	// Hash password.
	hash, err := auth.HashPassword(password)
	if err != nil {
		d.logger.Error("dashboard: hash password", "error", err)
		formData.Error = "Internal error"
		d.renderPage(w, "register.html", formData)
		return
	}

	// Create user.
	user, err := d.store.CreateUser(username, email, hash, "user")
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			formData.Error = "Username or email already taken"
		} else {
			d.logger.Error("dashboard: create user", "error", err)
			formData.Error = "Failed to create account"
		}
		d.renderPage(w, "register.html", formData)
		return
	}

	// Redeem invite code.
	if err := d.store.RedeemInviteCode(ic.ID, user.ID); err != nil {
		d.logger.Error("dashboard: redeem invite code", "error", err)
	}

	// Issue JWT and set cookie.
	d.setSessionCookie(w, user)
	d.setFlash(w, "success", "Account created! Welcome, "+user.Username+".")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// handleLogout clears the session and redirects to /login.
func (d *Dashboard) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleDashboard renders the main dashboard with tokens and tunnels.
func (d *Dashboard) handleDashboard(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())

	tokens, err := d.store.ListTokens()
	if err != nil {
		d.logger.Error("dashboard: list tokens", "error", err)
	}

	// Filter tokens for this user (unless admin).
	var userTokens []store.Token
	for _, t := range tokens {
		if claims.Role == "admin" || (t.UserID != nil && *t.UserID == claims.UserID) {
			userTokens = append(userTokens, t)
		}
	}

	flash, flashType := d.getFlash(r, w)
	d.renderPage(w, "dashboard.html", pageData{
		Title:     "Dashboard",
		Nav:       "dashboard",
		Claims:    claims,
		Flash:     flash,
		FlashType: flashType,
		Tokens:    userTokens,
	})
}

// handleAdminUsers renders the admin users page.
func (d *Dashboard) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())

	users, err := d.store.ListUsers()
	if err != nil {
		d.logger.Error("dashboard: list users", "error", err)
	}

	d.renderPage(w, "admin_users.html", pageData{
		Title:  "Users",
		Nav:    "admin-users",
		Claims: claims,
		Users:  users,
	})
}

// handleAdminInvites renders the admin invite codes page.
func (d *Dashboard) handleAdminInvites(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())

	codes, err := d.store.ListInviteCodes()
	if err != nil {
		d.logger.Error("dashboard: list invite codes", "error", err)
	}

	d.renderPage(w, "admin_invites.html", pageData{
		Title:       "Invite Codes",
		Nav:         "admin-invites",
		Claims:      claims,
		InviteCodes: codes,
	})
}

// ── HTMX Endpoints ───────────────────────────────────────────────────────────

// handleTokenForm returns the inline token creation form (GET).
func (d *Dashboard) handleTokenForm(w http.ResponseWriter, r *http.Request) {
	d.partial.ExecuteTemplate(w, "token_form.html", nil)
}

// handleTokenCreate creates a token via HTMX POST.
func (d *Dashboard) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims := claimsFromCtx(r.Context())
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	tok, rawToken, err := d.store.CreateToken(name, claims.UserID)
	if err != nil {
		d.logger.Error("dashboard: create token", "error", err)
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	d.logger.Info("dashboard: token created", "token_id", tok.ID, "user", claims.Username)

	data := struct {
		RawToken  string
		TokenName string
		TokenID   string
	}{
		RawToken:  rawToken,
		TokenName: name,
		TokenID:   tok.ID,
	}
	d.partial.ExecuteTemplate(w, "token_reveal.html", data)
}

// handleTokenAction handles DELETE /dashboard/tokens/:id (revoke).
func (d *Dashboard) handleTokenAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tokenID := strings.TrimPrefix(r.URL.Path, "/dashboard/tokens/")
	if tokenID == "" {
		http.Error(w, "Token ID required", http.StatusBadRequest)
		return
	}

	claims := claimsFromCtx(r.Context())
	if err := d.store.DeleteToken(tokenID); err != nil {
		d.logger.Error("dashboard: revoke token", "error", err, "token_id", tokenID)
		http.Error(w, "Failed to revoke token", http.StatusInternalServerError)
		return
	}

	d.logger.Info("dashboard: token revoked", "token_id", tokenID, "user", claims.Username)
	// Return empty response — HTMX removes the row via outerHTML swap.
	w.WriteHeader(http.StatusOK)
}

// handleTunnelList returns the tunnel table partial for HTMX polling.
func (d *Dashboard) handleTunnelList(w http.ResponseWriter, r *http.Request) {
	active := d.tunnels.ListActive()

	var tunnels []tunnelView
	for _, entry := range active {
		tunnels = append(tunnels, tunnelView{
			Name:        entry.Name,
			PublicURL:   entry.PublicURL,
			LocalPort:   entry.LocalPort,
			ConnectedAt: entry.CreatedAt.Format("Jan 02, 15:04"),
		})
	}

	data := struct {
		Tunnels []tunnelView
	}{Tunnels: tunnels}
	d.partial.ExecuteTemplate(w, "tunnel_list.html", data)
}

// handleInviteCreate generates a new invite code via HTMX POST.
func (d *Dashboard) handleInviteCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims := claimsFromCtx(r.Context())

	ic, code, err := d.store.CreateInviteCode(claims.UserID)
	if err != nil {
		d.logger.Error("dashboard: create invite code", "error", err)
		http.Error(w, "Failed to generate code", http.StatusInternalServerError)
		return
	}

	d.logger.Info("dashboard: invite code created", "code_id", ic.ID, "user", claims.Username)

	data := struct{ Code string }{Code: code}
	d.partial.ExecuteTemplate(w, "invite_reveal.html", data)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// renderPage renders a full page with the layout template.
func (d *Dashboard) renderPage(w http.ResponseWriter, name string, data pageData) {
	tmpl, ok := d.pages[name]
	if !ok {
		d.logger.Error("dashboard: template not found", "template", name)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		d.logger.Error("dashboard: render template",
			"template", name,
			"error", err,
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}


// setSessionCookie creates a JWT and sets it as an HttpOnly cookie.
func (d *Dashboard) setSessionCookie(w http.ResponseWriter, user *store.User) {
	token, err := d.jwt.CreateToken(auth.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
	if err != nil {
		d.logger.Error("dashboard: create JWT", "error", err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set true in production with TLS
		MaxAge:   int(24 * time.Hour / time.Second),
		SameSite: http.SameSiteLaxMode,
	})
}

// setFlash sets a flash message cookie (consumed on next page load).
func (d *Dashboard) setFlash(w http.ResponseWriter, flashType, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    fmt.Sprintf("%s:%s", flashType, message),
		Path:     "/",
		HttpOnly: true,
		MaxAge:   10,
		SameSite: http.SameSiteLaxMode,
	})
}

// getFlash reads and clears the flash message cookie.
func (d *Dashboard) getFlash(r *http.Request, w http.ResponseWriter) (string, string) {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil || cookie.Value == "" {
		return "", ""
	}

	// Clear the flash cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})

	parts := strings.SplitN(cookie.Value, ":", 2)
	if len(parts) != 2 {
		return cookie.Value, "success"
	}
	return parts[1], parts[0]
}
