package model

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// repeatable is the last write the repeat key runs again on the revision under the cursor (or the
// checked ones): the verb, and for a picker verb the flags it ran with and where it went. A
// destination left on jj's default (the parent, for squash and restore) stays relative, so a repeat
// squashes the next revision into its own parent; one the reader moved stays where it was, so a
// repeat rebases the next one onto the same place.
type repeatable struct {
	flags    keys.FlagSet
	action   string
	dest     string
	bookmark string
	message  string
	verb     pickVerb
	picker   bool
	// relative says dest was jj's default when the picker ran, so a repeat recomputes it.
	relative  bool
	inPlace   bool
	backwards bool
}

// remember records the write the repeat key runs next.
func (m *Model) remember(r repeatable) { m.last = r }

// rememberPick records the picker's command as the write to repeat.
func (m *Model) rememberPick(p *pickState) {
	m.remember(repeatable{
		action: pickVerbNames[p.verb], picker: true, verb: p.verb, flags: p.flags, dest: p.dest,
		relative: p.dest == p.initial, inPlace: p.inPlace, bookmark: p.bookmark, backwards: p.backwards,
		message: p.message,
	})
}

// repeatGate says why the repeat key cannot run: nothing written yet, or a picker verb whose fixed destination is
// the revision it would now act on.
func (m Model) repeatGate() keys.Availability {
	r := m.last
	if r.action == "" {
		return keys.Refuse("nothing to repeat yet: . runs the last write again on the cursor")
	}

	if r.picker && !r.relative && !r.inPlace && r.verb != pickNew && r.verb != pickBookmark &&
		slices.Contains(m.targets(), r.dest) {
		return keys.Refuse("the last " + r.action + " went to " + r.dest + ", which is what . would act on")
	}

	return keys.Allow()
}

// repeat runs the last write again on the cursor.
func (m Model) repeat() (tea.Model, tea.Cmd) {
	r := m.last
	if r.action == keys.ActionDescribe {
		return m.repeatDescribe(r.message)
	}

	if !r.picker {
		return m.revisionAction(r.action)
	}

	rev := m.log.Selected()
	if rev == nil {
		return m, nil
	}

	sources := m.targets()

	switch r.verb {
	case pickRestore:
		sources = []string{rev.ChangeID}
	case pickNew:
		sources = m.log.Checked()
	case pickBookmark:
		sources = nil
	case pickRebase, pickSquash, pickDuplicate:
	}

	m.pick = pickState{
		active: true, verb: r.verb, sources: sources, base: m.rows, flags: copyFlags(r.flags, pickFlagVerbs[r.verb]),
		bookmark: r.bookmark, backwards: r.backwards, message: r.message, inPlace: r.inPlace,
	}

	m.pick.dest = r.dest
	if r.relative {
		m.pick.dest = m.initialDest()
	}

	if r.verb == pickBookmark {
		m.pick.dest = rev.ChangeID
	}

	m.pick.initial = m.pick.dest
	if !r.relative {
		m.pick.initial = ""
	}

	return m.applyPick()
}

// copyFlags is a fresh set for verb holding what old holds, so the repeat's toggles cannot reach the
// set the last picker kept.
func copyFlags(old keys.FlagSet, verb string) keys.FlagSet {
	out := keys.NewFlagSet(verb)
	flags := keys.Flags(verb)

	for _, f := range flags {
		if f.Radio != "" && old.On(f.Letter) && !out.On(f.Letter) {
			out.Toggle(f.Letter)
		}
	}

	for _, f := range flags {
		if f.Radio == "" && old.On(f.Letter) != out.On(f.Letter) {
			out.Toggle(f.Letter)
		}
	}

	return out
}

// repeatableWrites are the log's verbs a repeat runs as they are: they take no destination. Of the b
// menu, the verbs that act on the bookmark already on the revision repeat (asking which when it
// carries several); create and rename write a name the reader typed for one place, so they do not.
//
//nolint:gochecknoglobals // a fixed table, read only.
var repeatableWrites = map[string]bool{
	help.ActionNew: true, help.ActionAbandon: true, help.ActionAbsorb: true, help.ActionBookmarkAdvance: true,
	help.ActionBookmarkDelete: true, help.ActionBookmarkForget: true, help.ActionBookmarkTrack: true,
}

// rememberDescribe records a description written in the prompt, which a repeat writes on the cursor
// (or the checked revisions) as it stands, the prompt skipped.
func (m *Model) rememberDescribe(message string) {
	m.remember(repeatable{action: keys.ActionDescribe, message: message})
}

// repeatDescribe writes message as the description of the cursor or the checked revisions.
func (m Model) repeatDescribe(message string) (tea.Model, tea.Cmd) {
	targets := m.targets()
	if len(targets) == 0 {
		return m, nil
	}

	revset := strings.Join(targets, " | ")

	cmd := m.act(joinIDs(targets), func(ctx context.Context, c *jj.Client) (string, error) {
		return c.Describe(ctx, revset, message)
	})

	return m, cmd
}

// noteWrite remembers a verb that takes no destination as the one to repeat.
func (m *Model) noteWrite(action string) {
	if repeatableWrites[action] {
		m.remember(repeatable{action: action})
	}
}
