package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadClientConfigValid verifies loading a well-formed client YAML.
func TestLoadClientConfigValid(t *testing.T) {
	content := `
server: fonzygrok.com:2222
token: fgk_xxxx
port: 3000
name: my-api
insecure: true
`
	path := writeTemp(t, "client.yaml", content)

	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig() error: %v", err)
	}

	if cfg.Server != "fonzygrok.com:2222" {
		t.Errorf("Server = %q, want %q", cfg.Server, "fonzygrok.com:2222")
	}
	if cfg.Token != "fgk_xxxx" {
		t.Errorf("Token = %q, want %q", cfg.Token, "fgk_xxxx")
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want %d", cfg.Port, 3000)
	}
	if cfg.Name != "my-api" {
		t.Errorf("Name = %q, want %q", cfg.Name, "my-api")
	}
	if !cfg.Insecure {
		t.Error("Insecure = false, want true")
	}
}

// TestLoadServerConfigValid verifies loading a well-formed server YAML.
func TestLoadServerConfigValid(t *testing.T) {
	content := `
data_dir: /data
domain: tunnel.fonzygrok.com
ssh:
  addr: ":2222"
http:
  addr: ":8080"
  tls: true
  tls_cert_dir: /data/certs
admin:
  addr: "127.0.0.1:9090"
`
	path := writeTemp(t, "server.yaml", content)

	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig() error: %v", err)
	}

	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/data")
	}
	if cfg.Domain != "tunnel.fonzygrok.com" {
		t.Errorf("Domain = %q, want %q", cfg.Domain, "tunnel.fonzygrok.com")
	}
	if cfg.SSH.Addr != ":2222" {
		t.Errorf("SSH.Addr = %q, want %q", cfg.SSH.Addr, ":2222")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want %q", cfg.HTTP.Addr, ":8080")
	}
	if !cfg.HTTP.TLS {
		t.Error("HTTP.TLS = false, want true")
	}
	if cfg.HTTP.TLSCertDir != "/data/certs" {
		t.Errorf("HTTP.TLSCertDir = %q, want %q", cfg.HTTP.TLSCertDir, "/data/certs")
	}
	if cfg.Admin.Addr != "127.0.0.1:9090" {
		t.Errorf("Admin.Addr = %q, want %q", cfg.Admin.Addr, "127.0.0.1:9090")
	}
}

// TestLoadClientConfigInvalidYAML verifies that malformed YAML produces
// a clear error message.
func TestLoadClientConfigInvalidYAML(t *testing.T) {
	content := `
server: fonzygrok.com:2222
token: [unclosed bracket
port: "not a number
`
	path := writeTemp(t, "bad.yaml", content)

	_, err := LoadClientConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "config: parse") {
		t.Errorf("error should contain 'config: parse', got: %v", err)
	}
	t.Logf("expected error: %v", err)
}

// TestLoadClientConfigMissingFile verifies that a missing file is not an error.
func TestLoadClientConfigMissingFile(t *testing.T) {
	cfg, err := LoadClientConfig("/nonexistent/path/client.yaml")
	if err != nil {
		t.Fatalf("LoadClientConfig() should not error on missing file, got: %v", err)
	}

	// Config should be zero-value.
	if cfg.Server != "" {
		t.Errorf("Server should be empty, got %q", cfg.Server)
	}
	if cfg.Port != 0 {
		t.Errorf("Port should be 0, got %d", cfg.Port)
	}
}

// TestLoadClientConfigEmptyPath verifies that an empty path returns zero config.
func TestLoadClientConfigEmptyPath(t *testing.T) {
	cfg, err := LoadClientConfig("")
	if err != nil {
		t.Fatalf("LoadClientConfig(\"\") error: %v", err)
	}
	if cfg.Server != "" || cfg.Port != 0 {
		t.Error("empty path should return zero-value config")
	}
}

// TestLoadServerConfigMissingFile verifies graceful handling of missing server config.
func TestLoadServerConfigMissingFile(t *testing.T) {
	cfg, err := LoadServerConfig("/nonexistent/path/server.yaml")
	if err != nil {
		t.Fatalf("LoadServerConfig() should not error on missing file, got: %v", err)
	}
	if cfg.DataDir != "" {
		t.Errorf("DataDir should be empty, got %q", cfg.DataDir)
	}
}

// TestMergeClientConfigFlagsOverrideFile verifies that non-zero flag values
// override file values.
func TestMergeClientConfigFlagsOverrideFile(t *testing.T) {
	file := &ClientConfig{
		Server:   "from-file.com:2222",
		Token:    "fgk_from_file",
		Port:     3000,
		Name:     "file-name",
		Insecure: false,
	}
	flags := &ClientConfig{
		Server: "from-flag.com:2222",
		Port:   8080,
		// Token, Name, Insecure left as zero — should NOT override.
	}

	merged := MergeClientConfig(file, flags)

	if merged.Server != "from-flag.com:2222" {
		t.Errorf("Server = %q, want %q (flag should override)", merged.Server, "from-flag.com:2222")
	}
	if merged.Token != "fgk_from_file" {
		t.Errorf("Token = %q, want %q (file should remain)", merged.Token, "fgk_from_file")
	}
	if merged.Port != 8080 {
		t.Errorf("Port = %d, want %d (flag should override)", merged.Port, 8080)
	}
	if merged.Name != "file-name" {
		t.Errorf("Name = %q, want %q (file should remain)", merged.Name, "file-name")
	}
	if merged.Insecure {
		t.Error("Insecure = true, want false (zero flag should not override)")
	}
}

// TestMergeClientConfigEmptyFileAllFlags verifies that flags work without any file config.
func TestMergeClientConfigEmptyFileAllFlags(t *testing.T) {
	file := &ClientConfig{}
	flags := &ClientConfig{
		Server:   "flag-server:2222",
		Token:    "fgk_flag",
		Port:     5000,
		Name:     "flag-name",
		Insecure: true,
	}

	merged := MergeClientConfig(file, flags)

	if merged.Server != "flag-server:2222" {
		t.Errorf("Server = %q, want %q", merged.Server, "flag-server:2222")
	}
	if merged.Port != 5000 {
		t.Errorf("Port = %d, want %d", merged.Port, 5000)
	}
	if !merged.Insecure {
		t.Error("Insecure = false, want true")
	}
}

// TestMergeServerConfigFlagsOverrideFile verifies server merge logic.
func TestMergeServerConfigFlagsOverrideFile(t *testing.T) {
	file := &ServerConfig{
		DataDir: "/data",
		Domain:  "tunnel.file.com",
		SSH:     SSHSection{Addr: ":2222"},
		HTTP:    HTTPSection{Addr: ":8080", TLS: false, TLSCertDir: "/data/certs"},
		Admin:   AdminSection{Addr: "127.0.0.1:9090"},
	}
	flags := &ServerConfig{
		Domain: "tunnel.flag.com",
		SSH:    SSHSection{Addr: ":3333"},
		HTTP:   HTTPSection{TLS: true},
	}

	merged := MergeServerConfig(file, flags)

	if merged.DataDir != "/data" {
		t.Errorf("DataDir = %q, want %q (file should remain)", merged.DataDir, "/data")
	}
	if merged.Domain != "tunnel.flag.com" {
		t.Errorf("Domain = %q, want %q (flag should override)", merged.Domain, "tunnel.flag.com")
	}
	if merged.SSH.Addr != ":3333" {
		t.Errorf("SSH.Addr = %q, want %q (flag should override)", merged.SSH.Addr, ":3333")
	}
	if merged.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want %q (file should remain)", merged.HTTP.Addr, ":8080")
	}
	if !merged.HTTP.TLS {
		t.Error("HTTP.TLS = false, want true (flag should override)")
	}
	if merged.HTTP.TLSCertDir != "/data/certs" {
		t.Errorf("HTTP.TLSCertDir = %q, want %q (file should remain)", merged.HTTP.TLSCertDir, "/data/certs")
	}
	if merged.Admin.Addr != "127.0.0.1:9090" {
		t.Errorf("Admin.Addr = %q, want %q (file should remain)", merged.Admin.Addr, "127.0.0.1:9090")
	}
}

// TestResolveClientConfigPathExplicit verifies explicit path wins.
func TestResolveClientConfigPathExplicit(t *testing.T) {
	got := ResolveClientConfigPath("/some/explicit.yaml")
	if got != "/some/explicit.yaml" {
		t.Errorf("ResolveClientConfigPath = %q, want %q", got, "/some/explicit.yaml")
	}
}

// TestResolveClientConfigPathLocalFile verifies ./fonzygrok.yaml detection.
func TestResolveClientConfigPathLocalFile(t *testing.T) {
	// Create a temp dir and cd into it.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Create ./fonzygrok.yaml
	os.WriteFile("fonzygrok.yaml", []byte("server: test\n"), 0o644)

	got := ResolveClientConfigPath("")
	if got != "./fonzygrok.yaml" {
		t.Errorf("ResolveClientConfigPath = %q, want %q", got, "./fonzygrok.yaml")
	}
}

// TestResolveClientConfigPathNone verifies that no config file returns empty.
func TestResolveClientConfigPathNone(t *testing.T) {
	// Run from a temp dir with no config files.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := ResolveClientConfigPath("")
	if got != "" {
		t.Errorf("ResolveClientConfigPath = %q, want empty", got)
	}
}

// writeTemp creates a temp file with the given content and returns its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
