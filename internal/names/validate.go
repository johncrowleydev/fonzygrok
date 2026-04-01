package names

import (
	"fmt"
	"regexp"
)

// namePattern matches valid subdomain names: lowercase alphanumeric + hyphens,
// must start and end with alphanumeric, 3-32 characters total.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`)

// reservedNames is the set of names that cannot be used as tunnel subdomains.
var reservedNames = map[string]bool{
	"www":    true,
	"api":    true,
	"admin":  true,
	"app":    true,
	"mail":   true,
	"ftp":    true,
	"ssh":    true,
	"dns":    true,
	"ns1":    true,
	"ns2":    true,
	"smtp":   true,
	"imap":   true,
	"pop":    true,
	"cdn":    true,
	"static": true,
	"assets": true,
	"docs":   true,
	"blog":   true,
	"status": true,
	"health": true,
	"tunnel": true,
}

// Validate checks whether a user-provided name is valid for use as a tunnel subdomain.
// Rules:
//   - 3–32 characters
//   - Lowercase alphanumeric and hyphens only
//   - Must start and end with alphanumeric (no leading/trailing hyphens)
//   - Must not be in the reserved names list
func Validate(name string) error {
	if len(name) < 3 {
		return fmt.Errorf("name %q is too short (minimum 3 characters)", name)
	}
	if len(name) > 32 {
		return fmt.Errorf("name %q is too long (maximum 32 characters)", name)
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name %q must be lowercase alphanumeric and hyphens, starting and ending with alphanumeric", name)
	}
	if IsReserved(name) {
		return fmt.Errorf("name %q is reserved", name)
	}
	return nil
}

// IsReserved reports whether the given name is in the reserved names list.
func IsReserved(name string) bool {
	return reservedNames[name]
}
