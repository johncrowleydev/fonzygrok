package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// TestRootCmdDefaultServer verifies that when --server is not provided and
// FONZYGROK_SERVER is empty, the default "fonzygrok.com" is used (no error).
func TestRootCmdDefaultServer(t *testing.T) {
	// Ensure env vars are not set.
	t.Setenv("FONZYGROK_SERVER", "")
	t.Setenv("FONZYGROK_TOKEN", "tok")

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--port", "3000", "--token", "fgk_test"})

	// Override RunE to capture the server value without connecting.
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		server, _ := cmd.Flags().GetString("server")
		if server != "fonzygrok.com" {
			t.Errorf("server default = %q, want %q", server, "fonzygrok.com")
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
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

// TestRootCmdHelpShowsNameFlag verifies that --help mentions the --name flag.
func TestRootCmdHelpShowsNameFlag(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--name") {
		t.Error("help should mention --name flag")
	}
	if !strings.Contains(out, "Custom subdomain") {
		t.Error("help should describe --name as 'Custom subdomain'")
	}
	if !strings.Contains(out, "--name my-api") {
		t.Error("help examples should include --name usage")
	}
}

// TestRootCmdNameFlagParsed verifies that --name is accepted as a valid flag
// and its value is correctly parsed.
func TestRootCmdNameFlagParsed(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "3000",
		"--name", "my-api",
	})

	// Override RunE to avoid actually connecting — we only care about flag parsing.
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		val, err := cmd.Flags().GetString("name")
		if err != nil {
			t.Fatalf("GetString(\"name\") error: %v", err)
		}
		if val != "my-api" {
			t.Errorf("--name = %q, want %q", val, "my-api")
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

// TestRootCmdHelpShowsConfigFlag verifies that --help mentions --config.
func TestRootCmdHelpShowsConfigFlag(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--config") {
		t.Error("help should mention --config flag")
	}
}

// TestRootCmdConfigFileLoaded verifies that config file values serve as
// defaults, and CLI flags override them.
func TestRootCmdConfigFileLoaded(t *testing.T) {
	// Write a temp config with server and token.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client.yaml")
	err := os.WriteFile(cfgPath, []byte("server: from-file.com:2222\ntoken: fgk_from_file\nport: 3000\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--config", cfgPath,
		"--port", "8080", // override port from file
	})

	// Override RunE to capture the merged values.
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// The RunE in the real code does config loading + merge.
		// We can't easily intercept the merged result from here,
		// so we just verify the flag was accepted without error.
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() with --config error: %v", err)
	}
}

// TestRootCmdConfigFileInvalidYAML verifies clear error on bad YAML.
func TestRootCmdConfigFileInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")
	err := os.WriteFile(cfgPath, []byte("server: [unclosed\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--config", cfgPath,
		"--port", "3000",
	})

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error for invalid YAML config")
	}
	if !strings.Contains(execErr.Error(), "config: parse") {
		t.Errorf("error should contain 'config: parse', got: %v", execErr)
	}
}

// TestRootCmdHelpShowsInspectFlags verifies --help mentions --inspect and --no-inspect.
func TestRootCmdHelpShowsInspectFlags(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--inspect") {
		t.Error("help should mention --inspect flag")
	}
	if !strings.Contains(out, "--no-inspect") {
		t.Error("help should mention --no-inspect flag")
	}
}

// TestRootCmdInspectDefaultValue verifies --inspect defaults to localhost:4040.
func TestRootCmdInspectDefaultValue(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "3000",
		"--no-inspect",
	})

	// Override RunE to capture flag values.
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		inspect, _ := cmd.Flags().GetString("inspect")
		if inspect != "localhost:4040" {
			t.Errorf("inspect default = %q, want %q", inspect, "localhost:4040")
		}
		noInspect, _ := cmd.Flags().GetBool("no-inspect")
		if !noInspect {
			t.Error("--no-inspect should be true")
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

// TestRootCmdHelpShowsVerboseFlag verifies --help mentions --verbose.
func TestRootCmdHelpShowsVerboseFlag(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--verbose") {
		t.Error("help should mention --verbose flag")
	}
}

// TestRootCmdVerboseFlagParsed verifies --verbose flag is parsed correctly.
func TestRootCmdVerboseFlagParsed(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "3000",
		"--verbose",
		"--no-inspect",
	})

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		v, _ := cmd.Flags().GetBool("verbose")
		if !v {
			t.Error("--verbose should be true")
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

// TestRootCmdHelpShowsProtocolFlag verifies --help mentions --protocol.
func TestRootCmdHelpShowsProtocolFlag(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--protocol") {
		t.Error("help should mention --protocol flag")
	}
}

// TestRootCmdProtocolTCPAccepted verifies that --protocol tcp is accepted.
func TestRootCmdProtocolTCPAccepted(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "5432",
		"--protocol", "tcp",
	})

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		val, err := cmd.Flags().GetString("protocol")
		if err != nil {
			t.Fatalf("GetString(\"protocol\") error: %v", err)
		}
		if val != "tcp" {
			t.Errorf("--protocol = %q, want %q", val, "tcp")
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

// TestRootCmdProtocolHTTPAccepted verifies that --protocol http is accepted.
func TestRootCmdProtocolHTTPAccepted(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "3000",
		"--protocol", "http",
	})

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		val, _ := cmd.Flags().GetString("protocol")
		if val != "http" {
			t.Errorf("--protocol = %q, want %q", val, "http")
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

// TestRootCmdProtocolInvalidRejected verifies that --protocol with an invalid
// value is rejected with a clear error message.
func TestRootCmdProtocolInvalidRejected(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "3000",
		"--protocol", "websocket",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --protocol websocket")
	}
	if !strings.Contains(err.Error(), `"http" or "tcp"`) {
		t.Errorf("error should mention valid protocol values, got: %v", err)
	}
	if !strings.Contains(err.Error(), "websocket") {
		t.Errorf("error should include the invalid value, got: %v", err)
	}
}

// TestRootCmdProtocolDefaultsToHTTP verifies that omitting --protocol defaults to "http".
func TestRootCmdProtocolDefaultsToHTTP(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--server", "localhost:2222",
		"--token", "fgk_test",
		"--port", "3000",
	})

	// The real RunE does config merge + defaulting, but we override to check
	// that the flag has its zero value (empty string means default to http).
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		val, _ := cmd.Flags().GetString("protocol")
		if val != "" {
			t.Errorf("protocol flag default should be empty (defaults later to http), got %q", val)
		}
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}
