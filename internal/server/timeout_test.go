package server

import (
	"log/slog"
	"testing"
	"time"
)

func TestEdgeRouterConfiguresDefaultHTTPServerTimeouts(t *testing.T) {
	edge := NewEdgeRouter(EdgeConfig{Addr: ":0"}, nil, slog.Default())

	if edge.server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", edge.server.ReadHeaderTimeout, 5*time.Second)
	}
	if edge.server.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %v, want disabled timeout", edge.server.ReadTimeout)
	}
	if edge.server.IdleTimeout != 120*time.Second {
		t.Fatalf("IdleTimeout = %v, want %v", edge.server.IdleTimeout, 120*time.Second)
	}
	if edge.server.WriteTimeout != 10*time.Minute {
		t.Fatalf("WriteTimeout = %v, want %v", edge.server.WriteTimeout, 10*time.Minute)
	}
}

func TestEdgeRouterUsesConfiguredHTTPServerTimeouts(t *testing.T) {
	edge := NewEdgeRouter(EdgeConfig{
		Addr:              ":0",
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
	}, nil, slog.Default())

	if edge.server.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", edge.server.ReadHeaderTimeout, 2*time.Second)
	}
	if edge.server.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout = %v, want %v", edge.server.ReadTimeout, 3*time.Second)
	}
	if edge.server.WriteTimeout != 4*time.Second {
		t.Fatalf("WriteTimeout = %v, want %v", edge.server.WriteTimeout, 4*time.Second)
	}
	if edge.server.IdleTimeout != 5*time.Second {
		t.Fatalf("IdleTimeout = %v, want %v", edge.server.IdleTimeout, 5*time.Second)
	}
}

func TestEdgeRedirectServerUsesEdgeTimeouts(t *testing.T) {
	edge := NewEdgeRouter(EdgeConfig{
		Addr:              ":0",
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
	}, nil, slog.Default())

	redirectServer := edge.newRedirectServer(":0", nil)
	if redirectServer.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", redirectServer.ReadHeaderTimeout, 2*time.Second)
	}
	if redirectServer.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout = %v, want %v", redirectServer.ReadTimeout, 3*time.Second)
	}
	if redirectServer.WriteTimeout != 4*time.Second {
		t.Fatalf("WriteTimeout = %v, want %v", redirectServer.WriteTimeout, 4*time.Second)
	}
	if redirectServer.IdleTimeout != 5*time.Second {
		t.Fatalf("IdleTimeout = %v, want %v", redirectServer.IdleTimeout, 5*time.Second)
	}
}

func TestAdminAPIConfiguresDefaultHTTPServerTimeouts(t *testing.T) {
	admin := NewAdminAPI(AdminConfig{Addr: ":0"}, nil, nil, nil, nil, slog.Default())

	if admin.server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", admin.server.ReadHeaderTimeout, 5*time.Second)
	}
	if admin.server.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %v, want %v", admin.server.ReadTimeout, 15*time.Second)
	}
	if admin.server.WriteTimeout != 30*time.Second {
		t.Fatalf("WriteTimeout = %v, want %v", admin.server.WriteTimeout, 30*time.Second)
	}
	if admin.server.IdleTimeout != 120*time.Second {
		t.Fatalf("IdleTimeout = %v, want %v", admin.server.IdleTimeout, 120*time.Second)
	}
}

func TestAdminAPIUsesConfiguredHTTPServerTimeouts(t *testing.T) {
	admin := NewAdminAPI(AdminConfig{
		Addr:              ":0",
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
	}, nil, nil, nil, nil, slog.Default())

	if admin.server.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", admin.server.ReadHeaderTimeout, 2*time.Second)
	}
	if admin.server.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout = %v, want %v", admin.server.ReadTimeout, 3*time.Second)
	}
	if admin.server.WriteTimeout != 4*time.Second {
		t.Fatalf("WriteTimeout = %v, want %v", admin.server.WriteTimeout, 4*time.Second)
	}
	if admin.server.IdleTimeout != 5*time.Second {
		t.Fatalf("IdleTimeout = %v, want %v", admin.server.IdleTimeout, 5*time.Second)
	}
}
