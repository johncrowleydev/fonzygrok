package server

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCountingReaderBasic(t *testing.T) {
	data := "hello, world!"
	reader := strings.NewReader(data)
	var counter atomic.Int64

	cr := NewCountingReader(reader, &counter)
	buf := make([]byte, 32)
	n, err := cr.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != len(data) {
		t.Errorf("bytes read: got %d, want %d", n, len(data))
	}
	if counter.Load() != int64(len(data)) {
		t.Errorf("counter: got %d, want %d", counter.Load(), len(data))
	}
}

func TestCountingReaderMultipleReads(t *testing.T) {
	data := strings.Repeat("x", 1000)
	reader := strings.NewReader(data)
	var counter atomic.Int64

	cr := NewCountingReader(reader, &counter)
	buf := make([]byte, 100)

	total := 0
	for {
		n, err := cr.Read(buf)
		total += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}

	if total != len(data) {
		t.Errorf("total bytes: got %d, want %d", total, len(data))
	}
	if counter.Load() != int64(len(data)) {
		t.Errorf("counter: got %d, want %d", counter.Load(), len(data))
	}
}

func TestCountingWriterBasic(t *testing.T) {
	var buf bytes.Buffer
	var counter atomic.Int64

	cw := NewCountingWriter(&buf, &counter)
	data := []byte("hello, metrics!")
	n, err := cw.Write(data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Errorf("bytes written: got %d, want %d", n, len(data))
	}
	if counter.Load() != int64(len(data)) {
		t.Errorf("counter: got %d, want %d", counter.Load(), len(data))
	}
	if buf.String() != string(data) {
		t.Errorf("buffer: got %q, want %q", buf.String(), string(data))
	}
}

func TestCountingWriterMultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	var counter atomic.Int64

	cw := NewCountingWriter(&buf, &counter)
	for i := 0; i < 100; i++ {
		cw.Write([]byte("test"))
	}

	if counter.Load() != 400 {
		t.Errorf("counter: got %d, want 400", counter.Load())
	}
}

func TestCountingReaderEOF(t *testing.T) {
	reader := strings.NewReader("")
	var counter atomic.Int64

	cr := NewCountingReader(reader, &counter)
	buf := make([]byte, 32)
	_, err := cr.Read(buf)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if counter.Load() != 0 {
		t.Errorf("counter should be 0 for empty read, got %d", counter.Load())
	}
}

func TestTunnelMetricsNewInstance(t *testing.T) {
	m := NewTunnelMetrics()
	snap := m.Snapshot()

	if snap.BytesIn != 0 {
		t.Errorf("BytesIn: got %d, want 0", snap.BytesIn)
	}
	if snap.BytesOut != 0 {
		t.Errorf("BytesOut: got %d, want 0", snap.BytesOut)
	}
	if snap.RequestsProxied != 0 {
		t.Errorf("RequestsProxied: got %d, want 0", snap.RequestsProxied)
	}
	if !snap.LastRequestAt.IsZero() {
		t.Errorf("LastRequestAt should be zero, got %v", snap.LastRequestAt)
	}
}

func TestTunnelMetricsRecordRequest(t *testing.T) {
	m := NewTunnelMetrics()
	before := time.Now().UTC()

	m.RecordRequest()
	m.RecordRequest()
	m.RecordRequest()

	snap := m.Snapshot()
	if snap.RequestsProxied != 3 {
		t.Errorf("RequestsProxied: got %d, want 3", snap.RequestsProxied)
	}
	if snap.LastRequestAt.Before(before) {
		t.Error("LastRequestAt should be after test start")
	}
}

func TestTunnelMetricsSnapshot(t *testing.T) {
	m := NewTunnelMetrics()
	m.BytesIn.Add(1024)
	m.BytesOut.Add(2048)
	m.RecordRequest()

	snap := m.Snapshot()
	if snap.BytesIn != 1024 {
		t.Errorf("BytesIn: got %d, want 1024", snap.BytesIn)
	}
	if snap.BytesOut != 2048 {
		t.Errorf("BytesOut: got %d, want 2048", snap.BytesOut)
	}
	if snap.RequestsProxied != 1 {
		t.Errorf("RequestsProxied: got %d, want 1", snap.RequestsProxied)
	}
}

func TestConcurrentMetricsAccess(t *testing.T) {
	m := NewTunnelMetrics()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.BytesIn.Add(100)
			m.BytesOut.Add(200)
			m.RecordRequest()
		}()
	}
	wg.Wait()

	snap := m.Snapshot()
	if snap.BytesIn != int64(goroutines*100) {
		t.Errorf("BytesIn: got %d, want %d", snap.BytesIn, goroutines*100)
	}
	if snap.BytesOut != int64(goroutines*200) {
		t.Errorf("BytesOut: got %d, want %d", snap.BytesOut, goroutines*200)
	}
	if snap.RequestsProxied != goroutines {
		t.Errorf("RequestsProxied: got %d, want %d", snap.RequestsProxied, goroutines)
	}
}

func TestConcurrentCountingReader(t *testing.T) {
	var counter atomic.Int64
	var wg sync.WaitGroup
	const goroutines = 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader := strings.NewReader(strings.Repeat("a", 100))
			cr := NewCountingReader(reader, &counter)
			io.ReadAll(cr)
		}()
	}
	wg.Wait()

	if counter.Load() != int64(goroutines*100) {
		t.Errorf("counter: got %d, want %d", counter.Load(), goroutines*100)
	}
}
