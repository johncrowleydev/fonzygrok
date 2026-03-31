// Package main is the entry point for the fonzygrok client binary.
// It connects to a fonzygrok server via SSH and exposes local services
// through public tunnel URLs.
package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	fmt.Printf("fonzygrok %s\n", Version)
	os.Exit(0)
}
