package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRootCmdVersion verifies that --version prints the version string.
func TestRootCmdVersion(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--version failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, Version) {
		t.Errorf("--version output should contain %q, got: %q", Version, out)
	}
}

// TestRootCmdHelp verifies that --help produces usage text.
func TestRootCmdHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--server") {
		t.Error("help should mention --server flag")
	}
	if !strings.Contains(out, "--token") {
		t.Error("help should mention --token flag")
	}
	if !strings.Contains(out, "--port") {
		t.Error("help should mention --port flag")
	}
	if !strings.Contains(out, "--insecure") {
		t.Error("help should mention --insecure flag")
	}
	if !strings.Contains(out, "Examples:") {
		t.Error("help should contain usage examples")
	}
}

// TestRootCmdMissingPort verifies that missing --port flag produces an error.
func TestRootCmdMissingPort(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--server", "localhost:2222", "--token", "fgk_test"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --port flag")
	}
}

// TestRootCmdInvalidPort verifies that an invalid port number is rejected.
func TestRootCmdInvalidPort(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--server", "localhost:2222", "--token", "fgk_test", "--port", "0"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for port=0")
	}
}

// TestRootCmdMissingServer verifies that missing --server (and no env var) errors.
func TestRootCmdMissingServer(t *testing.T) {
	// Ensure env vars are not set.
	t.Setenv("FONZYGROK_SERVER", "")
	t.Setenv("FONZYGROK_TOKEN", "tok")

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--port", "3000", "--token", "fgk_test"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --server")
	}
}

// TestRootCmdMissingToken verifies that missing --token (and no env var) errors.
func TestRootCmdMissingToken(t *testing.T) {
	t.Setenv("FONZYGROK_SERVER", "localhost:2222")
	t.Setenv("FONZYGROK_TOKEN", "")

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--port", "3000", "--server", "localhost:2222"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --token")
	}
}
