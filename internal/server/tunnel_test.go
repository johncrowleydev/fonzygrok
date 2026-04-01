package server

import (
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/proto"
	"github.com/fonzygrok/fonzygrok/internal/store"
)

func newTestTunnelManager(t *testing.T) (*TunnelManager, *store.Store) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Migrate(); err != nil {
		st.Close()
		t.Fatalf("store.Migrate: %v", err)
	}
	return NewTunnelManager("tunnel.test.com", st, testLogger()), st
}

func TestRegister(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456", RemoteAddr: "127.0.0.1:12345"}
	req := &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	}

	assignment, err := tm.Register(session, req)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if assignment.TunnelID == "" {
		t.Error("expected non-empty tunnel ID")
	}
	if len(assignment.TunnelID) != tunnelIDLen {
		t.Errorf("tunnel ID length: got %d, want %d", len(assignment.TunnelID), tunnelIDLen)
	}

	// Verify ID format: lowercase alphanumeric.
	matched, _ := regexp.MatchString(`^[a-z0-9]+$`, assignment.TunnelID)
	if !matched {
		t.Errorf("tunnel ID not lowercase alphanumeric: %q", assignment.TunnelID)
	}

	// Subdomain should be auto-generated name (adjective-noun), not the tunnel ID.
	namePattern := regexp.MustCompile(`^[a-z]+-[a-z]+$`)
	if !namePattern.MatchString(assignment.AssignedSubdomain) {
		t.Errorf("subdomain should be adjective-noun format: got %q", assignment.AssignedSubdomain)
	}
	if assignment.Name == "" {
		t.Error("expected non-empty Name in assignment")
	}
	if assignment.Name != assignment.AssignedSubdomain {
		t.Errorf("Name should equal AssignedSubdomain: got Name=%q, Subdomain=%q", assignment.Name, assignment.AssignedSubdomain)
	}
	if assignment.PublicURL == "" {
		t.Error("expected non-empty public URL")
	}
	if assignment.Protocol != "http" {
		t.Errorf("protocol: got %q, want %q", assignment.Protocol, "http")
	}

	expectedURL := "http://" + assignment.Name + ".tunnel.test.com"
	if assignment.PublicURL != expectedURL {
		t.Errorf("public URL: got %q, want %q", assignment.PublicURL, expectedURL)
	}
}

func TestRegisterCustomName(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}
	req := &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
		Name:      "my-api",
	}

	assignment, err := tm.Register(session, req)
	if err != nil {
		t.Fatalf("Register with custom name: %v", err)
	}

	if assignment.Name != "my-api" {
		t.Errorf("name: got %q, want %q", assignment.Name, "my-api")
	}
	if assignment.AssignedSubdomain != "my-api" {
		t.Errorf("subdomain: got %q, want %q", assignment.AssignedSubdomain, "my-api")
	}
	if assignment.PublicURL != "http://my-api.tunnel.test.com" {
		t.Errorf("public URL: got %q, want %q", assignment.PublicURL, "http://my-api.tunnel.test.com")
	}

	// Lookup by name should work.
	entry, ok := tm.LookupByName("my-api")
	if !ok {
		t.Fatal("expected to find tunnel by name")
	}
	if entry.Name != "my-api" {
		t.Errorf("entry name: got %q, want %q", entry.Name, "my-api")
	}
}

func TestRegisterDuplicateNameRejected(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session1 := &Session{TokenID: "tok_test123456"}
	session2 := &Session{TokenID: "tok_other789012"}
	req := &proto.TunnelRequest{LocalPort: 3000, Protocol: "http", Name: "taken-name"}

	_, err := tm.Register(session1, req)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}

	// Different token requesting the same name should be rejected.
	_, err = tm.Register(session2, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http", Name: "taken-name"})
	if err == nil {
		t.Fatal("expected error for duplicate name from different token")
	}
}

func TestRegisterSameTokenReregistration(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}
	req := &proto.TunnelRequest{LocalPort: 3000, Protocol: "http", Name: "reconnect-name"}

	a1, err := tm.Register(session, req)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}

	// Same token re-registering same name should succeed (reconnect case).
	a2, err := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http", Name: "reconnect-name"})
	if err != nil {
		t.Fatalf("re-Register same token: %v", err)
	}

	// Should get a new tunnel ID but same name.
	if a2.Name != a1.Name {
		t.Errorf("name changed: got %q, want %q", a2.Name, a1.Name)
	}
	if a2.TunnelID == a1.TunnelID {
		t.Error("expected new tunnel ID on re-registration")
	}
}

func TestRegisterReservedNameRejected(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}

	reserved := []string{"www", "api", "admin", "status", "health"}
	for _, name := range reserved {
		_, err := tm.Register(session, &proto.TunnelRequest{
			LocalPort: 3000,
			Protocol:  "http",
			Name:      name,
		})
		if err == nil {
			t.Errorf("expected error for reserved name %q", name)
		}
	}
}

func TestRegisterAutoGeneratedFormat(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	namePattern := regexp.MustCompile(`^[a-z]+-[a-z]+$`)
	session := &Session{TokenID: "tok_test123456"}

	for i := 0; i < 10; i++ {
		a, err := tm.Register(session, &proto.TunnelRequest{
			LocalPort: 3000 + i,
			Protocol:  "http",
		})
		if err != nil {
			t.Fatalf("Register[%d]: %v", i, err)
		}
		if !namePattern.MatchString(a.Name) {
			t.Errorf("auto-generated name %q does not match adjective-noun format", a.Name)
		}
	}
}

func TestDeregisterReleasesName(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http", Name: "reuse-me"})

	// Deregister should release the name.
	tm.Deregister(a.TunnelID)

	// Re-registering the same name should succeed.
	_, err := tm.Register(session, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http", Name: "reuse-me"})
	if err != nil {
		t.Fatalf("expected re-register after deregister to succeed: %v", err)
	}
}

func TestRegisterInvalidProtocol(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}
	_, err := tm.Register(session, &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "tcp",
	})
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
}

func TestRegisterInvalidPort(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}

	_, err := tm.Register(session, &proto.TunnelRequest{
		LocalPort: 0,
		Protocol:  "http",
	})
	if err == nil {
		t.Fatal("expected error for port 0")
	}

	_, err = tm.Register(session, &proto.TunnelRequest{
		LocalPort: 70000,
		Protocol:  "http",
	})
	if err == nil {
		t.Fatal("expected error for port > 65535")
	}
}

func TestLookup(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}
	assignment, _ := tm.Register(session, &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})

	entry, ok := tm.Lookup(assignment.TunnelID)
	if !ok {
		t.Fatal("expected to find tunnel")
	}
	if entry.TunnelID != assignment.TunnelID {
		t.Errorf("tunnel ID mismatch: got %q, want %q", entry.TunnelID, assignment.TunnelID)
	}
	if entry.LocalPort != 3000 {
		t.Errorf("local port: got %d, want %d", entry.LocalPort, 3000)
	}
	if entry.Session != session {
		t.Error("session reference mismatch")
	}
}

func TestLookupNotFound(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	_, ok := tm.Lookup("nonexistent")
	if ok {
		t.Error("expected lookup to return false for nonexistent tunnel")
	}
}

func TestDeregister(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}
	assignment, _ := tm.Register(session, &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})

	tm.Deregister(assignment.TunnelID)

	_, ok := tm.Lookup(assignment.TunnelID)
	if ok {
		t.Error("tunnel should not be found after deregister")
	}
}

func TestDeregisterNonexistent(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	// Should not panic.
	tm.Deregister("nonexistent")
}

func TestDeregisterBySession(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session1 := &Session{TokenID: "tok_session1"}
	session2 := &Session{TokenID: "tok_session2"}

	// Register tunnels for both sessions.
	a1, _ := tm.Register(session1, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})
	a2, _ := tm.Register(session1, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http"})
	a3, _ := tm.Register(session2, &proto.TunnelRequest{LocalPort: 4000, Protocol: "http"})

	// Deregister all tunnels for session1.
	tm.DeregisterBySession(session1)

	// Session1 tunnels should be gone.
	if _, ok := tm.Lookup(a1.TunnelID); ok {
		t.Error("session1 tunnel 1 should be deregistered")
	}
	if _, ok := tm.Lookup(a2.TunnelID); ok {
		t.Error("session1 tunnel 2 should be deregistered")
	}

	// Session2 tunnel should still exist.
	if _, ok := tm.Lookup(a3.TunnelID); !ok {
		t.Error("session2 tunnel should still exist")
	}
}

func TestListActive(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test123456"}

	// Empty initially.
	if len(tm.ListActive()) != 0 {
		t.Error("expected 0 active tunnels initially")
	}

	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http"})

	active := tm.ListActive()
	if len(active) != 2 {
		t.Errorf("expected 2 active tunnels, got %d", len(active))
	}
}

func TestConcurrentAccess(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup

	// Concurrent registrations.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session := &Session{TokenID: "tok_concurrent"}
			_, err := tm.Register(session, &proto.TunnelRequest{
				LocalPort: 3000 + i,
				Protocol:  "http",
			})
			if err != nil {
				t.Errorf("concurrent Register[%d]: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	active := tm.ListActive()
	if len(active) != numGoroutines {
		t.Errorf("expected %d active tunnels, got %d", numGoroutines, len(active))
	}

	// Concurrent lookups + deregisters.
	for _, entry := range active {
		wg.Add(2)
		id := entry.TunnelID
		go func() {
			defer wg.Done()
			tm.Lookup(id)
		}()
		go func() {
			defer wg.Done()
			tm.Deregister(id)
		}()
	}
	wg.Wait()
}

func TestTunnelIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := randomTunnelID(tunnelIDLen)
		if seen[id] {
			t.Fatalf("duplicate tunnel ID on iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}

func TestTunnelIDFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := randomTunnelID(tunnelIDLen)
		if len(id) != tunnelIDLen {
			t.Errorf("tunnel ID length: got %d, want %d", len(id), tunnelIDLen)
		}
		matched, _ := regexp.MatchString(`^[a-z0-9]+$`, id)
		if !matched {
			t.Errorf("tunnel ID not lowercase alphanumeric: %q", id)
		}
	}
}

func TestTunnelCreatedAt(t *testing.T) {
	tm, st := newTestTunnelManager(t)
	defer st.Close()

	before := time.Now().UTC()
	session := &Session{TokenID: "tok_test123456"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})
	after := time.Now().UTC()

	entry, _ := tm.Lookup(a.TunnelID)
	if entry.CreatedAt.Before(before) || entry.CreatedAt.After(after) {
		t.Errorf("created_at %v not between %v and %v", entry.CreatedAt, before, after)
	}
}
