package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/proto"
)

func TestControlChannelAccept(t *testing.T) {
	srv, _, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	// Open control channel.
	ch, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open control channel: %v", err)
	}
	ch.Close()
}

func TestControlChannelRejectDuplicate(t *testing.T) {
	srv, _, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	// First control channel should succeed.
	ch1, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open first control channel: %v", err)
	}
	defer ch1.Close()

	// Brief wait for server to register the control channel.
	time.Sleep(50 * time.Millisecond)

	// Second control channel should be rejected.
	_, _, err = client.OpenChannel("control", nil)
	if err == nil {
		t.Fatal("expected second control channel to be rejected")
	}
}

func TestControlChannelUnknownType(t *testing.T) {
	srv, _, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	// Unknown channel type should be rejected.
	_, _, err := client.OpenChannel("unknown-type", nil)
	if err == nil {
		t.Fatal("expected unknown channel type to be rejected")
	}
}

func TestTunnelRequestViaControl(t *testing.T) {
	srv, tm, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	// Open control channel.
	ch, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open control channel: %v", err)
	}
	defer ch.Close()

	// Send tunnel request.
	req := proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	}
	msg, err := proto.WrapPayload(proto.TypeTunnelRequest, req)
	if err != nil {
		t.Fatalf("wrap payload: %v", err)
	}

	encoder := proto.NewEncoder(ch)
	if err := encoder.Encode(msg); err != nil {
		t.Fatalf("encode tunnel request: %v", err)
	}

	// Read response.
	decoder := proto.NewDecoder(ch)
	resp, err := decoder.Decode()
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Type != proto.TypeTunnelAssignment {
		t.Fatalf("expected tunnel_assignment, got %q", resp.Type)
	}

	var assignment proto.TunnelAssignment
	if err := json.Unmarshal(resp.Payload, &assignment); err != nil {
		t.Fatalf("unmarshal assignment: %v", err)
	}

	if assignment.TunnelID == "" {
		t.Error("expected non-empty tunnel ID")
	}
	if assignment.Protocol != "http" {
		t.Errorf("protocol: got %q, want %q", assignment.Protocol, "http")
	}
	if assignment.AssignedSubdomain == "" {
		t.Error("expected non-empty subdomain")
	}
	if assignment.PublicURL == "" {
		t.Error("expected non-empty public URL")
	}

	// Verify tunnel is registered in tunnel manager.
	entry, ok := tm.Lookup(assignment.TunnelID)
	if !ok {
		t.Fatal("tunnel not found in tunnel manager")
	}
	if entry.LocalPort != 3000 {
		t.Errorf("local port: got %d, want %d", entry.LocalPort, 3000)
	}
}

func TestTunnelCloseViaControl(t *testing.T) {
	srv, tm, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	ch, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open control channel: %v", err)
	}
	defer ch.Close()

	encoder := proto.NewEncoder(ch)
	decoder := proto.NewDecoder(ch)

	// Register a tunnel first.
	req, _ := proto.WrapPayload(proto.TypeTunnelRequest, proto.TunnelRequest{
		LocalPort: 8080,
		Protocol:  "http",
	})
	encoder.Encode(req)
	resp, _ := decoder.Decode()

	var assignment proto.TunnelAssignment
	json.Unmarshal(resp.Payload, &assignment)

	// Now close the tunnel.
	closeMsg, _ := proto.WrapPayload(proto.TypeTunnelClose, proto.TunnelClose{
		TunnelID: assignment.TunnelID,
		Reason:   proto.ReasonClientDisconnect,
	})
	if err := encoder.Encode(closeMsg); err != nil {
		t.Fatalf("encode tunnel close: %v", err)
	}

	// Brief wait for server to process.
	time.Sleep(50 * time.Millisecond)

	_, ok := tm.Lookup(assignment.TunnelID)
	if ok {
		t.Error("tunnel should be deregistered after close")
	}
}

func TestUnknownMessageType(t *testing.T) {
	srv, _, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	ch, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open control channel: %v", err)
	}
	defer ch.Close()

	encoder := proto.NewEncoder(ch)
	decoder := proto.NewDecoder(ch)

	// Send unknown message type.
	msg, _ := proto.WrapPayload("unknown_type", map[string]string{"foo": "bar"})
	if err := encoder.Encode(msg); err != nil {
		t.Fatalf("encode unknown message: %v", err)
	}

	// Should receive error response.
	resp, err := decoder.Decode()
	if err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if resp.Type != proto.TypeError {
		t.Fatalf("expected error type, got %q", resp.Type)
	}

	var errMsg proto.ErrorMessage
	if err := json.Unmarshal(resp.Payload, &errMsg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errMsg.Code != proto.ErrInternalError {
		t.Errorf("error code: got %q, want %q", errMsg.Code, proto.ErrInternalError)
	}
}

func TestDispatchMultipleRequests(t *testing.T) {
	srv, tm, st, addr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	ch, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open control channel: %v", err)
	}
	defer ch.Close()

	encoder := proto.NewEncoder(ch)
	decoder := proto.NewDecoder(ch)

	// Request multiple tunnels.
	for i := 0; i < 3; i++ {
		msg, _ := proto.WrapPayload(proto.TypeTunnelRequest, proto.TunnelRequest{
			LocalPort: 3000 + i,
			Protocol:  "http",
		})
		if err := encoder.Encode(msg); err != nil {
			t.Fatalf("encode request %d: %v", i, err)
		}

		resp, err := decoder.Decode()
		if err != nil {
			t.Fatalf("decode response %d: %v", i, err)
		}
		if resp.Type != proto.TypeTunnelAssignment {
			t.Fatalf("request %d: expected tunnel_assignment, got %q", i, resp.Type)
		}
	}

	active := tm.ListActive()
	if len(active) != 3 {
		t.Errorf("expected 3 active tunnels, got %d", len(active))
	}
}
