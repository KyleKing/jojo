// Package main implements jojo: A terminal UI for Jujutsu, meant as a replacement for lazygit
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/config"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/model"
	"github.com/kyleking/jojo/internal/ui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// probe runs the two startup queries at once. Each jj invocation costs roughly what the first frame
// costs to paint, so running them in series is the whole of a visible delay before anything appears.
func probe(ctx context.Context, client *jj.Client, wantRevset bool) (string, string, error) {
	var (
		wait          sync.WaitGroup
		root, revset  string
		rootErr, rErr error
	)

	wait.Add(1)

	go func() {
		defer wait.Done()

		root, rootErr = client.Root(ctx)
	}()

	if wantRevset {
		wait.Add(1)

		go func() {
			defer wait.Done()

			revset, rErr = client.DefaultRevset(ctx)
		}()
	}

	wait.Wait()

	if rootErr != nil {
		return "", "", fmt.Errorf("not a jj repository: %w", rootErr)
	}

	if rErr != nil {
		return "", "", fmt.Errorf("reading revsets.log: %w", rErr)
	}

	return root, revset, nil
}

// configure reads config.toml and turns it into the three things the app is built from. A setting
// jojo cannot honor stops startup, since a TUI has nowhere to report it afterwards.
func configure() (config.Config, ui.Skin, help.Keymap, error) {
	settings, err := config.Load()
	if err != nil {
		return config.Config{}, ui.Skin{}, help.Keymap{}, fmt.Errorf("loading the config: %w", err)
	}

	skin, err := ui.Flavor(settings.Theme)
	if err != nil {
		return config.Config{}, ui.Skin{}, help.Keymap{}, fmt.Errorf("reading theme: %w", err)
	}

	keys, err := help.NewKeymap(settings.Keys)
	if err != nil {
		return config.Config{}, ui.Skin{}, help.Keymap{}, fmt.Errorf("reading keys: %w", err)
	}

	return settings, skin, keys, nil
}

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

	settings, skin, keys, err := configure()
	if err != nil {
		return err
	}

	ctx := context.Background()

	client := jj.NewClient(*repo)
	if err := client.CheckInstalled(); err != nil {
		return fmt.Errorf("jj is not usable: %w", err)
	}

	root, configured, err := probe(ctx, client, *revset == "")
	if err != nil {
		return err
	}

	if *revset == "" {
		*revset = configured
	}

	app := model.New(ctx, client, skin, root, *revset,
		model.Options{Keys: keys, Revsets: settings.Revsets})
	if _, err := tea.NewProgram(app, tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("jojo exited: %w", err)
	}

	return nil
}
