// Package main implements jojo: A terminal UI for Jujutsu, meant as a replacement for lazygit
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/keyusage"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/icons"
	"github.com/kyleking/aragonite/tui/theme"

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

	errUnknownCommand = errors.New("unknown command, the only one is keys")
	errFilteredClone  = errors.New("jj cannot read this repository's objects")
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

// configure reads config.toml and turns it into the two things the app is built from. A setting jojo
// cannot honor stops startup, since a TUI has nowhere to report it afterwards.
func configure() (ui.Skin, help.Keymap, config.Config, error) {
	settings, err := config.Load()
	if err != nil {
		return ui.Skin{}, help.Keymap{}, settings, fmt.Errorf("loading the config: %w", err)
	}

	skin, err := ui.Flavor(settings.Theme)
	if err != nil {
		return ui.Skin{}, help.Keymap{}, settings, fmt.Errorf("reading theme: %w", err)
	}

	skin.Icons = icons.New(icons.Resolve(settings.Icons), skin.Palette)

	keys, err := help.NewKeymap(settings.Keys)
	if err != nil {
		return ui.Skin{}, help.Keymap{}, settings, fmt.Errorf("reading keys: %w", err)
	}

	return skin, keys, settings, nil
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

	rec := keyusage.Open(model.UsageApp)
	defer rec.Close() //nolint:errcheck // a log that fails to close has nothing left to report

	rec.Invoke(cliRoot(flag.CommandLine), os.Args[1:])

	if *showVersion {
		fmt.Printf("jojo %s (commit: %s, built: %s)\n", version, commit, date)

		return nil
	}

	skin, keys, settings, err := configure()
	if err != nil {
		return err
	}

	switch flag.Arg(0) {
	case "":
	case keysCommand:
		return keysReport(os.Stdout, keys)
	default:
		return fmt.Errorf("%w: %q", errUnknownCommand, flag.Arg(0))
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

	if why := jj.PromisorProblem(root); why != "" {
		return fmt.Errorf("%w: %s; unfilter the clone: git -C %s config --unset "+
			"remote.origin.partialclonefilter && git config --unset remote.origin.promisor "+
			"&& git fetch --refetch", errFilteredClone, why, root)
	}

	if *revset == "" {
		*revset = configured
	}

	seen := loadSeen(rec, keys)

	layout, err := coach.ParseLayout(settings.CoachLayout)
	if err != nil {
		return fmt.Errorf("reading coach_layout: %w", err)
	}

	// The coach's ledger is local state, always on and holding no keystrokes: which tips were shown
	// and when each action last ran. A ledger that cannot be read starts fresh in memory.
	ledger, err := coach.OpenLedger(model.UsageApp)
	if err != nil {
		ledger, _ = coach.LedgerAt("") //nolint:errcheck // an in-memory ledger reads no file and cannot fail
	}

	app := model.New(ctx, client, skin, root, *revset,
		model.Options{
			Keys: keys, Seen: seen, Usage: rec, DiffTool: settings.DiffTool, OutlineRevset: settings.OutlineRevset,
			Ledger: ledger, CoachLayout: layout,
		})
	// The renderer downsamples to the profile the skin was fitted to, detected the one way, so the two
	// never disagree under tmux with COLORTERM set.
	profile := tea.WithColorProfile(theme.Profile(os.Stdout, os.Environ()))
	if _, err := tea.NewProgram(app, tea.WithContext(ctx), profile).Run(); err != nil {
		return fmt.Errorf("jojo exited: %w", err)
	}

	if err := ledger.Save(); err != nil {
		return fmt.Errorf("saving the coach's notes: %w", err)
	}

	return nil
}
