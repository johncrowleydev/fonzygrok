// Package main is the entry point for the fonzygrok client binary.
// It connects to a fonzygrok server via SSH and exposes local services
// through public tunnel URLs.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/client"
	"github.com/fonzygrok/fonzygrok/internal/config"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// newRootCmd creates the root cobra command for the fonzygrok CLI.
func newRootCmd() *cobra.Command {
	var (
		serverAddr string
		token      string
		port       int
		insecure   bool
		name       string
		protocol   string
		configPath string
		inspect    string
		noInspect  bool
		verbose    bool
	)

	cmd := &cobra.Command{
		Use:   "fonzygrok",
		Short: "Expose local services through public tunnel URLs",
		Long: `Fonzygrok is a self-hosted ngrok alternative. It connects to a
fonzygrok server via SSH and creates a public URL that tunnels HTTP
or TCP traffic to a local port on your machine.

Examples:
  fonzygrok --port 3000                              # https://<auto>.fonzygrok.com
  fonzygrok --name my-api --port 8080                 # https://my-api.fonzygrok.com
  fonzygrok --name my-db --port 5432 --protocol tcp   # tcp://fonzygrok.com:<port>
  fonzygrok --server self-hosted.dev --port 3000      # custom server`,
		Version: Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve config file: explicit > ./fonzygrok.yaml > ~/.fonzygrok.yaml
			cfgPath := config.ResolveClientConfigPath(configPath)
			fileCfg, err := config.LoadClientConfig(cfgPath)
			if err != nil {
				return err
			}

			// Merge: file values as defaults, flags override.
			flagCfg := &config.ClientConfig{
				Server:   serverAddr,
				Token:    token,
				Port:     port,
				Name:     name,
				Insecure: insecure,
				Protocol: protocol,
			}
			merged := config.MergeClientConfig(fileCfg, flagCfg)

			// Default protocol to "http" if not set.
			if merged.Protocol == "" {
				merged.Protocol = "http"
			}

			return runTunnel(cmd.Context(), merged.Server, merged.Token, merged.Port, merged.Insecure, merged.Name, merged.Protocol, inspect, noInspect, verbose)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Flags with environment variable fallbacks.
	cmd.Flags().StringVar(&serverAddr, "server", "fonzygrok.com", "Server address (host:port) [$FONZYGROK_SERVER]")
	cmd.Flags().StringVar(&token, "token", "", "API token for authentication [$FONZYGROK_TOKEN]")
	cmd.Flags().IntVar(&port, "port", 0, "Local port to expose (required)")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "Skip host key verification")
	cmd.Flags().StringVar(&name, "name", "", "Custom subdomain name for the tunnel URL")
	cmd.Flags().StringVar(&protocol, "protocol", "", `Tunnel protocol: "http" (default) or "tcp"`)
	cmd.Flags().StringVar(&configPath, "config", "", "Path to YAML config file (auto-detects ~/.fonzygrok.yaml)")
	cmd.Flags().StringVar(&inspect, "inspect", "localhost:4040", "Inspector web UI listen address")
	cmd.Flags().BoolVar(&noInspect, "no-inspect", false, "Disable the request inspector")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show JSON structured logs on stdout")

	// Wire up env var defaults. cobra doesn't do this natively.
	if env := os.Getenv("FONZYGROK_SERVER"); env != "" && serverAddr == "" {
		cmd.Flags().Set("server", env)
	}
	if env := os.Getenv("FONZYGROK_TOKEN"); env != "" && token == "" {
		cmd.Flags().Set("token", env)
	}

	cmd.MarkFlagRequired("port")

	return cmd
}

// runTunnel is the core logic: connect, request tunnel, proxy traffic.
func runTunnel(parent context.Context, serverAddr, token string, port int, insecure bool, name string, protocol string, inspectAddr string, noInspect bool, verbose bool) error {
	// Resolve env vars for server + token if flags were empty.
	if serverAddr == "" {
		serverAddr = os.Getenv("FONZYGROK_SERVER")
	}
	if token == "" {
		token = os.Getenv("FONZYGROK_TOKEN")
	}

	if serverAddr == "" {
		serverAddr = "fonzygrok.com"
	}
	if token == "" {
		return fmt.Errorf("--token or FONZYGROK_TOKEN is required")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}

	// Validate protocol.
	switch protocol {
	case "http", "tcp":
		// valid
	default:
		return fmt.Errorf("--protocol must be \"http\" or \"tcp\", got %q", protocol)
	}

	// Append default SSH port if not specified.
	if !strings.Contains(serverAddr, ":") {
		serverAddr = serverAddr + ":2222"
	}

	// Human-friendly output to stderr (per DEF-001 fix).
	display := client.NewDisplay(os.Stderr)

	// Structured JSON logger per GOV-006 / BLU-001 §7.
	// Default: Error level (suppresses JSON noise).
	// --verbose: Debug level (full JSON logs to stdout alongside Display on stderr).
	logLevel := slog.LevelError
	if verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Context with signal handling for graceful shutdown.
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := client.ClientConfig{
		ServerAddr: serverAddr,
		Token:      token,
		Insecure:   insecure,
	}

	connector := client.NewConnector(cfg, logger)

	// Banner + starting message via Display.
	display.Banner(Version)
	display.Connecting(serverAddr)

	logger.Info("fonzygrok starting",
		slog.String("version", Version),
		slog.String("server", serverAddr),
		slog.Int("local_port", port),
	)

	// Start inspector if enabled.
	var inspector *client.Inspector
	var inspectDisplay string
	if !noInspect {
		inspector = client.NewInspector(inspectAddr, logger)
		inspectDisplay = inspectAddr
		go func() {
			if err := inspector.Start(ctx); err != nil {
				logger.Warn("inspector stopped", slog.String("error", err.Error()))
			}
		}()
	}

	// ConnectWithRetry handles the full lifecycle:
	// connect → open control → request tunnel → proxy → reconnect on failure.
	retryStatusShown := false
	err := connector.ConnectWithRetryHooks(ctx, client.ConnectHooks{
		OnConnectionFailed: func(err error, attempt int, backoff time.Duration) {
			display.ConnectionFailed(err, attempt, int(backoff.Seconds()))
			retryStatusShown = true
		},
		OnDisconnected: func(err error, backoff time.Duration) {
			display.Disconnected()
			retryStatusShown = false
		},
		OnRetrying: func(backoff time.Duration) {
			if retryStatusShown {
				retryStatusShown = false
				return
			}
			display.Retrying(int(backoff.Seconds()))
		},
		OnConnected: func(connCtx context.Context) error {
			return onConnect(connCtx, connector, port, name, protocol, inspector, inspectDisplay, display, logger)
		},
	})

	if err != nil && err != context.Canceled {
		display.Error(err.Error())
		logger.Error("client exited with error", slog.String("error", err.Error()))
		return err
	}

	display.Shutdown()
	logger.Info("fonzygrok shutdown complete")
	return nil
}

// onConnect is called after each successful SSH connection.
// It opens the control channel, requests a tunnel, and starts the proxy.
func onConnect(ctx context.Context, connector *client.Connector, port int, name string, protocol string, inspector *client.Inspector, inspectAddr string, display *client.Display, logger *slog.Logger) error {
	display.Connected()

	// Open control channel.
	cc, err := connector.OpenControl()
	if err != nil {
		return fmt.Errorf("open control channel: %w", err)
	}

	// Request tunnel with the specified protocol.
	assignment, err := cc.RequestTunnel(port, protocol, name)
	if err != nil {
		cc.Close()
		return fmt.Errorf("request tunnel: %w", err)
	}

	// Pretty output via Display (to stderr).
	// Use TCP display format when protocol is tcp.
	if protocol == "tcp" {
		// Extract host from server address for TCP URL display.
		serverHost := connector.Host()
		display.TunnelEstablishedTCP(assignment.Name, serverHost, assignment.AssignedPort, port, inspectAddr)
	} else {
		display.TunnelEstablished(assignment.Name, assignment.PublicURL, port, inspectAddr)
	}
	display.Ready()

	logger.Info("tunnel active",
		slog.String("tunnel_id", assignment.TunnelID),
		slog.String("name", assignment.Name),
		slog.String("public_url", assignment.PublicURL),
		slog.String("protocol", protocol),
		slog.Int("local_port", port),
	)

	// Start proxy to handle incoming channels.
	proxy := client.NewLocalProxy(port, logger)
	if inspector != nil {
		proxy.Inspector = inspector
	}

	sshClient := connector.SSHClient()
	if sshClient == nil {
		cc.Close()
		return fmt.Errorf("SSH client is nil after connect")
	}

	// Register handlers for both HTTP and TCP proxy channels.
	// HandleChannels dispatches to the correct handler based on channel type.
	go proxy.HandleChannels(ctx, sshClient.HandleChannelOpen(client.ChannelTypeProxy))
	go proxy.HandleChannels(ctx, sshClient.HandleChannelOpen(client.ChannelTypeTCPProxy))

	go func() {
		<-ctx.Done()

		logger.Info("shutting down tunnel",
			slog.String("tunnel_id", assignment.TunnelID),
		)

		// Best-effort close tunnel on the control channel.
		cc.CloseTunnel(assignment.TunnelID)
		cc.Close()
	}()

	return nil
}
