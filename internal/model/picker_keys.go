package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/jj"
)

// pickRows is the picker's footer: the command line and its flags, one row more than the log's.
const pickRows = 2

// pickHeight is the log's rows less the picker's extra footer row.
func (m Model) pickHeight() int { return max(0, m.logHeight()-(pickRows-footerRows)) }

// pickMoves are the keys that walk the destination. They are the log's own movement keys, so
// choosing a destination feels like moving the cursor because it is.
var pickMoves = []string{"up", "k", "down", "j", "pgup", "ctrl+u", "pgdown", "ctrl+d", "g", "home", "G", keyEnd, "@"}

// Chip priorities: what the command moves and where survive a narrow footer longest, then the
// toggles, then the before/after switch. Unchosen options go before any of them.
const (
	prioPlace   = 5
	prioToggle  = 4
	prioCompare = 3
)

// handlePickKey owns every key while the picker is open, so a verb pressed by habit cannot run
// against the log underneath.
func (m Model) handlePickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.usage.Key(ctxPick, msg.String())

	switch pressed := msg.String(); {
	case pressed == "q" || pressed == keyQuit:
		// The program is about to exit, so the sandbox goes now rather than in a command nobody runs.
		if m.pick.sandbox != nil {
			_ = m.pick.sandbox.Close() //nolint:errcheck // swept on a later start if it will not delete.
		}

		m.pick, m.quitting = pickState{}, true

		return m, tea.Quit
	case pressed == "?":
		m.showHelp = true
		m.help = m.help.ScrollToStart()

		return m, nil
	case slices.Contains(pickMoves, pressed):
		cmd := m.moveDest(msg)

		return m, cmd
	}

	return m.pickFlagKey(m.keys.ResolveIn(help.ModePick, msg.String()))
}

func (m Model) pickFlagKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case keyEnter:
		return m.applyPick()
	case keyEsc:
		what := pickVerbNames[m.pick.verb]
		cmd := m.closePicker()

		return m, tea.Batch(cmd, m.say(what+" canceled, nothing ran"))
	case keyTab:
		m.pick.before = !m.pick.before
		m.showPicked()

		return m, nil
	}

	if !m.toggle(key) {
		return m, nil
	}

	cmd := m.changed()

	return m, cmd
}

// placeKeys and scopeKeys are the picker's radio groups: where the command lands, and what a rebase
// moves.
var (
	placeKeys = map[string]jj.RebaseDestinationMode{
		"o": jj.RebaseDestOnto, "a": jj.RebaseDestInsertAfter, "b": jj.RebaseDestInsertBefore,
	}
	scopeKeys = map[string]jj.RebaseScope{
		"r": jj.RebaseScopeRevisions, "s": jj.RebaseScopeSource, "B": jj.RebaseScopeBranch,
	}
)

// toggle flips the flag key names when the verb has it, and reports whether anything changed.
func (m *Model) toggle(key string) bool {
	p := &m.pick
	mode, isMode := placeKeys[key]
	scope, isScope := scopeKeys[key]

	switch {
	case isMode && p.verb != pickSquash:
		p.mode, p.inPlace = mode, false
	case key == "h" && p.verb == pickDuplicate:
		p.inPlace = true
	case isScope && p.verb == pickRebase:
		p.scope = scope
	default:
		return m.toggleFlag(key)
	}

	return true
}

// toggleFlag flips the on/off flags, each of which belongs to one verb.
func (m *Model) toggleFlag(key string) bool {
	p := &m.pick

	switch {
	case key == "e" && p.verb == pickRebase:
		p.skip = !p.skip
	case key == "e" && p.verb == pickSquash:
		p.keep = !p.keep
	case key == "m" && p.verb == pickSquash:
		p.combine = !p.combine
	case key == "w" && p.verb == pickNew:
		p.noEdit = !p.noEdit
	default:
		return false
	}

	return true
}

// moveDest walks the destination over the log as it was when the picker opened, which stays put
// while the preview redraws, so j always means the next row down in the log the reader knows.
// What moves cannot be its own destination, so the cursor steps over it.
func (m *Model) moveDest(msg tea.KeyPressMsg) tea.Cmd {
	nav := logview.New(logview.Styles{})
	nav.SetSize(m.width, max(1, m.pickHeight()))
	nav.SetRows(m.pick.base)
	nav.Focus(m.pick.dest)

	excluded := m.pick.verb == pickRebase || m.pick.verb == pickSquash
	start := m.pick.dest

	for range len(m.pick.base) {
		nav.Update(msg)

		rev := nav.Selected()
		if rev == nil || !excluded || !slices.Contains(m.pick.sources, rev.ChangeID) {
			break
		}

		// Past what moves in the direction pressed; the home and end keys turn into a step.
		msg = stepPast(msg)
	}

	rev := nav.Selected()
	if rev == nil || rev.ChangeID == start || (excluded && slices.Contains(m.pick.sources, rev.ChangeID)) {
		return nil
	}

	m.pick.dest = rev.ChangeID
	if m.pick.verb == pickDuplicate && m.pick.inPlace {
		m.pick.inPlace = false
	}

	return m.changed()
}

func stepPast(msg tea.KeyPressMsg) tea.KeyPressMsg {
	switch msg.String() {
	case "up", "k", "pgup", "ctrl+u", "G", keyEnd:
		return tea.KeyPressMsg{Code: 'k', Text: "k"}
	}

	return tea.KeyPressMsg{Code: 'j', Text: "j"}
}

// pickFooter is the command being built, then the flags that change it.
func (m Model) pickFooter() string {
	return m.pickStatus() + "\n" + m.pickChips()
}

func (m Model) pickStatus() string {
	command := m.pick.command()
	state, face := m.previewState()
	room := max(0, m.width-len(state)-1)
	line := m.skin.Accent.Render(table.Truncate(command, room))

	return padLine(line+" "+face.Render(state), m.width)
}

// previewState says how far the preview has got, since a stale or failed one must not pass for the
// graph the command will leave.
func (m Model) previewState() (string, lipgloss.Style) {
	switch {
	case m.pick.sandboxErr != nil:
		return "· no preview: " + oneLineErr(m.pick.sandboxErr), m.skin.Error
	case m.pick.shownFor != m.pick.command():
		return "· previewing…", m.skin.Muted
	case m.pick.previewErr != nil:
		return "· jj would refuse: " + oneLineErr(m.pick.previewErr), m.skin.Error
	case m.pick.unchanged:
		return "· nothing would change", m.skin.Muted
	case m.pick.before:
		return "· showing before", m.skin.Muted
	}

	return "· preview", m.skin.Muted
}

func oneLineErr(err error) string { return strings.Join(strings.Fields(err.Error()), " ") }

func padLine(line string, width int) string {
	if pad := width - lipgloss.Width(line); pad > 0 {
		return line + strings.Repeat(" ", pad)
	}

	return ansi.Truncate(line, width, "")
}

// pickChips are the picker's flags as key hints: a radio group draws its unchosen options flat, and
// a toggle draws flat while it is off, so the line reads as the command's current shape.
func (m Model) pickChips() string {
	h := m.keys.ModeHints(help.ModePick)
	p := &m.pick
	chip := func(action, what string, on bool, priority int) keyhint.Hint {
		hint := h[action]
		hint.What, hint.Off, hint.Priority = what, !on, priority

		return hint
	}

	var hints []keyhint.Hint

	if p.verb == pickRebase {
		hints = append(hints,
			chip("pick-revisions", "revs", p.scope == jj.RebaseScopeRevisions, prioPlace),
			chip("pick-source", "source", p.scope == jj.RebaseScopeSource, prioPlace),
			chip("pick-branch", "branch", p.scope == jj.RebaseScopeBranch, prioPlace))
	}

	if p.verb == pickDuplicate {
		hints = append(hints, chip("pick-here", "in place", p.inPlace, prioPlace))
	}

	if p.verb != pickSquash {
		placed := !p.inPlace
		hints = append(hints,
			chip("pick-onto", "onto", placed && p.mode == jj.RebaseDestOnto, prioPlace),
			chip("pick-after", "after", placed && p.mode == jj.RebaseDestInsertAfter, prioPlace),
			chip("pick-before", "before", placed && p.mode == jj.RebaseDestInsertBefore, prioPlace))
	}

	hints = append(hints, m.verbToggles(chip)...)

	compare := "before"
	if p.before {
		compare = "after"
	}

	hints = append(hints, chip("pick-compare", compare, true, prioCompare))

	apply, cancel := h["pick-apply"], h["pick-cancel"]
	apply.What, apply.Pin, cancel.What, cancel.Pin = pickVerbNames[p.verb], true, "cancel", true

	return padLine(keyhint.Fit(m.skin.KeyHintStyles(), append(hints, apply, cancel), m.width), m.width)
}

func (m Model) verbToggles(chip func(string, string, bool, int) keyhint.Hint) []keyhint.Hint {
	p := &m.pick

	switch p.verb {
	case pickRebase:
		return []keyhint.Hint{chip("pick-emptied", "skip emptied", p.skip, prioToggle)}
	case pickSquash:
		message := "dest message"
		if p.combine {
			message = "combine in $EDITOR"
		}

		return []keyhint.Hint{
			chip("pick-emptied", "keep emptied", p.keep, prioToggle), chip("pick-message", message, true, prioToggle),
		}
	case pickNew:
		return []keyhint.Hint{chip("pick-no-edit", "no-edit", p.noEdit, prioToggle)}
	case pickDuplicate:
	}

	return nil
}
