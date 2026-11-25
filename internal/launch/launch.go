// Package launch starts the program before it knows the terminal's background, which picks the
// flavor every face is built from.
//
// The background is asked through the program's own input, never before it: a query that reads the
// terminal ahead of the program (lipgloss.HasDarkBackground) also reads whatever was typed while it
// waited and throws it away with the reply, so the first key of a session, typed as the command was
// entered, never arrived. Here the reply is one message among the keys, which are held, in order,
// until the real model is built, and then handed to it.
package launch

import (
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Settle is how long a terminal has to answer before it is taken as dark. Every terminal in use
// answers the device-attributes query that follows the background one, which ends the wait at once;
// only a terminal that answers nothing waits this out.
const Settle = 2 * time.Second

// settledMsg is the end of the wait for an answer.
type settledMsg struct{}

// Model holds what arrives before the background does, then becomes the model build makes.
type Model struct {
	build  func(dark bool) tea.Model
	held   []tea.Msg
	settle time.Duration
}

// New is a launch that builds the program with build once the terminal says whether its
// background is dark.
func New(build func(dark bool) tea.Model) Model {
	return Model{build: build, settle: Settle}
}

// WithSettle is m waiting d for an answer.
func (m Model) WithSettle(d time.Duration) Model {
	m.settle = d

	return m
}

// Init asks for the background and then for the device attributes in one write, so they are
// answered in that order: a terminal that answers the second and not the first has no background
// to report.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.Raw(ansi.RequestBackgroundColor+ansi.RequestPrimaryDeviceAttributes),
		tea.Tick(m.settle, func(time.Time) tea.Msg { return settledMsg{} }),
	)
}

// Update builds the program on the first answer, or on the wait running out, and holds everything
// else until then. From then on the program is the model, so this one is never called again.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		return m.start(msg.IsDark())
	case uv.PrimaryDeviceAttributesEvent, settledMsg:
		return m.start(true)
	}

	m.held = append(m.held, msg)

	return m, nil
}

// start builds the program and hands it, in order, what arrived while it was being asked for.
func (m Model) start(dark bool) (tea.Model, tea.Cmd) {
	program := m.build(dark)
	cmds := make([]tea.Cmd, 0, len(m.held)+1)
	cmds = append(cmds, program.Init())

	for _, msg := range m.held {
		var cmd tea.Cmd

		program, cmd = program.Update(msg)
		cmds = append(cmds, cmd)
	}

	return program, tea.Batch(cmds...)
}

// View is the screen while the terminal is asked: nothing, on the alternate screen the program
// draws on, so the shell's screen does not flash between the two.
func (Model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true

	return view
}
