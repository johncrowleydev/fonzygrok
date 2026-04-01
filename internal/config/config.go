// Package config handles YAML configuration file loading for the
// fonzygrok client and server binaries. Configuration precedence:
// file < environment variables < CLI flags.
package config

// ServerConfig holds all server configuration values.
// Populated from YAML config files, with CLI flags overlaid on top.
type ServerConfig struct {
	DataDir string       `yaml:"data_dir"`
	Domain  string       `yaml:"domain"`
	SSH     SSHSection   `yaml:"ssh"`
	HTTP    HTTPSection  `yaml:"http"`
	Admin   AdminSection `yaml:"admin"`
}

// SSHSection holds SSH listener config.
type SSHSection struct {
	Addr string `yaml:"addr"`
}

// HTTPSection holds HTTP edge listener config.
type HTTPSection struct {
	Addr       string `yaml:"addr"`
	TLS        bool   `yaml:"tls"`
	TLSCertDir string `yaml:"tls_cert_dir"`
}

// AdminSection holds admin API listener config.
type AdminSection struct {
	Addr string `yaml:"addr"`
}

// ClientConfig holds all client configuration values.
// Populated from YAML config files, with CLI flags overlaid on top.
type ClientConfig struct {
	Server   string `yaml:"server"`
	Token    string `yaml:"token"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	Insecure bool   `yaml:"insecure"`
}
