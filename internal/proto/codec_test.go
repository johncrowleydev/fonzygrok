package proto

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	req := TunnelRequest{LocalPort: 3000, Protocol: "http"}
	msg, err := WrapPayload(TypeTunnelRequest, req)
	if err != nil {
		t.Fatalf("WrapPayload: %v", err)
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(msg); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	dec := NewDecoder(&buf)
	got, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Type != TypeTunnelRequest {
		t.Errorf("type: got %q, want %q", got.Type, TypeTunnelRequest)
	}

	var gotReq TunnelRequest
	if err := json.Unmarshal(got.Payload, &gotReq); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if gotReq != req {
		t.Errorf("payload: got %+v, want %+v", gotReq, req)
	}
}

func TestMultipleMessages(t *testing.T) {
	messages := []struct {
		msgType string
		payload interface{}
	}{
		{TypeTunnelRequest, TunnelRequest{LocalPort: 3000, Protocol: "http"}},
		{TypeTunnelAssignment, TunnelAssignment{
			TunnelID:          "a3f8x2",
			AssignedSubdomain: "a3f8x2",
			PublicURL:         "http://a3f8x2.tunnel.example.com",
			Protocol:          "http",
		}},
		{TypeTunnelClose, TunnelClose{TunnelID: "a3f8x2", Reason: ReasonClientDisconnect}},
		{TypeError, ErrorMessage{
			Code:        ErrSubdomainTaken,
			Message:     "subdomain taken",
			RequestType: TypeTunnelRequest,
		}},
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for _, m := range messages {
		msg, err := WrapPayload(m.msgType, m.payload)
		if err != nil {
			t.Fatalf("WrapPayload(%s): %v", m.msgType, err)
		}
		if err := enc.Encode(msg); err != nil {
			t.Fatalf("Encode(%s): %v", m.msgType, err)
		}
	}

	dec := NewDecoder(&buf)
	for i, m := range messages {
		got, err := dec.Decode()
		if err != nil {
			t.Fatalf("Decode[%d]: %v", i, err)
		}
		if got.Type != m.msgType {
			t.Errorf("msg[%d] type: got %q, want %q", i, got.Type, m.msgType)
		}
	}

	// Should return EOF after all messages
	_, err := dec.Decode()
	if err != io.EOF {
		t.Errorf("expected io.EOF after all messages, got %v", err)
	}
}

func TestDecodeEOFOnEmpty(t *testing.T) {
	dec := NewDecoder(bytes.NewReader(nil))
	_, err := dec.Decode()
	if err != io.EOF {
		t.Errorf("expected io.EOF on empty reader, got %v", err)
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	dec := NewDecoder(bytes.NewReader([]byte("not json\n")))
	_, err := dec.Decode()
	if err == nil {
		t.Error("expected error on invalid JSON, got nil")
	}
}

func TestEncodeNewlineDelimited(t *testing.T) {
	req := TunnelRequest{LocalPort: 8080, Protocol: "http"}
	msg, _ := WrapPayload(TypeTunnelRequest, req)

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(msg); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	if len(data) == 0 {
		t.Fatal("encoded data is empty")
	}
	if data[len(data)-1] != '\n' {
		t.Error("encoded message does not end with newline")
	}
	// Should be valid JSON without the trailing newline
	data = data[:len(data)-1]
	var msg2 ControlMessage
	if err := json.Unmarshal(data, &msg2); err != nil {
		t.Errorf("data without newline is not valid JSON: %v", err)
	}
}
