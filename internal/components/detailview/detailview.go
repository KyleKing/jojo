// Package detailview is the overlay behind the log's "v" key: everything about one revision the row
// has no room for, including the whole description rather than its first line.
package detailview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// CancelledMsg is sent when the overlay is dismissed.
type CancelledMsg struct{}

// Model holds the detail being shown.
type Model struct {
	skin   ui.Skin
	detail jj.Detail
	open   bool
}

// New returns a closed overlay drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows detail.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(detail jj.Detail) tea.Cmd {
	m.detail = detail
	m.open = true

	return nil
}

// Close hides the overlay.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the overlay is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Any key closes, because the overlay only reads.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(tea.KeyPressMsg); !ok || !m.open {
		return nil
	}

	m.Close()

	return func() tea.Msg { return CancelledMsg{} }
}

// labelWidth keeps the values in a column, measured from the longest label below.
const labelWidth = 9

// noDescription is shown in place of a revision nobody has described yet.
const noDescription = "(no description set)"

// View renders the metadata above the full description, centered in a width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{m.skin.Title.Render(m.detail.ChangeID)}
	lines = append(lines, m.fields(contentWidth)...)

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{{Key: "any key", What: "close"}})

	body := max(0, contentHeight-len(lines)-1)
	lines = append(lines, m.description(contentWidth, body)...)
	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

func (m Model) fields(width int) []string {
	rows := [][2]string{
		{"commit", m.detail.CommitID},
		{"author", m.detail.AuthorName + " <" + m.detail.AuthorEmail + ">"},
		{"authored", m.detail.Authored},
		{"committed", m.detail.Committed},
		{"parents", strings.Join(m.detail.Parents, " ")},
	}

	out := make([]string, 0, len(rows))

	for _, row := range rows {
		if row[1] == "" {
			continue
		}

		label := m.skin.Muted.Render(table.Pad(row[0], labelWidth, table.AlignLeft))
		out = append(out, label+" "+m.skin.Body.Render(table.Truncate(row[1], max(0, width-labelWidth-1))))
	}

	return out
}

// description keeps its own line breaks, which is the reason this overlay exists: the row can only
// ever show the first line.
func (m Model) description(width, body int) []string {
	text := m.detail.Description
	if strings.TrimSpace(text) == "" {
		return []string{m.skin.Muted.Render(noDescription)}
	}

	lines := strings.Split(text, "\n")
	if body > 0 && len(lines) > body {
		lines = lines[:body]
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, "")

	for _, line := range lines {
		out = append(out, m.skin.Body.Render(table.Truncate(line, width)))
	}

	return out
}
