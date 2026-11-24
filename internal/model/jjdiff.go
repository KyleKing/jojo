package model

import (
	"os/exec"

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

	return m.execThen(cmd, func(err error, wrote bool) tea.Msg {
		return messageMsg{text: jjdiff.Outcome(mode, rev, err, wrote), wrote: wrote && err == nil}
	})
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
		return m, m.finishExec(msg), true
	}

	return m, nil, false
}
