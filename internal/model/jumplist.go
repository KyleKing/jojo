package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/jj"
)

// The jump list is vim's: every jump (gg, G, @, gt, a ] or [ object, a label) leaves the row it
// jumped from on it, ctrl+o walks back through them and ctrl+i forward again. A row that has left the
// revset is stepped over rather than ending the walk.

// jumpListCap is how many jumps the list keeps, as vim's does.
const jumpListCap = 100

// jumpList is the rows jumps left from (back) and the ones ctrl+o left (forward).
type jumpList struct {
	back    []string
	forward []string
	// walking says the last key was ctrl+o, after which tab also walks forward, as it does in vim
	// where tab and ctrl+i are one key.
	walking bool
}

// noteJump records that the cursor jumped away from from, which drops whatever ctrl+o had left to
// walk forward to.
func (m *Model) noteJump(from *jj.Revision) {
	if from == nil {
		return
	}

	if n := len(m.jumps.back); n > 0 && m.jumps.back[n-1] == from.ChangeID {
		return
	}

	m.jumps.back = append(m.jumps.back, from.ChangeID)
	if len(m.jumps.back) > jumpListCap {
		m.jumps.back = m.jumps.back[len(m.jumps.back)-jumpListCap:]
	}

	m.jumps.forward = nil
}

// jumpGate says why ctrl+o or ctrl+i has nowhere to go.
func (m Model) jumpGate(action string) (keys.Availability, bool) {
	switch action {
	case keys.ActionJumpBack:
		if len(m.jumps.back) == 0 {
			return keys.Refuse("no jump to go back to yet: gg, G, @, gt, ]x, ]b, and ' each leave one"), true
		}
	case keys.ActionJumpForward:
		if len(m.jumps.forward) == 0 {
			return keys.Refuse("nothing to go forward to: ctrl+o goes back first"), true
		}
	default:
		return keys.Availability{}, false
	}

	return keys.Allow(), true
}

// walkJumps steps the jump list: back (ctrl+o) or forward (ctrl+i).
func (m Model) walkJumps(back bool) (Model, tea.Cmd) {
	from, to := &m.jumps.back, &m.jumps.forward
	if !back {
		from, to = to, from
	}

	here := ""
	if rev := m.log.Selected(); rev != nil {
		here = rev.ChangeID
	}

	for len(*from) > 0 {
		id := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]

		if id == here || !m.log.Focus(id) {
			continue
		}

		if here != "" {
			*to = append(*to, here)
		}

		m.jumps.walking = back

		return m, m.followSelected()
	}

	m.jumps.walking = false
	word := map[bool]string{true: "back", false: "forward"}[back]

	return m, m.say("no jump to go " + word + " to")
}
