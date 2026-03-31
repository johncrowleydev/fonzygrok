package proto

// TunnelState represents the lifecycle state of a tunnel per CON-001 §6.
type TunnelState int

const (
	// TunnelRequested indicates the client has requested a tunnel.
	TunnelRequested TunnelState = iota
	// TunnelAssigned indicates the server has assigned a subdomain.
	TunnelAssigned
	// TunnelActive indicates the tunnel is proxying traffic.
	TunnelActive
	// TunnelClosed indicates the tunnel has been shut down.
	TunnelClosed
)

// String returns the string representation of a TunnelState.
func (s TunnelState) String() string {
	switch s {
	case TunnelRequested:
		return "requested"
	case TunnelAssigned:
		return "assigned"
	case TunnelActive:
		return "active"
	case TunnelClosed:
		return "closed"
	default:
		return "unknown"
	}
}
