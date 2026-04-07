package server

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/store"
)

func newTestTCPEdge(t *testing.T, portMin, portMax int) (*TCPEdge, *TunnelManager, *store.Store) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Migrate(); err != nil {
		st.Close()
		t.Fatalf("store.Migrate: %v", err)
	}
	tm := NewTunnelManager("tunnel.test.com", st, testLogger())
	te := NewTCPEdge(portMin, portMax, tm, testLogger())
	tm.SetTCPEdge(te)
	return te, tm, st
}

func TestTCPEdge_AssignPort(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50200, 50210)
	defer st.Close()
	defer te.Shutdown()

	port, err := te.AssignPort()
	if err != nil {
		t.Fatalf("AssignPort: %v", err)
	}

	if port < 50200 || port > 50210 {
		t.Errorf("port out of range: got %d, want 50200-50210", port)
	}

	// Port should be listening.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("port %d not listening: %v", port, err)
	}
	conn.Close()
}

func TestTCPEdge_AssignPortUnique(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50220, 50225)
	defer st.Close()
	defer te.Shutdown()

	seen := make(map[int]bool)
	for i := 0; i < 6; i++ {
		port, err := te.AssignPort()
		if err != nil {
			t.Fatalf("AssignPort[%d]: %v", i, err)
		}
		if seen[port] {
			t.Fatalf("duplicate port assigned: %d", port)
		}
		seen[port] = true
	}
}

func TestTCPEdge_ReleasePort(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50230, 50235)
	defer st.Close()
	defer te.Shutdown()

	port, err := te.AssignPort()
	if err != nil {
		t.Fatalf("AssignPort: %v", err)
	}

	// Port should be listening before release.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("port %d not listening before release: %v", port, err)
	}
	conn.Close()

	// Release the port.
	te.ReleasePort(port)

	// Give the listener time to close.
	time.Sleep(100 * time.Millisecond)

	// Port should NOT be listening after release.
	conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Errorf("port %d still listening after release", port)
	}
}

func TestTCPEdge_PortExhaustion(t *testing.T) {
	// Range of exactly 3 ports.
	te, _, st := newTestTCPEdge(t, 50240, 50242)
	defer st.Close()
	defer te.Shutdown()

	for i := 0; i < 3; i++ {
		_, err := te.AssignPort()
		if err != nil {
			t.Fatalf("AssignPort[%d]: %v", i, err)
		}
	}

	// Fourth assignment should fail.
	_, err := te.AssignPort()
	if err == nil {
		t.Fatal("expected error for port exhaustion")
	}
	// Error message should mention exhaustion.
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

func TestTCPEdge_ReleaseAfterExhaustion(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50250, 50251)
	defer st.Close()
	defer te.Shutdown()

	p1, _ := te.AssignPort()
	p2, _ := te.AssignPort()

	// Exhausted.
	_, err := te.AssignPort()
	if err == nil {
		t.Fatal("expected exhaustion")
	}

	// Release one.
	te.ReleasePort(p1)
	time.Sleep(100 * time.Millisecond)

	// Now we should be able to assign again.
	p3, err := te.AssignPort()
	if err != nil {
		t.Fatalf("AssignPort after release: %v", err)
	}
	if p3 < 50250 || p3 > 50251 {
		t.Errorf("port out of range: %d", p3)
	}

	_ = p2 // suppress unused
}

func TestTCPEdge_Shutdown(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50260, 50265)
	defer st.Close()

	ports := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		port, err := te.AssignPort()
		if err != nil {
			t.Fatalf("AssignPort[%d]: %v", i, err)
		}
		ports = append(ports, port)
	}

	te.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// All ports should be closed.
	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			t.Errorf("port %d still listening after shutdown", port)
		}
	}
}

func TestTCPEdge_ConcurrentAssign(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50270, 50290)
	defer st.Close()
	defer te.Shutdown()

	const n = 10
	var wg sync.WaitGroup
	ports := make(chan int, n)
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			port, err := te.AssignPort()
			if err != nil {
				errs <- err
				return
			}
			ports <- port
		}()
	}

	wg.Wait()
	close(ports)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent AssignPort error: %v", err)
	}

	seen := make(map[int]bool)
	for port := range ports {
		if seen[port] {
			t.Errorf("duplicate port in concurrent test: %d", port)
		}
		seen[port] = true
	}

	if len(seen) != n {
		t.Errorf("expected %d unique ports, got %d", n, len(seen))
	}
}

func TestTCPEdge_ReleaseNonexistent(t *testing.T) {
	te, _, st := newTestTCPEdge(t, 50300, 50310)
	defer st.Close()
	defer te.Shutdown()

	// Should not panic.
	te.ReleasePort(99999)
}
