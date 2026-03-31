// Package main is the entry point for the fonzygrok-server binary.
// It provides the CLI for starting the tunnel server and managing tokens.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fonzygrok/fonzygrok/internal/server"
	"github.com/fonzygrok/fonzygrok/internal/store"
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Set the package-level version for server info / health endpoints.
	server.Version = version

	rootCmd := &cobra.Command{
		Use:     "fonzygrok-server",
		Short:   "Fonzygrok tunnel server",
		Version: version,
	}

	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(tokenCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// serveCmd starts all server subsystems.
func serveCmd() *cobra.Command {
	var (
		sshAddr   string
		httpAddr  string
		adminAddr string
		dataDir   string
		domain    string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the fonzygrok tunnel server",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			config := server.ServerConfig{
				DataDir: dataDir,
				Domain:  domain,
				SSH: server.SSHConfig{
					Addr: sshAddr,
				},
				Edge: server.EdgeConfig{
					Addr: httpAddr,
				},
				Admin: server.AdminConfig{
					Addr: adminAddr,
				},
			}

			srv, err := server.NewServer(config, logger)
			if err != nil {
				logger.Error("failed to create server", "error", err)
				return err
			}

			// Signal handling: SIGINT/SIGTERM → graceful shutdown.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				sig := <-sigCh
				logger.Info("received signal, shutting down", "signal", sig.String())
				cancel()
			}()

			return srv.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&sshAddr, "ssh-addr", ":2222", "SSH listen address")
	cmd.Flags().StringVar(&httpAddr, "http-addr", ":8080", "HTTP edge listen address")
	cmd.Flags().StringVar(&adminAddr, "admin-addr", "127.0.0.1:9090", "Admin API listen address")
	cmd.Flags().StringVar(&dataDir, "data-dir", "./data", "Data directory for database and host key")
	cmd.Flags().StringVar(&domain, "domain", "tunnel.localhost", "Base domain for tunnel routing")

	return cmd
}

// tokenCmd manages authentication tokens.
func tokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage authentication tokens",
	}

	cmd.AddCommand(tokenCreateCmd())
	cmd.AddCommand(tokenListCmd())
	cmd.AddCommand(tokenRevokeCmd())

	return cmd
}

func tokenCreateCmd() *cobra.Command {
	var name string
	var dataDir string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore(dataDir)
			if err != nil {
				return err
			}
			defer st.Close()

			tok, rawToken, err := st.CreateToken(name)
			if err != nil {
				return fmt.Errorf("create token: %w", err)
			}

			fmt.Printf("Token created successfully.\n\n")
			fmt.Printf("  ID:    %s\n", tok.ID)
			fmt.Printf("  Name:  %s\n", tok.Name)
			fmt.Printf("  Token: %s\n\n", rawToken)
			fmt.Printf("⚠️  Save this token now — it cannot be retrieved again.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Token name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&dataDir, "data-dir", "./data", "Data directory")

	return cmd
}

func tokenListCmd() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore(dataDir)
			if err != nil {
				return err
			}
			defer st.Close()

			tokens, err := st.ListTokens()
			if err != nil {
				return fmt.Errorf("list tokens: %w", err)
			}

			if len(tokens) == 0 {
				fmt.Println("No tokens found.")
				return nil
			}

			fmt.Printf("%-20s %-20s %-8s %-25s %-25s\n", "ID", "NAME", "ACTIVE", "CREATED", "LAST USED")
			fmt.Printf("%-20s %-20s %-8s %-25s %-25s\n", "----", "----", "------", "-------", "---------")
			for _, tok := range tokens {
				active := "yes"
				if !tok.IsActive {
					active = "no"
				}
				lastUsed := "never"
				if tok.LastUsedAt != nil {
					lastUsed = tok.LastUsedAt.Format("2006-01-02 15:04:05")
				}
				fmt.Printf("%-20s %-20s %-8s %-25s %-25s\n",
					tok.ID, tok.Name, active,
					tok.CreatedAt.Format("2006-01-02 15:04:05"),
					lastUsed,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "./data", "Data directory")
	return cmd
}

func tokenRevokeCmd() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke a token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenID := args[0]

			st, err := openStore(dataDir)
			if err != nil {
				return err
			}
			defer st.Close()

			if err := st.DeleteToken(tokenID); err != nil {
				return fmt.Errorf("revoke token: %w", err)
			}

			fmt.Printf("Token %s revoked.\n", tokenID)
			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "./data", "Data directory")
	return cmd
}

// openStore opens the database for CLI token commands.
func openStore(dataDir string) (*store.Store, error) {
	dbPath := dataDir + "/fonzygrok.db"

	// Ensure data dir exists.
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	st, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := st.Migrate(); err != nil {
		st.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return st, nil
}
