// Package diffpane shows a diff inside jojo rather than handing the terminal to jj's pager. The
// rendering comes from jj-diff, so a diff looks the same in both tools, and the scrolling comes
// from listview, so it behaves like every other scoped view here.
package diffpane

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/jj-diff/render"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/ui"
)

// CancelledMsg is sent when the pane is dismissed.
type CancelledMsg struct{}

// Model is the pane's state: the diff text it was opened with, kept so a resize re-renders rather
// than re-reads, and the scroll position over the rendered lines.
type Model struct {
	skin  ui.Skin
	title string
	text  string
	// files are the rendered-line indexes where each file's header lands, so [ and ] can step the
	// cursor between them.
	files []int
	list  listview.Model[string]
	width int
	// sideBySide survives a close, so the layout chosen once holds for the next diff too.
	sideBySide bool
}

// New returns a closed pane drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[string](skin)}
}

// Open shows gitDiff, which must come from `jj diff --git`, rendered to fit width.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) Open(title, gitDiff string, width int) tea.Cmd {
	m.title, m.text, m.width = title, gitDiff, width
	m.render()

	return nil
}

// SetWidth re-renders the diff for a new terminal width, since the lines were fitted to the old one.
func (m *Model) SetWidth(width int) {
	if width == m.width || !m.list.IsOpen() {
		return
	}

	cursor := m.list.Cursor()
	m.width = width

	m.render()
	m.list.Open(m.title, m.list.Items(), cursor)
}

func (m *Model) render() {
	lines := render.Lines(m.text, render.Options{
		Width:      m.list.InnerWidth(m.width),
		Palette:    paletteFor(m.skin),
		Syntax:     true,
		WordDiff:   true,
		SideBySide: m.sideBySide,
	})

	m.list.Open(m.title, lines, 0)
	m.files = fileBoundaries(m.text, lines)
}

// gitFileRE names the new-side path of each `diff --git` header, whether git quoted the paths or
// not.
var gitFileRE = regexp.MustCompile(`(?m)^diff --git (?:a/.*?|"a/.*?") "?b/(.*?)"?$`)

// fileBoundaries returns where each file's header line lands in the rendered output, in order.
// The renderer draws one `<M|A|D|R> path` line per file before its hunks, so an in-order walk of
// the headers in the diff text finds them without re-parsing the hunks.
func fileBoundaries(gitDiff string, lines []string) []int {
	var bounds []int

	from := 0

	for _, match := range gitFileRE.FindAllStringSubmatch(gitDiff, -1) {
		for i := from; i < len(lines); i++ {
			if fileHeader(ansi.Strip(lines[i]), match[1]) {
				bounds = append(bounds, i)
				from = i + 1

				break
			}
		}
	}

	return bounds
}

func fileHeader(stripped, path string) bool {
	return len(stripped) >= 2 &&
		strings.ContainsRune("MADR?", rune(stripped[0])) &&
		stripped[1] == ' ' &&
		strings.HasPrefix(path, stripped[2:])
}

// Close hides the pane.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the pane is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the pane is open.
//
//nolint:unparam // matches the other panels' Update signature; the caller stays uniform.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	name := key.String()
	if m.list.MoveKey(name) {
		return nil
	}

	switch name {
	case "[":
		m.stepFile(-1)
	case "]":
		m.stepFile(1)
	case "s":
		cursor := m.list.Cursor()
		m.sideBySide = !m.sideBySide

		m.render()
		m.list.Open(m.title, m.list.Items(), cursor)
	case "esc", "q":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

// stepFile moves the cursor to the header of the next or previous file, clamped at both ends.
func (m *Model) stepFile(dir int) {
	if len(m.files) == 0 {
		return
	}

	cursor := m.list.Cursor()
	target := m.files[0]

	if dir > 0 {
		target = m.files[len(m.files)-1]

		for _, file := range m.files {
			if file > cursor {
				target = file

				break
			}
		}
	} else {
		for i := len(m.files) - 1; i >= 0; i-- {
			if m.files[i] < cursor {
				target = m.files[i]

				break
			}
		}
	}

	m.list.Jump(target)
}

// View renders the diff centered in a width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "j k", What: "scroll"},
		{Key: "[ ]", What: "file"},
		{Key: "s", What: "side by side"},
		{Key: "esc", What: "close"},
	})
}

func (m Model) row(line string, selected bool) listview.Row {
	if selected {
		return listview.Row{Text: line, Style: m.skin.Selected}
	}

	return listview.Row{Text: line}
}

func paletteFor(s ui.Skin) render.Palette {
	p := s.Palette

	return render.Palette{
		File:        p.Mauve,
		Hunk:        p.Overlay1,
		Added:       p.Green,
		Removed:     p.Red,
		Context:     p.Text,
		AddedWord:   p.Surface1,
		RemovedWord: p.Surface1,
		Gutter:      p.Overlay0,
		Syntax: render.SyntaxPalette{
			Comment:  p.Overlay1,
			Keyword:  p.Mauve,
			String:   p.Green,
			Number:   p.Peach,
			Name:     p.Blue,
			Type:     p.Yellow,
			Operator: p.Sky,
		},
	}
}
