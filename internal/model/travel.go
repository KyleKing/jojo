package model

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/jj"
)

// travelDebounce is how long the op log's cursor must rest before the log at that operation is read,
// matching the docked diff's wait for the same reason: holding j costs one read, not one per row.
const travelDebounce = 120 * time.Millisecond

// confirmRestore is the confirmation standing over a restore chosen while looking at the past.
const confirmRestore = "restore"

// travelState is the log as it stood at an earlier operation. It is read with --at-op and
// --ignore-working-copy, so looking writes nothing, and every write verb is refused while it lasts:
// a verb aimed at a row from the past would land on the present, which is not what is on screen.
type travelState struct {
	err      error
	shownFor string
	ops      []jj.Operation
	rows     []jj.LogRow
	view     logview.Model
	index    int
	seq      int
	active   bool
}

type (
	travelOpsMsg struct {
		err  error
		ops  []jj.Operation
		step int
	}
	travelTickMsg struct{ seq int }
	travelLogMsg  struct {
		err  error
		op   string
		rows []jj.LogRow
	}
)

// op is the operation on screen.
func (t *travelState) op() jj.Operation {
	if t.index < 0 || t.index >= len(t.ops) {
		return jj.Operation{}
	}

	return t.ops[t.index]
}

// now reports whether the operation on screen is the one the repository is at.
func (t *travelState) now() bool { return t.op().Current || t.index == 0 }

// stepOp moves through the op log from the log itself: older is positive. The first step back
// reads the op log; a step forward onto the present ends the trip.
func (m *Model) stepOp(older int) tea.Cmd {
	if !m.travel.active {
		if older < 0 {
			return m.say("already at the current operation")
		}

		client, ctx := m.client, m.ctx

		return func() tea.Msg {
			ops, err := client.OpLog(ctx, opLogLimit)

			return travelOpsMsg{ops: ops, err: err, step: older}
		}
	}

	next := min(max(m.travel.index+older, 0), len(m.travel.ops)-1)
	if next == 0 {
		m.endTravel()

		return m.say("back at the current operation")
	}

	return m.travelTo(next)
}

// startTravel begins a trip over ops with the cursor at index, sharing the log's own revset.
func (m *Model) startTravel(ops []jj.Operation, index int) tea.Cmd {
	m.travel = travelState{active: true, ops: ops, view: logview.New(logview.StylesFrom(m.skin))}
	m.resize()

	return m.travelTo(index)
}

// travelTo puts the trip at index. The present needs no read, so it is drawn at once; anything older
// waits out the debounce.
func (m *Model) travelTo(index int) tea.Cmd {
	m.travel.index = index
	m.travel.seq++

	if m.travel.now() {
		m.travel.rows, m.travel.err, m.travel.shownFor = m.rows, nil, m.travel.op().ID
		m.travel.view.SetRows(m.rows)

		return nil
	}

	seq := m.travel.seq

	return m.after(travelDebounce, func(time.Time) tea.Msg { return travelTickMsg{seq: seq} })
}

func (m *Model) endTravel() {
	m.travel = travelState{}
	m.resize()
}

// readTravel reads the log at the operation on screen. A revset can name what did not exist then,
// a bookmark created since for one, so a failure falls back to jj's default log.
func (m Model) readTravel() tea.Cmd {
	client, op, revset, limit := m.client, m.travel.op().ID, m.revset, m.logWindow

	return load(m.ctx, func(ctx context.Context) (travelLogMsg, error) {
		rows, err := client.LogAt(ctx, op, revset, limit)
		if err != nil && revset != "" {
			rows, err = client.LogAt(ctx, op, "", limit)
		}

		return travelLogMsg{op: op, rows: rows, err: err}, nil
	}, func(m *Model, msg travelLogMsg) tea.Cmd { return m.landTravel(msg) })
}

func (m *Model) landTravel(msg travelLogMsg) tea.Cmd {
	if !m.travel.active || msg.op != m.travel.op().ID {
		return nil
	}

	m.travel.err, m.travel.shownFor = msg.err, msg.op
	if msg.err == nil {
		m.travel.rows = msg.rows
		m.travel.view.SetRows(msg.rows)
	}

	return nil
}

// handleTravelMsg covers the trip's background reads and the op log's cursor, which drives it.
func (m Model) handleTravelMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case travelOpsMsg:
		if msg.err != nil {
			m.fail(msg.err)

			return m, nil, true
		}

		if len(msg.ops) < minOps {
			return m, m.say("no earlier operation to look at"), true
		}

		cmd := m.startTravel(msg.ops, min(msg.step, len(msg.ops)-1))

		return m, cmd, true
	case travelTickMsg:
		if !m.travel.active || msg.seq != m.travel.seq {
			return m, nil, true
		}

		return m, m.readTravel(), true
	case oplogview.MovedMsg:
		if !m.travel.active {
			return m, nil, true
		}

		cmd := m.travelTo(msg.Ago)

		return m, cmd, true
	case oplogview.RestoreMsg:
		next, cmd := m.confirmRestore(msg.Operation)

		return next, cmd, true
	}

	return m, nil, false
}

// confirmRestore shows what a restore goes back to before it does: jj undo can take it back, but a
// restore replaces every revision at once, which deserves a look at the summary first.
func (m Model) confirmRestore(op jj.Operation) (Model, tea.Cmd) {
	ago := m.travel.index
	if m.opLog.IsOpen() {
		ago = m.opLog.Ago()
	}

	detail := []string{op.Description}
	if op.Args != "" {
		detail = append(detail, op.Args)
	}

	detail = append(detail, opsAgo(ago)+", "+op.Age, "", "every revision, bookmark, and @ goes back to how they were")
	m.restoring = op

	cmd := m.confirm.Open(confirm.Request{
		Key:    confirmRestore,
		Title:  "restore to operation " + op.ID + "?",
		Detail: detail,
		Danger: "u undoes the restore as one operation",
	})

	return m, cmd
}

func opsAgo(n int) string {
	if n == 1 {
		return "1 op ago"
	}

	return strconv.Itoa(n) + " ops ago"
}

// restoreConfirmed runs the restore the dialog agreed to and returns to the present it makes.
func (m Model) restoreConfirmed() (Model, tea.Cmd) {
	op := m.restoring.ID
	m.restoring = jj.Operation{}
	m.opLog.Close()
	m.endTravel()

	return m, m.run(undoHint, func(ctx context.Context) (string, error) { return m.client.OpRestore(ctx, op) })
}

// dispatchTravel is the whole of what the past answers to. Moving and stepping read; Enter restores
// after a confirmation; every other key says why it does nothing rather than doing it to the present.
func (m Model) dispatchTravel(action string) (tea.Model, tea.Cmd) {
	if key, ok := navKey(action); ok {
		m.travel.view.Update(key)

		return m, nil
	}

	switch action {
	case help.ActionOpOlder, help.ActionOpNewer:
		cmd := m.stepOp(opStep(action))

		return m, cmd
	case keys.ActionQuit, keys.ActionCancel:
		m.quitting = true

		return m, tea.Quit
	case keys.ActionHelp:
		m.openHelp()

		return m, nil
	case help.ActionOpLog:
		m.opLog.OpenAt(m.travel.ops, m.travel.index)

		return m, nil
	case keys.ActionBack:
		m.endTravel()

		return m, m.say("back at the current operation")
	case help.ActionTravelRestore:
		return m.confirmRestore(m.travel.op())
	}

	return m, m.say("read-only at operation " + m.travel.op().ID + ": " + m.keys.Label(keys.ActionBack) +
		" returns to now, " + m.keys.Label(help.ActionTravelRestore) + " restores it")
}

// travelBanner says, on the row the revset box would take, that the log is not the present.
func (m Model) travelBanner() string {
	op := m.travel.op()
	parts := make([]string, 0, bannerParts)
	parts = append(parts, "◷ viewing at op "+op.ID, opsAgo(m.travel.index))

	switch {
	case m.travel.now():
		parts = append(parts[:0], "◷ the current operation", op.ID)
	case m.travel.err != nil:
		parts = append(parts, "unreadable: "+oneLineErr(m.travel.err))
	case m.travel.shownFor != op.ID:
		parts = append(parts, "reading…")
	default:
		parts = append(parts, op.Description)
	}

	// The way out goes last and is kept whole, since it is the one thing on the row a reader must find.
	if !m.travel.now() {
		parts = append(parts, m.keys.Label(help.ActionTravelRestore)+" restore · "+
			m.keys.Label(keys.ActionBack)+" back to now")
	}

	text := table.Truncate(strings.Join(parts, " · "), m.width)
	if !m.travel.now() && len(parts) > 3 {
		way := " · " + parts[len(parts)-1]
		text = table.Truncate(strings.Join(parts[:len(parts)-1], " · "), max(0, m.width-ansi.StringWidth(way))) + way
	}

	return padLine(m.skin.Warning.Render(text), m.width)
}

// travelHints are the footer's while the log shows the past: the few keys that work, so the
// read-only rule is visible before anyone runs into it.
func (m Model) travelHints() []keyhint.Hint {
	hints := []keyhint.Hint{{
		Key:  m.keys.Label(help.ActionOpOlder) + " " + m.keys.Label(help.ActionOpNewer),
		What: "older / newer", Priority: 1,
	}}
	hints = append(hints, m.keys.Hints(help.ActionTravelRestore, help.ActionOpLog)...)

	if back, ok := m.keys.Hint(keys.ActionBack); ok {
		back.What = "now"
		hints = append(hints, back)
	}

	return append(hints, m.pinnedHelp())
}

// travelFooter replaces the log's footer: the message when one is up, else the notice and the keys.
func (m Model) travelFooter() string {
	message := m.message
	if message == "" {
		message = "read-only"
	}

	room := max(0, m.width-ansi.StringWidth(message)-len(keyhint.Gap))

	return padLine(m.skin.Subtitle.Render(message)+keyhint.Gap+
		keyhint.Fit(m.skin.FooterHintStyles(), m.travelHints(), room), m.width)
}

// opLogFrame draws the op log over the log as it stood at the operation under its cursor, so
// choosing where to restore to is looking at it rather than reading descriptions.
func (m Model) opLogFrame(width, height int) string {
	if !m.travel.active || height < minTravelSplit {
		return m.opLog.View(width, height)
	}

	listHeight := max(minOpList, height*opListShare/opListOf)
	logHeight := height - listHeight - 1

	view := m.travel.view
	view.SetSize(width, logHeight)

	return m.opLog.View(width, listHeight) + "\n" + m.travelBanner() + "\n" + view.View()
}

// Below minTravelSplit rows the op log keeps the frame to itself, since a list and a log split any
// smaller would each show too little to choose by.
// The op log takes opListShare of every opListOf rows, the rest going to the log it explains.
const (
	minTravelSplit = 16
	minOpList      = 7
	opListShare    = 2
	opListOf       = 5
)

// minOps is the fewest operations a trip needs: the present and one before it.
const minOps = 2

// bannerParts is the most pieces the banner joins.
const bannerParts = 5
