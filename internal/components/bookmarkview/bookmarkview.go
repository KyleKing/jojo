// Package bookmarkview is the panel listing `jj bookmark list` entries,
// letting the user set, advance, create, rename, delete, forget, track,
// untrack, or push a bookmark. The parent passes the rows it already has and
// performs the jj calls; this package never calls jj.
package bookmarkview

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// SetMsg is sent when the user asks to move or create the bookmark named Name
// onto Target.
type SetMsg struct {
	Name   string
	Target string
}

// AdvanceMsg is sent when the user asks to advance the bookmark named Name
// onto Target.
type AdvanceMsg struct {
	Name   string
	Target string
}

// CreateRequestedMsg is sent when the user asks to create a bookmark on
// Target; the parent opens a prompt for its name.
type CreateRequestedMsg struct {
	Target string
}

// RenameRequestedMsg is sent when the user asks to rename the bookmark named
// Name; the parent opens a prompt for its new name.
type RenameRequestedMsg struct {
	Name string
}

// DeleteMsg is sent when the user asks to delete the bookmark named Name.
type DeleteMsg struct {
	Name string
}

// ForgetMsg is sent when the user asks to forget the bookmark named Name.
type ForgetMsg struct {
	Name string
}

// TrackMsg is sent when the user asks to track the remote bookmark Ref.
type TrackMsg struct {
	Ref string
}

// UntrackMsg is sent when the user asks to untrack the remote bookmark Ref.
type UntrackMsg struct {
	Ref string
}

// PushMsg is sent when the user asks to push the bookmark named Name; the
// parent puts this behind a confirmation.
type PushMsg struct {
	Name string
}

// CancelledMsg is sent when the panel is dismissed without an action.
type CancelledMsg struct{}

// remoteOnlyWarning explains why s, c, d, and r refuse a remote row: those
// verbs are local-only in jj.
const remoteOnlyWarning = "that is a remote bookmark, act on the local one"

// Model is the panel's state: the bookmarks it lists, the change id the log's
// cursor is on, and the cursor into the bookmarks.
type Model struct {
	skin    ui.Skin
	warning string
	target  string
	marks   []jj.Bookmark
	cursor  int
	open    bool
}

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the panel over marks, with the cursor on the first entry. Target
// is the change id the panel renders in its title, and what s and a act
// against.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(marks []jj.Bookmark, target string) tea.Cmd {
	m.marks = marks
	m.target = target
	m.warning = ""
	m.cursor = 0
	m.open = true

	return nil
}

// Close hides the panel.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the panel is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the panel is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	return m.handleKey(key)
}

func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "g", "home":
		m.jumpTo(0)
	case "G", "end":
		m.jumpTo(len(m.marks) - 1)
	case "s":
		return m.set()
	case "a":
		return m.advance()
	case "c":
		return m.createRequested()
	case "r":
		return m.renameRequested()
	case "d":
		return m.delete()
	case "f":
		return m.forget()
	case "t":
		return m.toggleTrack()
	case "p":
		return m.push()
	case "esc", "q", "b":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) set() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.warning = remoteOnlyWarning

		return nil
	}

	if m.target == "" {
		m.warning = "no revision selected"

		return nil
	}

	m.Close()

	return func() tea.Msg { return SetMsg{Name: mark.Name, Target: m.target} }
}

func (m *Model) advance() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	if m.target == "" {
		m.warning = "no revision selected"

		return nil
	}

	m.Close()

	return func() tea.Msg { return AdvanceMsg{Name: mark.Name, Target: m.target} }
}

func (m *Model) createRequested() tea.Cmd {
	m.Close()

	return func() tea.Msg { return CreateRequestedMsg{Target: m.target} }
}

func (m *Model) renameRequested() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.warning = remoteOnlyWarning

		return nil
	}

	m.Close()

	return func() tea.Msg { return RenameRequestedMsg{Name: mark.Name} }
}

func (m *Model) delete() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.warning = remoteOnlyWarning

		return nil
	}

	m.Close()

	return func() tea.Msg { return DeleteMsg{Name: mark.Name} }
}

func (m *Model) forget() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	m.Close()

	return func() tea.Msg { return ForgetMsg{Name: mark.Name} }
}

func (m *Model) toggleTrack() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	if mark.Remote == "" {
		m.warning = "tracking only applies to a remote bookmark"

		return nil
	}

	m.Close()

	if mark.Tracked {
		return func() tea.Msg { return UntrackMsg{Ref: mark.Ref()} }
	}

	return func() tea.Msg { return TrackMsg{Ref: mark.Ref()} }
}

func (m *Model) push() tea.Cmd {
	mark, ok := m.selected()
	if !ok {
		return nil
	}

	m.Close()

	return func() tea.Msg { return PushMsg{Name: mark.Name} }
}

func (m Model) selected() (jj.Bookmark, bool) {
	if m.cursor < 0 || m.cursor >= len(m.marks) {
		return jj.Bookmark{}, false
	}

	return m.marks[m.cursor], true
}

func (m *Model) moveCursor(delta int) {
	if len(m.marks) == 0 {
		return
	}

	m.jumpTo(clamp(m.cursor+delta, 0, len(m.marks)-1))
}

func (m *Model) jumpTo(cursor int) {
	if len(m.marks) == 0 {
		return
	}

	m.cursor = clamp(cursor, 0, len(m.marks)-1)
	m.warning = ""
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// scrollCenterDivisor centers the cursor in the visible window rather than
// pinning it to an edge.
const scrollCenterDivisor = 2

// scrollWindow returns the half-open range of bookmarks to render, keeping
// cursor inside it, when body rows are available for total items.
func scrollWindow(cursor, total, body int) (int, int) {
	if body <= 0 || total <= body {
		return 0, total
	}

	start := clamp(cursor-body/scrollCenterDivisor, 0, total-body)

	return start, start + body
}

// footerRows is how much of the frame a clipped list gives up to the
// scroll-position line.
const footerRows = 1

// View renders the title (with the target change id), a warning line when
// set, one row per bookmark, a scroll-position footer when clipped, and a key
// hint line, centered in a width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{m.skin.Title.Render(m.titleText())}

	if m.warning != "" {
		lines = append(lines, m.skin.Error.Render(m.warning))
	}

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "s", What: "set"},
		{Key: "a", What: "advance"},
		{Key: "c", What: "create"},
		{Key: "r", What: "rename"},
		{Key: "d", What: "delete"},
		{Key: "f", What: "forget"},
		{Key: "t", What: "track"},
		{Key: "p", What: "push"},
		{Key: "esc", What: "close"},
	})

	body := max(1, contentHeight-len(lines)-1)
	clipped := len(m.marks) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	start, end := scrollWindow(m.cursor, len(m.marks), body)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.marks[i], i == m.cursor, contentWidth))
	}

	if clipped {
		lines = append(lines, m.scrollFooter(start, end, len(m.marks), contentWidth))
	}

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

func (m Model) titleText() string {
	if m.target == "" {
		return "bookmarks"
	}

	return "bookmarks @ " + m.target
}

func (m Model) renderRow(mark jj.Bookmark, selected bool, width int) string {
	text := table.Truncate(rowText(mark), width)

	if selected {
		return m.skin.Selected.Render(text)
	}

	if mark.Conflict {
		return m.skin.Error.Render(text)
	}

	return m.skin.Body.Render(text)
}

// refWidth, changeIDWidth, and stateWidth keep the descriptions in a column, since bookmark refs
// vary and jj shortens a change id to whatever is unambiguous.
const (
	refWidth      = 20
	changeIDWidth = 8
	stateWidth    = 8
)

func rowText(mark jj.Bookmark) string {
	marker := " "
	if mark.Conflict {
		marker = "!"
	}

	return marker + " " + fmt.Sprintf("%s %s %s %s",
		table.Pad(mark.Ref(), refWidth, table.AlignLeft),
		table.Pad(mark.ChangeID, changeIDWidth, table.AlignLeft),
		table.Pad(trackState(mark), stateWidth, table.AlignLeft),
		mark.Description)
}

func trackState(mark jj.Bookmark) string {
	if mark.Ahead > 0 || mark.Behind > 0 {
		return fmt.Sprintf("↑%d ↓%d", mark.Ahead, mark.Behind)
	}

	if mark.Tracked {
		return "tracked"
	}

	return ""
}

func (m Model) scrollFooter(start, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
