package model

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/empty"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/palette"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/trailview"
	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/jj"
)

// The palette's own commands, which no key presses: the coach's band and its tips. Each is an action
// no binding names, so a run cannot be mistaken for one to dispatch.
const (
	cmdNext    = ":next"
	cmdTipsOff = ":tips off"
	cmdTipsOn  = ":tips on"
)

// screenGroup files the palette's own commands in the list.
const screenGroup = "The screen"

// The order of next steps within one tone, the highest first.
const (
	rankFirst  = 3
	rankSecond = 2
	rankThird  = 1
)

// The coach's detectors: how many line-downs in a row read as stepping to a revision the long way,
// and how many palette runs of one action teach its key.
const (
	walkRun        = 12
	viaPaletteRuns = 3
)

// logState is what the coach's rules read about the log: the revision under the cursor, @, and what
// the log as a whole holds.
type logState struct {
	cursor *jj.Revision
	wc     *jj.Revision
	// below is the first local bookmark on the row right under the cursor, which b a tugs up.
	below string
	// checked counts the checked revisions, and empty says the revset matched nothing.
	checked int
	empty   bool
	// ahead is a bookmark on the cursor's revision ahead of its remote, and split one that moved
	// both ways on a fetch.
	ahead bool
	split bool
	// merged is the pull request a submit found merged for the cursor's change, 0 for none.
	merged int
	// unread counts the log's revisions with hunks not yet read, of those whose diffs were counted.
	unread int
}

// state is the log as the coach's rules read it.
func (m Model) state() logState {
	s := logState{cursor: m.log.Selected(), checked: len(m.log.Checked()), empty: m.loaded() && !m.anyRevision()}

	s.below = m.bookmarkBelow()

	for _, row := range m.rows {
		if row.Revision != nil && row.Revision.WorkingCopy {
			s.wc = row.Revision
		}

		if row.Revision != nil && m.strips[row.Revision.ChangeID].Unread() > 0 {
			s.unread++
		}
	}

	if s.cursor != nil {
		if pr := m.submit.prs[s.cursor.ChangeID]; pr != nil && pr.State == gh.StateMerged {
			s.merged = pr.Number
		}

		for _, b := range s.cursor.Bookmarks {
			s.ahead = s.ahead || (strings.HasSuffix(b, "*") && !strings.Contains(b, "@"))
			s.split = s.split || strings.HasSuffix(b, "??")
		}
	}

	return s
}

// bookmarkBelow is the first local bookmark on the revision drawn right under the cursor, "" when it
// has none: the bookmark a commit on top of it has left behind.
func (m Model) bookmarkBelow() string {
	cursor := m.log.Selected()
	if cursor == nil {
		return ""
	}

	for i, row := range m.rows {
		if row.Revision == nil || row.Revision.ChangeID != cursor.ChangeID {
			continue
		}

		for _, next := range m.rows[i+1:] {
			if next.Revision != nil {
				if len(next.Revision.LocalBookmarks) > 0 && !next.Revision.Immutable {
					return next.Revision.LocalBookmarks[0]
				}

				return ""
			}
		}
	}

	return ""
}

// loaded reports whether a log read has landed, so an empty log before the first is not taken for a
// revset that matched nothing.
func (m Model) loaded() bool { return m.revsetLoaded }

// anyRevision reports whether the log holds a revision rather than only graph lines.
func (m Model) anyRevision() bool {
	for _, row := range m.rows {
		if row.Revision != nil {
			return true
		}
	}

	return false
}

// shortID is the first few characters of a change id, as a sentence names one.
func shortID(rev *jj.Revision) string {
	const idLen = 8

	return rev.ChangeID[:min(idLen, len(rev.ChangeID))]
}

// onWC reports whether the cursor is on @.
func (s logState) onWC() bool { return s.cursor != nil && s.cursor.WorkingCopy }

// dirtyWC reports whether @ has changes and no description.
func (s logState) dirtyWC() bool { return s.wc != nil && !s.wc.Empty && s.wc.Description == "" }

// logRules are the log's next steps: what is wrong first, then the natural next step, then what is
// worth knowing. Each names the action that takes it, and the keymap names its key.
func logRules() []coach.Rule[logState] {
	cursorIs := func(test func(*jj.Revision) bool) func(logState) bool {
		return func(s logState) bool { return s.cursor != nil && test(s.cursor) }
	}
	says := func(text string) func(logState) string { return func(logState) string { return text } }

	return append(append(readRules(), bookmarkRules()...), []coach.Rule[logState]{
		{
			ID: "jojo.resolve", Action: help.ActionResolve, Tone: coach.Do, Priority: rankFirst,
			When: cursorIs(func(r *jj.Revision) bool { return r.Conflict }),
			Say:  func(s logState) string { return shortID(s.cursor) + " is conflicted" },
			Why:  says("so is every revision stacked on it until it is resolved"),
			Cmd:  "jj resolve",
		},
		{
			ID: "jojo.converge", Action: help.ActionConverge, Tone: coach.Do, Priority: rankSecond,
			When: cursorIs(func(r *jj.Revision) bool { return r.Divergent }),
			Say:  func(s logState) string { return shortID(s.cursor) + " is divergent: one change on two commits" },
			Why:  says("converging keeps one side"),
			Cmd:  "jj converge",
		},
		{
			ID: "jojo.immutable", Action: help.ActionNew, Tone: coach.Do, Priority: rankThird,
			When: cursorIs(func(r *jj.Revision) bool { return r.Immutable && !r.WorkingCopy }),
			Say:  func(s logState) string { return shortID(s.cursor) + " is immutable, so jj refuses to rewrite it" },
			Why:  says("start a change on top of it instead"),
			Cmd:  "jj new <rev>",
		},
		{
			ID: "jojo.restack-merged", Action: help.ActionRebase, Tone: coach.Do, Priority: rankFirst,
			When: func(s logState) bool { return s.merged > 0 },
			Say: func(s logState) string {
				return "#" + strconv.Itoa(s.merged) + " for " + shortID(s.cursor) + " merged"
			},
			Why: says("rebase what is stacked on it onto trunk (-s, then gt), then submit again"),
			Cmd: "jj rebase -s <child> -o trunk()",
		},
		{
			ID: "jojo.empty-log", Action: help.ActionRevset, Tone: coach.Do,
			When: func(s logState) bool { return s.empty },
			Say:  says("the revset matched nothing"),
			Why:  says("edit it, or reload once something matches"),
			Cmd:  "jj log -r <revset>",
		},
		{
			ID: "jojo.describe", Action: keys.ActionDescribe, Tone: coach.Next, Priority: rankSecond,
			When: func(s logState) bool { return s.onWC() && s.dirtyWC() },
			Say:  says("@ has changes but no description"),
			Why:  says("name it, then start the next change on top"),
			Cmd:  "jj describe -r @",
		},
		{
			ID: "jojo.to-wc", Action: help.ActionWorkingCopy, What: "go to @", Tone: coach.Next, Priority: rankThird,
			When: func(s logState) bool { return !s.onWC() && s.dirtyWC() && s.checked == 0 },
			Say:  says("@ has changes but no description"),
			Why:  says("jump there and describe it"),
		},
		{
			ID: "jojo.new", Action: help.ActionNew, Tone: coach.Next,
			When: func(s logState) bool { return s.onWC() && !s.wc.Empty && s.wc.Description != "" },
			Say:  says("@ is described"),
			Why:  says("start the next change on top of it"),
			Cmd:  "jj new @",
		},
		{
			ID: "jojo.push", Action: help.ActionPush, Tone: coach.Next,
			When: func(s logState) bool { return s.ahead },
			Say:  says("a bookmark here is ahead of its remote"),
			Why:  says("push it"),
			Cmd:  "jj git push -r <rev>",
		},
		{
			ID: "jojo.checked", Action: help.ActionRebase, Tone: coach.Could, Priority: rankThird,
			When: func(s logState) bool { return s.checked > 0 },
			Say: func(s logState) string {
				return strconv.Itoa(s.checked) + " checked: s, r, and a act on all of them"
			},
			Why: says("esc clears the checks"),
		},
		{
			ID: "jojo.abandon-empty", Action: help.ActionAbandon, Tone: coach.Could,
			When: cursorIs(func(r *jj.Revision) bool {
				return r.Empty && !r.WorkingCopy && !r.Immutable && r.Description == ""
			}),
			Say: func(s logState) string { return shortID(s.cursor) + " is empty and undescribed" },
			Why: says("abandon it, or describe it and fill it later"),
			Cmd: "jj abandon <rev>",
		},
	}...)
}

// bookmarkRules are the next steps a bookmark under the cursor suggests: settling one that moved
// both ways, and tugging one a commit on top has left behind.
func bookmarkRules() []coach.Rule[logState] {
	says := func(text string) func(logState) string { return func(logState) string { return text } }

	return []coach.Rule[logState]{
		{
			ID: "jojo.bookmark-split", Action: help.ActionBookmarkResolve, Tone: coach.Do,
			When: func(s logState) bool { return s.split },
			Say:  says("a bookmark here moved both ways"),
			Why:  says("pick the side it settles on"),
			Cmd:  "jj bookmark set <name> -r <rev>",
		},
		{
			ID: "jojo.tug", Action: help.ActionBookmarkAdvance, Tone: coach.Could,
			When: func(s logState) bool {
				r := s.cursor

				return s.below != "" && r != nil && len(r.LocalBookmarks) == 0 && !r.Immutable && !r.Empty &&
					r.Description != ""
			},
			Say: func(s logState) string { return s.below + " sits just below " + shortID(s.cursor) },
			Why: says("tug it up to here before you push"),
			Cmd: "jj bookmark advance --to <rev>",
		},
	}
}

// logTips are keys worth knowing that a reader may not reach for, in the order to teach them: the
// ones that find everything else first.
func logTips() []coach.Suggestion {
	return []coach.Suggestion{
		tip("palette", keys.ActionPalette, "finds any command by name, git words too, and runs it"),
		tip("undo", keys.ActionUndo, "undoes the last jj operation, whatever it was"),
		tip("dock", help.ActionPreview, "docks the diff beside the log, following the cursor"),
		tip("trail", keys.ActionMessages, "lists every jj and gh command this session ran"),
		tip("outline", keys.ActionOutline, "opens the stack under the cursor as chapters"),
		tip("oplog", help.ActionOpLog, "shows the operation log, and the log as it stood at each"),
		tip("absorb", help.ActionAbsorb, "moves each changed line into the revision that last touched it"),
	}
}

// tip is an unlearned-key tip, its id under jojo's.
func tip(id, action, say string) coach.Suggestion {
	return coach.Suggestion{ID: "jojo.tip." + id, Action: action, Say: say}
}

// newCoach builds the coach for one session from the keymap, which fills each suggestion's key, and
// the ledger, which remembers tips and runs across sessions.
func newCoach(km help.Keymap, ledger *coach.Ledger) *coach.Coach[logState] {
	if ledger == nil {
		ledger, _ = coach.LedgerAt("") //nolint:errcheck // an in-memory ledger reads no file and cannot fail
	}

	return coach.New(coach.Config[logState]{
		Ledger: ledger, Keymap: km.Keys(), Rules: logRules(), Tips: logTips(),
		Detectors: []coach.Detector{
			coach.Repeat(keys.ActionLineDown, walkRun, coach.Suggestion{
				ID: "jojo.tip.search", Action: keys.ActionSearch,
				Say: "finds a revision by its text, rather than stepping to it",
			}),
			coach.Instead([]string{help.ActionEdit, help.ActionNew}, coach.Suggestion{
				ID: "jojo.tip.new-alone", Action: help.ActionNew,
				Say: "alone already starts a change on top of the one under the cursor",
			}),
			coach.Instead([]string{keys.ActionDescribe, help.ActionNew}, coach.Suggestion{
				ID: "jojo.tip.commit", Action: help.ActionCommit,
				Say: "describes @ and starts the next change in one step",
			}),
			coach.ViaPalette(viaPaletteRuns, func(action string) (string, string, bool) {
				b, ok := km.Binding(action)
				if !ok || len(b.Keys) == 0 {
					return "", "", false
				}

				return b.Label(), word(b), true
			}),
		},
	})
}

// gate answers whether action can do anything where the cursor is, and why not. The footer dims what
// it refuses, the palette and the legend say the reason, and pressing the key says it in the status
// line rather than doing nothing. Only the log's verbs are gated; every other screen's keys act on
// what that screen shows.
func (m Model) gate(action string) keys.Availability {
	if a, ok := m.readGate(m.screen(), action); ok {
		return a
	}

	if m.screen() == help.Outline && action == help.ActionSubmit {
		return m.submitGate()
	}

	if m.pick.active {
		return m.pickGate(action)
	}

	if m.screen() != keys.Log {
		return keys.Allow()
	}

	rev := m.log.Selected()
	if rev == nil {
		if revisionVerbs[action] {
			return keys.Refuse("no revision under the cursor; L edits the revset")
		}

		return keys.Allow()
	}

	if a, ok := bookmarkGate(action, rev); ok {
		return a
	}

	return m.revisionGate(action, rev)
}

// revisionGate is gate for a log with a revision under the cursor.
func (m Model) revisionGate(action string, rev *jj.Revision) keys.Availability {
	checked := len(m.log.Checked())

	switch action {
	case help.ActionResolve:
		if !rev.Conflict {
			return keys.Refuse(shortID(rev) + " has no conflict")
		}
	case help.ActionConverge:
		if !m.anyDivergent() {
			return keys.Refuse("nothing in the log is divergent")
		}
	case help.ActionCommit:
		if !rev.WorkingCopy {
			return keys.Refuse("commits @, the working copy; @ jumps there")
		}
	case help.ActionEdit:
		if rev.WorkingCopy {
			return keys.Refuse(shortID(rev) + " is @ already")
		}
	case help.ActionAbsorb:
		if rev.Empty {
			return keys.Refuse(shortID(rev) + " is empty; there is nothing to absorb")
		}
	case help.ActionParallelize:
		if checked < 2 { //nolint:mnd // two make a pair
			return keys.Refuse("needs two or more checked; space checks one")
		}
	case help.ActionBisect:
		if checked != 2 { //nolint:mnd // a range has two ends
			return keys.Refuse("needs exactly two checked, the good and the bad; space checks one")
		}
	}

	return keys.Allow()
}

// anyDivergent reports whether the log holds a divergent revision.
func (m Model) anyDivergent() bool {
	for _, row := range m.rows {
		if row.Revision != nil && row.Revision.Divergent {
			return true
		}
	}

	return false
}

// revisionVerbs are the log's actions that act on the revision under the cursor, which an empty log
// has none of.
//
//nolint:gochecknoglobals // a fixed table, read only.
var revisionVerbs = map[string]bool{
	help.ActionDiff: true, help.ActionFiles: true, help.ActionCompare: true, help.ActionNew: true,
	help.ActionNewPlaced: true, help.ActionEdit: true, help.ActionDiffedit: true, keys.ActionDescribe: true,
	keys.ActionDescribeEditor: true, help.ActionCommit: true, help.ActionSquash: true, help.ActionSplit: true,
	help.ActionDuplicate: true, help.ActionRebase: true, help.ActionRebaseSubtree: true, help.ActionRestore: true,
	help.ActionAbandon: true, help.ActionAbsorb: true, help.ActionResolve: true, help.ActionPush: true,
	help.ActionBookmarkCreate: true, help.ActionBookmarkSet: true, help.ActionBookmarkAdvance: true,
	help.ActionBookmarkRename: true, help.ActionBookmarkDelete: true, help.ActionBookmarkForget: true,
	help.ActionBookmarkTrack: true, help.ActionBookmarkResolve: true,
	keys.ActionInspect: true, keys.ActionYank: true, help.ActionCheck: true,
}

// refuse answers a press of action that the gate refuses, reporting whether it did: the status line
// says why, and the usage log notes a key reached for where it could not work.
func (m Model) refuse(screen keys.Context, action string) (tea.Cmd, bool) {
	a := m.gate(action)
	if a.OK {
		return nil, false
	}

	m.usage.Gated(usageContext(screen), action)

	return m.say(m.keys.Label(action) + "  " + a.Why), true
}

// entries are the palette's rows for the screen the reader is on: its keys, taught and gated, with
// recent runs from the ledger. The command palette adds what no key presses: the log's
// palette-only verbs and the screen's own commands.
func (m Model) entries(mode palette.Mode) []palette.Entry {
	screen := m.screen()
	es := palette.FromKeymap(m.keys.Keys(), screen, m.gate, m.about)

	if mode == palette.Run && screen == keys.Log {
		for _, b := range m.keys.PaletteOnly() {
			a := m.about[b.Action]
			es = append(es, palette.Entry{
				Action: b.Action, What: b.What, Group: b.Group, Cmd: a.Cmd, Also: a.Also, Prose: a.Prose,
				Flags: a.Flags,
			})
		}
	}

	if mode == palette.Run {
		es = append(es, m.screenCommands()...)
	}

	return palette.WithSeen(es, m.coach.Ledger(), false)
}

// screenCommands are the palette's own rows: the coach's band and its tips.
func (m Model) screenCommands() []palette.Entry {
	band := "show the next steps as a band over the footer"
	if m.band {
		band = "show the next step in the line over the footer"
	}

	return []palette.Entry{
		command(cmdNext, band, "The band names each next step, why, and the jj command it runs.",
			"next step", "coach", "hint"),
		command(cmdTipsOff, "stop tips about keys not yet used", "", "tips"),
		command(cmdTipsOn, "show tips about keys not yet used", "", "tips"),
	}
}

// command is one of the palette's own rows, which no key presses: its name stands in the key column,
// and typed whole it ranks first.
func command(name, what, prose string, also ...string) palette.Entry {
	e := palette.Entry{Action: name, Key: name, Keys: []string{bare(name)}, What: what, Group: screenGroup, Also: also}
	if prose != "" {
		e.Prose = []string{prose}
	}

	return e
}

// bare is a palette command's name without its colon, which typed whole ranks the command first:
// "next" is :next, not "next pane".
func bare(command string) string { return strings.TrimPrefix(command, ":") }

// openPalette opens the command palette (Run) or the legend (Browse) over the screen the reader is on.
func (m *Model) openPalette(mode palette.Mode) {
	m.pal.Open(mode, m.entries(mode))
}

// runPalette carries out what the palette ran: one of its own commands, or an action dispatched by
// name on the screen it was opened over, never a replayed key, so a rebind cannot change what runs.
// A run from the command palette says the key that would have done it.
func (m Model) runPalette(run palette.RunMsg) (tea.Model, tea.Cmd) {
	switch run.Action {
	case cmdNext:
		m.band = !m.band
		m.resize()

		where := map[bool]string{true: "in a band", false: "in the line over the footer"}[m.band]

		return m, m.say("next steps " + where)
	case cmdTipsOff, cmdTipsOn:
		m.coach.Ledger().SetTipsOff(run.Action == cmdTipsOff)

		return m, m.say("tips " + strings.TrimPrefix(run.Action, ":tips "))
	}

	screen := m.screen()
	now := time.Now()

	m.coach.Observe(coach.Event{At: now, Action: run.Action, Palette: true})
	m.usage.Action(usageContext(screen), run.Action, "")

	var (
		next tea.Model
		cmd  tea.Cmd
	)

	if _, picks := pickVerbOf(run.Action); picks && len(flagLetters(run.Flags)) > 0 {
		letters := flagLetters(run.Flags)
		next, cmd = m.openWithFlags(run.Action, letters)
	} else {
		next, cmd = m.dispatch(screen, run.Action)
	}

	if run.From != palette.Run || run.Key == "" {
		return next, cmd
	}

	b, _ := m.keys.Binding(run.Action)

	return next, tea.Batch(m.say("ran "+b.What+" · next time press "+run.Key), cmd)
}

// flagLetters are the letters of words such as "-u" typed after a palette search.
func flagLetters(words []string) []string {
	var out []string

	for _, w := range words {
		if letter, ok := strings.CutPrefix(w, "-"); ok && letter != "" && !strings.HasPrefix(letter, "-") {
			out = append(out, letter)
		}
	}

	return out
}

// pickVerbOf is the picker an action opens, false for one that opens none.
func pickVerbOf(action string) (pickVerb, bool) {
	switch action {
	case help.ActionSquash:
		return pickSquash, true
	case help.ActionRebase, help.ActionRebaseSubtree:
		return pickRebase, true
	case help.ActionDuplicate:
		return pickDuplicate, true
	case help.ActionNewPlaced:
		return pickNew, true
	case help.ActionRestore:
		return pickRestore, true
	}

	return 0, false
}

// openWithFlags opens the picker an action opens with the flags a palette search named already on,
// "squash -i" opening squash's picker choosing hunks. A flag the picker turns on itself stays on.
func (m Model) openWithFlags(action string, letters []string) (tea.Model, tea.Cmd) {
	if a := m.gate(action); !a.OK {
		return m, m.say(a.Why)
	}

	rev := m.log.Selected()
	if rev == nil {
		return m, nil
	}

	verb, _ := pickVerbOf(action)
	sources := m.targets()

	switch verb {
	case pickRestore:
		sources = []string{rev.ChangeID}
	case pickNew:
		sources = m.log.Checked()
	case pickRebase, pickSquash, pickDuplicate, pickBookmark:
	}

	if action == help.ActionRebaseSubtree {
		letters = append([]string{"s"}, letters...)
	}

	presets := make([]string, 0, len(letters))

	for _, l := range letters {
		if verb == pickSquash && l == "u" {
			continue // squash's picker turns -u on itself
		}

		presets = append(presets, l)
	}

	cmd := m.openPicker(verb, sources, presets...)

	return m, cmd
}

// motions are the actions that only move, or only open the ways of finding a command, which the coach
// watches for a long way round but never counts as a command run: the palette's recent list is for
// the commands themselves.
//
//nolint:gochecknoglobals // a fixed table, read only.
var motions = map[string]bool{
	keys.ActionLineDown: true, keys.ActionLineUp: true, keys.ActionHalfPageDown: true, keys.ActionHalfPageUp: true,
	keys.ActionTop: true, keys.ActionBottom: true, keys.ActionHelp: true, keys.ActionPalette: true,
	keys.ActionBack: true, keys.ActionCancel: true, keys.ActionQuit: true, keys.ActionSwitchPane: true,
}

// observe tells the coach and the usage log about an action a key just ran.
func (m Model) observe(screen keys.Context, action, key string) {
	m.coach.Observe(coach.Event{At: time.Now(), Action: action, Motion: motions[action]})

	// A motion's key is already in the log, and a line per step would say nothing a key does not.
	if !motions[action] {
		m.usage.Action(usageContext(screen), action, key)
	}
}

// busy reports whether a text field, an overlay, or a picker is up, which holds tips back.
func (m Model) busy() bool {
	return m.openModal() != "" || m.pal.IsOpen() || m.pick.active || m.travel.active || m.focus == focusRevset
}

// idle asks the coach for a tip once the reader has stopped, saves the ledger, which is the natural
// lull to write it in, and arms the next ask.
func (m Model) idle(msg coach.IdleMsg) tea.Cmd {
	m.coach.Tip(msg.At, m.busy())

	var failed tea.Cmd
	if err := m.coach.Ledger().Save(); err != nil {
		failed = m.say("could not save the coach's notes: " + err.Error())
	}

	return tea.Batch(failed, m.coach.WakeAt(msg.At))
}

// nextStep is the step the coach points at in the log, false when none holds or the log is not on
// screen.
func (m Model) nextStep() (coach.Suggestion, bool) {
	if m.screen() != keys.Log {
		return coach.Suggestion{}, false
	}

	return m.coach.Next(m.state(), m.gate)
}

// bandLines is the coach's band, nil unless the band layout is on and it has something to say.
func (m Model) bandLines() []string {
	if !m.band || m.screen() != keys.Log || m.width <= 0 {
		return nil
	}

	var tip *coach.Suggestion
	if t, ok := m.coach.Showing(); ok {
		tip = &t
	}

	return coach.BandLines(m.skin.Skin, m.coach.All(m.state(), m.gate), tip, m.width)
}

// tipLine is the tip on show drawn for the footer's leading slot, "" when none is, or when the band
// draws it instead.
func (m Model) tipLine() string {
	tip, ok := m.coach.Showing()
	if !ok || m.band {
		return ""
	}

	return coach.TipLine(m.skin.Skin, tip, m.width/2) //nolint:mnd // a tip may take half the footer
}

// discoverMsg answers the messages the palette and the coach send back, reporting whether msg was one.
func (m Model) discoverMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case palette.RunMsg:
		next, cmd := m.runPalette(msg)

		return next, cmd, true
	case palette.RefusedMsg:
		m.usage.Gated(usageContext(m.screen()), msg.Action)

		return m, nil, true
	case coach.IdleMsg:
		return m, m.idle(msg), true
	}

	return m, nil, false
}

// paletteKey sends a key or a paste to the open palette, logging the keys that are not typed into
// its query.
func (m Model) paletteKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && !m.pal.Typing() {
		ctx := ctxPalette
		if m.pal.Mode() == palette.Browse {
			ctx = ctxHelp
		}

		m.usage.Key(ctx, key.String())
	}

	cmd := m.pal.Update(msg)

	return m, cmd
}

// word is a binding's footer word, as a status line names what ran.
func word(b keys.Binding) string {
	if b.Short != "" {
		return b.Short
	}

	return b.What
}

// trailMsg answers what gm's list asks for: a call's whole stderr, its shell line on the clipboard,
// or the way back from a write.
func (m Model) trailMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case trailview.ShowMsg:
		text := msg.Call.Shell() + "\n\n" + strings.TrimSpace(msg.Call.Stderr)
		if strings.TrimSpace(msg.Call.Stderr) == "" {
			text = msg.Call.Shell() + "\n\n(jj wrote nothing to stderr)"
		}

		return m, m.output.Open("exit "+strconv.Itoa(msg.Call.Exit)+" · "+msg.Call.Shell(), text), true
	case trailview.CopiedMsg:
		return m, tea.Batch(tea.SetClipboard(msg.Line), m.say("copied "+msg.Line)), true
	case trailview.UndoMsg:
		m.trailView.Close()

		return m, m.undoCall(msg), true
	case trailview.CancelledMsg:
		return m, nil, true
	}

	return m, nil, false
}

// undoCall reverses a write from the trail: the newest with jj undo, an older one by reverting the
// operation it made, which keeps everything done since.
func (m Model) undoCall(msg trailview.UndoMsg) tea.Cmd {
	if msg.Newest {
		return m.undo()
	}

	if msg.Call.Op == "" {
		return m.say("no operation is known for that write; o opens the op log")
	}

	op, client := msg.Call.Op, m.client

	return m.run(undoHint, func(ctx context.Context) (string, error) { return client.OpRevert(ctx, op) })
}

// emptyLog is the card an empty log draws in place of its rows: what is empty, why, and the keys out.
func (m Model) emptyLog() string {
	card := empty.Empty{
		Title: "revset " + strconv.Quote(m.revset) + " matched nothing",
		Why: "No revision in this repository is in the revset. Edit it, or reload once a write " +
			"makes something match.",
		Hints: m.keys.Hints(help.ActionRevset, keys.ActionReload, keys.ActionPalette),
	}

	return strings.Join(empty.Card(m.skin.Skin, card, m.logBox[0], m.logBox[1]), "\n")
}
