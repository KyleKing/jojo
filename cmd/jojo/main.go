// Package main implements jojo: A terminal UI for Jujutsu, meant as a replacement for lazygit
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/model"
	"github.com/kyleking/jojo/internal/ui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	repo := flag.String("repo", ".", "repository to browse")
	revset := flag.String("r", "", "revset to log, defaulting to the repository's revsets.log")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("jojo %s (commit: %s, built: %s)\n", version, commit, date)

		return nil
	}

	ctx := context.Background()

	client := jj.NewClient(*repo)
	if err := client.CheckInstalled(ctx); err != nil {
		return fmt.Errorf("jj is not usable: %w", err)
	}

	if _, err := client.Root(ctx); err != nil {
		return fmt.Errorf("%s is not a jj repository: %w", *repo, err)
	}

	if *revset == "" {
		configured, err := client.DefaultRevset(ctx)
		if err != nil {
			return fmt.Errorf("reading revsets.log: %w", err)
		}

		*revset = configured
	}

	app := model.New(ctx, client, ui.Detected(), *repo, *revset)
	if _, err := tea.NewProgram(app, tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("jojo exited: %w", err)
	}

	return nil
}
