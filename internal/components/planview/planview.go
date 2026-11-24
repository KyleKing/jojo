// Package planview draws a stack submit's plan before anything runs: a row per change saying what
// its pull request will get (new, update, retarget, merged), the bookmark it goes up under, and the
// base it targets, then what the reader should know, then the exact jj and gh commands Enter runs.
// The parent owns the plan and every key; this package only draws it and scrolls.
package planview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/empty"
	"github.com/kyleking/aragonite/tui/preview"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/stack"
	"github.com/kyleking/jojo/internal/ui"
)

// Column keys.
const (
	colChange   = "change"
	colAction   = "action"
	colPR       = "pr"
	colBookmark = "bookmark"
	colBase     = "base"
	colSubject  = "subject"
)

// Column bounds, in display cells.
const (
	changeWidth = 8
	actionWidth = 10
	prMax       = 9
	refMin      = 8
	refMax      = 24
	subjectMin  = 10
	// The share of spare cells: the subject reads best, the refs next.
	subjectWeight = 3
	refWeight     = 2
)

// Collapse order: the change, what happens to it, and where it goes hold longest.
const (
	prioPR = iota + 1
	prioSubject
	prioBookmark
)

// Model is the plan on screen.
type Model struct {
	err     error
	plan    *stack.Plan
	skin    ui.Skin
	top     int
	width   int
	height  int
	loading bool
	running bool
}

// New is an empty plan view.
func New(skin ui.Skin) Model { return Model{skin: skin} }

// SetLoading shows that the plan is being read.
func (m *Model) SetLoading() { m.plan, m.err, m.loading, m.running, m.top = nil, nil, true, false, 0 }

// SetPlan shows plan.
func (m *Model) SetPlan(plan stack.Plan) {
	m.plan, m.err, m.loading, m.running = &plan, nil, false, false
}

// SetErr shows why the plan could not be read or run.
func (m *Model) SetErr(err error) { m.err, m.loading, m.running = err, false, false }

// SetRunning marks the plan as running.
func (m *Model) SetRunning(running bool) { m.running = running }

// Plan is the plan on screen, nil while it is read or when it failed.
func (m Model) Plan() *stack.Plan { return m.plan }

// Running reports a plan being carried out.
func (m Model) Running() bool { return m.running }

// Loading reports a plan still being read.
func (m Model) Loading() bool { return m.loading }

// Err is why the plan is not on screen.
func (m Model) Err() error { return m.err }

// SetSize sets the room the plan draws in.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.clamp()
}

// Scroll moves the view by delta lines, reporting whether it moved.
func (m *Model) Scroll(delta int) bool {
	before := m.top
	m.top += delta
	m.clamp()

	return m.top != before
}

// ScrollTo moves to the top (0) or, with a negative line, the bottom.
func (m *Model) ScrollTo(line int) {
	m.top = line
	if line < 0 {
		m.top = len(m.body())
	}

	m.clamp()
}

func (m *Model) clamp() {
	m.top = max(0, min(m.top, len(m.body())-m.bodyHeight()))
}

func (m Model) bodyHeight() int { return max(0, m.height-1) }

// View draws the title and the body into exactly the view's height.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	lines := []string{m.titleLine()}

	switch {
	case m.err != nil && m.plan == nil:
		card := empty.Empty{Title: "No plan", Why: oneLine(m.err.Error())}

		return strings.Join(append(lines, empty.Card(m.skin.Skin, card, m.width, m.height-1)...), "\n")
	case m.loading:
		card := empty.Empty{
			Title: "Reading the stack",
			Why:   "jj's descriptions and trunk, then gh pr view for each bookmark; nothing is written yet.",
		}

		return strings.Join(append(lines, empty.Card(m.skin.Skin, card, m.width, m.height-1)...), "\n")
	}

	body := m.body()
	end := min(len(body), m.top+m.bodyHeight())
	lines = append(lines, body[m.top:end]...)

	return strings.Join(preview.Pad(lines, m.height, m.width), "\n")
}

// titleLine names what Enter does: "Submit · 3 pull requests onto main · draft".
func (m Model) titleLine() string {
	title := m.skin.Title.Render("Submit")

	var rest string

	if p := m.plan; p != nil {
		rest = " · " + strconv.Itoa(len(p.Open())) + " pull requests onto " + p.Trunk
		if p.Draft {
			rest += " · draft"
		}

		if more := len(m.body()) - m.top - m.bodyHeight(); more > 0 {
			rest += " · " + strconv.Itoa(more) + " more ↓"
		}
	}

	room := m.width - lipgloss.Width(title)
	if room <= 0 {
		return table.Truncate("Submit", m.width)
	}

	return title + m.skin.Muted.Render(table.Truncate(rest, room))
}

// body is every line under the title: the steps, what was left out, the notes, and the commands.
func (m Model) body() []string {
	p := m.plan
	if p == nil {
		return nil
	}

	layout := m.layout()
	lines := []string{strings.Repeat(" ", table.Gutter) + m.skin.Muted.Render(table.Header(layout))}

	for k := range p.Steps {
		s := &p.Steps[k]
		lines = append(lines, m.step(s, layout))
	}

	for _, c := range p.Skipped {
		left := "  " + c.ID + " left out: the empty working copy on top"
		lines = append(lines, m.skin.Muted.Render(table.Truncate(left, m.width)))
	}

	styles := preview.SkinStyles(m.skin.Skin)

	if len(p.Notes) > 0 {
		lines = append(lines, "", preview.LabelRule(styles, "Before you submit", m.width))
		for _, n := range p.Notes {
			for j, l := range preview.Wrap(m.skin.Warning, n, m.width-len("• ")) {
				lead := "  "
				if j == 0 {
					lead = m.skin.Warning.Render("• ")
				}

				lines = append(lines, lead+l)
			}
		}
	}

	lines = append(lines, "", preview.LabelRule(styles, "Runs · "+strconv.Itoa(len(p.Commands)), m.width))

	if len(p.Commands) == 0 {
		return append(lines, m.skin.Muted.Render("  nothing: every pull request is up to date"))
	}

	for _, c := range p.Commands {
		lines = append(lines, m.command(c.Line())...)
	}

	return lines
}

// command is one command of the plan, wrapped rather than cut, since the point of the plan is that
// the reader sees exactly what will run: the tool in the accent, continuation lines indented under it.
func (m Model) command(line string) []string {
	const lead, hang = "  ", "      "

	tool, rest, _ := strings.Cut(line, " ")
	wrapped := wrapWords(rest, max(1, m.width-len(lead+tool+" ")), max(1, m.width-len(hang)))
	out := make([]string, 0, len(wrapped))
	out = append(out, lead+m.skin.Accent.Render(tool)+" "+wrapped[0])

	for _, l := range wrapped[1:] {
		out = append(out, hang+l)
	}

	return out
}

// wrapWords breaks text at spaces only, first to width first and then to width rest, since a
// command line broken at a hyphen ("--" then "bookmark") misreads. A word longer than a line is cut.
func wrapWords(text string, first, rest int) []string {
	var (
		out  []string
		line string
	)

	width := first

	for _, word := range strings.Split(text, " ") {
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
			line += " " + word
		default:
			out = append(out, table.Truncate(line, width))
			line, width = word, rest
		}
	}

	return append(out, table.Truncate(line, width))
}

func (m Model) layout() table.Layout {
	widest := func(text func(*stack.Step) string) int {
		n := 0
		for k := range m.plan.Steps {
			s := &m.plan.Steps[k]
			n = max(n, lipgloss.Width(text(s)))
		}

		return n
	}

	bound := func(least, most, w int) (int, int) { return min(least, w), max(min(most, w), min(least, w)) }
	prLo, prHi := bound(prMax, prMax, max(widest(prText), len("pr")))
	bmLo, bmHi := bound(refMin, refMax, widest(bookmarkText))
	baseLo, baseHi := bound(refMin, refMax, widest(baseText))

	return table.Fit([]table.Column{
		{Key: colChange, Title: "change", Min: changeWidth, Max: changeWidth},
		{Key: colAction, Title: "action", Min: actionWidth, Max: actionWidth},
		{Key: colPR, Title: "pr", Min: prLo, Max: prHi, Weight: 1, Priority: prioPR},
		{Key: colBookmark, Title: "bookmark", Min: bmLo, Max: bmHi, Weight: refWeight, Priority: prioBookmark},
		{Key: colBase, Title: "base", Min: baseLo, Max: baseHi, Weight: refWeight},
		{Key: colSubject, Title: "subject", Min: subjectMin, Weight: subjectWeight, Priority: prioSubject},
	}, m.width-table.Gutter)
}

func prText(s *stack.Step) string {
	if s.PR == nil || s.Action == stack.Create {
		return "—"
	}

	text := "#" + strconv.Itoa(s.PR.Number)
	if s.PR.IsDraft && s.PR.State == gh.StateOpen {
		text += " draft"
	}

	return text
}

func bookmarkText(s *stack.Step) string {
	if s.NewBookmark {
		return "+" + s.Bookmark
	}

	return s.Bookmark
}

func baseText(s *stack.Step) string {
	if s.Action == stack.Retarget {
		return "→ " + s.Base + " (was " + s.WasBase + ")"
	}

	return "→ " + s.Base
}

func (m Model) step(s *stack.Step, layout table.Layout) string {
	face := map[stack.Action]lipgloss.Style{
		stack.Create: m.skin.Success, stack.Update: m.skin.Accent, stack.Retarget: m.skin.Warning,
		stack.Unchanged: m.skin.Muted, stack.Merged: m.skin.Muted,
	}[s.Action]

	text := map[string]table.Cell{
		colChange:   {Text: s.Change.ID, Style: m.skin.ChangeIDPrefix},
		colAction:   {Text: s.Action.String(), Style: face},
		colPR:       {Text: prText(s), Style: m.skin.Muted},
		colBookmark: {Text: bookmarkText(s), Style: m.skin.Bookmark},
		colBase:     {Text: baseText(s), Style: m.skin.Muted},
		colSubject:  {Text: s.Change.Subject},
	}

	cells := make([]table.Cell, 0, len(layout.Columns))
	for _, col := range layout.Columns {
		cells = append(cells, text[col.Key])
	}

	return strings.Repeat(" ", table.Gutter) + table.RowCells(layout, cells, lipgloss.NewStyle())
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
