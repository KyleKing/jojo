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

// MotionMsg is a ] or [ motion over an object the pane does not hold itself: r steps the change's
// versions and ] or [ (doubled, as in ]]) its chapter in the outline, which the parent owns. Dir is
// 1 forward and -1 back.
type MotionMsg struct {
	Object string
	Dir    int
}

// Objects of the ] and [ motions.
const (
	ObjectHunk    = "h"
	ObjectFile    = "f"
	ObjectRound   = "r"
	ObjectChapter = "]"
)

// Model is the pane's state: the diff text it was opened with, kept so a resize re-renders rather
// than re-reads, and the scroll position over the rendered lines.
type Model struct {
	skin  ui.Skin
	title string
	text  string
	// files are the rendered-line indexes where each file's header lands, so [ and ] can step the
	// cursor between them, and paths names the file each one heads. hunks are where each hunk's label
	// lands, in the same order.
	files []int
	paths []string
	hunks []int
	// last is the motion n repeats, and hints the keys the framed pane's footer offers.
	last  motion
	hints []keyhint.Hint
	// key is what the rows on show were drawn from, and plain that they were drawn without syntax
	// colors, which Colorize then asks for. colors keeps the colored diffs this pane has drawn.
	colors *colorCache
	key    drawKey
	list   listview.Model[string]
	width  int
	// sideBySide survives a close, so the layout chosen once holds for the next diff too.
	sideBySide bool
	// docked draws the pane flat beside the log rather than framed over it, so its lines take the
	// whole width it is given.
	docked bool
	plain  bool
}

// New returns a closed pane drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[string](skin), colors: newColorCache()}
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

// render draws the diff for the pane's width: in color when a colored copy is already kept, and
// otherwise plain, leaving the colors to Colorize so the update loop never waits on a lexer.
func (m *Model) render() {
	p := m.Painter(m.width)
	key := p.withText(m.text)

	d, ok := m.colors.get(key)
	if !ok {
		var drawnOff bool
		if d, drawnOff = m.colors.take(key); !drawnOff {
			d = p.draw(key, false)
		}
	}

	// A diff with no file in it has no code to color, only jj's prose.
	m.key, m.plain = key, !ok && len(d.files) > 0
	m.files, m.paths, m.hunks = d.files, d.paths, d.hunks
	m.list.Open(m.title, d.lines, 0)
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
func prose(skin ui.Skin, line string, width int) string {
	text := table.Truncate(line, width)

	switch {
	case strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " "):
		return skin.Heading.Render(text)
	case strings.HasPrefix(line, "+ "):
		return skin.Added.Render(text)
	case strings.HasPrefix(line, "- "):
		return skin.Removed.Render(text)
	}

	return skin.Body.Render(text)
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

// Lines is the diff as rendered, one entry per line.
func (m Model) Lines() []string { return m.list.Items() }

// Title is what the pane was last opened under.
func (m Model) Title() string { return m.list.Title() }

// Close hides the pane.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the pane is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the pane is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	if m.list.MoveKey(key.String()) {
		return nil
	}

	switch key.String() {
	case "esc", "q":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

// motion is one ] or [ step over an object, which n repeats and N reverses.
type motion struct {
	object string
	dir    int
}

// Motion steps over object (ObjectHunk, ObjectFile, ObjectRound, ObjectChapter) in dir, 1 forward and
// -1 back. An object the pane holds moves its cursor; one the parent holds goes up as a MotionMsg.
// The keys that press it are the caller's: the shared keymap's ]h, [f, and the rest.
func (m *Model) Motion(object string, dir int) tea.Cmd {
	m.last = motion{object: object, dir: dir}

	return m.move(m.last)
}

// ToggleSideBySide switches between the unified and the side-by-side layout, keeping the cursor.
func (m *Model) ToggleSideBySide() {
	cursor := m.list.Cursor()
	m.sideBySide = !m.sideBySide

	m.render()
	m.list.Open(m.title, m.list.Items(), cursor)
}

// SetHints is the footer the framed pane draws, which the caller builds from its keymap so a rebind
// reaches it.
func (m *Model) SetHints(hints []keyhint.Hint) { m.hints = hints }

// Repeat steps the last motion again, reversed for a negative dir, and steps hunks before any
// motion has been made, so n reads through a diff from the start.
func (m *Model) Repeat(dir int) tea.Cmd {
	last := m.last
	if last.object == "" {
		last = motion{object: ObjectHunk, dir: 1}
	}

	last.dir *= dir

	return m.move(last)
}

func (m *Model) move(step motion) tea.Cmd {
	switch step.object {
	case ObjectHunk:
		m.StepHunk(step.dir)
	case ObjectFile:
		m.stepFile(step.dir)
	default:
		return func() tea.Msg { return MotionMsg{Object: step.object, Dir: step.dir} }
	}

	return nil
}

// StepHunk moves the cursor to the next or previous hunk's label, clamped at both ends, and is how
// the outline beside a docked pane reads through it without taking the focus.
func (m *Model) StepHunk(dir int) {
	m.last = motion{object: ObjectHunk, dir: dir}
	m.list.Jump(step(m.hunks, m.list.Cursor(), dir))
}

// step is the first mark past cursor in dir, or the last one that way when none is past it.
func step(marks []int, cursor, dir int) int {
	if len(marks) == 0 {
		return cursor
	}

	if dir > 0 {
		for _, mark := range marks {
			if mark > cursor {
				return mark
			}
		}

		return marks[len(marks)-1]
	}

	for i := len(marks) - 1; i >= 0; i-- {
		if marks[i] < cursor {
			return marks[i]
		}
	}

	return marks[0]
}

// hunkLabelRE is the line range the renderer ends a hunk's label with ("func a  L1–3", "L10–12").
var hunkLabelRE = regexp.MustCompile(`(?:^| {2})L\d+(?:–\d+)?$`)

// hunkLabels is where each hunk's label lands in rendered, searched only after a file's header so a
// line of code that happens to end like a label is never taken for one: a diff line starts with its
// sign or its bar, and a label never does.
func hunkLabels(rendered []string, files []int) []int {
	var out []int

	if len(files) == 0 {
		return nil
	}

	for i := files[0]; i < len(rendered); i++ {
		line := ansi.Strip(rendered[i])
		if line == "" || strings.ContainsRune(" +-│▎▌▐┃", firstRune(line)) {
			continue
		}

		if hunkLabelRE.MatchString(strings.TrimRight(line, " ")) {
			out = append(out, i)
		}
	}

	return out
}

// stepFile moves the cursor to the header of the next or previous file, clamped at both ends.
func (m *Model) stepFile(dir int) {
	m.list.Jump(step(m.files, m.list.Cursor(), dir))
}

// View renders the diff centered in a width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, m.hints)
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

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}

	return 0
}
