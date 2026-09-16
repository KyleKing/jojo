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
	"github.com/kyleking/jojo/internal/components/evologview"
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

// logLimit caps a single log query, because a revset matching the whole
// repository costs jj a diff stat per revision.
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

// redoPointMsg carries the operation the repository sat at before an undo,
// which is the only thing that makes the redo key meaningful.
type redoPointMsg struct {
	id string
}

type opLogLoadedMsg struct {
	err error
	ops []jj.Operation
}

type evologLoadedMsg struct {
	err      error
	changeID string
	entries  []jj.Evolution
}

type workspacesLoadedMsg struct {
	err    error
	spaces []jj.Workspace
}

// Model is the application.
type Model struct {
	// ctx is the program lifetime, so quitting cancels a jj call already
	// running rather than leaving it to finish into a dead model.
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
	confirm    confirm.Model
	forcible   forcible
	pushing    jj.PushOptions
	pendingOn  string
	redoOp     string
	revset     string
	message    string
	rows       []jj.LogRow
	log        logview.Model
	focus      focus
	pending    pending
	width      int
	height     int
	showForge  bool
	showHelp   bool
	quitting   bool
}

// New builds the root model against a repository jj already recognizes.
func New(ctx context.Context, client *jj.Client, skin ui.Skin, repoPath, revset string) Model {
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
		confirm:    confirm.New(skin),
		palette:    palette.New(skin),
		status:     statusbar.New(skin),
		help:       help.New(skin),
		revset:     revset,
	}
}

// Init loads the log, so the first frame is an empty log rather than a blocked
// terminal.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadLog(m.revset), m.forge.Init())
}

func (m Model) loadLog(revset string) tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		rows, err := client.Log(ctx, revset, logLimit)

		return logLoadedMsg{revset: revset, rows: rows, err: err}
	}
}

// Update routes messages. Key handling lives in handleKeyPress so there is one
// place a new binding is added.
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
		m.resize()

		return m, nil
	case revsetbox.ChangedMsg:
		return m, m.loadLog(msg.Revset)
	case logview.SelectedMsg:
		cmd := m.followBookmark(msg.Revision)

		return m, cmd
	case logLoadedMsg:
		return m.applyLog(msg)
	case messageMsg:
		m.message = msg.text

		return m, tea.Tick(messageLife, func(time.Time) tea.Msg { return clearMessageMsg{} })
	case clearMessageMsg:
		m.message = ""

		return m, nil
	case redoPointMsg:
		m.redoOp = msg.id

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	return m.routeToPanes(msg)
}

// routeToPanes hands anything the root does not own to every sub-model, because
// their debounce and poll ticks are their own unexported types and would
// otherwise never come back to the component that scheduled them.
func (m Model) routeToPanes(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := tea.Batch(m.revsetBox.Update(msg), m.forge.Update(msg))

	return m, cmd
}

// applyLog keeps the last good log on screen when a revset fails, because an
// expression is invalid for most of the time it is being typed.
func (m Model) applyLog(msg logLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if errors.Is(msg.err, jj.ErrRevsetSyntax) {
			m.revsetBox.SetError(msg.err)

			return m, nil
		}

		m.message = msg.err.Error()

		return m, nil
	}

	m.revsetBox.SetError(nil)
	m.revset = msg.revset
	m.rows = msg.rows
	m.log.SetRows(msg.rows)
	m.resize()

	cmd := m.forge.SetBookmark(bookmarkNearWorkingCopy(msg.rows))

	return m, cmd
}

// followBookmark points the forge pane at the selected revision's bookmark, so
// moving onto a revision shows that revision's pull request. A revision with no
// bookmark of its own leaves the pane where it is.
func (m Model) followBookmark(rev *jj.Revision) tea.Cmd {
	name := branchName(rev)
	if name == "" {
		return nil
	}

	return m.forge.SetBookmark(name)
}

// resize hands each pane its share of the terminal. The log takes whatever the
// revset box, the forge pane, and the footer leave.
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

func (m Model) render() string {
	if m.quitting || m.width == 0 {
		return ""
	}

	switch {
	case m.showHelp:
		return m.help.View(m.width, m.height)
	case m.confirm.IsOpen():
		return m.confirm.View(m.width, m.height)
	case m.bookmarks.IsOpen():
		return m.bookmarks.View(m.width, m.height)
	case m.prompt.IsOpen():
		return m.prompt.View(m.width, m.height)
	case m.destPicker.IsOpen():
		return m.destPicker.View(m.width, m.height)
	case m.opLog.IsOpen():
		return m.opLog.View(m.width, m.height)
	case m.spaces.IsOpen():
		return m.spaces.View(m.width, m.height)
	case m.evolog.IsOpen():
		return m.evolog.View(m.width, m.height)
	case m.palette.IsOpen():
		return m.palette.View(m.width, m.height)
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
