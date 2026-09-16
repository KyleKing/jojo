// Package destpicker is a scrollable revision picker modal, used to choose the
// destination of a jj squash --into or a jj rebase --onto. The parent passes
// the rows it already has, so this never calls jj.
package destpicker

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/filter"
	"github.com/kyleking/aragonite/tui/keyhint"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// Placement is where the picked revision sits relative to whatever the caller is moving. Only a
// rebase offers anything but PlaceOn.
type Placement int

// Placements the picker can confirm.
const (
	PlaceOn Placement = iota
	PlaceAfter
	PlaceBefore
)

// PickedMsg is sent when a confirm key takes a mutable revision.
type PickedMsg struct {
	Title     string
	Revision  jj.Revision
	Placement Placement
}

// CancelledMsg is sent when Esc dismisses the picker without picking one.
type CancelledMsg struct{}

// Option configures a Model at construction.
type Option func(*Model)

// OpenOption configures one opening of the picker.
type OpenOption func(*Model)

// AllowImmutable lets the picker return a revision jj protects, which is right for a read such as a
// diff and wrong for a destination the picker is choosing for a write.
func AllowImmutable() OpenOption {
	return func(m *Model) { m.allowImmutable = true }
}

// WithPlacement offers the two keys that land beside the picked revision instead of on it, which
// only a rebase can act on.
func WithPlacement() OpenOption {
	return func(m *Model) { m.placing = true }
}

// Model is the picker's state: the candidate revisions, the live filter over
// them, and the cursor into the filtered list.
type Model struct {
	skin           ui.Skin
	title          string
	filter         string
	revisions      []jj.Revision
	list           listview.Model[jj.Revision]
	allowImmutable bool
	placing        bool
}

// New returns a closed picker drawn in skin.
func New(skin ui.Skin, opts ...Option) Model {
	m := Model{skin: skin, list: listview.New[jj.Revision](skin)}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// Open shows the picker over rows, omitting graph-only lines and the
// revision whose change id is exclude, with the filter cleared.
//
//nolint:unparam // matches prompt.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(title string, rows []jj.LogRow, exclude string, opts ...OpenOption) tea.Cmd {
	m.title = title
	m.filter = ""
	m.allowImmutable, m.placing = false, false

	for _, opt := range opts {
		opt(m)
	}

	revisions := make([]jj.Revision, 0, len(rows))

	for _, row := range rows {
		if row.Revision == nil || row.Revision.ChangeID == exclude {
			continue
		}

		revisions = append(revisions, *row.Revision)
	}

	m.revisions = revisions
	m.recomputeMatches()

	return nil
}

// Close hides the picker.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the picker is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the picker is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	return m.handleKey(key)
}

// handleKey routes only the non-printable movement keys through the list, since j and k have to
// type into the filter like any other letter.
func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	name := key.String()

	switch name {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		return m.pick(PlaceOn)
	case "alt+a":
		return m.place(PlaceAfter)
	case "alt+b":
		return m.place(PlaceBefore)
	case "up", "down", "ctrl+n", "ctrl+p", "pgup", "pgdown", "home", "end":
		m.list.MoveKey(name)

		return nil
	case "backspace":
		m.setFilter(trimLastRune(m.filter))

		return nil
	}

	if key.Text != "" {
		m.setFilter(m.filter + key.Text)
	}

	return nil
}

func (m *Model) place(where Placement) tea.Cmd {
	if !m.placing {
		return nil
	}

	return m.pick(where)
}

func (m *Model) pick(where Placement) tea.Cmd {
	rev, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if rev.Immutable && !m.allowImmutable {
		m.list.Warn("cannot pick an immutable revision")

		return nil
	}

	title := m.title
	m.Close()

	return func() tea.Msg { return PickedMsg{Title: title, Revision: rev, Placement: where} }
}

func (m *Model) setFilter(s string) {
	m.filter = s
	m.recomputeMatches()
}

func (m *Model) recomputeMatches() {
	pattern := m.filter

	matches := filter.Keep(m.revisions, func(rev jj.Revision) bool {
		return matchesFilter(rev, pattern)
	})

	m.list.Open(m.title+" /"+m.filter, matches, 0)
}

func matchesFilter(rev jj.Revision, pattern string) bool {
	if pattern == "" {
		return true
	}

	if filter.Match(pattern, rev.ChangeID) || filter.Match(pattern, rev.Description) {
		return true
	}

	for _, bookmark := range rev.Bookmarks {
		if filter.Match(pattern, bookmark) {
			return true
		}
	}

	return false
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}

	return string(runes[:len(runes)-1])
}

// View renders the title with the live filter, matching rows, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	hints := []keyhint.Hint{{Key: "enter", What: "pick"}}
	if m.placing {
		hints = append(hints,
			keyhint.Hint{Key: "alt+a", What: "insert after"},
			keyhint.Hint{Key: "alt+b", What: "insert before"})
	}

	return m.list.View(width, height, m.row, append(hints, keyhint.Hint{Key: "esc", What: "cancel"}))
}

func (m Model) row(rev jj.Revision, selected bool) listview.Row {
	style := m.skin.Body

	if rev.Immutable {
		style = m.skin.Immutable
	}

	if selected {
		style = m.skin.Selected
	}

	return listview.Row{Text: rowText(rev), Style: style}
}

func rowText(rev jj.Revision) string {
	var b strings.Builder

	b.WriteString(rev.ChangeID)

	if len(rev.Bookmarks) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(rev.Bookmarks, ","))
	}

	if rev.Description != "" {
		b.WriteString(" ")
		b.WriteString(rev.Description)
	}

	return b.String()
}
