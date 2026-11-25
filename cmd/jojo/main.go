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
	"github.com/kyleking/aragonite/seen"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/icons"
	"github.com/kyleking/aragonite/tui/launch"
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

// configure reads config.toml and turns it into the two things the app is built from: the skin, as
// a picker the terminal's background chooses from when ask says the theme leaves it to the terminal,
// and the keymap. A setting jojo cannot honor stops startup, since a TUI has nowhere to report it
// afterwards.
func configure() (ui.Picker, bool, help.Keymap, config.Config, error) {
	settings, err := config.Load()
	if err != nil {
		return nil, false, help.Keymap{}, settings, fmt.Errorf("loading the config: %w", err)
	}

	flavor, ask, err := ui.Flavor(settings.Theme)
	if err != nil {
		return nil, false, help.Keymap{}, settings, fmt.Errorf("reading theme: %w", err)
	}

	km, err := help.NewKeymap(settings.Keys)
	if err != nil {
		return nil, false, help.Keymap{}, settings, fmt.Errorf("reading keys: %w", err)
	}

	set := icons.Resolve(settings.Icons)
	pick := func(dark bool) ui.Skin {
		skin := flavor(dark)

		return skin.WithIcons(icons.New(set, skin.Palette))
	}

	return pick, ask, km, settings, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// openReads is the read-mark store for the repository at root, shared by every workspace of it and
// with jj-diff, which opens the same directory. A repository whose store or state directory cannot be
// found runs without marks (nil): the screen says so where a mark is asked for.
func openReads(root string) *seen.Store {
	store, err := seen.JJRepo(root)
	if err != nil {
		return nil
	}

	dir, err := seen.StateDir("jojo", "seen", seen.RepoKey(store))
	if err != nil {
		return nil
	}

	return seen.Open(dir)
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

	pick, ask, keys, settings, err := configure()
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

	pressed := loadSeen(rec, keys)

	layout, err := coach.ParseLayout(settings.CoachLayout)
	if err != nil {
		return fmt.Errorf("reading coach_layout: %w", err)
	}

	ledger := openLedger()

	opts := model.Options{
		Keys: keys, Seen: pressed, Usage: rec, DiffTool: settings.DiffTool, OutlineRevset: settings.OutlineRevset,
		StackBookmark: settings.StackBookmark,
		Ledger:        ledger, CoachLayout: layout, NoVim: !settings.VimOn(),
	}
	// The legend names the icon set and why; configure resolved the same set from the same inputs.
	why := icons.Explain(settings.Icons)
	opts.Icons = &why
	if reads := openReads(root); reads != nil {
		opts.Reads = reads
	}

	build := func(dark bool) tea.Model { return model.New(ctx, client, pick(dark), root, *revset, opts) }

	// A theme left to the terminal is asked for inside the program, where its answer cannot eat a key.
	var app tea.Model = launch.New(build)
	if !ask {
		app = build(true)
	}

	// The renderer downsamples to the profile the skin was fitted to, detected the one way, so the two
	// never disagree under tmux with COLORTERM set.
	profile := tea.WithColorProfile(theme.Profile(os.Stdout, os.Environ()))
	if _, err := tea.NewProgram(app, tea.WithContext(ctx), profile).Run(); err != nil {
		return fmt.Errorf("jojo exited: %w", err)
	}

	return saveLedger(ledger)
}

// openLedger is the coach's ledger: local state, always on and holding no keystrokes, which says
// which tips were shown, when each action last ran, and whether the welcome card was dismissed. A
// ledger that cannot be opened is nil: the model keeps one in memory, and the welcome card, which
// asks this one, stays down rather than greeting the reader on every launch.
func openLedger() *coach.Ledger {
	ledger, err := coach.OpenLedger(model.UsageApp)
	if err != nil {
		return nil
	}

	return ledger
}

// saveLedger writes the ledger back, nothing when there is none.
func saveLedger(ledger *coach.Ledger) error {
	if ledger == nil {
		return nil
	}

	if err := ledger.Save(); err != nil {
		return fmt.Errorf("saving the coach's notes: %w", err)
	}

	return nil
}
