// Package revsetbox is the log view's revset expression box: autocomplete over revset
// functions and repo aliases, signature help inside a call, and a debounced ChangedMsg.
package revsetbox

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// debounceInterval is how long the box waits after the last keystroke before emitting
// ChangedMsg, since a revset is invalid mid-edit most of the time it's being typed.
const debounceInterval = 300 * time.Millisecond

const maxCandidates = 8

// AliasSource fetches the repo's revset aliases (as bare names, e.g. "trunk", not
// "trunk()"), such as by running `jj config list --include-defaults revset-aliases`.
type AliasSource func(ctx context.Context) ([]string, error)

// ChangedMsg is sent, debounced, whenever the revset text settles after an edit or is
// submitted with Enter while balanced. The parent re-runs its log query with Revset.
type ChangedMsg struct {
	Revset string
}

type aliasesLoadedMsg struct {
	err     error
	aliases []string
}

type debounceTickMsg struct {
	generation int
}

// Option configures a Model at construction.
type Option func(*Model)

// WithAliases sets the source the box queries once, on first Focus, for the repo's
// revset aliases. Without it, or if the source errors, completion falls back to the
// built-in function table alone.
func WithAliases(source AliasSource) Option {
	return func(m *Model) { m.aliasSource = source }
}

// Model is the revset box's state: the wrapped textinput, the completion popup, the
// signature-help line, and the last error the parent reported via SetError.
type Model struct {
	err              error
	aliasSource      AliasSource
	signature        string
	aliases          []string
	candidates       []string
	input            textinput.Model
	selectedIdx      int
	debounceGen      int
	aliasesRequested bool
	completionOpen   bool
}

// New returns a box preloaded with initial (typically revsets.log's value), cursor at
// the end.
func New(initial string, opts ...Option) Model {
	input := textinput.New()
	input.Prompt = "r: "
	input.SetValue(initial)
	input.CursorEnd()

	m := Model{input: input}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// Value returns the current revset text.
func (m Model) Value() string {
	return m.input.Value()
}

// SetValue replaces the revset text, moving the cursor to the end and recomputing
// completion and signature-help state.
func (m *Model) SetValue(v string) {
	m.input.SetValue(v)
	m.input.CursorEnd()
	m.refresh()
}

// SetError records a validation error from the parent, rendered dimmed under the box
// without disturbing the value. Pass nil to clear it.
func (m *Model) SetError(err error) {
	m.err = err
}

// Focus gives the box keyboard focus and, the first time, kicks off the alias fetch.
func (m *Model) Focus() tea.Cmd {
	cmd := m.input.Focus()

	if m.aliasSource == nil || m.aliasesRequested {
		return cmd
	}

	m.aliasesRequested = true
	source := m.aliasSource

	fetch := func() tea.Msg {
		aliases, err := source(context.Background())

		return aliasesLoadedMsg{aliases: aliases, err: err}
	}

	return tea.Batch(cmd, fetch)
}

// Blur removes keyboard focus and closes the completion popup.
func (m *Model) Blur() {
	m.input.Blur()
	m.closeCompletion()
}

// Focused reports whether the box has keyboard focus.
func (m Model) Focused() bool {
	return m.input.Focused()
}

// Update handles one message. Key handling only runs while the box is focused.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case aliasesLoadedMsg:
		if msg.err == nil {
			m.aliases = msg.aliases
		}

		return nil
	case debounceTickMsg:
		if msg.generation != m.debounceGen {
			return nil
		}

		revset := m.input.Value()

		return func() tea.Msg { return ChangedMsg{Revset: revset} }
	case tea.KeyPressMsg:
		if !m.Focused() {
			return nil
		}

		return m.handleKey(msg)
	default:
		return nil
	}
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.closeCompletion()

		return nil
	case "enter":
		return m.submit()
	case "tab":
		if m.completionOpen {
			m.acceptCandidate()

			return m.scheduleDebounce()
		}

		return nil
	case "shift+tab":
		if m.completionOpen {
			m.moveCandidate(-1)
		}

		return nil
	case "up", "down":
		if m.completionOpen {
			delta := 1
			if msg.String() == "up" {
				delta = -1
			}

			m.moveCandidate(delta)

			return nil
		}
	}

	before := m.input.Value()

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)
	m.refresh()

	if m.input.Value() == before {
		return cmd
	}

	return tea.Batch(cmd, m.scheduleDebounce())
}

// submit emits ChangedMsg immediately, bypassing the debounce, when the expression is
// balanced. An unbalanced one is ordinary mid-typing state, not an error to surface.
func (m *Model) submit() tea.Cmd {
	if !Balanced(m.input.Value()) {
		return nil
	}

	m.debounceGen++

	revset := m.input.Value()

	return func() tea.Msg { return ChangedMsg{Revset: revset} }
}

func (m *Model) scheduleDebounce() tea.Cmd {
	m.debounceGen++
	generation := m.debounceGen

	return tea.Tick(debounceInterval, func(time.Time) tea.Msg {
		return debounceTickMsg{generation: generation}
	})
}

func (m *Model) refresh() {
	runes := []rune(m.input.Value())
	pos := m.input.Position()

	m.refreshCompletion(runes, pos)
	m.refreshSignature(runes, pos)
}

func (m *Model) refreshCompletion(runes []rune, pos int) {
	if inQuote(runes, pos) {
		m.closeCompletion()

		return
	}

	start, end := identifierRange(runes, pos)
	if start == end {
		m.closeCompletion()

		return
	}

	matches := candidates(string(runes[start:end]), m.aliases)
	if len(matches) == 0 {
		m.closeCompletion()

		return
	}

	if !equalStrings(matches, m.candidates) {
		m.selectedIdx = 0
	}

	m.candidates = matches
	m.completionOpen = true
}

func (m *Model) refreshSignature(runes []rune, pos int) {
	name, ok := enclosingCall(runes, pos)
	if !ok {
		m.signature = ""

		return
	}

	help, ok := signatureHelp(name, m.aliases)
	if !ok {
		m.signature = ""

		return
	}

	m.signature = help
}

func (m *Model) moveCandidate(delta int) {
	n := len(m.candidates)
	if n == 0 {
		return
	}

	m.selectedIdx = ((m.selectedIdx+delta)%n + n) % n
}

func (m *Model) closeCompletion() {
	m.completionOpen = false
	m.candidates = nil
	m.selectedIdx = 0
}

// acceptCandidate replaces the identifier under the cursor with the highlighted
// candidate's call form, e.g. `ancestors(mi` -> `ancestors(mine()`, cursor inside.
func (m *Model) acceptCandidate() {
	if !m.completionOpen || m.selectedIdx >= len(m.candidates) {
		return
	}

	name := m.candidates[m.selectedIdx]
	runes := []rune(m.input.Value())
	start, end := identifierRange(runes, m.input.Position())

	var b strings.Builder
	b.WriteString(string(runes[:start]))
	b.WriteString(name)
	b.WriteString("()")
	b.WriteString(string(runes[end:]))

	m.input.SetValue(b.String())
	m.input.SetCursor(start + len([]rune(name)) + 1)
	m.closeCompletion()
	m.refresh()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}

	return true
}

// View renders the input line, then the completion popup or signature-help line, then
// the last error if one is set, joined as width-wide lines.
func (m Model) View(width int) string {
	if width <= 0 {
		return ""
	}

	input := m.input
	input.SetWidth(max(width-len([]rune(input.Prompt)), 1))

	lines := []string{input.View()}

	switch {
	case m.completionOpen:
		lines = append(lines, m.renderCompletion(width)...)
	case m.signature != "":
		lines = append(lines, signatureStyle.Render(truncate(m.signature, width)))
	}

	if m.err != nil {
		lines = append(lines, errorStyle.Render(truncate(m.err.Error(), width)))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderCompletion(width int) []string {
	shown := min(len(m.candidates), maxCandidates)
	lines := make([]string, 0, shown)

	for i := range shown {
		name := m.candidates[i]
		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render(truncate(name, width)))

			continue
		}

		lines = append(lines, completionStyle.Render(truncate(name, width)))
	}

	return lines
}

var (
	signatureStyle  = lipgloss.NewStyle().Faint(true)
	errorStyle      = lipgloss.NewStyle().Faint(true)
	completionStyle = lipgloss.NewStyle()
	selectedStyle   = lipgloss.NewStyle().Reverse(true)
)

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= width {
		return s
	}

	return string(runes[:width])
}
