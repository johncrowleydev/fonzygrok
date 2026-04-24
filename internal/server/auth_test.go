package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/store"
)

// --- T-060: Auth Middleware ---

func TestAuthMiddlewareBearerToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("GET", "/api/v1/me", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddlewareSessionCookie(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddlewareNoToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareExpiredToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	// Create a JWT manager with 1ms expiry.
	expiredMgr, _ := auth.NewJWTManager("", 1*time.Millisecond)
	hash, _ := auth.HashPassword("testpassword1")
	user, _ := st.CreateUser("expireduser", "expired@test.com", hash, "admin")
	token, _ := expiredMgr.CreateToken(auth.Claims{
		UserID: user.ID, Username: user.Username, Role: user.Role,
	})
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestRequireRoleForbidden(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	// Create a regular user (not admin).
	hash, _ := auth.HashPassword("userpassword1")
	user, _ := st.CreateUser("regularuser", "user@test.com", hash, "user")
	token, _ := admin.jwt.CreateToken(auth.Claims{
		UserID: user.ID, Username: user.Username, Role: user.Role,
	})

	// Try to access admin-only endpoint.
	req := authReq(httptest.NewRequest("GET", "/api/v1/users", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// --- T-061: Registration ---

func TestRegisterSuccess(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	// Create admin and invite code.
	hash, _ := auth.HashPassword("adminpassword1")
	adminUser, _ := st.CreateUser("admin", "admin@test.com", hash, "admin")
	_, code, _ := st.CreateInviteCode(adminUser.ID)

	body := `{"username":"newuser","email":"new@test.com","password":"securepass1","invite_code":"` + code + `"}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Email    string `json:"email"`
			Role     string `json:"role"`
		} `json:"user"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.User.Username != "newuser" {
		t.Errorf("username: got %q, want %q", resp.User.Username, "newuser")
	}
	if resp.User.Role != "user" {
		t.Errorf("role: got %q, want %q", resp.User.Role, "user")
	}
	if resp.Token == "" {
		t.Error("expected JWT token")
	}

	// Set-Cookie should be present.
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected session cookie")
	}
}

func TestRegisterInvalidInviteCode(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	body := `{"username":"newuser","email":"new@test.com","password":"securepass1","invite_code":"BADCODE1"}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	hash, _ := auth.HashPassword("adminpassword1")
	adminUser, _ := st.CreateUser("admin", "admin@test.com", hash, "admin")
	_, code1, _ := st.CreateInviteCode(adminUser.ID)
	_, code2, _ := st.CreateInviteCode(adminUser.ID)

	// First registration.
	body := `{"username":"dupeuser","email":"first@test.com","password":"securepass1","invite_code":"` + code1 + `"}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first register: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	// Second with same username.
	body = `{"username":"dupeuser","email":"second@test.com","password":"securepass1","invite_code":"` + code2 + `"}`
	req = httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w = httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate register: expected 409, got %d", w.Code)
	}
}

func TestRegisterBadUsername(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	body := `{"username":"ab","email":"a@b.com","password":"securepass1","invite_code":"ABCD1234"}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short username, got %d", w.Code)
	}
}

func TestRegisterWeakPassword(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	body := `{"username":"validuser","email":"a@b.com","password":"short","invite_code":"ABCD1234"}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d", w.Code)
	}
}

func TestRegisterMissingFields(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	body := `{"username":"onlyuser"}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d", w.Code)
	}
}

// --- T-062: Login ---

func TestLoginWithUsername(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	hash, _ := auth.HashPassword("correctpassword")
	st.CreateUser("loginuser", "login@test.com", hash, "user")

	body := `{"username":"loginuser","password":"correctpassword"}`
	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.User.Username != "loginuser" {
		t.Errorf("username: got %q", resp.User.Username)
	}
	if resp.Token == "" {
		t.Error("expected JWT")
	}
}

func TestLoginWithEmail(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	hash, _ := auth.HashPassword("correctpassword")
	st.CreateUser("emaillogin", "emaillogin@test.com", hash, "user")

	body := `{"username":"emaillogin@test.com","password":"correctpassword"}`
	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	hash, _ := auth.HashPassword("correctpassword")
	st.CreateUser("wrongpw", "wp@test.com", hash, "user")

	body := `{"username":"wrongpw","password":"wrongpassword1"}`
	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// Error should be generic — no username/password hint.
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "Invalid credentials" {
		t.Errorf("expected generic message, got %q", resp["message"])
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	body := `{"username":"ghost","password":"anypassword11"}`
	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// --- T-063: Logout ---

func TestLogout(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("POST", "/api/v1/logout", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Session cookie should be cleared.
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "session" && c.MaxAge > 0 {
			t.Error("session cookie should have MaxAge <= 0")
		}
	}
}

// --- T-064: User Info ---

func TestMeEndpoint(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("GET", "/api/v1/me", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Username != "testadmin" {
		t.Errorf("username: got %q, want %q", resp.Username, "testadmin")
	}
	if resp.Email != "admin@test.com" {
		t.Errorf("email: got %q", resp.Email)
	}
}

// --- T-065: Authenticated Token CRUD ---

func TestAuthenticatedTokenCreate(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	body := `{"name":"my-cli-token"}`
	req := authReq(httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(body)), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !strings.HasPrefix(resp.Token, "fgk_") {
		t.Errorf("token should start with fgk_, got: %s", resp.Token)
	}
}

func TestUserCanOnlySeeOwnTokens(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	// Create two users.
	hash, _ := auth.HashPassword("password12345")
	user1, _ := st.CreateUser("user1", "u1@test.com", hash, "user")
	user2, _ := st.CreateUser("user2", "u2@test.com", hash, "user")

	// Create tokens for each user.
	st.CreateToken("user1-token", user1.ID)
	st.CreateToken("user2-token", user2.ID)

	// User1 should only see their token.
	jwt1, _ := admin.jwt.CreateToken(auth.Claims{
		UserID: user1.ID, Username: user1.Username, Role: user1.Role,
	})

	req := authReq(httptest.NewRequest("GET", "/api/v1/tokens", nil), jwt1)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	var resp struct {
		Tokens []struct {
			Name string `json:"name"`
		} `json:"tokens"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tokens) != 1 {
		t.Fatalf("user1 should see 1 token, got %d", len(resp.Tokens))
	}
	if resp.Tokens[0].Name != "user1-token" {
		t.Errorf("expected user1-token, got %q", resp.Tokens[0].Name)
	}
}

func TestUserCannotDeleteOthersToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	hash, _ := auth.HashPassword("password12345")
	user1, _ := st.CreateUser("user1", "u1@test.com", hash, "user")
	user2, _ := st.CreateUser("user2", "u2@test.com", hash, "user")

	tok2, _, _ := st.CreateToken("user2-token", user2.ID)

	jwt1, _ := admin.jwt.CreateToken(auth.Claims{
		UserID: user1.ID, Username: user1.Username, Role: user1.Role,
	})

	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tokens/"+tok2.ID, nil), jwt1)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (user1 cannot delete user2's token)", w.Code)
	}
}

func TestAdminCanSeeAllTokens(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	// Create tokens owned by different users.
	hash, _ := auth.HashPassword("password12345")
	user1, _ := st.CreateUser("user1", "u1@test.com", hash, "user")
	st.CreateToken("user1-token", user1.ID)
	st.CreateToken("admin-token", "") // legacy unowned

	req := authReq(httptest.NewRequest("GET", "/api/v1/tokens", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	var resp struct {
		Tokens []interface{} `json:"tokens"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tokens) < 2 {
		t.Errorf("admin should see all tokens, got %d", len(resp.Tokens))
	}
}

// --- T-066: Invite Codes (Admin Only) ---

func TestCreateInviteCode(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("POST", "/api/v1/invite-codes", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Code) != 8 {
		t.Errorf("code length: got %d, want 8", len(resp.Code))
	}
}

func TestListInviteCodes(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	// Create a code via the API.
	req := authReq(httptest.NewRequest("POST", "/api/v1/invite-codes", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	// List codes.
	req = authReq(httptest.NewRequest("GET", "/api/v1/invite-codes", nil), token)
	w = httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		InviteCodes []interface{} `json:"invite_codes"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.InviteCodes) != 1 {
		t.Errorf("expected 1 invite code, got %d", len(resp.InviteCodes))
	}
}

func TestInviteCodesRequireAdmin(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	hash, _ := auth.HashPassword("password12345")
	user, _ := st.CreateUser("regular", "regular@test.com", hash, "user")
	token, _ := admin.jwt.CreateToken(auth.Claims{
		UserID: user.ID, Username: user.Username, Role: user.Role,
	})

	req := authReq(httptest.NewRequest("GET", "/api/v1/invite-codes", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", w.Code)
	}
}

// --- T-067: User Management ---

func TestListUsersAdmin(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("GET", "/api/v1/users", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Users []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"users"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(resp.Users))
	}
	if resp.Users[0].Role != "admin" {
		t.Errorf("role: got %q, want %q", resp.Users[0].Role, "admin")
	}
}

// --- T-068: CORS ---

func TestCORSMiddlewareRejectsUntrustedOriginWithoutDatabase(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), nil)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("untrusted origin must not be reflected, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("untrusted origin must not receive credentials, got %q", got)
	}
}

func TestCORSMiddlewareAllowsConfiguredOriginWithoutDatabase(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"https://dashboard.example.com"})

	req := httptest.NewRequest("OPTIONS", "/api/v1/health", nil)
	req.Header.Set("Origin", "https://dashboard.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://dashboard.example.com" {
		t.Fatalf("configured origin should be allowed, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("configured origin should allow credentials, got %q", got)
	}
}

func TestCORSHeaders(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("OPTIONS", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Error("missing CORS Allow-Origin header")
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("missing CORS Allow-Credentials header")
	}
}

func TestCORSOnRegularRequest(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("untrusted cross-origin request must not receive credentialed CORS headers, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("untrusted cross-origin request must not allow credentials, got %q", got)
	}
}

func TestCORSAllowsSameOriginOnlyByDefault(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("OPTIONS", "http://admin.example.com/api/v1/health", nil)
	req.Host = "admin.example.com"
	req.Header.Set("Origin", "http://admin.example.com")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://admin.example.com" {
		t.Fatalf("same-origin request should receive CORS allow-origin, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("same-origin request should allow credentials, got %q", got)
	}
}

// --- Edge cases ---

func TestHealthStaysPublic(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	// No auth — should still work.
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health should be public, got %d", w.Code)
	}
}

func TestRegisterEndpointIsPublic(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	// Register doesn't need auth — but still validates input.
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	// Should get 400 (validation error), NOT 401 (unauthorized).
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTokensEndpointRequiresAuth(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/tokens", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("tokens endpoint should require auth, got %d", w.Code)
	}
}

func TestTunnelsEndpointRequiresAuth(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/tunnels", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("tunnels endpoint should require auth, got %d", w.Code)
	}
}

func TestMetricsEndpointRequiresAuth(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("metrics endpoint should require auth, got %d", w.Code)
	}
}

func TestGlobalTunnelEndpointsRequireAdminRole(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	userToken := addTestUserAuth(t, admin, st)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/tunnels", ""},
		{"GET", "/api/v1/metrics", ""},
		{"DELETE", "/api/v1/tunnels/tun_missing", ""},
		{"PUT", "/api/v1/tunnels/tun_missing/ratelimit", `{"requests_per_second":1,"burst":1}`},
		{"PUT", "/api/v1/tunnels/tun_missing/acl", `{"allow_cidrs":["127.0.0.1/32"]}`},
		{"DELETE", "/api/v1/tunnels/tun_missing/acl", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := authReq(httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)), userToken)
			w := httptest.NewRecorder()
			admin.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for non-admin tunnel/metrics endpoint, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// Helper to create a regular (non-admin) user token.
func addTestUserAuth(t *testing.T, admin *AdminAPI, st *store.Store) string {
	t.Helper()
	hash, _ := auth.HashPassword("userpassword1")
	user, _ := st.CreateUser("regularuser", "regularuser@test.com", hash, "user")
	token, _ := admin.jwt.CreateToken(auth.Claims{
		UserID: user.ID, Username: user.Username, Role: user.Role,
	})
	return token
}
