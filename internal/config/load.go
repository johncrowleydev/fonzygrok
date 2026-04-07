package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadServerConfig reads a server YAML config file at path.
// Returns a zero-value config (not an error) if the file does not exist.
// Returns a descriptive error if the YAML is malformed.
func LoadServerConfig(path string) (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := loadYAML(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadClientConfig reads a client YAML config file at path.
// Returns a zero-value config (not an error) if the file does not exist.
// Returns a descriptive error if the YAML is malformed.
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := loadYAML(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadYAML reads path, unmarshals YAML into dst.
// Missing file → no error (dst is left as zero-value).
// Malformed YAML → error with line number.
func loadYAML(path string, dst interface{}) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // missing file is not an error
		}
		return fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, dst); err != nil {
		// yaml.v3 includes line/column in its error messages.
		return fmt.Errorf("config: parse %s: %w", path, err)
	}

	return nil
}

// ResolveClientConfigPath returns the config file path to use.
// Priority: explicit > ./fonzygrok.yaml > ~/.fonzygrok.yaml > "".
func ResolveClientConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}

	// Check ./fonzygrok.yaml
	if fileExists("./fonzygrok.yaml") {
		return "./fonzygrok.yaml"
	}

	// Check ~/.fonzygrok.yaml
	home, err := os.UserHomeDir()
	if err == nil {
		p := home + "/.fonzygrok.yaml"
		if fileExists(p) {
			return p
		}
	}

	return ""
}

// MergeClientConfig overlays flag values on top of file values.
// A flag value overrides the file value if the flag is non-zero.
func MergeClientConfig(file *ClientConfig, flags *ClientConfig) *ClientConfig {
	merged := *file

	if flags.Server != "" {
		merged.Server = flags.Server
	}
	if flags.Token != "" {
		merged.Token = flags.Token
	}
	if flags.Port != 0 {
		merged.Port = flags.Port
	}
	if flags.Name != "" {
		merged.Name = flags.Name
	}
	if flags.Insecure {
		merged.Insecure = true
	}
	if flags.Protocol != "" {
		merged.Protocol = flags.Protocol
	}

	return &merged
}

// MergeServerConfig overlays flag values on top of file values.
// A flag value overrides the file value if the flag is non-zero/non-empty.
func MergeServerConfig(file *ServerConfig, flags *ServerConfig) *ServerConfig {
	merged := *file

	if flags.DataDir != "" {
		merged.DataDir = flags.DataDir
	}
	if flags.Domain != "" {
		merged.Domain = flags.Domain
	}
	if flags.SSH.Addr != "" {
		merged.SSH.Addr = flags.SSH.Addr
	}
	if flags.HTTP.Addr != "" {
		merged.HTTP.Addr = flags.HTTP.Addr
	}
	if flags.HTTP.TLS {
		merged.HTTP.TLS = true
	}
	if flags.HTTP.TLSCertDir != "" {
		merged.HTTP.TLSCertDir = flags.HTTP.TLSCertDir
	}
	if flags.Admin.Addr != "" {
		merged.Admin.Addr = flags.Admin.Addr
	}

	return &merged
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
