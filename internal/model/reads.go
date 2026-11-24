package model

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/seen"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/empty"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/readmark"
)

// ReadState is the seam read marks plug into: whether a reader has read one hunk of one change,
// the hunk named by its content (seen.Parse) so a mark survives a rebase and drops once the hunk's
// text changes. A *seen.Store answers it as it is; without one noReads does.
type ReadState interface {
	Unread(changeID, hunkHash string) bool
}

// ReadMarker records marks, saving them before it returns. *seen.Store implements it.
type ReadMarker interface {
	Mark(changeID string, read bool, live []seen.Ref, ids ...seen.ID) error
}

// ReadReloader forgets what a change's marks were, so the next ask reads them again: after another
// tool, jj-diff, may have saved some. *seen.Store implements it.
type ReadReloader interface {
	Reload(changeID string)
}

// errMarksOff is a mark asked of a screen with no store to keep it in.
var errMarksOff = errors.New("read marks are off: no state directory for this repository")

// noReads remembers nothing, so every hunk reads as unread and a mark says why it cannot be kept.
type noReads struct{}

func (noReads) Unread(string, string) bool { return true }

func (noReads) Mark(string, bool, []seen.Ref, ...seen.ID) error { return errMarksOff }

func (noReads) Reload(string) {}

// liveHunks is a change's whole hunk list as last read, and the commit it was read at.
type liveHunks struct {
	commit string
	refs   []seen.Ref
}

// heldHunk is a hunk (k from 0) or a file (k -1) marked read under U, which stays drawn until the
// cursor leaves it, so a mark does not pull the line out from under the reader.
type heldHunk struct {
	path string
	k    int
	set  bool
}

// paneReads is what one diff pane keeps about the change it shows, for its read marks.
type paneReads struct {
	// change is the change whose whole diff the pane shows, "" while it shows a round, another kind
	// of diff, or nothing, and text that diff.
	change string
	text   string
	title  string
	// shown is the pane's hunks in order, the change's whole list or, under U, the ones drawn, and
	// byPath the same grouped by file, which is how the pane names a hunk.
	shown  []seen.Ref
	byPath map[string][]seen.Ref
	held   heldHunk
}

// readsState is what the screen keeps about read marks beyond the store.
type readsState struct {
	// live is each change's whole hunk list as last read, by change id. A map, so the copies Update
	// makes of the model share it.
	live map[string]liveHunks
	// dock is the docked diff's, and full the full-screen diff's, which Enter opens over the log.
	dock paneReads
	full paneReads
	// only is U: both panes draw only the hunks not yet read.
	only bool
}

// readView is one diff pane that keeps read marks, with what it holds: the dock or the full-screen
// diff. Every read key acts through one, so the two answer space, ]u, [u, and U the same way.
type readView struct {
	pane  *diffpane.Model
	state *paneReads
	full  bool
}

// dockReads is the docked diff as a readView.
func (m *Model) dockReads() readView {
	return readView{pane: &m.dock, state: &m.marks.dock}
}

// fullReads is the full-screen diff as a readView.
func (m *Model) fullReads() readView {
	return readView{pane: &m.diff, state: &m.marks.full, full: true}
}

// activeReads is the pane the read keys act in: the full-screen diff while it is open, else the dock.
func (m *Model) activeReads() readView {
	if m.openModal() == ctxDiff {
		return m.fullReads()
	}

	return m.dockReads()
}

// marksOff reports whether marks cannot be kept, which every read key then says.
func (m Model) marksOff() bool {
	_, none := m.reads.(noReads)

	return none
}

// stripOf counts how much of refs a reader has read in change.
func (m Model) stripOf(changeID string, refs []seen.Ref) readmark.Strip {
	strip := readmark.Strip{Total: len(refs)}

	for _, r := range refs {
		if !m.unread(changeID, string(r.ID)) {
			strip.Read++
		}
	}

	return strip
}

// noteRead files a change's whole hunk list and its read strip, for the chapter and the log row that
// show it.
func (m *Model) noteRead(changeID, commit string, refs []seen.Ref) {
	m.marks.live[changeID] = liveHunks{commit: commit, refs: refs}
	m.refreshStrip(changeID)
}

// refreshStrip counts a change's strip again from its hunk list, after a mark or a reload.
func (m *Model) refreshStrip(changeID string) {
	live, ok := m.marks.live[changeID]
	if !ok {
		return
	}

	strip := m.stripOf(changeID, live.refs)
	m.strips[changeID] = strip
	m.outline.view.SetRead(changeID, strip)
	m.log.SetUnread(changeID, strip.Label())
}

// reloadMarks reads every known change's marks again, since a program jojo handed the terminal to
// (jj-diff) may have saved some.
func (m *Model) reloadMarks() {
	reloader, ok := m.reads.(ReadReloader)
	if !ok {
		return
	}

	for change := range m.marks.live {
		if scope := m.scopes.of(change); scope != "" {
			reloader.Reload(scope)
		}

		m.refreshStrip(change)
	}
}

// showChange puts a change's whole diff in v, filtered to what is unread under U, landing on keep's
// file when it is still drawn.
func (m *Model) showChange(v readView, changeID, title, text, keep string) {
	*v.state = paneReads{change: changeID, text: text, title: title}
	m.drawChange(v, keep)
}

// drawChange draws the change v holds, through U's filter when it is on.
func (m *Model) drawChange(v readView, keep string) {
	st := v.state
	refs := m.marks.live[st.change].refs
	text, shown := st.text, refs

	if m.marks.only {
		filtered, kept, ok := filterDiff(text, refs, func(r seen.Ref) bool {
			return m.unread(st.change, string(r.ID))
		})
		if ok {
			text, shown = filtered, kept
		}
	}

	st.shown, st.byPath = shown, groupByPath(shown)

	switch {
	case !v.full:
		m.showDockText(st.title, text, keep)
	case keep == "":
		v.pane.Open(st.title, text, m.width)
	default:
		v.pane.Show(st.title, text, m.width, keep)
	}

	scope, byPath, reads := m.scopes.of(st.change), st.byPath, m.reads
	v.pane.SetRead(func(path string, k int) bool {
		in := byPath[path]

		return scope != "" && k < len(in) && !reads.Unread(scope, string(in[k].ID))
	})
}

// leaveChange says v no longer shows a change as it stands (a round, or a diff that is not one
// change's whole diff, is up), so read keys are refused there and no hunk draws as read.
func leaveChange(v readView) {
	*v.state = paneReads{}
	v.pane.SetRead(nil)
}

func groupByPath(refs []seen.Ref) map[string][]seen.Ref {
	out := map[string][]seen.Ref{}
	for _, r := range refs {
		out[r.Path] = append(out[r.Path], r)
	}

	return out
}

// filterDiff is gitDiff with only the hunks keep answers true for, each file's header kept while any
// of its hunks is, and the refs it kept. Hunks pair with refs in patch order, which is how seen.Parse
// lists them; when the counts disagree the diff is answered whole, with ok false.
func filterDiff(gitDiff string, refs []seen.Ref, keep func(seen.Ref) bool) (string, []seen.Ref, bool) {
	var (
		out, head, body strings.Builder
		kept            []seen.Ref
		n               = -1
		inHead, taking  bool
	)

	flush := func() {
		if body.Len() > 0 {
			out.WriteString(head.String())
			out.WriteString(body.String())
		}

		head.Reset()
		body.Reset()
	}

	for _, line := range strings.SplitAfter(gitDiff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()

			inHead, taking = true, false
			head.WriteString(line)
		case strings.HasPrefix(line, "@@"):
			n++
			inHead = false

			if n >= len(refs) {
				return gitDiff, refs, false
			}

			taking = keep(refs[n])
			if taking {
				kept = append(kept, refs[n])
				body.WriteString(line)
			}
		case inHead:
			head.WriteString(line)
		case taking:
			body.WriteString(line)
		}
	}

	flush()

	if n+1 != len(refs) {
		return gitDiff, refs, false
	}

	return out.String(), kept, true
}

// readableIn is why v cannot keep marks right now, "" when it can.
func (m Model) readableIn(v readView) string {
	change := v.state.change

	switch {
	case m.marksOff():
		return errMarksOff.Error()
	case !v.full && !m.docked:
		return "the diff is not docked: " + m.keys.Label(help.ActionPreview) + " docks it"
	case v.full && change == "":
		return "read marks are kept on one change's whole diff, and this is " + v.state.title
	case change == "":
		return "a round is on screen; marks are for the change as it stands, which ]r past the newest returns to"
	case len(m.marks.live[change].refs) == 0:
		return "nothing to read: " + change + " changes no lines"
	}

	return ""
}

// unreadIn counts the hunks of a change a reader has not read.
func (m Model) unreadIn(changeID string) int {
	return m.stripOf(changeID, m.marks.live[changeID].refs).Unread()
}

// readGate answers the read keys: the mark, the unread motions, and U, on whichever screen they were
// pressed. The second result is false for an action that is not one of them.
func (m Model) readGate(screen keys.Context, action string) (keys.Availability, bool) {
	motion := action == keys.NextAction(keys.ObjUnread) || action == keys.PrevAction(keys.ObjUnread)
	if action != keys.ActionRead && action != keys.ActionOnlyUnread && !motion {
		return keys.Availability{}, false
	}

	if m.marksOff() {
		return keys.Refuse(errMarksOff.Error()), true
	}

	switch {
	case screen == keys.View:
		return m.paneGate(m.activeReads(), action, motion), true
	case screen == help.Outline && action == keys.ActionRead:
		if rev := m.outline.view.Selected(); rev == nil || rev.Empty {
			return keys.Refuse("nothing to read in an empty chapter"), true
		}
	case motion && (screen == help.Outline || screen == keys.Log):
		return m.revisionsGate(m.unreadRevisions(screen)), true
	}

	return keys.Allow(), true
}

// paneGate answers the read keys pressed in a diff pane, docked or full screen.
func (m Model) paneGate(v readView, action string, motion bool) keys.Availability {
	if why := m.readableIn(v); why != "" && action != keys.ActionOnlyUnread {
		return keys.Refuse(why)
	}

	change := v.state.change
	if total := len(m.marks.live[change].refs); motion && m.unreadIn(change) == 0 {
		return keys.Refuse("all " + plural(total, "hunk") + " read")
	}

	return keys.Allow()
}

// revisionsGate refuses an unread motion over revisions none of which has anything left to read.
func (m Model) revisionsGate(revs []*jj.Revision) keys.Availability {
	counted, unread := 0, 0

	readable := 0

	for _, rev := range revs {
		if rev.Empty {
			continue
		}

		readable++

		if m.counted(rev) {
			counted++

			if m.strips[rev.ChangeID].Unread() > 0 {
				unread++
			}
		}
	}

	switch {
	case unread > 0:
		return keys.Allow()
	case counted == 0:
		return keys.Refuse("no diff here is counted yet: " + m.keys.Label(keys.ActionOutline) +
			" reads a stack's, " + m.keys.Label(help.ActionPreview) + " the one under the cursor")
	case counted < readable:
		return keys.Refuse("every counted revision is read (" + strconv.Itoa(readable-counted) + " not counted yet)")
	}

	return keys.Refuse("all " + plural(counted, "revision") + " read")
}

// counted reports whether a revision's hunks are known: its diff was read.
func (m Model) counted(rev *jj.Revision) bool {
	_, ok := m.marks.live[rev.ChangeID]

	return ok
}

// plural is n and noun, with an s past one.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return strconv.Itoa(n) + " " + noun + "s"
}

// unreadRevisions is the revisions an unread motion steps through on screen: the outline's chapters
// or the log's rows, in screen order.
func (m Model) unreadRevisions(screen keys.Context) []*jj.Revision {
	var out []*jj.Revision

	if screen == help.Outline {
		for _, c := range m.outline.view.Chapters() {
			out = append(out, c.Rev)
		}

		return out
	}

	for _, row := range m.rows {
		if row.Revision != nil {
			out = append(out, row.Revision)
		}
	}

	return out
}

// markedMsg is a mark saved, or the reason it was not.
type markedMsg struct {
	err    error
	change string
	what   string
}

// hunksReadMsg is a change's hunk list read in the background, so its strip is known before the dock
// reaches it.
type hunksReadMsg struct {
	err    error
	change string
	commit string
	refs   []seen.Ref
	// then marks the whole change once its hunks are known: what space on an unread chapter asks.
	then bool
}

// markCmd saves a mark off the update loop, the change id, hunk list, and ids copied first, and
// answers what to say once it lands.
func (m Model) markCmd(change string, read bool, live []seen.Ref, ids []seen.ID, what string) tea.Cmd {
	marker, ok := m.reads.(ReadMarker)
	if !ok {
		marker = noReads{}
	}

	// The scope is read here, on the update loop, which is the only place the book is written.
	scope := m.scopes.of(change)
	if scope == "" && !m.marksOff() {
		return func() tea.Msg { return markedMsg{change: change, err: errNoScope, what: what} }
	}

	live, ids = slices.Clone(live), slices.Clone(ids)

	return func() tea.Msg {
		err := marker.Mark(scope, read, live, ids...)

		return markedMsg{change: change, err: err, what: what}
	}
}

// markPane is space in a diff pane: the hunk under the cursor toggles, and on a file's heading the
// whole file goes read, or unread when all of it already is.
func (m *Model) markPane(v readView) tea.Cmd {
	path, k, ok := v.pane.HunkAt()
	if !ok {
		return m.say("no hunk under the cursor")
	}

	change := v.state.change
	live := m.marks.live[change].refs

	if k < 0 {
		var ids []seen.ID

		all := true

		for _, r := range live {
			if r.Path == path {
				ids = append(ids, r.ID)
				all = all && !m.unread(change, string(r.ID))
			}
		}

		if len(ids) == 0 {
			return m.say(path + " has no hunks to read")
		}

		m.hold(v, path, -1)

		return m.markCmd(change, !all, live, ids, path+" "+readWord(!all))
	}

	in := v.state.byPath[path]
	if k >= len(in) {
		return m.say("no hunk under the cursor")
	}

	id := in[k].ID
	read := m.unread(change, string(id))
	m.hold(v, path, k)

	return m.markCmd(change, read, live, []seen.ID{id}, "hunk "+readWord(read))
}

// hold keeps what was just marked drawn under U until the cursor leaves it.
func (m *Model) hold(v readView, path string, k int) {
	if m.marks.only {
		v.state.held = heldHunk{path: path, k: k, set: true}
	}
}

func readWord(read bool) string {
	if read {
		return "read"
	}

	return "unread"
}

// markChapter is space in the outline: the whole chapter goes read, or unread when all of it already
// is. A chapter whose hunks are not known yet is read first, in the same command.
func (m *Model) markChapter() tea.Cmd {
	rev := m.outline.view.Selected()
	if rev == nil {
		return nil
	}

	if live, ok := m.marks.live[rev.ChangeID]; ok && (rev.CommitID == "" || live.commit == rev.CommitID) {
		return m.markWhole(rev.ChangeID, live.refs)
	}

	return m.readHunks(rev.ChangeID, rev.CommitID, true)
}

// markWhole marks every hunk of a change read, or unread when every one already is.
func (m Model) markWhole(change string, refs []seen.Ref) tea.Cmd {
	ids := make([]seen.ID, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}

	if len(ids) == 0 {
		return m.say("nothing to read: " + change + " changes no lines")
	}

	read := m.stripOf(change, refs).Unread() > 0

	return m.markCmd(change, read, refs, ids, change+" "+readWord(read))
}

// readHunks reads a change's diff for its hunk list alone, and marks it whole afterwards when then
// is set.
func (m Model) readHunks(change, commit string, then bool) tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		text, err := client.GitDiff(ctx, jj.DiffTarget{Revision: change})

		return hunksReadMsg{change: change, commit: commit, refs: seen.Parse([]byte(text)), err: err, then: then}
	}
}

// countStack reads the hunk list of every chapter not yet counted at its commit, one after another,
// so the outline's strips fill as each lands and ]u knows where to go.
func (m Model) countStack() tea.Cmd {
	if m.marksOff() {
		return nil
	}

	var cmds []tea.Cmd

	for _, c := range m.outline.view.Chapters() {
		live, ok := m.marks.live[c.Rev.ChangeID]
		if c.Rev.Empty || (ok && live.commit != "" && live.commit == c.Rev.CommitID) {
			continue
		}

		cmds = append(cmds, m.readHunks(c.Rev.ChangeID, c.Rev.CommitID, false))
	}

	return tea.Sequence(cmds...)
}

// handleReadMsg lands a saved mark and a hunk list read in the background.
func (m Model) handleReadMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case markedMsg:
		if msg.err != nil {
			m.fail(msg.err)

			return m, nil, true
		}

		m.refreshStrip(msg.change)

		left := m.strips[msg.change].Unread()
		text := msg.what + " · " + plural(left, "hunk") + " left"

		if left == 0 {
			text = msg.what + " · all read"
		}

		return m, m.say(text), true
	case hunksReadMsg:
		if msg.err != nil {
			if msg.then {
				m.fail(msg.err)
			}

			return m, nil, true
		}

		m.noteRead(msg.change, msg.commit, msg.refs)

		if msg.then {
			return m, m.markWhole(msg.change, msg.refs), true
		}

		return m, nil, true
	}

	return m, nil, false
}

// settleHeld drops a hunk marked read under U once the cursor has left it in the pane the keys act
// in, redrawing the filter on the hunk the cursor moved to.
func (m *Model) settleHeld() {
	v := m.activeReads()

	held := v.state.held
	if !held.set || !m.marks.only || v.state.change == "" {
		return
	}

	path, k, ok := v.pane.HunkAt()
	if ok && path == held.path && k == held.k {
		return
	}

	v.state.held = heldHunk{}
	m.redraw(v, path, k, ok)
}

// redraw draws v again keeping the cursor on the hunk at path's k (its heading for a negative k),
// named by the hunk since the filter renumbers every row; ok false lands nowhere in particular.
func (m *Model) redraw(v readView, path string, k int, ok bool) {
	var at *seen.Ref

	if ok && k >= 0 && k < len(v.state.byPath[path]) {
		r := v.state.byPath[path][k]
		at = &r
	}

	m.drawChange(v, path)

	if ok {
		m.land(v, path, at)
	}
}

// land puts v's cursor back on ref (or on path's heading when ref is nil) after a redraw, or on the
// first hunk drawn after it when it is no longer drawn.
func (m *Model) land(v readView, path string, ref *seen.Ref) {
	if ref == nil {
		v.pane.JumpHunk(path, -1)

		return
	}

	live := m.marks.live[v.state.change].refs
	from := slices.IndexFunc(live, func(r seen.Ref) bool { return r.ID == ref.ID && r.Path == ref.Path })

	for _, r := range live[max(0, from):] {
		if k := slices.IndexFunc(v.state.byPath[r.Path], func(s seen.Ref) bool { return s.ID == r.ID }); k >= 0 {
			v.pane.JumpHunk(r.Path, k)

			return
		}
	}
}

// stepUnread moves v's cursor to the next (dir 1) or previous (-1) hunk not yet read.
func (m *Model) stepUnread(v readView, dir int) tea.Cmd {
	if why := m.readableIn(v); why != "" {
		return m.say(why)
	}

	shown := v.state.shown
	pos := position(v)
	change := v.state.change

	for i := pos + dir; i >= 0 && i < len(shown); i += dir {
		if !m.unread(change, string(shown[i].ID)) {
			continue
		}

		in := v.state.byPath[shown[i].Path]
		v.pane.JumpHunk(shown[i].Path, slices.IndexFunc(in, func(r seen.Ref) bool { return r.ID == shown[i].ID }))
		m.settleHeld()

		return nil
	}

	if dir > 0 {
		return m.say("no unread hunk below")
	}

	return m.say("no unread hunk above")
}

// position is the index in v's shown hunks of the one under its cursor. On a file's heading it sits
// just before the file's first hunk, and above every file it is -1.
func position(v readView) int {
	path, k, ok := v.pane.HunkAt()
	if !ok {
		return -1
	}

	first := slices.IndexFunc(v.state.shown, func(r seen.Ref) bool { return r.Path == path })
	if first < 0 {
		return -1
	}

	if k < 0 {
		return first - 1
	}

	return first + k
}

// stepUnreadRev moves the outline's or the log's cursor to the next or previous revision with an
// unread hunk. A revision whose diff is not counted yet is stepped over, and the status line says
// how many were.
func (m Model) stepUnreadRev(screen keys.Context, dir int) (Model, tea.Cmd) {
	revs := m.unreadRevisions(screen)

	cursor := m.log.Selected()
	if screen == help.Outline {
		cursor = m.outline.view.Selected()
	}

	at := -1
	if cursor != nil {
		at = slices.IndexFunc(revs, func(r *jj.Revision) bool { return r.ChangeID == cursor.ChangeID })
	}

	if at < 0 && dir < 0 {
		at = len(revs)
	}

	skipped := 0

	for i := at + dir; i >= 0 && i < len(revs); i += dir {
		strip := m.strips[revs[i].ChangeID]
		if revs[i].Empty {
			continue
		}

		if !m.counted(revs[i]) {
			skipped++

			continue
		}

		if strip.Unread() == 0 {
			continue
		}

		var cmd tea.Cmd

		if screen == help.Outline {
			m.outline.view.Focus(revs[i].ChangeID)
			cmd = m.followDock(m.dockTarget())
		} else {
			m.log.Focus(revs[i].ChangeID)
			rev := m.log.Selected()
			cmd = tea.Batch(m.followBookmark(rev), m.followDock(rev), m.loadStats())
		}

		say := shortID(revs[i]) + ": " + plural(strip.Unread(), "unread hunk")
		if skipped > 0 {
			say += " · stepped over " + strconv.Itoa(skipped) + " not counted yet"
		}

		return m, tea.Batch(cmd, m.say(say))
	}

	where := map[bool]string{true: "below", false: "above"}[dir > 0]
	say := "no revision with unread hunks " + where

	if skipped > 0 {
		say += " (" + strconv.Itoa(skipped) + " not counted yet)"
	}

	return m, m.say(say)
}

// toggleOnlyUnread is U: both diff panes draw only what is unread, or everything again, each keeping
// its cursor on the hunk it was on.
func (m *Model) toggleOnlyUnread() tea.Cmd {
	m.marks.only = !m.marks.only

	for _, v := range []readView{m.dockReads(), m.fullReads()} {
		v.state.held = heldHunk{}

		if v.state.change != "" && (!v.full || m.diff.IsOpen()) {
			path, k, ok := v.pane.HunkAt()
			m.redraw(v, path, k, ok)
		}
	}

	if !m.marks.only {
		return m.say("every hunk")
	}

	v := m.activeReads()

	say := onlyUnread
	if live := m.marks.live[v.state.change].refs; v.state.change != "" {
		say += ": " + strconv.Itoa(len(v.state.shown)) + " of " + plural(len(live), "hunk")
	}

	if !m.docked && !v.full {
		say += "; " + m.keys.Label(help.ActionPreview) + " docks the diff"
	}

	return m.say(say)
}

// onlyUnread is what U is called where ]u's "unread" sits beside it.
const onlyUnread = "only unread"

// tellOnlyUnread names U's hint apart from ]u's: the two share a word in the shared table.
func tellOnlyUnread(km help.Keymap, hints []keyhint.Hint) []keyhint.Hint {
	only := km.Label(keys.ActionOnlyUnread)
	for i := range hints {
		if hints[i].Key == only {
			hints[i].What = onlyUnread
		}
	}

	return hints
}

// readCard is what v draws under U once nothing in its change is left to read, nil otherwise: beside
// the log behind the dock's rule, or full width over it.
func (m Model) readCard(v readView, width, height int) []string {
	st := v.state
	if !m.marks.only || st.change == "" || len(st.shown) > 0 ||
		len(m.marks.live[st.change].refs) == 0 || width <= 0 || height <= 1 {
		return nil
	}

	total := len(m.marks.live[st.change].refs)
	card := empty.Empty{
		Title: st.change + " is read",
		Why:   "All " + plural(total, "hunk") + " of it are marked read. U shows every hunk again.",
		Hints: m.keys.Hints(keys.ActionOnlyUnread, keys.NextAction(keys.ObjChapter)),
	}

	if v.full {
		card.Hints = m.keys.Hints(keys.ActionOnlyUnread, keys.ActionBack)
	}

	if len(card.Hints) > 0 {
		card.Hints[0].What = "every hunk"
	}

	inner, rule := diffpane.DockWidth(width), m.skin.Muted.Render("│ ")
	if v.full {
		inner, rule = width, ""
	}

	lines := []string{rule + m.skin.Muted.Render(table.Truncate(st.title, inner))}

	for _, line := range empty.Card(m.skin.Skin, card, inner, height-1) {
		lines = append(lines, rule+line)
	}

	return lines
}

// readRules are the log's next steps about read marks: revisions here with something left to read.
func readRules() []coach.Rule[logState] {
	return []coach.Rule[logState]{{
		ID: "jojo.unread", Action: keys.NextAction(keys.ObjUnread), Tone: coach.Next, Priority: rankThird,
		When: func(s logState) bool { return s.unread > 0 && s.checked == 0 },
		Say: func(s logState) string {
			if s.unread == 1 {
				return "a revision here has unread hunks"
			}

			return strconv.Itoa(s.unread) + " revisions here have unread hunks"
		},
		Why: func(logState) string {
			return "step to the next one; the dock shows what is left, and space marks it read"
		},
	}}
}
