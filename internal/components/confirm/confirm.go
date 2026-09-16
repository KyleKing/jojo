// Package confirm is a hard-to-reflex confirmation modal for jj actions that
// jj undo cannot cleanly reverse: a git push, a force push, or anything run
// with --ignore-immutable.
package confirm

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"

	"github.com/kyleking/jojo/internal/ui"
)

// Request describes the action awaiting confirmation.
type Request struct {
	Key    string
	Title  string
	Danger string
	Detail []string
}

// ConfirmedMsg is sent when y confirms the action named by Key.
type ConfirmedMsg struct {
	Key string
}

// CancelledMsg is sent when esc, q, or n dismisses the dialog.
type CancelledMsg struct{}

// Model is the dialog's state: the pending request and whether it is shown.
type Model struct {
	skin ui.Skin
	req  Request
	open bool
}

// New returns a closed dialog drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the dialog for req.
//
//nolint:unparam // matches prompt.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(req Request) tea.Cmd {
	m.req = req
	m.open = true

	return nil
}

// Close hides the dialog.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the dialog is currently shown.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the dialog is
// open, and only y confirms: enter is swallowed so leaning on it can never
// substitute for a deliberate yes.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	switch key.String() {
	case "y":
		req := m.req
		m.Close()

		return func() tea.Msg { return ConfirmedMsg{Key: req.Key} }
	case "n", "esc", "q":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

// View renders the title, detail lines, the danger line, and a key hint
// line, centered in a width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	if contentWidth < 1 || contentHeight < 1 {
		return ui.TooSmall(width, height)
	}

	lines := []string{m.skin.Title.Render(m.req.Title)}

	for _, detail := range m.req.Detail {
		lines = append(lines, m.skin.Body.Render(detail))
	}

	if m.req.Danger != "" {
		lines = append(lines, m.skin.Error.Render(m.req.Danger))
	}

	hint := ui.FitHints(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "y", What: "yes"},
		{Key: "n", What: "no"},
	}, contentWidth)

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}
