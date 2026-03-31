package proto

import (
	"encoding/json"
	"testing"
)

func TestTunnelRequestRoundTrip(t *testing.T) {
	orig := TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
		Subdomain: "myapp",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal TunnelRequest: %v", err)
	}
	var got TunnelRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal TunnelRequest: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestTunnelRequestOmitEmptySubdomain(t *testing.T) {
	req := TunnelRequest{LocalPort: 8080, Protocol: "http"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := m["subdomain"]; ok {
		t.Error("expected subdomain to be omitted when empty")
	}
}

func TestTunnelAssignmentRoundTrip(t *testing.T) {
	orig := TunnelAssignment{
		TunnelID:          "a3f8x2",
		AssignedSubdomain: "a3f8x2",
		PublicURL:         "http://a3f8x2.tunnel.example.com",
		Protocol:          "http",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal TunnelAssignment: %v", err)
	}
	var got TunnelAssignment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal TunnelAssignment: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestTunnelCloseRoundTrip(t *testing.T) {
	orig := TunnelClose{
		TunnelID: "a3f8x2",
		Reason:   ReasonClientDisconnect,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal TunnelClose: %v", err)
	}
	var got TunnelClose
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal TunnelClose: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestErrorMessageRoundTrip(t *testing.T) {
	orig := ErrorMessage{
		Code:        ErrSubdomainTaken,
		Message:     "The requested subdomain 'myapp' is already in use",
		RequestType: TypeTunnelRequest,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal ErrorMessage: %v", err)
	}
	var got ErrorMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal ErrorMessage: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestControlMessageRoundTrip(t *testing.T) {
	req := TunnelRequest{LocalPort: 3000, Protocol: "http"}
	msg, err := WrapPayload(TypeTunnelRequest, req)
	if err != nil {
		t.Fatalf("WrapPayload: %v", err)
	}
	if msg.Type != TypeTunnelRequest {
		t.Errorf("type: got %q, want %q", msg.Type, TypeTunnelRequest)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal ControlMessage: %v", err)
	}

	var got ControlMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal ControlMessage: %v", err)
	}
	if got.Type != msg.Type {
		t.Errorf("type mismatch: got %q, want %q", got.Type, msg.Type)
	}

	var gotReq TunnelRequest
	if err := json.Unmarshal(got.Payload, &gotReq); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if gotReq != req {
		t.Errorf("payload mismatch: got %+v, want %+v", gotReq, req)
	}
}

func TestTunnelRequestJSONTags(t *testing.T) {
	req := TunnelRequest{LocalPort: 3000, Protocol: "http", Subdomain: "test"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	expected := []string{"local_port", "protocol", "subdomain"}
	for _, key := range expected {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestTunnelAssignmentJSONTags(t *testing.T) {
	a := TunnelAssignment{
		TunnelID:          "abc",
		AssignedSubdomain: "abc",
		PublicURL:         "http://abc.example.com",
		Protocol:          "http",
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	expected := []string{"tunnel_id", "assigned_subdomain", "public_url", "protocol"}
	for _, key := range expected {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}
