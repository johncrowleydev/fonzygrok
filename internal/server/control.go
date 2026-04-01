package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/fonzygrok/fonzygrok/internal/proto"
	"golang.org/x/crypto/ssh"
)

// TunnelRegistrar is the interface used by the ControlHandler to register
// and deregister tunnels. Implemented by TunnelManager.
type TunnelRegistrar interface {
	Register(session *Session, req *proto.TunnelRequest) (*proto.TunnelAssignment, error)
	Deregister(tunnelID string)
	DeregisterBySession(session *Session)
}

// ControlHandler manages the control channel for an SSH session.
// It reads ControlMessages, dispatches them by type, and writes responses.
// Per CON-001 §4: one control channel per session, newline-delimited JSON.
type ControlHandler struct {
	session   *Session
	tunnels   TunnelRegistrar
	logger    *slog.Logger
	mu        sync.Mutex
	hasCtrl   bool
}

// NewControlHandler creates a ControlHandler for the given session.
func NewControlHandler(session *Session, tunnels TunnelRegistrar, logger *slog.Logger) *ControlHandler {
	return &ControlHandler{
		session: session,
		tunnels: tunnels,
		logger:  logger,
	}
}

// HandleChannels processes incoming SSH channel requests for a session.
// It accepts one "control" channel and rejects duplicates. Per CON-001 §4.1.
// It also handles "direct-tcpip" and other channel types as needed.
func (h *ControlHandler) HandleChannels(chans <-chan ssh.NewChannel) {
	for newChan := range chans {
		switch newChan.ChannelType() {
		case "control":
			h.handleControlChannel(newChan)
		default:
			h.logger.Warn("ssh: rejecting unknown channel type",
				"type", newChan.ChannelType(),
				"remote_addr", h.session.RemoteAddr,
			)
			newChan.Reject(ssh.UnknownChannelType, "unsupported channel type")
		}
	}

	// Session disconnected — clean up all tunnels for this session.
	h.logger.Info("ssh: session channels closed, cleaning up",
		"token_id", h.session.TokenID,
		"remote_addr", h.session.RemoteAddr,
	)
	h.tunnels.DeregisterBySession(h.session)
}

// HandleGlobalRequests discards global requests (keepalive handled at SSH transport level).
func (h *ControlHandler) HandleGlobalRequests(reqs <-chan *ssh.Request) {
	for req := range reqs {
		if req.WantReply {
			req.Reply(false, nil)
		}
	}
}

// handleControlChannel accepts or rejects a control channel request.
func (h *ControlHandler) handleControlChannel(newChan ssh.NewChannel) {
	h.mu.Lock()
	if h.hasCtrl {
		h.mu.Unlock()
		h.logger.Warn("ssh: rejecting duplicate control channel",
			"remote_addr", h.session.RemoteAddr,
		)
		newChan.Reject(ssh.ResourceShortage, "only one control channel per session")
		return
	}
	h.hasCtrl = true
	h.mu.Unlock()

	ch, _, err := newChan.Accept()
	if err != nil {
		h.logger.Error("ssh: accept control channel", "error", err)
		return
	}

	go h.readControlLoop(ch)
}

// readControlLoop reads and dispatches control messages from the channel.
func (h *ControlHandler) readControlLoop(ch ssh.Channel) {
	defer ch.Close()

	decoder := proto.NewDecoder(ch)
	encoder := proto.NewEncoder(ch)

	for {
		msg, err := decoder.Decode()
		if err != nil {
			if err != io.EOF {
				h.logger.Error("ssh: read control message",
					"remote_addr", h.session.RemoteAddr,
					"error", err,
				)
			}
			return
		}

		h.dispatch(msg, encoder)
	}
}

// dispatch routes a ControlMessage to the appropriate handler.
func (h *ControlHandler) dispatch(msg *proto.ControlMessage, encoder *proto.Encoder) {
	switch msg.Type {
	case proto.TypeTunnelRequest:
		h.handleTunnelRequest(msg, encoder)
	case proto.TypeTunnelClose:
		h.handleTunnelClose(msg, encoder)
	default:
		h.logger.Warn("ssh: unknown control message type",
			"type", msg.Type,
			"remote_addr", h.session.RemoteAddr,
		)
		h.sendError(encoder, proto.ErrInternalError, fmt.Sprintf("unknown message type: %s", msg.Type), msg.Type)
	}
}

// handleTunnelRequest processes a tunnel_request message.
func (h *ControlHandler) handleTunnelRequest(msg *proto.ControlMessage, encoder *proto.Encoder) {
	var req proto.TunnelRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		h.logger.Error("ssh: unmarshal tunnel request",
			"remote_addr", h.session.RemoteAddr,
			"error", err,
		)
		h.sendError(encoder, proto.ErrInternalError, "invalid tunnel request payload", proto.TypeTunnelRequest)
		return
	}

	assignment, err := h.tunnels.Register(h.session, &req)
	if err != nil {
		h.logger.Warn("ssh: tunnel registration failed",
			"remote_addr", h.session.RemoteAddr,
			"error", err,
		)
		h.sendError(encoder, proto.ErrInternalError, err.Error(), proto.TypeTunnelRequest)
		return
	}

	h.logger.Info("ssh: tunnel registered",
		"tunnel_id", assignment.TunnelID,
		"name", assignment.Name,
		"subdomain", assignment.AssignedSubdomain,
		"local_port", req.LocalPort,
		"token_id", h.session.TokenID,
		"remote_addr", h.session.RemoteAddr,
	)

	resp, err := proto.WrapPayload(proto.TypeTunnelAssignment, assignment)
	if err != nil {
		h.logger.Error("ssh: wrap tunnel assignment", "error", err)
		return
	}
	if err := encoder.Encode(resp); err != nil {
		h.logger.Error("ssh: send tunnel assignment", "error", err)
	}
}

// handleTunnelClose processes a tunnel_close message.
func (h *ControlHandler) handleTunnelClose(msg *proto.ControlMessage, encoder *proto.Encoder) {
	var close proto.TunnelClose
	if err := json.Unmarshal(msg.Payload, &close); err != nil {
		h.logger.Error("ssh: unmarshal tunnel close",
			"remote_addr", h.session.RemoteAddr,
			"error", err,
		)
		return
	}

	h.logger.Info("ssh: tunnel close requested",
		"tunnel_id", close.TunnelID,
		"reason", close.Reason,
		"remote_addr", h.session.RemoteAddr,
	)

	h.tunnels.Deregister(close.TunnelID)
}

// sendError writes an error message to the control channel.
func (h *ControlHandler) sendError(encoder *proto.Encoder, code, message, requestType string) {
	errMsg := proto.ErrorMessage{
		Code:        code,
		Message:     message,
		RequestType: requestType,
	}
	resp, err := proto.WrapPayload(proto.TypeError, errMsg)
	if err != nil {
		h.logger.Error("ssh: wrap error message", "error", err)
		return
	}
	if err := encoder.Encode(resp); err != nil {
		h.logger.Error("ssh: send error message", "error", err)
	}
}
