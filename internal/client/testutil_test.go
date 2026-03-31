package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/fonzygrok/fonzygrok/internal/proto"
	"golang.org/x/crypto/ssh"
)

// testSSHServer is a minimal in-process SSH server used for testing the
// client without the real fonzygrok server (which is built in parallel
// by Dev A). It supports token auth, control channels, and proxy channels.
type testSSHServer struct {
	listener   net.Listener
	sshConfig  *ssh.ServerConfig
	addr       string
	validToken string
	t          *testing.T
}

// startTestSSHServer starts a minimal SSH server that:
//   - Accepts SSH connections with token auth (password = validToken)
//   - Accepts "control" channels and replies to TunnelRequest with TunnelAssignment
//   - Accepts "proxy" channels and echoes data back
//
// Returns the server address and a cleanup function.
func startTestSSHServer(t *testing.T, validToken string) (addr string, cleanup func()) {
	t.Helper()

	srv := &testSSHServer{
		validToken: validToken,
		t:          t,
	}

	// Generate a throwaway host key.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("testSSHServer: generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("testSSHServer: create signer: %v", err)
	}

	srv.sshConfig = &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() != SSHUsername {
				return nil, fmt.Errorf("invalid username: %s", conn.User())
			}
			if string(password) != validToken {
				return nil, fmt.Errorf("invalid token")
			}
			return &ssh.Permissions{}, nil
		},
	}
	srv.sshConfig.AddHostKey(signer)

	// Listen on a random port.
	srv.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testSSHServer: listen: %v", err)
	}
	srv.addr = srv.listener.Addr().String()

	go srv.acceptLoop()

	return srv.addr, func() {
		srv.listener.Close()
	}
}

// acceptLoop accepts incoming TCP connections and performs the SSH handshake.
func (s *testSSHServer) acceptLoop() {
	for {
		tcpConn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go s.handleConn(tcpConn)
	}
}

// handleConn performs the SSH server handshake and dispatches channels.
func (s *testSSHServer) handleConn(tcpConn net.Conn) {
	defer tcpConn.Close()

	srvConn, chans, reqs, err := ssh.NewServerConn(tcpConn, s.sshConfig)
	if err != nil {
		return // auth failure or handshake error — expected in negative tests
	}
	defer srvConn.Close()

	// Discard global requests.
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		switch newCh.ChannelType() {
		case ChannelTypeControl:
			go s.handleControlChannel(newCh)
		case ChannelTypeProxy:
			go s.handleProxyChannel(newCh)
		default:
			newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
		}
	}
}

// handleControlChannel accepts a control channel and responds to tunnel requests.
func (s *testSSHServer) handleControlChannel(newCh ssh.NewChannel) {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	decoder := proto.NewDecoder(ch)
	encoder := proto.NewEncoder(ch)

	for {
		msg, err := decoder.Decode()
		if err != nil {
			return // channel closed or read error
		}

		switch msg.Type {
		case proto.TypeTunnelRequest:
			var req proto.TunnelRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				s.sendError(encoder, proto.ErrInternalError, "bad request payload", proto.TypeTunnelRequest)
				continue
			}
			// Respond with a canned assignment.
			assignment := proto.TunnelAssignment{
				TunnelID:          "test01",
				AssignedSubdomain: "test01",
				PublicURL:         "http://test01.tunnel.example.com",
				Protocol:          req.Protocol,
			}
			resp, _ := proto.WrapPayload(proto.TypeTunnelAssignment, assignment)
			if err := encoder.Encode(resp); err != nil {
				return
			}

		case proto.TypeTunnelClose:
			// Acknowledged silently.
			return

		default:
			s.sendError(encoder, proto.ErrInternalError, "unknown message type", msg.Type)
		}
	}
}

// handleProxyChannel accepts a proxy channel and echoes back data.
func (s *testSSHServer) handleProxyChannel(newCh ssh.NewChannel) {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	// Echo: copy everything received back to the channel.
	io.Copy(ch, ch)
}

// sendError writes an Error message on the control channel.
func (s *testSSHServer) sendError(encoder *proto.Encoder, code, message, reqType string) {
	errMsg := proto.ErrorMessage{
		Code:        code,
		Message:     message,
		RequestType: reqType,
	}
	resp, _ := proto.WrapPayload(proto.TypeError, errMsg)
	encoder.Encode(resp)
}

// startTestSSHServerWithErrorResponse starts a test server that always
// responds to TunnelRequest with an Error message. Useful for testing
// error handling paths.
func startTestSSHServerWithErrorResponse(t *testing.T, validToken string, errCode, errMessage string) (addr string, cleanup func()) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("testSSHServer: generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("testSSHServer: create signer: %v", err)
	}

	sshCfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != validToken {
				return nil, fmt.Errorf("invalid token")
			}
			return &ssh.Permissions{}, nil
		},
	}
	sshCfg.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testSSHServer: listen: %v", err)
	}

	go func() {
		for {
			tcpConn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				srvConn, chans, reqs, err := ssh.NewServerConn(c, sshCfg)
				if err != nil {
					return
				}
				defer srvConn.Close()
				go ssh.DiscardRequests(reqs)

				for newCh := range chans {
					if newCh.ChannelType() == ChannelTypeControl {
						ch, chReqs, err := newCh.Accept()
						if err != nil {
							continue
						}
						go ssh.DiscardRequests(chReqs)
						decoder := proto.NewDecoder(ch)
						encoder := proto.NewEncoder(ch)

						msg, err := decoder.Decode()
						if err != nil {
							ch.Close()
							continue
						}
						if msg.Type == proto.TypeTunnelRequest {
							errResp := proto.ErrorMessage{
								Code:        errCode,
								Message:     errMessage,
								RequestType: proto.TypeTunnelRequest,
							}
							resp, _ := proto.WrapPayload(proto.TypeError, errResp)
							encoder.Encode(resp)
						}
						ch.Close()
					} else {
						newCh.Reject(ssh.UnknownChannelType, "not supported")
					}
				}
			}(tcpConn)
		}
	}()

	return listener.Addr().String(), func() { listener.Close() }
}
