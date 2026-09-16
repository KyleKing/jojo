// Package model is jojo's root Bubble Tea model: it owns the panes, routes every
// key, and is the only place that calls jj.
package model

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/components/statusbar"
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

type logLoadedMsg struct {
	err    error
	revset string
	rows   []jj.LogRow
}

type messageMsg struct {
	text string
}

type clearMessageMsg struct{}

// Model is the application.
type Model struct {
	skin     ui.Skin
	status   statusbar.Model
	client   *jj.Client
	help     help.Model
	revset   string
	message  string
	rows     []jj.LogRow
	log      logview.Model
	width    int
	height   int
	showHelp bool
	quitting bool
}

// New builds the root model against a repository jj already recognizes.
func New(client *jj.Client, skin ui.Skin, revset string) Model {
	return Model{
		client: client,
		skin:   skin,
		log:    logview.New(logview.StylesFrom(skin)),
		status: statusbar.New(skin),
		help:   help.New(skin),
		revset: revset,
	}
}

// Init loads the log, so the first frame is an empty log rather than a blocked
// terminal.
func (m Model) Init() tea.Cmd {
	return m.loadLog(m.revset)
}

func (m Model) loadLog(revset string) tea.Cmd {
	client := m.client

	return func() tea.Msg {
		rows, err := client.Log(context.Background(), revset, logLimit)

		return logLoadedMsg{revset: revset, rows: rows, err: err}
	}
}

// Update routes messages. Key handling lives in handleKeyPress so there is one
// place a new binding is added.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.log.SetSize(m.width, m.logHeight())

		return m, nil
	case logLoadedMsg:
		return m.applyLog(msg), nil
	case messageMsg:
		m.message = msg.text

		return m, tea.Tick(messageLife, func(time.Time) tea.Msg { return clearMessageMsg{} })
	case clearMessageMsg:
		m.message = ""

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	return m, nil
}

func (m Model) applyLog(msg logLoadedMsg) Model {
	if msg.err != nil {
		m.message = msg.err.Error()

		return m
	}

	m.revset = msg.revset
	m.rows = msg.rows
	m.log.SetRows(msg.rows)
	m.log.SetSize(m.width, m.logHeight())

	return m
}

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		return m.handleHelpKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true

		return m, tea.Quit
	case "?":
		m.showHelp = true
		m.help = m.help.ScrollToStart()

		return m, nil
	case "u":
		return m, m.mutate("undone", m.client.Undo)
	case "ctrl+r":
		return m, m.loadLog(m.revset)
	}

	return m, m.log.Update(msg)
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?":
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

// mutate runs a jj write then reloads the log. Every routine jj edit is
// reversible, so it reports itself with an undo hint rather than a confirm gate.
func (m Model) mutate(done string, run func(context.Context) error) tea.Cmd {
	revset := m.revset

	return tea.Sequence(
		func() tea.Msg {
			if err := run(context.Background()); err != nil {
				return messageMsg{text: err.Error()}
			}

			return messageMsg{text: done + ", press u to undo"}
		},
		m.loadLog(revset),
	)
}

func (m Model) logHeight() int {
	return max(0, m.height-footerRows)
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

	if m.showHelp {
		return m.help.View(m.width, m.height)
	}

	return m.log.View() + "\n" + m.footer()
}
