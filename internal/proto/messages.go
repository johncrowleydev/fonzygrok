// Package proto defines the control channel message types used for
// communication between the fonzygrok client and server over SSH.
// All types conform to CON-001 §8.
package proto

import "encoding/json"

// Control message type constants used in the ControlMessage.Type field.
const (
	TypeTunnelRequest    = "tunnel_request"
	TypeTunnelAssignment = "tunnel_assignment"
	TypeTunnelClose      = "tunnel_close"
	TypeError            = "error"
)

// ControlMessage is the envelope for all control channel messages.
// Messages are newline-delimited JSON on the control channel.
type ControlMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// TunnelRequest is sent by the client to request a new tunnel.
type TunnelRequest struct {
	LocalPort int    `json:"local_port"`
	Protocol  string `json:"protocol"`
	Subdomain string `json:"subdomain,omitempty"`
}

// TunnelAssignment is sent by the server after a successful tunnel setup.
type TunnelAssignment struct {
	TunnelID          string `json:"tunnel_id"`
	AssignedSubdomain string `json:"assigned_subdomain"`
	PublicURL         string `json:"public_url"`
	Protocol          string `json:"protocol"`
}

// TunnelClose is sent by either side to close a tunnel.
type TunnelClose struct {
	TunnelID string `json:"tunnel_id"`
	Reason   string `json:"reason"`
}

// TunnelClose reason constants per CON-001 §4.3.
const (
	ReasonClientDisconnect = "client_disconnect"
	ReasonServerShutdown   = "server_shutdown"
	ReasonTokenRevoked     = "token_revoked"
	ReasonIdleTimeout      = "idle_timeout"
)

// ErrorMessage is sent by the server when a request fails.
type ErrorMessage struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	RequestType string `json:"request_type"`
}

// Error code constants per CON-001 §4.4.
const (
	ErrInvalidProtocol    = "INVALID_PROTOCOL"
	ErrSubdomainTaken     = "SUBDOMAIN_TAKEN"
	ErrSubdomainInvalid   = "SUBDOMAIN_INVALID"
	ErrMaxTunnelsExceeded = "MAX_TUNNELS_EXCEEDED"
	ErrInternalError      = "INTERNAL_ERROR"
)

// WrapPayload creates a ControlMessage with the given type and payload.
// It returns an error if the payload cannot be marshaled to JSON.
func WrapPayload(msgType string, payload interface{}) (*ControlMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &ControlMessage{
		Type:    msgType,
		Payload: raw,
	}, nil
}
