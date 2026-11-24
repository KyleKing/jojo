package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/palette"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
)

// pickRows is the picker's footer: the command line and its flags, one row more than the log's.
const pickRows = 2

// pickHeight is the log's rows less the picker's extra footer row.
func (m Model) pickHeight() int { return max(0, m.logHeight()-(pickRows-footerRows)) }

// Chip priorities: what the command moves and where survive a narrow footer longest, then the
// toggles, then the before/after switch. Unchosen options go before any of them.
const (
	prioPlace   = 5
	prioToggle  = 4
	prioCompare = 3
)

// dispatchPick owns every key while the picker is open, so a verb pressed by habit cannot run
// against the log underneath. A flag is "- letter" as jj spells it.
func (m Model) dispatchPick(action string) (tea.Model, tea.Cmd) {
	if letter, ok := strings.CutPrefix(action, pickFlagVerbs[m.pick.verb]+"-flag-"); ok {
		if !m.pick.flags.Toggle(letter) {
			return m, nil
		}

		cmd := m.changed()

		return m, cmd
	}

	if key, ok := navKey(action); ok {
		cmd := m.moveDest(key)

		return m, cmd
	}

	switch action {
	case keys.ActionQuit, keys.ActionCancel:
		// The program is about to exit, so the sandbox goes now rather than in a command nobody runs.
		if m.pick.sandbox != nil {
			_ = m.pick.sandbox.Close() //nolint:errcheck // swept on a later start if it will not delete.
		}

		m.pick, m.quitting = pickState{}, true

		return m, tea.Quit
	case keys.ActionHelp:
		m.openPalette(palette.Browse)

		return m, nil
	case help.ActionPickApply:
		return m.applyPick()
	case keys.ActionBack:
		what := pickVerbNames[m.pick.verb]
		cmd := m.closePicker()

		return m, tea.Batch(cmd, m.say(what+" canceled, nothing ran"))
	case help.ActionPickCompare:
		m.pick.before = !m.pick.before
		m.showPicked()

		return m, nil
	case help.ActionPickClear:
		if m.pick.verb != pickDuplicate || m.pick.inPlace {
			return m, nil
		}

		m.pick.inPlace = true

		cmd := m.changed()

		return m, cmd
	}

	return m, nil
}

// moveDest walks the destination over the log as it was when the picker opened, which stays put
// while the preview redraws, so j always means the next row down in the log the reader knows.
// What moves cannot be its own destination, so the cursor steps over it.
func (m *Model) moveDest(msg tea.KeyPressMsg) tea.Cmd {
	nav := logview.New(logview.Styles{})
	nav.SetSize(m.width, max(1, m.pickHeight()))
	nav.SetRows(m.pick.base)
	nav.Focus(m.pick.dest)

	excluded := m.pick.verb == pickRebase || m.pick.verb == pickSquash || m.pick.verb == pickRestore
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

// wordOnto, wordAfter, and wordBefore are the placement chips' words, shared by every verb that places.
const (
	wordOnto   = "onto"
	wordAfter  = "after"
	wordBefore = "before"
)

// flagWords are the chips' words, short enough for a footer, keyed by verb and flag letter. The help
// page spells each flag out in full from aragonite's table.
var flagWords = map[string]map[string]string{
	keys.VerbRebase: {
		"r": "revs", "s": "source", "b": "branch", "o": wordOnto, "A": wordAfter, "B": wordBefore,
		"e": "skip emptied", "d": "keep divergent", "p": "simplify parents",
	},
	keys.VerbSquash:    {"t": "into", "u": "dest message", "k": "keep emptied", "i": "choose hunks"},
	keys.VerbDuplicate: {"o": wordOnto, "A": wordAfter, "B": wordBefore},
	keys.VerbNew:       {"o": wordOnto, "A": wordAfter, "B": wordBefore, "n": "no-edit"},
	keys.VerbRestore:   {"i": "choose hunks", "d": "keep descendants"},
}

// pickChips are the picker's flags as key hints, keyed as jj spells them: a radio group draws its
// unchosen options flat, and a toggle draws flat while it is off, so the line reads as the
// command's current shape.
func (m Model) pickChips() string {
	p := &m.pick
	verb := pickFlagVerbs[p.verb]
	var hints []keyhint.Hint

	for _, f := range help.PickerFlags(verb) {
		h, ok := m.keys.Hint(keys.FlagAction(verb, f.Letter))
		if !ok {
			continue
		}

		on := p.flags.On(f.Letter)
		if f.Radio == keys.RadioPlacement && p.verb == pickDuplicate && p.inPlace {
			on = false
		}

		h.What, h.Off, h.Priority = flagWords[verb][f.Letter], !on, prioToggle
		if f.Radio != "" {
			h.Priority = prioPlace
		}

		hints = append(hints, h)
	}

	if p.verb == pickDuplicate {
		if h, ok := m.keys.Hint(help.ActionPickClear); ok {
			h.What, h.Off, h.Priority = "in place", !p.inPlace, prioPlace
			hints = append(hints, h)
		}
	}

	// p flips the graph between the repository as it is and as the command leaves it, so the chip
	// names the view it switches to.
	if h, ok := m.keys.Hint(help.ActionPickCompare); ok {
		h.What, h.Priority = "show before", prioCompare
		if p.before {
			h.What = "show after"
		}

		hints = append(hints, h)
	}

	for _, pinned := range []struct{ action, what string }{
		{help.ActionPickApply, pickVerbNames[p.verb]}, {keys.ActionBack, "cancel"},
	} {
		if h, ok := m.keys.Hint(pinned.action); ok {
			h.What, h.Pin = pinned.what, true
			hints = append(hints, h)
		}
	}

	return padLine(keyhint.Fit(m.skin.FooterHintStyles(), hints, m.width), m.width)
}
