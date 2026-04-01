package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// maxEntries is the ring buffer capacity.
	maxEntries = 100
	// maxBodyPreview is the max bytes of body to capture.
	maxBodyPreview = 1024
)

// RequestEntry captures a single proxied request/response.
type RequestEntry struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	StatusCode      int               `json:"status_code"`
	DurationMs      float64           `json:"duration_ms"`
	RequestSize     int64             `json:"request_size"`
	ResponseSize    int64             `json:"response_size"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	BodyPreview     string            `json:"body_preview,omitempty"`
}

// Inspector captures request/response metadata and broadcasts to SSE clients.
// It exposes a local HTTP server for the inspection UI.
type Inspector struct {
	addr    string
	logger  *slog.Logger
	entries []RequestEntry
	mu      sync.RWMutex
	subs    map[chan RequestEntry]struct{}
	subsMu  sync.Mutex
	nextID  int
}

// NewInspector creates a new Inspector that will serve the UI on addr.
func NewInspector(addr string, logger *slog.Logger) *Inspector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Inspector{
		addr:    addr,
		logger:  logger,
		entries: make([]RequestEntry, 0, maxEntries),
		subs:    make(map[chan RequestEntry]struct{}),
	}
}

// Addr returns the configured listen address.
func (ins *Inspector) Addr() string {
	return ins.addr
}

// Start launches the inspector HTTP server. Blocks until ctx is cancelled.
func (ins *Inspector) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ins.handleUI)
	mux.HandleFunc("/api/requests", ins.handleRequests)
	mux.HandleFunc("/api/requests/stream", ins.handleSSE)

	srv := &http.Server{
		Addr:    ins.addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", ins.addr)
	if err != nil {
		return fmt.Errorf("inspector: listen %s: %w", ins.addr, err)
	}

	ins.logger.Info("inspector started", slog.String("addr", "http://"+ins.addr))

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("inspector: serve: %w", err)
	}
	return nil
}

// Record adds an entry to the ring buffer and broadcasts to SSE subscribers.
func (ins *Inspector) Record(entry RequestEntry) {
	ins.mu.Lock()
	ins.nextID++
	entry.ID = fmt.Sprintf("req-%d", ins.nextID)
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	if len(ins.entries) >= maxEntries {
		// Evict oldest entry (ring buffer).
		ins.entries = ins.entries[1:]
	}
	ins.entries = append(ins.entries, entry)
	ins.mu.Unlock()

	// Broadcast to SSE subscribers (non-blocking).
	ins.subsMu.Lock()
	for ch := range ins.subs {
		select {
		case ch <- entry:
		default:
			// Slow subscriber, skip.
		}
	}
	ins.subsMu.Unlock()
}

// Entries returns a copy of all current entries.
func (ins *Inspector) Entries() []RequestEntry {
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	result := make([]RequestEntry, len(ins.entries))
	copy(result, ins.entries)
	return result
}

// Clear removes all entries.
func (ins *Inspector) Clear() {
	ins.mu.Lock()
	ins.entries = ins.entries[:0]
	ins.mu.Unlock()
}

// Len returns the current number of entries.
func (ins *Inspector) Len() int {
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	return len(ins.entries)
}

// handleUI serves the embedded HTML inspector page.
func (ins *Inspector) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// Serve static assets from embedded FS.
		serveInspectorAsset(w, r)
		return
	}
	serveInspectorAsset(w, r)
}

// handleRequests returns all stored entries as JSON, or clears them on DELETE.
func (ins *Inspector) handleRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entries := ins.Entries()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)

	case http.MethodDelete:
		ins.Clear()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"cleared"}`))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSE streams new entries via Server-Sent Events.
func (ins *Inspector) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan RequestEntry, 16)
	ins.subsMu.Lock()
	ins.subs[ch] = struct{}{}
	ins.subsMu.Unlock()

	defer func() {
		ins.subsMu.Lock()
		delete(ins.subs, ch)
		ins.subsMu.Unlock()
	}()

	// Send initial heartbeat.
	fmt.Fprintf(w, ": heartbeat\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry := <-ch:
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
