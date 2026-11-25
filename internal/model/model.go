// Package model is jojo's root Bubble Tea model: it owns the panes, routes every
// key, and is the only place that calls jj.
package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/forge/github"
	"github.com/kyleking/aragonite/keyusage"
	"github.com/kyleking/aragonite/trail"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/icons"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/launch"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/palette"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/detailview"
	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/components/forgepane"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/components/outlineview"
	"github.com/kyleking/jojo/internal/components/planview"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/components/revsetbox"
	"github.com/kyleking/jojo/internal/components/roundsview"
	"github.com/kyleking/jojo/internal/components/search"
	"github.com/kyleking/jojo/internal/components/statusbar"
	"github.com/kyleking/jojo/internal/components/textview"
	"github.com/kyleking/jojo/internal/components/trailview"
	"github.com/kyleking/jojo/internal/components/workspaceview"
	jforge "github.com/kyleking/jojo/internal/forge"
	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/jjdiff"
	"github.com/kyleking/jojo/internal/readmark"
	"github.com/kyleking/jojo/internal/ui"
)

// logLimit caps one query, because a revset over the whole repository costs a diff stat per row.
// The cap doubles each time the cursor reaches the last loaded row, so a long history pages in
// rather than cutting off at the first window.
const logLimit = 500

// messageLife is how long a "done, press u to undo" line stays up.
const messageLife = 6 * time.Second

// besideMargin is the blank column between the log and a diff docked beside it.
const besideMargin = 1

// footerRows is the height the footer takes out of the log.
const footerRows = 1

// Keys the root routes before any pane sees them.
const (
	keyTab   = "tab"
	keyEsc   = "esc"
	keyEnter = "enter"
	keyQuit  = "ctrl+c"
	keyEnd   = "end"
)

type logLoadedMsg struct {
	err    error
	revset string
	rows   []jj.LogRow
}

type messageMsg struct {
	text string
	// output is everything a failed command wrote. The trail keeps it whole for gm; set, it marks
	// the message as a failure, which the footer offers gm for.
	output string
	// wrote marks a write that went through, which ends the batch any checks were made for.
	wrote bool
}

type clearMessageMsg struct{}

// Model is the application.
type Model struct {
	// ctx is the program lifetime, so quitting cancels a jj call already running.
	ctx      context.Context //nolint:containedctx // the model owns every jj call it starts.
	skin     ui.Skin
	repoPath string
	status   statusbar.Model
	client   *jj.Client
	// matcher turns key presses into actions, holding a prefix (g, b, ], [, \, -) while it waits.
	// It is a pointer, so the copies Update makes of the model share the one waiting prefix.
	matcher *keys.Matcher
	// tick schedules the debounces and the message timeout: tea.Tick in the program, and whatever a
	// test passes as Options.Ticker, so a test can wait on what a timer delivers rather than on time.
	tick Ticker
	// pal is the palette, `:` to search and run and `?` to browse and explain, and coach what it
	// suggests next and teaches; trail is every jj call the client made. All three are pointers, so
	// the copies Update makes of the model share them.
	pal   *palette.Model
	coach *coach.Coach[logState]
	// icons is Options.Icons: the icon set and why, said under the legend while it is open.
	icons *icons.Resolution
	// welcome is the first-run card, a pointer so the copies Update makes share whether it is up.
	welcome    *coach.Welcome
	trail      *trail.Trail
	about      map[string]palette.About
	trailView  trailview.Model
	revsetBox  revsetbox.Model
	forge      forgepane.Model
	prompt     prompt.Model
	destPicker destpicker.Model
	opLog      oplogview.Model
	spaces     workspaceview.Model
	roundsBox  roundsview.Model
	find       search.Model
	bookmarks  bookmarkview.Model
	detail     detailview.Model
	files      fileview.Model
	diff       diffpane.Model
	dock       diffpane.Model
	confirm    confirm.Model
	output     textview.Model
	forcible   forcible
	pushing    pendingPush
	// asking is the footer's open question, nil when none waits on a key.
	asking *ask
	// restoring is the operation a restore confirmation is standing over.
	restoring jj.Operation
	onSubmit  submitted
	onEditor  submitted
	onPick    picked
	keys      help.Keymap
	usage     *keyusage.Recorder
	revset    string
	// startRevset is the revset the log opened on, which the revset row only spends a line on once
	// something else is asked for.
	startRevset string
	message     string
	// jjDiff is what the probe found about jj-diff, the zero Tool until it answers, and jjDiffProbe
	// the probe, which a test replaces.
	jjDiff      jjdiff.Tool
	jjDiffProbe func(context.Context) jjdiff.Tool
	// diffPreference is config.toml's diff_tool, and diffTool what it resolved to.
	diffPreference string
	diffTool       string
	// splitKnown is every change the log held before a split that wrote, until the commits it made
	// have each been asked for a description; nil otherwise.
	splitKnown map[string]bool
	// typeahead holds the keys typed while a prompt is being read (opening), which it is then given.
	typeahead []tea.KeyPressMsg
	// early holds the keys pressed before the first log read landed, failed ones included, which act
	// once it has: until then there is no row for a j to move to or an e to edit. A log that failed
	// still answers the keys that report it or quit.
	early launch.Keys
	// converging is the divergent change the converge dialog is standing over, "" for all of them.
	converging string
	lastOp     string
	rows       []jj.LogRow
	// stats is every commit's size read so far, and statsAsked the commits a read is out for. Both
	// are maps, so the copies Update makes of the model share them.
	stats      statsCache
	statsAsked map[string]bool
	// versions is every change evolution read this session, keyed by the commit it was read at, and
	// interdiffs every round's diff, keyed by its two commits. Neither can go stale: a commit id
	// names its content.
	versions   map[string][]jj.Version
	interdiffs map[string]string
	// dockCache is every whole diff the dock has read, keyed by commit id, so a revisited revision
	// draws without asking jj again. A map, so the copies Update makes share it.
	dockCache map[string]dockLoadedMsg
	// strips is the read strip of each change whose whole diff the dock has read, and reads the seam
	// that says which hunks were read.
	strips map[string]readmark.Strip
	reads  ReadState
	// scopes names each change's full id, which marks are filed under.
	scopes *scopeBook
	marks  readsState
	// outlineTemplate is config.toml's outline_revset, with {rev} for the revision under the cursor.
	outlineTemplate string
	// outline is the stack outline, which takes the log's place while it is active, and rounds the
	// docked diff stepping through one change's versions.
	outline outlineState
	rounds  roundsState
	// submit is a stack submit's plan, which takes the outline's place while it is active.
	submit submitState
	// synced is the last sync's report while its restack is still on offer, which the coach reads.
	synced syncReport
	// last is the write the repeat key runs again.
	last repeatable
	// jumps is the jump list ctrl+o and ctrl+i walk, and labels a label jump waiting for its label.
	jumps  jumpList
	labels labeling
	// history is the log narrowed to one file's history (*), and landOn the change the cursor goes to
	// once the next log read lands, which entering and leaving a history set.
	history historyState
	landOn  string
	// diffRevision is the revision the full-screen diff shows, "" for a comparison or an operation,
	// and dockShown the one the dock last drew: * reads a file in either.
	diffRevision string
	dockShown    string
	log          logview.Model
	// pick is the in-log destination picker, and travel the log as it stood at an earlier operation.
	// Each takes the keys while it is active.
	travel travelState
	pick   pickState
	focus  focus
	// pickSession numbers picker openings, so a sandbox or a preview arriving for one already closed
	// is dropped.
	pickSession int
	width       int
	height      int
	logWindow   int
	// dockSeq numbers the docked diff's reads, so only the newest one lands.
	dockSeq int
	// bandShown is how many rows the band took when the layout was last sized.
	bandShown int
	// promptShown is how many rows the docked prompt took when the layout was last sized.
	promptShown int
	// logBox is the log's width and height as last sized, which an empty log's card fills.
	logBox [2]int
	docked bool
	// failing says the message on screen is the last failure, so the footer can offer its full text.
	failing   bool
	showForge bool
	// band draws the coach's next steps as a labeled band over the footer rather than in the
	// inspector line, which config.toml's coach_layout or :next sets.
	band bool
	// revsetLoaded is whether a log read has landed, so an empty log before it is not a revset that
	// matched nothing.
	revsetLoaded bool
	logFull      bool
	quitting     bool
	// opening says a prompt is being read and will open: keys go to typeahead until it does.
	opening bool
	// revsetShown is whether the layout last gave the revset row its line.
	revsetShown bool
}

// Ticker schedules fn to run after d, the way tea.Tick does.
type Ticker func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd

// after is d's tick through the model's Ticker.
func (m Model) after(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	if m.tick == nil {
		return tea.Tick(d, fn)
	}

	return m.tick(d, fn)
}

// Options are the settings a user states in config.toml, which New folds into the model rather than
// reading itself, so a test can pass its own without a file on disk.
type Options struct {
	// Seen is what earlier sessions logged, nil when there is none, and brightens the keys not pressed lately.
	Seen *keyusage.Usage
	// Usage records this session's keys, nil when recording is off.
	Usage *keyusage.Recorder
	// Icons is which icon set the file list draws and why, for the legend's last row to say; nil
	// leaves the row as the footer.
	Icons *icons.Resolution
	// Reads answers which hunks have been read, nil for none: the seam aragonite/seen plugs into.
	Reads ReadState
	// DiffTool is config.toml's diff_tool: "" or "auto", "none", or a tool name.
	DiffTool string
	// OutlineRevset is config.toml's outline_revset, "" for DefaultOutlineRevset.
	OutlineRevset string
	// JJDiff answers what the jj-diff on PATH can do, nil for jjdiff.Probe.
	JJDiff func(context.Context) jjdiff.Tool
	// Ticker replaces tea.Tick for the model's own timers, nil for tea.Tick.
	Ticker Ticker
	// Ledger is where the coach remembers what it has shown and what was run, across sessions, and
	// whether the welcome card was dismissed; nil
	// keeps it in memory for this session alone.
	Ledger *coach.Ledger
	// Trail is where every jj call is recorded, nil for a new one.
	Trail *trail.Trail
	// StackBookmark is config.toml's stack_bookmark: the name a submit gives a change with no
	// bookmark, {change} and {subject} filled in; "" for stack.DefaultTemplate.
	StackBookmark string
	// GH configures the gh a submit runs, which a test points at a fake forge.
	GH   []gh.Option
	Keys help.Keymap
	// CoachLayout is config.toml's coach_layout: inline (the default) or a band over the footer.
	CoachLayout coach.Layout
}

// New builds the root model against a repository jj already recognizes.
func New(ctx context.Context, client *jj.Client, skin ui.Skin, repoPath, revset string, opts Options) Model {
	km := opts.Keys
	if opts.Seen != nil {
		seen, now := *opts.Seen, time.Now()
		km = km.WithLevel(func(key string) int { return seen.Level(ctxLog, key, now) })
	}

	reads := opts.Reads
	if reads == nil {
		reads = noReads{}
	}

	files := fileview.New(skin)
	files.SetHints(fileHints(km))

	diff := diffpane.New(skin)
	diff.SetHints(diffHints(km, false))

	log := opts.Trail
	if log == nil {
		log = trail.New(0)
	}

	client.WithTrail(log)

	probe := opts.JJDiff
	if probe == nil {
		probe = cachedJJDiffProbe()
	}

	// The forge pane's polls run through the same gh as a submit, so gm lists them beside it.
	ghCLI := gh.New(repoPath, log, append([]gh.Option{gh.WithRepo(ghRepo(client))}, opts.GH...)...)
	pollCtx := github.WithRunner(ctx, ghCLI.Poll)

	m := Model{
		ctx:         ctx,
		jjDiffProbe: probe,
		client:      client,
		skin:        skin,
		repoPath:    repoPath,
		log:         logview.New(logview.StylesFrom(skin)),
		revsetBox:   revsetbox.New(revset, revsetbox.WithAliases(aliasSource(client))),
		forge:       forgepane.New(pollCtx, forgeStyles(skin), repoPath, "", jforge.WithFocused(false)),
		prompt:      prompt.New(skin),
		destPicker:  destpicker.New(skin),
		opLog:       oplogview.New(skin),
		spaces:      workspaceview.New(skin),
		roundsBox:   roundsview.New(skin),
		outline:     outlineState{view: outlineview.New(skin)},
		submit: submitState{
			view: planview.New(skin), gh: ghCLI, prs: map[string]*gh.PR{}, prAt: map[string]time.Time{},
			template: opts.StackBookmark, draft: true,
		},
		rounds:     roundsState{at: -1},
		versions:   map[string][]jj.Version{},
		interdiffs: map[string]string{},
		dockCache:  map[string]dockLoadedMsg{},
		strips:     map[string]readmark.Strip{},
		reads:      reads,
		scopes:     newScopeBook(reads),
		marks:      readsState{live: map[string]liveHunks{}},

		outlineTemplate: opts.OutlineRevset,
		find:            search.New(skin),
		bookmarks:       bookmarkview.New(skin),
		detail:          detailview.New(skin),
		files:           files,
		diff:            diff,
		dock:            diffpane.NewDocked(skin),
		confirm:         confirm.New(skin),
		output:          textview.New(skin),
		trailView:       trailview.New(skin),
		pal:             palette.New(skin.Skin),
		trail:           log,
		about:           help.About(km),
		band:            opts.CoachLayout == coach.Band,
		icons:           opts.Icons,
		status:          statusbar.New(skin),
		keys:            km,
		matcher:         keys.NewMatcher(km.Keys()),
		usage:           opts.Usage,
		tick:            opts.Ticker,
		revset:          revset,
		startRevset:     revset,
		logWindow:       logLimit,
		stats:           statsCache{},
		statsAsked:      map[string]bool{},

		diffPreference: opts.DiffTool,
	}

	m.coach = newCoach(km, opts.Ledger)
	// The card asks the ledger the caller passed, not the coach's in-memory stand-in for none, which
	// is fresh on every launch and would greet the reader every time.
	m.welcome = newWelcome(km)
	m.welcome.OpenIfDue(opts.Ledger)
	m.submit.ghMissing = m.submit.gh.Installed()
	m.outline.view.SetEmptyHints(km.Hints(keys.ActionBack, help.ActionRevset))

	return m
}

// cachedJJDiffProbe is the jj-diff probe remembered under the user's cache directory, which asks
// the binary only when it changed since the last launch.
func cachedJJDiffProbe() func(context.Context) jjdiff.Tool {
	cache, err := os.UserCacheDir()
	if err == nil {
		cache = filepath.Join(cache, "jojo")
	}

	return func(ctx context.Context) jjdiff.Tool {
		return jjdiff.ProbeCached(ctx, exec.LookPath, jjdiff.Exec, cache)
	}
}

// Init loads the log, so the first frame is an empty log rather than a blocked terminal. A [keys]
// table written in names the keymap no longer uses still works, and says so once in the footer.
func (m Model) Init() tea.Cmd {
	var warn tea.Cmd
	if w := m.keys.Warnings(); len(w) > 0 {
		warn = m.say(strings.Join(w, "; "))
	}

	// The op the footer names is read once the log lands (applyLog), not here as well.
	return tea.Batch(m.loadLog(m.revset), m.forge.Init(), m.loadDiffTool(), m.probeJJDiff("", ""), warn)
}

func (m Model) loadLog(revset string) tea.Cmd {
	client, ctx, limit := m.client, m.ctx, m.logWindow

	return func() tea.Msg {
		rows, err := client.Log(ctx, revset, limit)

		return logLoadedMsg{revset: revset, rows: rows, err: err}
	}
}

// fullMargins gives a resized terminal back its whole height to scroll in, then repaints. The
// renderer confines a scroll with DECSTBM and restores the margins by number, as the height it last
// drew at. A frame flushed just before the resize reaches the terminal after it, so a window that
// grew keeps the old height as its bottom margin, and every newline the repaint writes at that row
// scrolls the page up instead of moving down: the log sits rows above the revset line until
// something redraws it. The margins are reset without numbers, which means the whole screen at any
// size, around a saved cursor since DECSTBM homes it, and the repaint follows because the one the
// resize asked for may already have drawn inside the stale margins.
func fullMargins() tea.Cmd {
	return tea.Sequence(
		tea.Raw(ansi.SaveCursor+ansi.SetTopBottomMargins(0, 0)+ansi.RestoreCursor),
		tea.ClearScreen,
	)
}

// Update routes messages, then asks for the colors of any diff the update drew plain, so no path
// that re-draws a diff (a new revision, a resize, a layout toggle) has to remember to.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if colored, ok := msg.(diffpane.ColoredMsg); ok {
		m.dock.Colored(colored)
		m.diff.Colored(colored)

		return m, nil
	}

	next, cmd := m.route(msg)

	after, ok := next.(Model)
	if !ok {
		return next, cmd
	}

	// Focusing or leaving the revset box, or a log read under another revset, adds or drops the
	// revset row, which every pane under it is sized around; the coach's band grows and shrinks with
	// what it has to say, and the docked prompt with the lines typed into it.
	if after.revsetShown != after.revsetRow() || after.bandShown != len(after.bandLines()) ||
		after.promptShown != after.prompt.Rows(after.height) {
		after.resize()
	}

	if colorize := tea.Batch(after.dock.Colorize(), after.diff.Colorize()); colorize != nil {
		return after, tea.Batch(cmd, colorize)
	}

	return after, cmd
}

// route sends a message to whatever handles it. Keys go to handleKeyPress, the one place a binding is
// added.
func (m Model) route(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok && m.pal.IsOpen() {
		return m.paletteKey(paste)
	}

	// Method expressions rather than method values: a method value binds a copy of the model, and
	// building thirteen of them per message was a quarter of every keypress.
	for _, handle := range []func(Model, tea.Msg) (tea.Model, tea.Cmd, bool){
		Model.handleModeMsg, Model.handleSubmitMsg, Model.handleOverlayMsg, Model.handleFindMsg, Model.discoverMsg,
		Model.trailMsg, Model.handleBookmarkMsg, Model.bookmarkMenuMsg, Model.syncMsg, Model.edgeMsgs,
		Model.handleExternalMsg, Model.handleReadMsg, Model.handleHistoryMsg, Model.flowMsg,
	} {
		if next, cmd, handled := handle(m, msg); handled {
			return next, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.diff.SetWidth(m.width)
		m.resize()

		return m, tea.Batch(m.loadStats(), fullMargins())
	case revsetbox.ChangedMsg:
		// A revset typed in a file's history replaces it, so Esc has nothing left to return to.
		m.history = historyState{}

		return m, m.loadLog(msg.Revset)
	case logview.SelectedMsg:
		cmd := tea.Batch(m.followBookmark(msg.Revision), m.followDock(m.dockTarget()), m.loadStats())

		return m, cmd
	case logLoadedMsg:
		return m.landLog(msg)
	case statsMsg:
		return m.applyStats(msg)
	case staleMsg:
		cmd := m.openStale(msg.err)

		return m, cmd
	case divergentPreviewMsg:
		return m.offerDivergent(msg)
	case messageMsg, clearMessageMsg:
		return m.showMessage(msg)
	case headOpMsg, diffToolMsg:
		return m.noteState(msg), nil
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	return m.routeToPanes(msg)
}

// flowMsg is what follows the verbs that hand off or leave work undone: the terminal coming back into
// focus, a push jj refused, and a split that made commits to describe.
func (m Model) flowMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.FocusMsg:
		return m, m.refreshOnFocus(), true
	case pushRefusedMsg:
		m.message, m.failing = m.landOnRefusal(msg.err), true

		return m, tea.Batch(m.after(messageLife, func(time.Time) tea.Msg { return clearMessageMsg{} }),
			m.followDock(m.dockTarget())), true
	case splitWroteMsg:
		m.splitKnown = msg.known
		next, cmd := m.showMessage(msg.done)

		return next, cmd, true
	}

	return m, nil, false
}

// refreshOnFocus reads the log again when the terminal comes back into focus, which is the log read
// that snapshots the working copy: an edit saved in another window while jojo sat behind it shows
// at once. A screen mid-question (a prompt, the picker, the past) is left as it is, since a new log
// would move what it is asking about.
func (m Model) refreshOnFocus() tea.Cmd {
	if m.openModal() != "" || m.pick.active || m.travel.active || m.opening {
		return nil
	}

	return m.loadLog(m.revset)
}

// noteState files what a background read learned about the repository or the machine.
func (m Model) noteState(msg tea.Msg) Model {
	switch msg := msg.(type) {
	case headOpMsg:
		m.lastOp = msg.description
	case diffToolMsg:
		m.diffTool = msg.tool
	}

	return m
}

// showMessage puts a status line up, or takes it down once it has had its time.
func (m Model) showMessage(msg tea.Msg) (Model, tea.Cmd) {
	shown, ok := msg.(messageMsg)
	if !ok {
		m.message, m.failing = "", false

		return m, nil
	}

	m.message, m.failing = shown.text, shown.output != ""
	if shown.wrote {
		m.log.ClearChecked()

		m.synced = syncReport{}
	}

	return m, m.after(messageLife, func(time.Time) tea.Msg { return clearMessageMsg{} })
}

// routeToPanes fans out what the root does not own: a pane's debounce and poll ticks are its own
// unexported types and would otherwise never come back. The docked diff's reads are the root's own,
// since the root decides which revision it follows.
func (m Model) routeToPanes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleDockMsg(msg); handled {
		return next, cmd
	}

	cmd := tea.Batch(m.revsetBox.Update(msg), m.forge.Update(msg))

	return m, cmd
}

// applyLog keeps the last good log on screen when a revset fails, since one is invalid for most of
// the time it is being typed.
func (m Model) applyLog(msg logLoadedMsg) (Model, tea.Cmd) {
	// Any read landing, a failed one included, lets keys through from here on. landLog has already
	// taken what was held; a caller that applies a read directly held nothing.
	m.early.Release()

	if msg.err != nil {
		if errors.Is(msg.err, jj.ErrRevsetSyntax) {
			m.revsetBox.SetError(msg.err)

			return m, nil
		}

		if errors.Is(msg.err, jj.ErrStaleWorkingCopy) {
			cmd := m.openStale(msg.err)

			return m, cmd
		}

		m.fail(msg.err)

		return m, nil
	}

	m.revsetBox.SetError(nil)
	m.revsetLoaded = true
	m.revset = msg.revset
	m.rows = msg.rows
	m.learnScopes(revisionsOf(msg.rows))
	m.logFull = len(msg.rows) >= m.logWindow
	m.log.SetRows(msg.rows)

	if m.landOn != "" {
		m.log.Focus(m.landOn)
		m.landOn = ""
	}

	m.resize()

	// A write changes what the docked diff shows without moving the cursor, so it reads again, and
	// an open outline reads its stack again for the same reason.
	outline := tea.Cmd(nil)
	if m.outline.active {
		outline = m.readOutline(m.outline.revset)
	}

	cmd := tea.Batch(m.forge.SetBookmark(bookmarkNearWorkingCopy(msg.rows)), m.loadHeadOp(),
		m.loadDock(m.dockTarget()), m.loadStats(), outline, m.describeSplit())

	return m, cmd
}

// revisionsOf is the rows that are revisions.
func revisionsOf(rows []jj.LogRow) []*jj.Revision {
	out := make([]*jj.Revision, 0, len(rows))
	for _, r := range rows {
		if r.Revision != nil {
			out = append(out, r.Revision)
		}
	}

	return out
}

// followBookmark points the forge pane at the selected revision's bookmark. A revision carrying
// none leaves the pane where it is.
func (m Model) followBookmark(rev *jj.Revision) tea.Cmd {
	name := branchName(rev)
	if name == "" {
		return nil
	}

	return m.forge.SetBookmark(name)
}

// resize gives the log whatever the revset box, the search bar, the forge pane, and the footer
// leave, less what the docked diff takes beside or under it.
func (m *Model) resize() {
	forgeHeight := 0
	if m.showForge {
		forgeHeight = forgeRows
	}

	m.forge.SetSize(m.width, forgeHeight)
	m.revsetShown = m.revsetRow()
	m.bandShown = len(m.bandLines())
	m.promptShown = m.prompt.Rows(m.height)
	// The outline's columns depend on whether it shares the width, and the dock's share on them.

	width, height := m.width, m.logHeight()

	if m.docked {
		dockWidth, dockHeight := m.dockSize()
		m.dock.SetWidth(diffpane.DockWidth(dockWidth))

		if m.dockSideBySide() {
			// A cell of margin keeps a row's last column (a count, a strip) off the dock's rule.
			width -= dockWidth + besideMargin
		} else {
			height -= dockHeight
		}
	}

	m.log.SetSize(width, height)
	m.logBox = [2]int{width, height}
	m.outline.view.SetSize(width, height)
	m.submit.view.SetSize(m.width, max(0, m.height-pickRows))
	m.pick.view.SetSize(m.width, m.pickHeight())
	m.travel.view.SetSize(m.width, m.logHeight())
}

// logHeight is the rows between the revset box and whatever sits under the log, docked diff included.
func (m Model) logHeight() int {
	forgeHeight := 0
	if m.showForge {
		forgeHeight = forgeRows
	}

	findHeight := 0
	if m.find.IsOpen() {
		findHeight = findRows
	}

	return max(0, m.height-footerRows-m.revsetHeight()-forgeHeight-findHeight-m.inspectorRows()-m.bandShown-
		m.promptShown)
}

// revsetRow reports whether the revset box gets its row: while it is being edited, and once the log
// shows anything but the revset it opened on. The opening revset is already in the footer, so a row
// repeating it is a line taken from the log for nothing.
func (m Model) revsetRow() bool {
	return m.history.active || m.focus == focusRevset || m.revsetBox.Focused() ||
		m.revset != m.startRevset || m.revsetBox.Value() != m.startRevset
}

// revsetHeight is the rows above the log: the revset row when it shows, and the banner that stands
// in its place while the log shows the past.
func (m Model) revsetHeight() int {
	if m.travel.active || m.revsetRow() {
		return revsetRows
	}

	return 0
}

// revsetPane is the revset row as a pane, none when it is hidden.
func (m Model) revsetPane() []string {
	if !m.revsetRow() {
		return nil
	}

	if m.history.active && !m.revsetBox.Focused() {
		return []string{m.historyBanner()}
	}

	return []string{m.revsetBox.View(m.width)}
}

// View renders the log above the footer, with the help legend over both.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	// Coming back to the terminal reads the log again (refreshOnFocus), so an edit made in another
	// window shows without ctrl+l.
	view.ReportFocus = true

	return view
}

// overlay is the frame drawn in place of the log, or nil when none is open. One table rather than a
// chain of branches, so an overlay added here cannot be drawn in the wrong order. The order is the
// stacking order: a diff opened from the file list draws over it and closing the diff uncovers it.
func (m Model) overlay() func(width, height int) string {
	frames := []struct {
		view func(width, height int) string
		open bool
	}{
		{m.output.View, m.output.IsOpen()},
		{m.trailView.View, m.trailView.IsOpen()},
		{m.confirm.View, m.confirm.IsOpen()},
		{m.bookmarks.View, m.bookmarks.IsOpen()},
		{m.detail.View, m.detail.IsOpen()},
		{m.diffFrame, m.diff.IsOpen()},
		{m.files.View, m.files.IsOpen()},
		{m.destPicker.View, m.destPicker.IsOpen()},
		{m.opLogFrame, m.opLog.IsOpen()},
		{m.spaces.View, m.spaces.IsOpen()},
		{m.roundsBox.View, m.roundsBox.IsOpen()},
	}

	for _, frame := range frames {
		if frame.open {
			return frame.view
		}
	}

	return nil
}

func (m Model) render() string { return m.withWelcome(m.screenFrame()) }

// screenFrame is the frame under the welcome card.
func (m Model) screenFrame() string {
	if m.quitting || m.width == 0 {
		return ""
	}

	if view := m.overlay(); view != nil {
		// An overlay takes the frame, footer included, so a failure raised while one is open keeps a
		// row under it rather than happening out of sight.
		if m.message == "" || m.height <= footerRows {
			return m.discovered(m.withPrompt(view(m.width, m.height)), 0)
		}

		return m.discovered(m.withPrompt(view(m.width, m.height-footerRows)+"\n"+m.overlayFooter()), footerRows)
	}

	if m.submit.active {
		return m.discovered(m.whichKey(m.withPrompt(m.submitFrame())), pickRows)
	}

	if m.pick.active || m.travel.active {
		keep := footerRows
		if m.pick.active {
			keep = pickRows
		}

		return m.discovered(m.whichKey(m.withPrompt(m.modeFrame())), keep)
	}

	panes := append(m.revsetPane(), m.logPane())
	if m.find.IsOpen() {
		panes = append(panes, m.find.View(m.width))
	}

	if m.showForge {
		panes = append(panes, m.forge.View())
	}

	panes = append(panes, m.inspector(m.dockTarget(), m.overflow())...)
	panes = append(panes, m.bandLines()...)

	// The prompt docks over the footer, the log shortened for it, so what it describes stays in view.
	if m.prompt.IsOpen() {
		panes = append(panes, m.prompt.Docked(m.width, m.height), m.promptFooter())

		return m.discovered(strings.Join(panes, "\n"), footerRows)
	}

	return m.discovered(m.whichKey(strings.Join(append(panes, m.footer()), "\n")), footerRows)
}

// discovered draws the open palette over the frame's rows above its last keep, which stay in view: the
// footer says where the reader is and how to get out. The legend keeps at least the last row and says
// there which icon set the file list draws, why, and how to change it.
func (m Model) discovered(frame string, keep int) string {
	if !m.pal.IsOpen() {
		return frame
	}

	lines := strings.Split(frame, "\n")

	// The legend says which icon set is drawn in the frame's last row, which it keeps clear of itself.
	legend := m.pal.Mode() == palette.Browse && m.icons != nil
	if legend {
		keep = max(keep, 1)
	}

	body := max(0, len(lines)-keep)
	out := append(m.pal.Over(lines[:body], m.width), lines[body:]...)

	if legend && len(out) > 0 {
		out[len(out)-1] = m.iconsLine()
	}

	return strings.Join(out, "\n")
}

// panelMargin is the columns the which-key panel leaves the log at its widest, so a page of flags is
// read whole rather than cut at half the width; it covers the log only while a prefix waits.
const panelMargin = 4

// whichKey floats the keys that can follow a waiting prefix over the bottom right of the frame, so a
// chord never has to be remembered whole. It sits above the footer, which keeps saying where the
// reader is and how to get out.
func (m Model) whichKey(frame string) string {
	hints := m.matcher.Panel(m.screen())
	if len(hints) == 0 {
		return frame
	}

	head := keyhint.Hint{Head: true, What: strings.Join(m.matcher.Pending(), "") + "…"}
	box := keyhint.Panel(m.skin.PanelStyles(), append([]keyhint.Hint{head}, hints...), max(1, m.width-panelMargin))

	// The picker's footer is two rows, the live command line over its flags, and both stay in view.
	keep := footerRows
	if m.pick.active || m.submit.active {
		keep = pickRows
	}

	lines := strings.Split(frame, "\n")
	body := max(0, len(lines)-keep)

	return strings.Join(append(overlay.BottomRight(lines[:body], box, m.width), lines[body:]...), "\n")
}

// modeFrame is the log while it is picking a destination or showing the past: the docked diff
// steps aside, since neither mode's rows are the ones it would be following.
func (m Model) modeFrame() string {
	if m.pick.active {
		panes := append(m.revsetPane(), m.pick.view.View())
		if m.showForge {
			panes = append(panes, m.forge.View())
		}

		panes = append(panes, m.inspector(m.pick.view.Selected(), m.pick.view.Overflow())...)

		return strings.Join(append(panes, m.pickFooter()), "\n")
	}

	panes := []string{m.travelBanner(), m.travel.view.View()}
	if m.showForge {
		panes = append(panes, m.forge.View())
	}

	panes = append(panes, m.inspector(m.travel.view.Selected(), m.travel.view.Overflow())...)

	return strings.Join(append(panes, m.travelFooter()), "\n")
}

// diffFrame is the full-screen diff, or the card saying its change is read once U has nothing left
// to draw.
func (m Model) diffFrame(width, height int) string {
	if card := m.readCard(m.fullReads(), width, height); card != nil {
		return strings.Join(card, "\n")
	}

	return m.diff.View(width, height)
}

// dockView is the docked diff, or the card saying its change is read once U has nothing left to draw.
func (m Model) dockView(width, height int) string {
	if card := m.readCard(m.dockReads(), width, height); card != nil {
		return strings.Join(card, "\n")
	}

	return m.dock.DockView(width, height, m.focus == focusDock)
}

// logPane is the log, with the docked diff beside it on a wide terminal or under it on a narrow one.
func (m Model) logPane() string {
	left := m.log.View()
	if m.outline.active {
		left = m.outline.view.View()
	} else if m.loaded() && !m.anyRevision() {
		left = m.emptyLog()
	}

	if !m.docked {
		return left
	}

	dockWidth, dockHeight := m.dockSize()
	dock := m.dockView(dockWidth, dockHeight)

	if !m.dockSideBySide() {
		return left + "\n" + dock
	}

	logWidth := m.width - dockWidth
	logLines := strings.Split(left, "\n")
	dockLines := strings.Split(dock, "\n")

	for i := range logLines {
		pad := max(0, logWidth-ansi.StringWidth(logLines[i]))
		logLines[i] += strings.Repeat(" ", pad)

		if i < len(dockLines) {
			logLines[i] += dockLines[i]
		}
	}

	return strings.Join(logLines, "\n")
}
