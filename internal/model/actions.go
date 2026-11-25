package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/trail"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/jjdiff"
)

// targets is the one rule every verb that takes several revisions follows: the checked revisions when
// any are checked, else the one under the cursor. The footer counts the checks while they stand, and
// a successful write or Esc clears them, so a check cannot quietly redirect a later action.
func (m Model) targets() []string {
	if checked := m.log.Checked(); len(checked) > 0 {
		return checked
	}

	if rev := m.log.Selected(); rev != nil {
		return []string{rev.ChangeID}
	}

	return nil
}

// revisionAction runs the verbs on the revision under the cursor (or the checked ones). Each
// executes immediately and offers undo rather than opening a confirm gate, because the operation log
// already covers them; the verbs that need a destination open the in-log picker.
func (m Model) revisionAction(action string) (tea.Model, tea.Cmd) {
	rev := m.log.Selected()
	if rev == nil {
		return m, nil
	}

	if action == keys.ActionRepeat {
		return m.repeat()
	}

	if cmd, handled := m.writeAction(action, *rev); handled {
		return m, cmd
	}

	if cmd, handled := m.viewAction(action, *rev); handled {
		return m, cmd
	}

	return m.openAction(action, *rev)
}

// writeAction covers the verbs that need no argument, so they run as the key lands.
func (m *Model) writeAction(action string, rev jj.Revision) (tea.Cmd, bool) {
	targets := m.targets()
	m.noteWrite(action)

	switch action {
	case help.ActionNew:
		return m.act(joinIDs(targets),
			func(ctx context.Context, c *jj.Client) (string, error) { return c.New(ctx, targets) }), true
	case help.ActionEdit:
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Edit(ctx, rev.ChangeID) }), true
	case help.ActionAbandon:
		return m.abandon(targets), true
	case help.ActionAbsorb:
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Absorb(ctx, rev.ChangeID) }), true
	case help.ActionParallelize:
		return m.parallelize(), true
	case help.ActionCheck:
		m.log.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})

		return nil, true
	}

	return nil, false
}

// viewAction covers the verbs that read rather than write, each opening a panel or a diff.
func (m Model) viewAction(action string, rev jj.Revision) (tea.Cmd, bool) {
	switch action {
	case keys.ActionInspect:
		return m.loadDetail(rev.ChangeID), true
	case keys.ActionYank:
		return tea.Batch(tea.SetClipboard(rev.ChangeID), m.say("yanked "+rev.ChangeID)), true
	case help.ActionFiles:
		return m.filesKey(rev), true
	case help.ActionResolve:
		return m.resolveKey(rev), true
	case help.ActionPush:
		targets := m.targets()

		return m.previewPush(joinIDs(targets), jj.PushOptions{Revisions: targets}), true
	case help.ActionDiff:
		return m.showRevisionDiff(rev), true
	case help.ActionCompare:
		return m.openCompare(rev.ChangeID), true
	}

	return nil, false
}

// openAction covers the verbs that need an argument, a terminal, or a view of their own.
func (m Model) openAction(action string, rev jj.Revision) (tea.Model, tea.Cmd) {
	switch action {
	case keys.ActionDescribe:
		cmd := m.loadDescribe(rev)

		return m, cmd
	case keys.ActionDescribeEditor:
		return m, m.describeInEditor(rev.ChangeID)
	case help.ActionCommit:
		return m.openCommitPrompt(rev)
	case help.ActionDiffedit:
		return m, m.exec("edited "+rev.ChangeID+undoHint, m.hunkArgs("diffedit", "-r", rev.ChangeID)...)
	case help.ActionSplit:
		return m, m.openJJDiff(jjdiff.Split, rev.ChangeID)
	case help.ActionBisect:
		return m.openBisect()
	case help.ActionBookmarkCreate:
		target := rev.ChangeID

		return m.askBookmarkName("new bookmark on "+target, func(m Model, name string) (Model, tea.Cmd) {
			return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
				return m.client.BookmarkSet(ctx, name, target)
			})
		}), nil
	}

	if next, cmd, handled := m.bookmarkMenu(action, rev); handled {
		return next, cmd
	}

	cmd := m.openVerb(action, rev)

	return m, cmd
}

// openVerb puts the log into picking mode for a verb that needs a destination. New takes the checked
// revisions as extra parents and nothing else, so with none checked it is a destination alone, and
// restore acts on the one revision under the cursor.
func (m *Model) openVerb(action string, rev jj.Revision) tea.Cmd {
	targets := m.targets()

	switch action {
	case help.ActionSquash:
		return m.openPicker(pickSquash, targets)
	case help.ActionDuplicate:
		return m.openPicker(pickDuplicate, targets)
	case help.ActionNewPlaced:
		return m.openPicker(pickNew, m.log.Checked())
	case help.ActionRestore:
		return m.openPicker(pickRestore, []string{rev.ChangeID})
	case help.ActionRebase:
		return m.openPicker(pickRebase, targets)
	case help.ActionRebaseSubtree:
		return m.openPicker(pickRebase, targets, "s")
	}

	return nil
}

// openCommitPrompt asks for the message the working copy is committed with. Commit only acts on @,
// so on any other row it says where to go rather than committing something else.
func (m Model) openCommitPrompt(rev jj.Revision) (tea.Model, tea.Cmd) {
	if !rev.WorkingCopy {
		return m, m.say("c commits @, the working copy: @ jumps there")
	}

	m.onSubmit = func(m Model, text string) (Model, tea.Cmd) {
		if strings.TrimSpace(text) == "" {
			return m, m.say("commit needs a message, nothing ran")
		}

		cmd := m.act("@", func(ctx context.Context, c *jj.Client) (string, error) { return c.Commit(ctx, text) })

		return m, cmd
	}

	return m, m.prompt.OpenText("commit @", rev.Description)
}

// shell hands the terminal to $SHELL in the repository root and reloads the log on return, since
// whatever ran there may have rewritten it.
func (m Model) shell() tea.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "sh"
	}

	cmd := exec.CommandContext(m.ctx, sh) //nolint:gosec // G204: the user's own shell, by design.
	cmd.Dir = m.repoPath

	return m.execCommand("back from the shell", cmd)
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}

	if len(ids) == 1 {
		return ids[0]
	}

	return ids[0] + " and " + strconv.Itoa(len(ids)-1) + " more"
}

// exec hands the terminal to jj, which is how the interactive verbs reach jj's own diff editor.
func (m Model) exec(done string, args ...string) tea.Cmd {
	//nolint:gosec // G204: the binary is a literal and args are built here, never from user input.
	cmd := exec.CommandContext(m.ctx, "jj", args...)
	cmd.Dir = m.repoPath

	return m.execCommand(done, cmd)
}

func (m Model) execCommand(done string, cmd *exec.Cmd) tea.Cmd {
	// The repaint covers whatever the tool printed, so the message has to name the command itself.
	// An exit status says nothing a reader who just quit the editor does not already know, so the
	// line reports the one fact that matters: nothing was written.
	what := strings.Join(cmd.Args, " ")

	return m.execThen(cmd, func(err error, _ bool) tea.Msg {
		if err != nil {
			return messageMsg{text: what + " did not finish"}
		}

		return messageMsg{text: done}
	})
}

// execThen suspends the TUI, hands cmd the terminal, and once it has exited says what outcome makes
// of its exit and whether jj's operation log moved, then reads the log again, since the program may
// have rewritten anything. The jj queue is held while it runs, so no background read races its
// writes. A sequence does not wait for the program to exit before running its next command, so
// everything after the exit runs from execDoneMsg, which the program sends once it has.
//
// The trail records the program as a write, since it was handed the terminal to change something;
// what it wrote went to the terminal, so its row has no stderr.
func (m Model) execThen(cmd *exec.Cmd, outcome func(err error, wrote bool) tea.Msg) tea.Cmd {
	client := m.client
	before := new(string)
	call := new(trail.Call)

	return tea.Sequence(
		func() tea.Msg {
			*before = headOp(m.ctx, client)
			client.AcquireQueue()
			// The trail names the program as a reader would type it, not the path PATH resolved.
			argv := slices.Clone(cmd.Args)
			if len(argv) > 0 {
				argv[0] = filepath.Base(argv[0])
			}

			*call = trail.Call{Argv: argv, Dir: cmd.Dir, Start: time.Now()}

			return nil
		},
		tea.ExecProcess(cmd, func(err error) tea.Msg {
			call.Took, call.Exit = time.Since(call.Start), trail.ExitCode(err)
			if err != nil && call.Exit == -1 {
				call.Stderr = err.Error()
			}

			return execDoneMsg{err: err, before: *before, outcome: outcome, call: *call}
		}),
	)
}

// execDoneMsg is a program jojo handed the terminal to having exited, with its call for the trail,
// which is filed once it is known whether the program wrote.
type execDoneMsg struct {
	err     error
	outcome func(err error, wrote bool) tea.Msg
	before  string
	call    trail.Call
}

// finishExec gives the queue back, files the call in the trail as a write only when jj's operation
// log moved under it, then says how the program went and reads the log it left. A split canceled in
// jj-diff wrote nothing, and the trail counted it as a write.
func (m Model) finishExec(msg execDoneMsg) tea.Cmd {
	client, ctx, log := m.client, m.ctx, m.trail

	return tea.Sequence(
		func() tea.Msg {
			client.ReleaseQueue()

			wrote := headOp(ctx, client) != msg.before
			msg.call.Write = wrote
			log.Add(msg.call)

			return msg.outcome(msg.err, wrote)
		},
		m.loadLog(m.revset),
	)
}

// headOp is the id of jj's newest operation, "" when it cannot be read.
func headOp(ctx context.Context, client *jj.Client) string {
	ops, err := client.OpLog(ctx, 1)
	if err != nil || len(ops) == 0 {
		return ""
	}

	return ops[0].ID
}

func (Model) say(text string) tea.Cmd {
	return func() tea.Msg { return messageMsg{text: text} }
}

// undoHint replaces a confirm gate, because the operation log makes every one of these reversible.
const undoHint = ", press u to undo"

// wrote turns a write's outcome into the status line: the command that ran, or the offer of the
// escape hatch when jj refused it as immutable.
//
//nolint:ireturn // a tea.Msg is the return type the runtime takes; naming a concrete one would lie.
func wrote(ran, hint string, err error) tea.Msg {
	switch {
	case errors.Is(err, jj.ErrNothingChanged):
		return messageMsg{text: ran + ": nothing changed"}
	case errors.Is(err, jj.ErrImmutable):
		return refusedMsg{err: err}
	case errors.Is(err, jj.ErrStaleWorkingCopy):
		return staleMsg{err: err}
	case err != nil:
		return messageMsg{text: err.Error(), output: jj.FullOutput(err)}
	}

	return messageMsg{text: ran + hint, wrote: true}
}

// run executes a jj write then reloads the log. A write that went through is echoed from the trail,
// the exact command the client ran and the way back: "jj squash --from wkwy --into rnzx · press u to
// undo", so every write teaches its command line.
func (m Model) run(hint string, mutate func(context.Context) (string, error)) tea.Cmd {
	revset, ctx, log := m.revset, m.ctx, m.trail

	return tea.Sequence(
		func() tea.Msg {
			ran, err := mutate(ctx)

			return echoed(log, ran, hint, wrote(ran, hint, err))
		},
		m.loadLog(revset),
	)
}

// echoed is msg with a write's status line taken from the trail's record of the call that ran, when
// the trail has it.
//
//nolint:ireturn // a tea.Msg is the return type the runtime takes; naming a concrete one would lie.
func echoed(log *trail.Trail, ran, hint string, msg tea.Msg) tea.Msg {
	done, ok := msg.(messageMsg)
	if !ok || !done.wrote {
		return msg
	}

	if call, found := log.Last(func(c *trail.Call) bool { return c.Write && !c.Failed() && c.Shell() == ran }); found {
		done.text = trail.Echo("", &call, strings.TrimPrefix(hint, ", "))
	}

	return done
}

// undo and redo are jj's own, which step through the operation log the way an editor's undo stack
// does and, unlike restoring an operation by id, refuse an operation another workspace performed.
func (m Model) undo() tea.Cmd {
	return m.run(", press ctrl+r to redo", m.client.Undo)
}

func (m Model) redo() tea.Cmd {
	return m.run(undoHint, m.client.Redo)
}
