// Package diffpane shows a diff inside jojo rather than handing the terminal to jj's pager. The
// rendering comes from jj-diff, so a diff looks the same in both tools, and the scrolling comes
// from listview, so it behaves like every other scoped view here.
package diffpane

import (
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"
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
	// cursor between them, and paths names the file each one heads.
	files []int
	paths []string
	list  listview.Model[string]
	width int
	// sideBySide survives a close, so the layout chosen once holds for the next diff too.
	sideBySide bool
	// docked draws the pane flat beside the log rather than framed over it, so its lines take the
	// whole width it is given.
	docked bool
}

// New returns a closed pane drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[string](skin)}
}

// NewDocked returns a closed pane that draws with DockView, beside the log rather than over it.
func NewDocked(skin ui.Skin) Model {
	m := New(skin)
	m.docked = true

	return m
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
	width := m.list.InnerWidth(m.width)
	if m.docked {
		width = m.width
	}

	opts := render.Options{
		Width:      width,
		Palette:    paletteFor(m.skin),
		Syntax:     true,
		WordDiff:   true,
		SideBySide: m.sideBySide,
		SignBar:    m.skin.SignBar(),
		HunkLabels: true,
	}

	var lines []string

	m.files, m.paths = nil, nil

	for _, seg := range segments(m.text) {
		if !seg.diff {
			for _, line := range seg.lines {
				lines = append(lines, m.prose(line, width))
			}

			continue
		}

		text := strings.Join(seg.lines, "\n") + "\n"
		rendered := render.Lines(text, opts)
		bounds, paths := fileHeaders(text, rendered)

		for _, bound := range bounds {
			m.files = append(m.files, bound+len(lines))
		}

		m.paths = append(m.paths, paths...)
		lines = append(lines, rendered...)
	}

	m.list.Open(m.title, lines, 0)
}

// segment is a run of lines that is either a git diff or the prose jj writes around one, which is
// what `jj op show -p` interleaves: a header, then each changed commit followed by its patch.
type segment struct {
	lines []string
	diff  bool
}

// diffLinePrefixes are how every line inside a git diff starts, so the first line without one is
// where jj's prose resumes.
var diffLinePrefixes = []string{
	"diff --git ", "index ", "--- ", "+++ ", "@@", " ", "+", "-", "\\", "new file", "deleted file",
	"old mode", "new mode", "similarity", "rename ", "copy ", "Binary files",
}

// segments splits text into diffs and the prose between them. A plain diff is one segment, so a
// revision's diff renders exactly as before.
func segments(text string) []segment {
	var out []segment

	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		diff := strings.HasPrefix(line, "diff --git ") ||
			(len(out) > 0 && out[len(out)-1].diff && inDiff(line))

		if len(out) == 0 || out[len(out)-1].diff != diff {
			out = append(out, segment{diff: diff})
		}

		out[len(out)-1].lines = append(out[len(out)-1].lines, line)
	}

	if len(out) == 1 && !out[0].diff && strings.TrimSpace(text) == "" {
		return nil
	}

	return out
}

func inDiff(line string) bool {
	for _, prefix := range diffLinePrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}

	return false
}

// prose draws a line jj wrote around a diff: a section heading, a commit added or removed by the
// operation, or anything else as body text.
func (m Model) prose(line string, width int) string {
	text := table.Truncate(line, width)

	switch {
	case strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " "):
		return m.skin.Heading.Render(text)
	case strings.HasPrefix(line, "+ "):
		return m.skin.Added.Render(text)
	case strings.HasPrefix(line, "- "):
		return m.skin.Removed.Render(text)
	}

	return m.skin.Body.Render(text)
}

// Show replaces the diff and lands on the header of keep when the new diff touches that file, else on
// the first file. It is how the docked pane follows the log without losing the reader's place.
func (m *Model) Show(title, gitDiff string, width int, keep string) {
	m.title, m.text, m.width = title, gitDiff, width
	m.render()

	if i := slices.Index(m.paths, keep); keep != "" && i >= 0 {
		m.list.Jump(m.files[i])

		return
	}

	if len(m.files) > 0 {
		m.list.Jump(m.files[0])
	}
}

// CurrentPath names the file the cursor is in, "" before the first header or with nothing open.
func (m Model) CurrentPath() string {
	if !m.list.IsOpen() {
		return ""
	}

	cursor, path := m.list.Cursor(), ""

	for i, bound := range m.files {
		if bound > cursor {
			break
		}

		path = m.paths[i]
	}

	return path
}

// gitFileRE names the new-side path of each `diff --git` header, whether git quoted the paths or
// not.
var gitFileRE = regexp.MustCompile(`(?m)^diff --git (?:a/.*?|"a/.*?") "?b/(.*?)"?$`)

// fileBoundaries returns where each file's header line lands in the rendered output, in order.
// The renderer draws one `<M|A|D|R> path` line per file before its hunks, so an in-order walk of
// the headers in the diff text finds them without re-parsing the hunks.
func fileBoundaries(gitDiff string, lines []string) []int {
	bounds, _ := fileHeaders(gitDiff, lines)

	return bounds
}

// fileHeaders is fileBoundaries with the path each boundary heads.
func fileHeaders(gitDiff string, lines []string) ([]int, []string) {
	var (
		bounds []int
		paths  []string
	)

	from := 0

	for _, match := range gitFileRE.FindAllStringSubmatch(gitDiff, -1) {
		for i := from; i < len(lines); i++ {
			if fileHeader(ansi.Strip(lines[i]), match[1]) {
				bounds = append(bounds, i)
				paths = append(paths, match[1])
				from = i + 1

				break
			}
		}
	}

	return bounds, paths
}

func fileHeader(stripped, path string) bool {
	return len(stripped) >= 2 &&
		strings.ContainsRune("MADR?", rune(stripped[0])) &&
		stripped[1] == ' ' &&
		strings.HasPrefix(path, stripped[2:])
}

// Title is what the pane was last opened under.
func (m Model) Title() string { return m.list.Title() }

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

// dockRule is the column the docked pane gives up to separate itself from the log.
const dockRule = "│ "

// DockWidth is how many cells a diff line gets in a docked pane width cells wide.
func DockWidth(width int) int {
	return max(0, width-ansi.StringWidth(dockRule))
}

// DockView draws the pane flat into exactly height lines of width cells: a title, then the lines
// around the cursor. The cursor row is only marked while the pane has the keys, since an unfocused
// highlight would read as a second cursor beside the log's.
func (m Model) DockView(width, height int, focused bool) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	inner := DockWidth(width)
	rule := m.skin.Muted.Render(dockRule)

	titleStyle := m.skin.Muted
	if focused {
		titleStyle = m.skin.Title
	}

	lines := []string{rule + titleStyle.Render(table.Truncate(m.title, inner))}
	items := m.list.Items()

	if len(items) == 0 {
		lines = append(lines, rule+m.skin.Muted.Render(table.Truncate("nothing changed", inner)))
	}

	body := height - 1
	start := min(max(0, m.list.Cursor()-body/2), max(0, len(items)-body))

	for i := start; i < len(items) && len(lines) < height; i++ {
		line := table.Truncate(items[i], inner)
		if focused && i == m.list.Cursor() {
			line = m.skin.Selected.Render(ansi.Strip(line))
		}

		lines = append(lines, rule+line)
	}

	for len(lines) < height {
		lines = append(lines, rule)
	}

	return strings.Join(lines, "\n")
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
		AddedWash:   s.Diff.AddWash,
		RemovedWash: s.Diff.RemoveWash,
		AddedWord:   s.Diff.AddMark,
		RemovedWord: s.Diff.RemoveMark,
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
