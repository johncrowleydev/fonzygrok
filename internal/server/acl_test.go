package server

import (
	"net"
	"testing"
)

func TestACL_AllowMode(t *testing.T) {
	acl, err := ParseACL("allow", []string{"192.168.1.0/24", "10.0.0.5/32"})
	if err != nil {
		t.Fatalf("ParseACL: %v", err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.1", true},
		{"192.168.1.254", true},
		{"10.0.0.5", true},
		{"10.0.0.6", false},
		{"172.16.0.1", false},
		{"8.8.8.8", false},
	}

	for _, tt := range tests {
		got := acl.Check(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("Allow mode: Check(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestACL_DenyMode(t *testing.T) {
	acl, err := ParseACL("deny", []string{"192.168.1.0/24", "10.0.0.0/8"})
	if err != nil {
		t.Fatalf("ParseACL: %v", err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.1", false},
		{"192.168.1.254", false},
		{"10.0.0.5", false},
		{"10.255.255.255", false},
		{"172.16.0.1", true},
		{"8.8.8.8", true},
	}

	for _, tt := range tests {
		got := acl.Check(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("Deny mode: Check(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestACL_CIDRRanges(t *testing.T) {
	tests := []struct {
		name  string
		cidrs []string
		ip    string
		want  bool
	}{
		{"/32 exact match", []string{"10.0.0.5/32"}, "10.0.0.5", true},
		{"/32 no match", []string{"10.0.0.5/32"}, "10.0.0.6", false},
		{"/24 in range", []string{"10.0.0.0/24"}, "10.0.0.100", true},
		{"/24 out of range", []string{"10.0.0.0/24"}, "10.0.1.1", false},
		{"/16 in range", []string{"10.0.0.0/16"}, "10.0.255.1", true},
		{"/16 out of range", []string{"10.0.0.0/16"}, "10.1.0.1", false},
		{"/8 in range", []string{"10.0.0.0/8"}, "10.255.255.255", true},
		{"/0 match all", []string{"0.0.0.0/0"}, "203.0.113.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acl, err := ParseACL("allow", tt.cidrs)
			if err != nil {
				t.Fatalf("ParseACL: %v", err)
			}
			got := acl.Check(net.ParseIP(tt.ip))
			if got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestACL_EmptyAllowsAll(t *testing.T) {
	// Allow mode with empty list = allow all.
	acl1, _ := ParseACL("allow", nil)
	if !acl1.Check(net.ParseIP("203.0.113.1")) {
		t.Error("empty allow list should allow all")
	}

	// Deny mode with empty list = allow all.
	acl2, _ := ParseACL("deny", nil)
	if !acl2.Check(net.ParseIP("203.0.113.1")) {
		t.Error("empty deny list should allow all")
	}

	// Nil ACL should allow all.
	var acl3 *ACL
	if !acl3.Check(net.ParseIP("203.0.113.1")) {
		t.Error("nil ACL should allow all")
	}
}

func TestACL_InvalidCIDR(t *testing.T) {
	_, err := ParseACL("allow", []string{"not-a-cidr"})
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}

	_, err = ParseACL("allow", []string{"192.168.1.0/24", "bad"})
	if err == nil {
		t.Fatal("expected error when one CIDR is invalid")
	}
}

func TestACL_InvalidMode(t *testing.T) {
	_, err := ParseACL("block", []string{"10.0.0.0/8"})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestACL_ToConfig(t *testing.T) {
	acl, _ := ParseACL("allow", []string{"10.0.0.0/8", "192.168.1.0/24"})
	cfg := acl.ToConfig()

	if cfg.Mode != "allow" {
		t.Errorf("mode: got %q, want %q", cfg.Mode, "allow")
	}
	if len(cfg.CIDRs) != 2 {
		t.Errorf("expected 2 CIDRs, got %d", len(cfg.CIDRs))
	}
}

func TestACL_NilToConfig(t *testing.T) {
	var acl *ACL
	cfg := acl.ToConfig()
	if cfg != nil {
		t.Error("nil ACL ToConfig should return nil")
	}
}
