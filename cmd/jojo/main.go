// Package main implements jojo: A terminal UI for Jujutsu, meant as a replacement for lazygit
package main

import (
	"fmt"
	"os"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version":
			fmt.Printf("jojo %s (commit: %s, built: %s)\n", version, commit, date)
			os.Exit(0)
		case "-h", "--help":
			printHelp()
			os.Exit(0)
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Replace this stub with the real entry point.
	// Consider using internal/ packages for organization.
	fmt.Println("jojo: Not yet implemented")
	return nil
}

func printHelp() {
	fmt.Printf(`jojo - A terminal UI for Jujutsu, meant as a replacement for lazygit

Usage:
  jojo [options]

Options:
  -h, --help     Show this help message
  -v, --version  Show version information
`)
}
