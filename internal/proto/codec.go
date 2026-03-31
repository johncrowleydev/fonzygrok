package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Encoder writes newline-delimited JSON messages to an io.Writer.
// Each message is a single JSON object followed by a newline character.
type Encoder struct {
	w io.Writer
}

// NewEncoder creates a new Encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode marshals msg to JSON and writes it followed by a newline.
// Returns an error if marshaling or writing fails.
func (e *Encoder) Encode(msg *ControlMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("proto: marshal message: %w", err)
	}
	data = append(data, '\n')
	_, err = e.w.Write(data)
	if err != nil {
		return fmt.Errorf("proto: write message: %w", err)
	}
	return nil
}

// Decoder reads newline-delimited JSON messages from an io.Reader.
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder creates a new Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{scanner: bufio.NewScanner(r)}
}

// Decode reads the next newline-delimited JSON message.
// Returns io.EOF when there are no more messages.
func (d *Decoder) Decode() (*ControlMessage, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return nil, fmt.Errorf("proto: read message: %w", err)
		}
		return nil, io.EOF
	}
	var msg ControlMessage
	if err := json.Unmarshal(d.scanner.Bytes(), &msg); err != nil {
		return nil, fmt.Errorf("proto: unmarshal message: %w", err)
	}
	return &msg, nil
}
