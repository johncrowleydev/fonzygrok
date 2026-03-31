// Package main is the entry point for the fonzygrok-server binary.
// It starts the tunnel server which accepts SSH connections from clients
// and routes public HTTP traffic through established tunnels.
package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	fmt.Printf("fonzygrok-server %s\n", Version)
	os.Exit(0)
}
