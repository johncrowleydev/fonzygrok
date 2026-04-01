package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestInspectorRingBufferCapacity verifies the 100-entry ring buffer limit.
func TestInspectorRingBufferCapacity(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())

	// Record 150 entries — only last 100 should remain.
	for i := 0; i < 150; i++ {
		ins.Record(RequestEntry{
			Method: "GET",
			Path:   fmt.Sprintf("/path-%d", i),
		})
	}

	if ins.Len() != maxEntries {
		t.Errorf("Len() = %d, want %d", ins.Len(), maxEntries)
	}

	entries := ins.Entries()
	// First entry should be path-50 (entries 0-49 were evicted).
	if entries[0].Path != "/path-50" {
		t.Errorf("first entry Path = %q, want %q", entries[0].Path, "/path-50")
	}
	// Last entry should be path-149.
	if entries[maxEntries-1].Path != "/path-149" {
		t.Errorf("last entry Path = %q, want %q", entries[maxEntries-1].Path, "/path-149")
	}
}

// TestInspectorRingBufferEviction verifies the 101st entry evicts the first.
func TestInspectorRingBufferEviction(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())

	// Fill to capacity.
	for i := 0; i < 100; i++ {
		ins.Record(RequestEntry{
			Method: "GET",
			Path:   fmt.Sprintf("/request-%d", i),
		})
	}

	if ins.Len() != 100 {
		t.Fatalf("Len() = %d, want 100", ins.Len())
	}

	// Record 101st.
	ins.Record(RequestEntry{Method: "POST", Path: "/request-100"})

	if ins.Len() != 100 {
		t.Errorf("Len() = %d after 101st entry, want 100", ins.Len())
	}

	entries := ins.Entries()
	// First entry should now be /request-1.
	if entries[0].Path != "/request-1" {
		t.Errorf("first entry after eviction = %q, want %q", entries[0].Path, "/request-1")
	}
}

// TestInspectorClear verifies clearing the buffer.
func TestInspectorClear(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())

	ins.Record(RequestEntry{Method: "GET", Path: "/test"})
	ins.Record(RequestEntry{Method: "POST", Path: "/test"})

	ins.Clear()

	if ins.Len() != 0 {
		t.Errorf("Len() = %d after Clear(), want 0", ins.Len())
	}
}

// TestInspectorSSEBroadcast verifies that Record broadcasts to SSE subscribers.
func TestInspectorSSEBroadcast(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())

	// Subscribe manually.
	ch := make(chan RequestEntry, 5)
	ins.subsMu.Lock()
	ins.subs[ch] = struct{}{}
	ins.subsMu.Unlock()

	entry := RequestEntry{
		Method:     "POST",
		Path:       "/api/data",
		StatusCode: 201,
	}
	ins.Record(entry)

	select {
	case got := <-ch:
		if got.Method != "POST" {
			t.Errorf("broadcast Method = %q, want %q", got.Method, "POST")
		}
		if got.Path != "/api/data" {
			t.Errorf("broadcast Path = %q, want %q", got.Path, "/api/data")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE broadcast")
	}
}

// TestInspectorIDAssignment verifies sequential ID assignment.
func TestInspectorIDAssignment(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())

	ins.Record(RequestEntry{Method: "GET", Path: "/a"})
	ins.Record(RequestEntry{Method: "GET", Path: "/b"})

	entries := ins.Entries()
	if entries[0].ID != "req-1" {
		t.Errorf("first ID = %q, want %q", entries[0].ID, "req-1")
	}
	if entries[1].ID != "req-2" {
		t.Errorf("second ID = %q, want %q", entries[1].ID, "req-2")
	}
}

// TestInspectorHTTPGetRequests verifies GET /api/requests returns JSON.
func TestInspectorHTTPGetRequests(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())
	ins.Record(RequestEntry{Method: "GET", Path: "/hello", StatusCode: 200})
	ins.Record(RequestEntry{Method: "POST", Path: "/world", StatusCode: 201})

	req := httptest.NewRequest(http.MethodGet, "/api/requests", nil)
	w := httptest.NewRecorder()
	ins.handleRequests(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var entries []RequestEntry
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatalf("unmarshal error: %v, body: %s", err, body)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Method != "GET" {
		t.Errorf("entries[0].Method = %q, want %q", entries[0].Method, "GET")
	}
}

// TestInspectorHTTPDeleteRequests verifies DELETE /api/requests clears buffer.
func TestInspectorHTTPDeleteRequests(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())
	ins.Record(RequestEntry{Method: "GET", Path: "/test"})

	req := httptest.NewRequest(http.MethodDelete, "/api/requests", nil)
	w := httptest.NewRecorder()
	ins.handleRequests(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ins.Len() != 0 {
		t.Errorf("Len() = %d after DELETE, want 0", ins.Len())
	}
}

// TestInspectorHTTPServeUI verifies GET / returns HTML.
func TestInspectorHTTPServeUI(t *testing.T) {
	ins := NewInspector("localhost:0", slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	ins.handleUI(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Fonzygrok Inspector") {
		t.Error("response body should contain 'Fonzygrok Inspector'")
	}
}

// TestInspectorStart verifies the HTTP server starts and serves.
func TestInspectorStart(t *testing.T) {
	ins := NewInspector("127.0.0.1:0", slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start on a random port.
	errCh := make(chan error, 1)
	go func() {
		errCh <- ins.Start(ctx)
	}()

	// Give the server a moment to start.
	time.Sleep(100 * time.Millisecond)

	// Cancel to shut it down.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start() error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start() didn't return after context cancellation")
	}
}
