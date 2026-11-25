package model

import (
	"bytes"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jjdiff"
)

// jjDiffMsg carries what the probe found about jj-diff, and the launch that was waiting on it.
type jjDiffMsg struct {
	mode jjdiff.Mode
	rev  string
	tool jjdiff.Tool
}

// probed reports whether the probe has answered, found or not.
func probed(t jjdiff.Tool) bool { return t.Path != "" || t.Why != "" }

// probeJJDiff asks once, off the update loop, what jj-diff can do, and on its answer launches mode on
// rev when either is set.
func (m Model) probeJJDiff(mode jjdiff.Mode, rev string) tea.Cmd {
	ctx, probe := m.ctx, m.jjDiffProbe

	return func() tea.Msg {
		return jjDiffMsg{tool: probe(ctx), mode: mode, rev: rev}
	}
}

// landJJDiff keeps the probe's answer and runs the launch that waited on it.
func (m Model) landJJDiff(msg jjDiffMsg) (Model, tea.Cmd) {
	m.jjDiff = msg.tool
	if msg.mode == "" {
		return m, nil
	}

	return m, m.openJJDiff(msg.mode, msg.rev)
}

// openJJDiff hands the terminal to jj-diff's split columns or conflict viewer on rev, then reads the
// log again and says in the footer whether it wrote. Without a jj-diff that has the mode, the verb
// still runs, through jj's own interactive split or resolve, and the footer says why.
func (m Model) openJJDiff(mode jjdiff.Mode, rev string) tea.Cmd {
	if !probed(m.jjDiff) {
		return m.probeJJDiff(mode, rev)
	}

	if !m.jjDiff.Modes {
		return m.withoutJJDiff(mode, rev)
	}

	//nolint:gosec // G204: the binary was found on PATH by the probe and the args are built here.
	cmd := exec.CommandContext(m.ctx, m.jjDiff.Path, jjdiff.Args(mode, rev)...)
	cmd.Dir = m.repoPath

	// jj-diff draws on the terminal and says what it did, or why it could not run, on stderr.
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	known := m.knownChanges()

	return m.execThen(cmd, func(err error, wrote bool) tea.Msg {
		done := messageMsg{text: jjdiff.Outcome(mode, rev, err, wrote, stderr.String()), wrote: wrote && err == nil}
		if mode != jjdiff.Split || !done.wrote {
			return done
		}

		return splitWroteMsg{done: done, known: known}
	})
}

// splitWroteMsg is a split that wrote, with every change the log held before it, so the commits the
// split made can be told apart and asked for a description.
type splitWroteMsg struct {
	known map[string]bool
	done  messageMsg
}

// knownChanges is every change id the log holds.
func (m Model) knownChanges() map[string]bool {
	known := make(map[string]bool, len(m.rows))
	for _, row := range m.rows {
		if row.Revision != nil {
			known[row.Revision.ChangeID] = true
		}
	}

	return known
}

// describeSplit asks for a description of the next commit a split made that has none, once the log
// read after the split lands: a push refuses an undescribed commit, and the split's new column is
// one nobody has named yet. Each describe reads the log again and lands here for the next; esc on
// the prompt skips the rest.
func (m *Model) describeSplit() tea.Cmd {
	if m.splitKnown == nil || m.openModal() != "" {
		return nil
	}

	for _, row := range m.rows {
		rev := row.Revision
		if rev == nil || m.splitKnown[rev.ChangeID] || rev.Immutable || strings.TrimSpace(rev.Description) != "" {
			continue
		}

		// Asked once: a description left empty on purpose is not asked for again.
		m.splitKnown[rev.ChangeID] = true
		m.log.Focus(rev.ChangeID)

		return tea.Batch(m.loadDescribe(*rev), m.say("the split made "+rev.ChangeID+": describe it (esc skips)"))
	}

	m.splitKnown = nil

	return nil
}

// withoutJJDiff is the verb through jj alone, its footer line naming what jj-diff lacked.
func (m Model) withoutJJDiff(mode jjdiff.Mode, rev string) tea.Cmd {
	if mode == jjdiff.Resolve {
		return m.execCommand(fallbackLine(m.jjDiff, mode), m.client.ResolveCommand(m.ctx, rev))
	}

	return m.exec(fallbackLine(m.jjDiff, mode), m.hunkArgs("split", "-i", "-r", rev)...)
}

// fallbackLine is the footer after jj's own split or resolve stood in for jj-diff's.
func fallbackLine(tool jjdiff.Tool, mode jjdiff.Mode) string {
	verb := "split with jj split -i"
	if mode == jjdiff.Resolve {
		verb = "resolved with jj resolve"
	}

	return tool.Why + "; " + verb + undoHint
}

// handleExternalMsg covers the programs jojo hands the terminal to: jj-diff's probe, and any program
// having exited.
func (m Model) handleExternalMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case jjDiffMsg:
		next, cmd := m.landJJDiff(msg)

		return next, cmd, true
	case execDoneMsg:
		// jj-diff, or any program handed the terminal, may have saved marks of its own.
		m.reloadMarks()

		return m, m.finishExec(msg), true
	}

	return m, nil, false
}
