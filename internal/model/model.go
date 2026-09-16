// Package model is jojo's root Bubble Tea model: it owns the panes, routes every
// key, and is the only place that calls jj.
package model

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/forgepane"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/components/revsetbox"
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

// handleOverlayMsg covers what the modals report back, and what the root loads to open one.
func (m Model) handleOverlayMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case prompt.SubmittedMsg:
		next, cmd := m.handleSubmitted(msg)

		return next, cmd, true
	case destpicker.PickedMsg:
		next, cmd := m.handlePicked(msg)

		return next, cmd, true
	case prompt.CancelledMsg, destpicker.CancelledMsg, oplogview.CancelledMsg, workspaceview.CancelledMsg:
		return m.cancelModals(), nil, true
	case opLogLoadedMsg:
		next, cmd := m.applyOpLog(msg)

		return next, cmd, true
	case oplogview.RestoreMsg:
		next, cmd := m.restoreOperation(msg)

		return next, cmd, true
	case workspacesLoadedMsg:
		next, cmd := m.applyWorkspaces(msg)

		return next, cmd, true
	case workspaceview.AddRequestedMsg:
		next, cmd := m.openWorkspaceAdd()

		return next, cmd, true
	case workspaceview.ForgetMsg:
		next, cmd := m.forgetWorkspace(msg)

		return next, cmd, true
	}

	return m, nil, false
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

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		return m.handleHelpKey(msg)
	}

	if cmd, handled := m.routeToModal(msg); handled {
		return m, cmd
	}

	switch m.focus {
	case focusRevset:
		return m.handleRevsetKey(msg)
	case focusForge:
		return m.handleForgeKey(msg)
	case focusLog:
	}

	if next, cmd, handled := m.handleRootKey(msg.String()); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleActionKey(msg.String()); handled {
		return next, cmd
	}

	cmd := m.log.Update(msg)

	return m, cmd
}

// routeToModal hands every key to whichever overlay is open, because a modal
// sharing j and k with the log underneath would move both cursors at once.
func (m *Model) routeToModal(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case m.prompt.IsOpen():
		return m.prompt.Update(msg), true
	case m.destPicker.IsOpen():
		return m.destPicker.Update(msg), true
	case m.opLog.IsOpen():
		return m.opLog.Update(msg), true
	case m.spaces.IsOpen():
		return m.spaces.Update(msg), true
	}

	return nil, false
}

// handleRootKey covers the keys that belong to the application rather than to
// the revision under the cursor.
func (m Model) handleRootKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "L":
		m.focus = focusRevset

		cmd := m.revsetBox.Focus()

		return m, cmd, true
	case keyTab:
		next, cmd := m.cycleFocus()

		return next, cmd, true
	case "p":
		next, cmd := m.toggleForge()

		return next, cmd, true
	case "q", "ctrl+c":
		m.quitting = true

		return m, tea.Quit, true
	case "?":
		m.showHelp = true
		m.help = m.help.ScrollToStart()

		return m, nil, true
	case "u":
		return m, m.undo(), true
	case "ctrl+r":
		return m, m.redo(), true
	case "ctrl+l":
		return m, m.loadLog(m.revset), true
	case "o":
		return m, m.loadOpLog(), true
	case "w":
		return m, m.loadWorkspaces(), true
	}

	return m, nil, false
}

// cycleFocus walks Tab through the panes that are open, so a focused pane owns
// its own keys and a mnemonic cannot mean two things at once.
func (m Model) cycleFocus() (tea.Model, tea.Cmd) {
	switch {
	case m.focus == focusLog:
		m.focus = focusRevset

		cmd := m.revsetBox.Focus()

		return m, cmd
	case m.focus == focusRevset && m.showForge:
		m.revsetBox.Blur()
		m.focus = focusForge

		return m, nil
	default:
		m.revsetBox.Blur()
		m.focus = focusLog

		return m, nil
	}
}

func (m Model) handleForgeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		m.focus = focusLog

		return m, nil
	case keyTab:
		return m.cycleFocus()
	}

	cmd := m.forge.Update(msg)

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

func (m Model) handleRevsetKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == keyTab {
		return m.cycleFocus()
	}

	if msg.String() == keyEsc {
		m.revsetBox.Blur()
		m.focus = focusLog

		return m, nil
	}

	cmd := m.revsetBox.Update(msg)
	m.resize()

	return m, cmd
}

func (m Model) toggleForge() (tea.Model, tea.Cmd) {
	m.showForge = !m.showForge
	if !m.showForge && m.focus == focusForge {
		m.focus = focusLog
	}

	m.resize()

	cmd := m.forge.SetFocused(m.showForge)

	return m, cmd
}

// resize hands each pane its share of the terminal. The log takes whatever the
// revset box, the forge pane, and the footer leave.
func (m *Model) resize() {
	forgeHeight := 0
	if m.showForge {
		forgeHeight = forgeRows
	}

	m.forge.SetSize(m.width, forgeHeight)
	m.log.SetSize(m.width, max(0, m.height-footerRows-revsetRows-forgeHeight))
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc, "q", "?":
		m.showHelp = false
	case "up", "k":
		m.help = m.help.ScrollBy(-1)
	case "down", "j":
		m.help = m.help.ScrollBy(1)
	case "g", "home":
		m.help = m.help.ScrollToStart()
	case "G", "end":
		m.help = m.help.ScrollToEnd()
	}

	return m, nil
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
	case m.prompt.IsOpen():
		return m.prompt.View(m.width, m.height)
	case m.destPicker.IsOpen():
		return m.destPicker.View(m.width, m.height)
	case m.opLog.IsOpen():
		return m.opLog.View(m.width, m.height)
	case m.spaces.IsOpen():
		return m.spaces.View(m.width, m.height)
	}

	panes := []string{m.revsetBox.View(m.width), m.log.View()}
	if m.showForge {
		panes = append(panes, m.forge.View())
	}

	return strings.Join(append(panes, m.footer()), "\n")
}
