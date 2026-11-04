// Package bookmarkview is the panel listing `jj bookmark list` entries,
// letting the user set, advance, create, rename, delete, forget, track,
// untrack, or push a bookmark. The parent passes the rows it already has and
// performs the jj calls; this package never calls jj.
package bookmarkview

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/listview"
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

// ResolveMsg is sent when the user asks to settle a conflicted bookmark onto
// Ref, which is the remote-tracking side: it is the version that already
// exported, so it is the right default when the conflict is a local rewrite
// after a push.
type ResolveMsg struct {
	Name string
	Ref  string
}

// CancelledMsg is sent when the panel is dismissed without an action.
type CancelledMsg struct{}

// remoteOnlyWarning explains why s, c, d, and r refuse a remote row: those
// verbs are local-only in jj.
const remoteOnlyWarning = "that is a remote bookmark, act on the local one"

// Model is the panel's state: the bookmarks it lists, the change id the log's
// cursor is on, and the cursor into the bookmarks.
type Model struct {
	skin   ui.Skin
	target string
	list   listview.Model[jj.Bookmark]
}

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[jj.Bookmark](skin)}
}

// Open shows the panel over marks, with the cursor on the first entry. Target
// is the change id the panel renders in its title, and what s and a act
// against.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(marks []jj.Bookmark, target string) tea.Cmd {
	m.target = target
	m.list.Open(titleText(target), marks, 0)

	return nil
}

// Close hides the panel.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the panel is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the panel is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	return m.handleKey(key)
}

func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	name := key.String()
	if m.list.MoveKey(name) {
		return nil
	}

	switch name {
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
	case "x":
		return m.resolve()
	case "esc", "q", "b":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) set() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.list.Warn(remoteOnlyWarning)

		return nil
	}

	if m.target == "" {
		m.list.Warn("no revision selected")

		return nil
	}

	m.Close()

	return func() tea.Msg { return SetMsg{Name: mark.Name, Target: m.target} }
}

func (m *Model) advance() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if m.target == "" {
		m.list.Warn("no revision selected")

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
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.list.Warn(remoteOnlyWarning)

		return nil
	}

	m.Close()

	return func() tea.Msg { return RenameRequestedMsg{Name: mark.Name} }
}

func (m *Model) delete() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.list.Warn(remoteOnlyWarning)

		return nil
	}

	m.Close()

	return func() tea.Msg { return DeleteMsg{Name: mark.Name} }
}

func (m *Model) forget() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	m.Close()

	return func() tea.Msg { return ForgetMsg{Name: mark.Name} }
}

func (m *Model) toggleTrack() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if mark.Remote == "" {
		m.list.Warn("tracking only applies to a remote bookmark")

		return nil
	}

	m.Close()

	if mark.Tracked {
		return func() tea.Msg { return UntrackMsg{Ref: mark.Ref()} }
	}

	return func() tea.Msg { return TrackMsg{Ref: mark.Ref()} }
}

func (m *Model) push() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	m.Close()

	return func() tea.Msg { return PushMsg{Name: mark.Name} }
}

// resolve settles a conflicted bookmark on its remote side, which only exists when a remote row
// carries the same name. A tracked remote wins over an untracked one, since tracking is the pair jj
// already treats as the bookmark's other half.
func (m *Model) resolve() tea.Cmd {
	mark, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if mark.Remote != "" {
		m.list.Warn(remoteOnlyWarning)

		return nil
	}

	if !mark.Conflict {
		m.list.Warn("only a conflicted bookmark resolves")

		return nil
	}

	remote, found := "", false
	for _, other := range m.list.Items() {
		if other.Name == mark.Name && other.Remote != "" {
			remote, found = other.Remote, true
			if other.Tracked {
				break
			}
		}
	}

	if !found {
		m.list.Warn("no remote side to resolve to")

		return nil
	}

	m.Close()

	return func() tea.Msg { return ResolveMsg{Name: mark.Name, Ref: mark.Name + "@" + remote} }
}

// View renders the title (with the target change id), a warning line when
// set, one row per bookmark, a scroll-position footer when clipped, and a key
// hint line, centered in a width by height frame.
func (m Model) View(width, height int) string {
	hints := []keyhint.Hint{
		{Key: "s", What: "set"},
		{Key: "a", What: "advance"},
		{Key: "c", What: "create"},
		{Key: "r", What: "rename"},
		{Key: "d", What: "delete"},
		{Key: "f", What: "forget"},
		{Key: "t", What: "track"},
		{Key: "p", What: "push"},
	}

	// Resolve only earns its hint when a row can take it.
	for _, mark := range m.list.Items() {
		if mark.Conflict && mark.Remote == "" {
			hints = append(hints, keyhint.Hint{Key: "x", What: "resolve"})

			break
		}
	}

	return m.list.View(width, height, m.row, append(hints, keyhint.Hint{Key: "esc", What: "close"}))
}

func titleText(target string) string {
	if target == "" {
		return "bookmarks"
	}

	return "bookmarks @ " + target
}

func (m Model) row(mark jj.Bookmark, selected bool) listview.Row {
	style := m.skin.Body

	if mark.Conflict {
		style = m.skin.Error
	}

	if selected {
		style = m.skin.Selected
	}

	return listview.Row{Text: rowText(mark), Style: style}
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
