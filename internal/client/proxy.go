package client

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// bufSize is the buffer size for io.Copy operations.
const bufSize = 32 * 1024

// http502Response is the canned response written when the local service
// is unreachable, per CON-001 §5.4.
var http502Response = []byte(
	"HTTP/1.1 502 Bad Gateway\r\n" +
		"Content-Type: text/plain\r\n" +
		"X-Fonzygrok-Error: true\r\n" +
		"\r\n" +
		"fonzygrok: tunnel error - local service unreachable\r\n",
)

// bufPool is a sync.Pool of byte slices used for bidirectional copy,
// avoiding per-request heap allocation.
var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, bufSize)
		return &b
	},
}

// LocalProxy accepts incoming SSH proxy channels and forwards traffic
// to a local service on the configured port.
type LocalProxy struct {
	localPort int
	logger    *slog.Logger
	Inspector *Inspector
}

// NewLocalProxy creates a new LocalProxy that dials the given local port.
func NewLocalProxy(localPort int, logger *slog.Logger) *LocalProxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &LocalProxy{
		localPort: localPort,
		logger:    logger,
	}
}

// HandleChannels processes incoming SSH channel requests until ctx is cancelled
// or channelChan is closed. Each accepted channel is handled in its own goroutine.
func (lp *LocalProxy) HandleChannels(ctx context.Context, channelChan <-chan ssh.NewChannel) {
	for {
		select {
		case <-ctx.Done():
			lp.logger.Info("proxy: context cancelled, stopping channel handler")
			return
		case newCh, ok := <-channelChan:
			if !ok {
				lp.logger.Info("proxy: channel source closed")
				return
			}
			if newCh.ChannelType() != ChannelTypeProxy {
				lp.logger.Warn("proxy: rejecting unknown channel type",
					slog.String("type", newCh.ChannelType()),
				)
				newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
				continue
			}
			go lp.handleSingleChannel(newCh)
		}
	}
}

// handleSingleChannel accepts a proxy channel and bridges it to localhost.
func (lp *LocalProxy) handleSingleChannel(newCh ssh.NewChannel) {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		lp.logger.Error("proxy: accept channel failed",
			slog.String("error", err.Error()),
		)
		return
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	start := time.Now()

	localAddr := fmt.Sprintf("127.0.0.1:%d", lp.localPort)
	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		lp.logger.Warn("proxy: local service unreachable, sending 502",
			slog.String("addr", localAddr),
			slog.String("error", err.Error()),
		)
		ch.Write(http502Response)
		return
	}
	defer localConn.Close()

	lp.logger.Debug("proxy: connected to local service",
		slog.String("addr", localAddr),
	)

	// If inspector is active, wrap streams to capture metadata.
	var reqCapture *requestCapture
	var respCapture *responseCapture
	var chReader io.Reader = ch
	var localReader io.Reader = localConn

	if lp.Inspector != nil {
		reqCapture = &requestCapture{}
		respCapture = &responseCapture{}
		chReader = io.TeeReader(ch, reqCapture)
		localReader = io.TeeReader(localConn, respCapture)
	}

	// Bidirectional copy using pooled buffers.
	var wg sync.WaitGroup
	wg.Add(2)

	// SSH channel → local service
	go func() {
		defer wg.Done()
		lp.copyWithPool(localConn, chReader)
		// Signal the local connection that no more data is coming.
		if tc, ok := localConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// Local service → SSH channel
	go func() {
		defer wg.Done()
		lp.copyWithPool(ch, localReader)
		ch.CloseWrite()
	}()

	wg.Wait()

	// Record to inspector after round-trip completes.
	if lp.Inspector != nil && reqCapture != nil {
		entry := buildRequestEntry(reqCapture, respCapture, start)
		lp.Inspector.Record(entry)
	}
}

// copyWithPool copies from src to dst using a pooled buffer.
func (lp *LocalProxy) copyWithPool(dst io.Writer, src io.Reader) {
	bufp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufp)

	_, err := io.CopyBuffer(dst, src, *bufp)
	if err != nil && !isClosedError(err) {
		lp.logger.Debug("proxy: copy finished",
			slog.String("error", err.Error()),
		)
	}
}

// isClosedError reports whether err is a benign "use of closed" error.
func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	// net.ErrClosed and io.EOF are expected during normal shutdown.
	if err == io.EOF {
		return true
	}
	// Check for the net package's closed network connection error.
	var opErr *net.OpError
	if ok := (interface{})(err).(*net.OpError); ok == opErr {
		// fallthrough to string check
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe")
}

// requestCapture captures the first bytes of the HTTP request for inspection.
type requestCapture struct {
	buf   bytes.Buffer
	limit int
}

// Write captures up to maxBodyPreview bytes of the request.
func (rc *requestCapture) Write(p []byte) (int, error) {
	if rc.buf.Len() < maxBodyPreview*4 {
		remaining := maxBodyPreview*4 - rc.buf.Len()
		if len(p) > remaining {
			rc.buf.Write(p[:remaining])
		} else {
			rc.buf.Write(p)
		}
	}
	return len(p), nil
}

// responseCapture captures the first bytes of the HTTP response for inspection.
type responseCapture struct {
	buf bytes.Buffer
}

// Write captures up to maxBodyPreview bytes of the response.
func (rc *responseCapture) Write(p []byte) (int, error) {
	if rc.buf.Len() < maxBodyPreview*4 {
		remaining := maxBodyPreview*4 - rc.buf.Len()
		if len(p) > remaining {
			rc.buf.Write(p[:remaining])
		} else {
			rc.buf.Write(p)
		}
	}
	return len(p), nil
}

// buildRequestEntry parses captured request/response data into a RequestEntry.
func buildRequestEntry(req *requestCapture, resp *responseCapture, start time.Time) RequestEntry {
	entry := RequestEntry{
		Timestamp:       start,
		DurationMs:      float64(time.Since(start).Milliseconds()),
		RequestSize:     int64(req.buf.Len()),
		ResponseSize:    int64(resp.buf.Len()),
		RequestHeaders:  make(map[string]string),
		ResponseHeaders: make(map[string]string),
	}

	// Parse request first line: "GET /path HTTP/1.1"
	reqReader := bufio.NewReader(bytes.NewReader(req.buf.Bytes()))
	if line, err := reqReader.ReadString('\n'); err == nil {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 2 {
			entry.Method = parts[0]
			entry.Path = parts[1]
		}
	}

	// Parse request headers.
	for {
		line, err := reqReader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" || err != nil {
			break
		}
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			entry.RequestHeaders[key] = val
		}
	}

	// Parse response first line: "HTTP/1.1 200 OK"
	respReader := bufio.NewReader(bytes.NewReader(resp.buf.Bytes()))
	if line, err := respReader.ReadString('\n'); err == nil {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 2 {
			if code, err := strconv.Atoi(parts[1]); err == nil {
				entry.StatusCode = code
			}
		}
	}

	// Parse response headers and capture body preview.
	for {
		line, err := respReader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" || err != nil {
			break
		}
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			entry.ResponseHeaders[key] = val
		}
	}

	// Remaining is body — capture first 1KB.
	bodyBytes := make([]byte, maxBodyPreview)
	n, _ := respReader.Read(bodyBytes)
	if n > 0 {
		entry.BodyPreview = string(bodyBytes[:n])
	}

	return entry
}

