package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/fonzygrok/fonzygrok/internal/proto"
	"golang.org/x/crypto/ssh"
)

// ControlChannel wraps an SSH channel carrying newline-delimited JSON
// control messages per CON-001 §4.
type ControlChannel struct {
	ch      ssh.Channel
	encoder *proto.Encoder
	decoder *proto.Decoder
	logger  *slog.Logger
}

// newControlChannel creates a ControlChannel over the given SSH channel.
func newControlChannel(ch ssh.Channel, logger *slog.Logger) *ControlChannel {
	return &ControlChannel{
		ch:      ch,
		encoder: proto.NewEncoder(ch),
		decoder: proto.NewDecoder(ch),
		logger:  logger,
	}
}

// RequestTunnel sends a TunnelRequest and reads the server response.
// It returns the TunnelAssignment on success, or an error if the server
// responds with an Error message or the channel fails.
func (cc *ControlChannel) RequestTunnel(localPort int, protocol string) (*proto.TunnelAssignment, error) {
	req := proto.TunnelRequest{
		LocalPort: localPort,
		Protocol:  protocol,
	}

	msg, err := proto.WrapPayload(proto.TypeTunnelRequest, req)
	if err != nil {
		return nil, fmt.Errorf("control: wrap tunnel request: %w", err)
	}

	cc.logger.Info("requesting tunnel",
		slog.Int("local_port", localPort),
		slog.String("protocol", protocol),
	)

	if err := cc.encoder.Encode(msg); err != nil {
		return nil, fmt.Errorf("control: send tunnel request: %w", err)
	}

	// Read the response.
	resp, err := cc.decoder.Decode()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("control: server closed channel before responding")
		}
		return nil, fmt.Errorf("control: read response: %w", err)
	}

	switch resp.Type {
	case proto.TypeTunnelAssignment:
		var assignment proto.TunnelAssignment
		if err := json.Unmarshal(resp.Payload, &assignment); err != nil {
			return nil, fmt.Errorf("control: unmarshal assignment: %w", err)
		}
		cc.logger.Info("tunnel assigned",
			slog.String("tunnel_id", assignment.TunnelID),
			slog.String("public_url", assignment.PublicURL),
		)
		return &assignment, nil

	case proto.TypeError:
		var errMsg proto.ErrorMessage
		if err := json.Unmarshal(resp.Payload, &errMsg); err != nil {
			return nil, fmt.Errorf("control: unmarshal error response: %w", err)
		}
		return nil, fmt.Errorf("control: server error [%s]: %s", errMsg.Code, errMsg.Message)

	default:
		return nil, fmt.Errorf("control: unexpected response type: %q", resp.Type)
	}
}

// CloseTunnel sends a TunnelClose message to the server.
func (cc *ControlChannel) CloseTunnel(tunnelID string) error {
	closeMsg := proto.TunnelClose{
		TunnelID: tunnelID,
		Reason:   proto.ReasonClientDisconnect,
	}

	msg, err := proto.WrapPayload(proto.TypeTunnelClose, closeMsg)
	if err != nil {
		return fmt.Errorf("control: wrap tunnel close: %w", err)
	}

	cc.logger.Info("closing tunnel",
		slog.String("tunnel_id", tunnelID),
	)

	if err := cc.encoder.Encode(msg); err != nil {
		return fmt.Errorf("control: send tunnel close: %w", err)
	}
	return nil
}

// Close shuts down the control channel.
func (cc *ControlChannel) Close() error {
	cc.logger.Debug("closing control channel")
	if err := cc.ch.Close(); err != nil {
		return fmt.Errorf("control: close channel: %w", err)
	}
	return nil
}
