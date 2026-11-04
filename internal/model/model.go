// Package model is jojo's root Bubble Tea model: it owns the panes, routes every
// key, and is the only place that calls jj.
package model

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/detailview"
	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/components/evologview"
	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/components/forgepane"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/components/palette"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/components/revsetbox"
	"github.com/kyleking/jojo/internal/components/search"
	"github.com/kyleking/jojo/internal/components/statusbar"
	"github.com/kyleking/jojo/internal/components/workspaceview"
	jforge "github.com/kyleking/jojo/internal/forge"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// logLimit caps one query, because a revset over the whole repository costs a diff stat per row.
const logLimit = 500

// messageLife is how long a "done, press u to undo" line stays up.
const messageLife = 6 * time.Second

// footerRows is the height the footer takes out of the log.
const footerRows = 1

// Keys the root routes before any pane sees them.
const (
	keyTab = "tab"
	keyEsc = "esc"
)

type logLoadedMsg struct {
	err    error
	revset string
	rows   []jj.LogRow
}

type messageMsg struct {
	text string
}

type clearMessageMsg struct{}

// redoPointMsg carries the operation the repository sat at before an undo.
type redoPointMsg struct {
	id string
}

// Model is the application.
type Model struct {
	// ctx is the program lifetime, so quitting cancels a jj call already running.
	ctx        context.Context //nolint:containedctx // the model owns every jj call it starts.
	skin       ui.Skin
	repoPath   string
	status     statusbar.Model
	client     *jj.Client
	help       help.Model
	revsetBox  revsetbox.Model
	forge      forgepane.Model
	prompt     prompt.Model
	destPicker destpicker.Model
	opLog      oplogview.Model
	spaces     workspaceview.Model
	evolog     evologview.Model
	find       search.Model
	palette    palette.Model
	bookmarks  bookmarkview.Model
	detail     detailview.Model
	files      fileview.Model
	diff       diffpane.Model
	confirm    confirm.Model
	forcible   forcible
	pushing    pendingPush
	onSubmit   submitted
	onPick     picked
	keys       help.Keymap
	redoOp     string
	revset     string
	message    string
	lastOp     string
	rows       []jj.LogRow
	log        logview.Model
	focus      focus
	width      int
	height     int
	showForge  bool
	showHelp   bool
	quitting   bool
}

// Options are the settings a user states in config.toml, which New folds into the model rather than
// reading itself, so a test can pass its own without a file on disk.
type Options struct {
	Keys help.Keymap
}

// New builds the root model against a repository jj already recognizes.
func New(ctx context.Context, client *jj.Client, skin ui.Skin, repoPath, revset string, opts Options) Model {
	return Model{
		ctx:        ctx,
		client:     client,
		skin:       skin,
		repoPath:   repoPath,
		log:        logview.New(logview.StylesFrom(skin)),
		revsetBox:  revsetbox.New(revset, revsetbox.WithAliases(aliasSource(client))),
		forge:      forgepane.New(ctx, forgeStyles(skin), repoPath, "", jforge.WithFocused(false)),
		prompt:     prompt.New(skin),
		destPicker: destpicker.New(skin),
		opLog:      oplogview.New(skin),
		spaces:     workspaceview.New(skin),
		evolog:     evologview.New(skin),
		find:       search.New(skin),
		bookmarks:  bookmarkview.New(skin),
		detail:     detailview.New(skin),
		files:      fileview.New(skin),
		diff:       diffpane.New(skin),
		confirm:    confirm.New(skin),
		palette:    palette.New(skin),
		status:     statusbar.New(skin),
		help:       help.New(skin, opts.Keys),
		keys:       opts.Keys,
		revset:     revset,
	}
}

// Init loads the log, so the first frame is an empty log rather than a blocked terminal.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadLog(m.revset), m.loadHeadOp(), m.forge.Init())
}

func (m Model) loadLog(revset string) tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		rows, err := client.Log(ctx, revset, logLimit)

		return logLoadedMsg{revset: revset, rows: rows, err: err}
	}
}

// Update routes messages. Keys go to handleKeyPress, the one place a binding is added.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleOverlayMsg(msg); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleFindMsg(msg); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleBookmarkMsg(msg); handled {
		return next, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.diff.SetWidth(m.width)
		m.resize()

		return m, nil
	case revsetbox.ChangedMsg:
		return m, m.loadLog(msg.Revset)
	case logview.SelectedMsg:
		cmd := m.followBookmark(msg.Revision)

		return m, cmd
	case logLoadedMsg:
		return m.applyLog(msg)
	case staleMsg:
		return m, m.openStale(msg.err)
	case messageMsg:
		m.message = msg.text

		return m, tea.Tick(messageLife, func(time.Time) tea.Msg { return clearMessageMsg{} })
	case clearMessageMsg:
		m.message = ""

		return m, nil
	case redoPointMsg:
		m.redoOp = msg.id

		return m, nil
	case headOpMsg:
		m.lastOp = msg.description

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	return m.routeToPanes(msg)
}

// routeToPanes fans out what the root does not own: a pane's debounce and poll ticks are its own
// unexported types and would otherwise never come back.
func (m Model) routeToPanes(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := tea.Batch(m.revsetBox.Update(msg), m.forge.Update(msg))

	return m, cmd
}

// applyLog keeps the last good log on screen when a revset fails, since one is invalid for most of
// the time it is being typed.
func (m Model) applyLog(msg logLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if errors.Is(msg.err, jj.ErrRevsetSyntax) {
			m.revsetBox.SetError(msg.err)

			return m, nil
		}

		if errors.Is(msg.err, jj.ErrStaleWorkingCopy) {
			return m, m.openStale(msg.err)
		}

		m.message = msg.err.Error()

		return m, nil
	}

	m.revsetBox.SetError(nil)
	m.revset = msg.revset
	m.rows = msg.rows
	m.log.SetRows(msg.rows)
	m.resize()

	cmd := tea.Batch(m.forge.SetBookmark(bookmarkNearWorkingCopy(msg.rows)), m.loadHeadOp())

	return m, cmd
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
// leave.
func (m *Model) resize() {
	forgeHeight := 0
	if m.showForge {
		forgeHeight = forgeRows
	}

	findHeight := 0
	if m.find.IsOpen() {
		findHeight = findRows
	}

	m.forge.SetSize(m.width, forgeHeight)
	m.log.SetSize(m.width, max(0, m.height-footerRows-revsetRows-forgeHeight-findHeight))
}

// View renders the log above the footer, with the help legend over both.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true

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
		{m.help.View, m.showHelp},
		{m.confirm.View, m.confirm.IsOpen()},
		{m.bookmarks.View, m.bookmarks.IsOpen()},
		{m.detail.View, m.detail.IsOpen()},
		{m.diff.View, m.diff.IsOpen()},
		{m.files.View, m.files.IsOpen()},
		{m.prompt.View, m.prompt.IsOpen()},
		{m.destPicker.View, m.destPicker.IsOpen()},
		{m.opLog.View, m.opLog.IsOpen()},
		{m.spaces.View, m.spaces.IsOpen()},
		{m.evolog.View, m.evolog.IsOpen()},
		{m.palette.View, m.palette.IsOpen()},
	}

	for _, frame := range frames {
		if frame.open {
			return frame.view
		}
	}

	return nil
}

func (m Model) render() string {
	if m.quitting || m.width == 0 {
		return ""
	}

	if view := m.overlay(); view != nil {
		return view(m.width, m.height)
	}

	panes := []string{m.revsetBox.View(m.width), m.log.View()}
	if m.find.IsOpen() {
		panes = append(panes, m.find.View(m.width))
	}

	if m.showForge {
		panes = append(panes, m.forge.View())
	}

	return strings.Join(append(panes, m.footer()), "\n")
}
