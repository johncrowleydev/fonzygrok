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
	"syscall"

	"github.com/fonzygrok/fonzygrok/internal/client"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
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
	)

	cmd := &cobra.Command{
		Use:   "fonzygrok",
		Short: "Expose local services through public tunnel URLs",
		Long: `Fonzygrok is a self-hosted ngrok alternative. It connects to a
fonzygrok server via SSH and creates a public URL that tunnels HTTP
traffic to a local port on your machine.

Examples:
  fonzygrok --server tunnel.example.com:2222 --token fgk_xxx --port 3000
  fonzygrok --server localhost:2222 --token fgk_xxx --port 8080 --insecure
  FONZYGROK_SERVER=tunnel.example.com:2222 FONZYGROK_TOKEN=fgk_xxx fonzygrok --port 3000`,
		Version: Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTunnel(cmd.Context(), serverAddr, token, port, insecure)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Flags with environment variable fallbacks.
	cmd.Flags().StringVar(&serverAddr, "server", "", "Server address (host:port) [$FONZYGROK_SERVER]")
	cmd.Flags().StringVar(&token, "token", "", "API token for authentication [$FONZYGROK_TOKEN]")
	cmd.Flags().IntVar(&port, "port", 0, "Local port to expose (required)")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "Skip host key verification")

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
func runTunnel(parent context.Context, serverAddr, token string, port int, insecure bool) error {
	// Resolve env vars for server + token if flags were empty.
	if serverAddr == "" {
		serverAddr = os.Getenv("FONZYGROK_SERVER")
	}
	if token == "" {
		token = os.Getenv("FONZYGROK_TOKEN")
	}

	// Validate required fields.
	if serverAddr == "" {
		return fmt.Errorf("--server or FONZYGROK_SERVER is required")
	}
	if token == "" {
		return fmt.Errorf("--token or FONZYGROK_TOKEN is required")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}

	// Structured JSON logger per GOV-006 / BLU-001 §7.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
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

	logger.Info("fonzygrok starting",
		slog.String("version", Version),
		slog.String("server", serverAddr),
		slog.Int("local_port", port),
	)

	// ConnectWithRetry handles the full lifecycle:
	// connect → open control → request tunnel → proxy → reconnect on failure.
	err := connector.ConnectWithRetry(ctx, func() error {
		return onConnect(ctx, connector, port, logger)
	})

	if err != nil && err != context.Canceled {
		logger.Error("client exited with error", slog.String("error", err.Error()))
		return err
	}

	logger.Info("fonzygrok shutdown complete")
	return nil
}

// onConnect is called after each successful SSH connection.
// It opens the control channel, requests a tunnel, and starts the proxy.
func onConnect(ctx context.Context, connector *client.Connector, port int, logger *slog.Logger) error {
	// Open control channel.
	cc, err := connector.OpenControl()
	if err != nil {
		return fmt.Errorf("open control channel: %w", err)
	}

	// Request tunnel.
	assignment, err := cc.RequestTunnel(port, "http")
	if err != nil {
		cc.Close()
		return fmt.Errorf("request tunnel: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\n  ✔ Tunnel established!\n")
	fmt.Fprintf(os.Stderr, "  ↳ Public URL: %s\n", assignment.PublicURL)
	fmt.Fprintf(os.Stderr, "  ↳ Forwarding: %s → localhost:%d\n\n", assignment.PublicURL, port)

	logger.Info("tunnel active",
		slog.String("tunnel_id", assignment.TunnelID),
		slog.String("public_url", assignment.PublicURL),
		slog.Int("local_port", port),
	)

	// Start proxy to handle incoming channels.
	proxy := client.NewLocalProxy(port, logger)

	sshClient := connector.SSHClient()
	if sshClient == nil {
		cc.Close()
		return fmt.Errorf("SSH client is nil after connect")
	}

	// HandleChannels blocks until ctx is cancelled or channels close.
	// Run it in a goroutine so we can detect context cancellation here too.
	go proxy.HandleChannels(ctx, sshClient.HandleChannelOpen(client.ChannelTypeProxy))

	// Block until context is done (signal or disconnect detected by caller).
	<-ctx.Done()

	logger.Info("shutting down tunnel",
		slog.String("tunnel_id", assignment.TunnelID),
	)

	// Best-effort close tunnel on the control channel.
	cc.CloseTunnel(assignment.TunnelID)
	cc.Close()

	return nil
}
