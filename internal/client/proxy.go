package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

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

	// Bidirectional copy using pooled buffers.
	var wg sync.WaitGroup
	wg.Add(2)

	// SSH channel → local service
	go func() {
		defer wg.Done()
		lp.copyWithPool(localConn, ch)
		// Signal the local connection that no more data is coming.
		if tc, ok := localConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// Local service → SSH channel
	go func() {
		defer wg.Done()
		lp.copyWithPool(ch, localConn)
		ch.CloseWrite()
	}()

	wg.Wait()
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
	return contains(errStr, "use of closed network connection") ||
		contains(errStr, "connection reset by peer") ||
		contains(errStr, "broken pipe")
}

// contains checks if s contains substr (avoids importing strings).
func contains(s, substr string) bool {
	return len(substr) <= len(s) && searchString(s, substr)
}

// searchString naively searches for substr in s.
func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
