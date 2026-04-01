package server

import (
	"io"
	"sync/atomic"
	"time"
)

// TunnelMetrics holds per-tunnel traffic counters.
// All fields are accessed atomically for concurrent safety.
type TunnelMetrics struct {
	BytesIn         atomic.Int64
	BytesOut        atomic.Int64
	RequestsProxied atomic.Int64
	LastRequestAt   atomic.Value // stores time.Time
}

// NewTunnelMetrics creates a new TunnelMetrics instance.
func NewTunnelMetrics() *TunnelMetrics {
	m := &TunnelMetrics{}
	m.LastRequestAt.Store(time.Time{})
	return m
}

// Snapshot returns a copy of the current metrics as plain values.
type MetricsSnapshot struct {
	BytesIn         int64     `json:"bytes_in"`
	BytesOut        int64     `json:"bytes_out"`
	RequestsProxied int64     `json:"requests_proxied"`
	LastRequestAt   time.Time `json:"last_request_at,omitempty"`
}

// Snapshot returns a point-in-time snapshot of the metrics.
func (m *TunnelMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		BytesIn:         m.BytesIn.Load(),
		BytesOut:        m.BytesOut.Load(),
		RequestsProxied: m.RequestsProxied.Load(),
		LastRequestAt:   m.LastRequestAt.Load().(time.Time),
	}
}

// RecordRequest increments the request counter and updates LastRequestAt.
func (m *TunnelMetrics) RecordRequest() {
	m.RequestsProxied.Add(1)
	m.LastRequestAt.Store(time.Now().UTC())
}

// CountingReader wraps an io.Reader and atomically increments a byte counter.
type CountingReader struct {
	reader  io.Reader
	counter *atomic.Int64
}

// NewCountingReader wraps r and increments counter on every Read.
func NewCountingReader(r io.Reader, counter *atomic.Int64) *CountingReader {
	return &CountingReader{reader: r, counter: counter}
}

// Read implements io.Reader, adding the number of bytes read to the counter.
func (cr *CountingReader) Read(p []byte) (int, error) {
	n, err := cr.reader.Read(p)
	if n > 0 {
		cr.counter.Add(int64(n))
	}
	return n, err
}

// CountingWriter wraps an io.Writer and atomically increments a byte counter.
type CountingWriter struct {
	writer  io.Writer
	counter *atomic.Int64
}

// NewCountingWriter wraps w and increments counter on every Write.
func NewCountingWriter(w io.Writer, counter *atomic.Int64) *CountingWriter {
	return &CountingWriter{writer: w, counter: counter}
}

// Write implements io.Writer, adding the number of bytes written to the counter.
func (cw *CountingWriter) Write(p []byte) (int, error) {
	n, err := cw.writer.Write(p)
	if n > 0 {
		cw.counter.Add(int64(n))
	}
	return n, err
}
