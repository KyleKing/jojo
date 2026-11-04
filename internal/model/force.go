package model

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/jj"
)

// Keys the confirmation dialog reports back, naming what a yes runs.
const (
	confirmForce     = "force"
	confirmPush      = "push"
	confirmStale     = "stale"
	confirmDivergent = "divergent"
)

// pendingPush is the push a confirmation is standing over, kept whole so the message it reports
// names what was pushed rather than rebuilding it from the flags.
type pendingPush struct {
	what string
	opts jj.PushOptions
}

// refusedMsg carries a write jj declined because the target is protected.
type refusedMsg struct {
	err error
}

// forcible is the last write that could be retried past jj's immutable guard. Keeping the closure
// rather than the arguments means a retry runs the same call, including a description already typed.
type forcible struct {
	fn   func(context.Context, *jj.Client) (string, error)
	what string
}

// act runs a write and remembers how to run it again with the immutable guard off, so a refusal can
// offer the way past rather than reporting a dead end.
func (m *Model) act(what string, fn func(context.Context, *jj.Client) (string, error)) tea.Cmd {
	m.forcible = forcible{fn: fn, what: what}
	client := m.client

	return m.run(undoHint, func(ctx context.Context) (string, error) { return fn(ctx, client) })
}

// offerForce explains why jj refused and what the escape costs, because the refusal is the one place
// a user meets jj's immutability rule and it is easier to obey a rule that says why.
func (m Model) offerForce(msg refusedMsg) (tea.Model, tea.Cmd) {
	if m.forcible.fn == nil {
		m.message = msg.err.Error()

		return m, nil
	}

	cmd := m.confirm.Open(confirm.Request{
		Key:   confirmForce,
		Title: "rewrite " + m.forcible.what + " anyway?",
		Detail: []string{
			"jj refuses because it is in immutable_heads() or one of its ancestors,",
			"which is how jj marks what has already been published.",
		},
		Danger: "u undoes this here, not on a machine that already has the old revision",
	})

	return m, cmd
}

func (m Model) handleConfirmed(msg confirm.ConfirmedMsg) (tea.Model, tea.Cmd) {
	switch msg.Key {
	case confirmForce:
		forced := m.forcible
		m.forcible = forcible{}
		client := m.client.IgnoringImmutable()

		return m, m.run(undoHint, func(ctx context.Context) (string, error) { return forced.fn(ctx, client) })
	case confirmPush:
		pending := m.pushing
		m.pushing = pendingPush{}

		return m, m.push(pending)
	case confirmStale:
		return m, m.run(undoHint, m.client.UpdateStaleWorkspace)
	case confirmDivergent:
		return m, m.run(undoHint, m.client.AbandonDivergent)
	}

	return m, nil
}

func (m Model) cancelConfirm() Model {
	m.confirm.Close()
	m.forcible = forcible{}
	m.pushing = pendingPush{}

	return m
}
