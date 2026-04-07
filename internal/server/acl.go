package server

import (
	"fmt"
	"net"
)

// ACL provides per-tunnel IP access control. It supports two modes:
//   - "allow" (whitelist): only IPs matching AllowCIDRs are permitted.
//   - "deny" (blacklist): IPs matching DenyCIDRs are blocked.
//
// An empty CIDR list means "allow all" regardless of mode.
type ACL struct {
	AllowCIDRs []net.IPNet
	DenyCIDRs  []net.IPNet
	Mode       string // "allow" or "deny"
}

// Check returns true if the given IP is allowed by this ACL.
//
//   - allow mode: IP must match at least one AllowCIDR. Empty list = allow all.
//   - deny mode: IP must NOT match any DenyCIDR. Empty list = allow all.
//   - nil ACL always allows (callers should nil-check before calling).
func (a *ACL) Check(ip net.IP) bool {
	if a == nil {
		return true
	}

	switch a.Mode {
	case "allow":
		if len(a.AllowCIDRs) == 0 {
			return true
		}
		for _, cidr := range a.AllowCIDRs {
			if cidr.Contains(ip) {
				return true
			}
		}
		return false

	case "deny":
		if len(a.DenyCIDRs) == 0 {
			return true
		}
		for _, cidr := range a.DenyCIDRs {
			if cidr.Contains(ip) {
				return false
			}
		}
		return true

	default:
		// Unknown mode defaults to allow.
		return true
	}
}

// ParseACL creates an ACL from string CIDR notation.
// Returns an error if any CIDR string is invalid.
func ParseACL(mode string, cidrs []string) (*ACL, error) {
	if mode != "allow" && mode != "deny" {
		return nil, fmt.Errorf("invalid ACL mode: %q (must be \"allow\" or \"deny\")", mode)
	}

	acl := &ACL{Mode: mode}

	for _, cidrStr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidrStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidrStr, err)
		}

		switch mode {
		case "allow":
			acl.AllowCIDRs = append(acl.AllowCIDRs, *ipNet)
		case "deny":
			acl.DenyCIDRs = append(acl.DenyCIDRs, *ipNet)
		}
	}

	return acl, nil
}

// ACLConfig is the JSON representation for API requests/responses.
type ACLConfig struct {
	Mode  string   `json:"mode"`
	CIDRs []string `json:"cidrs"`
}

// ToConfig converts an ACL to its API representation.
func (a *ACL) ToConfig() *ACLConfig {
	if a == nil {
		return nil
	}
	cfg := &ACLConfig{Mode: a.Mode}
	switch a.Mode {
	case "allow":
		for _, cidr := range a.AllowCIDRs {
			cfg.CIDRs = append(cfg.CIDRs, cidr.String())
		}
	case "deny":
		for _, cidr := range a.DenyCIDRs {
			cfg.CIDRs = append(cfg.CIDRs, cidr.String())
		}
	}
	return cfg
}
