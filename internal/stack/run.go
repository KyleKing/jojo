package stack

import (
	"context"
	"errors"
	"fmt"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/jj"
)

// Forge runs a plan's gh calls, each exactly as the plan's Command lists it: a create, which
// answers the new pull request, and an edit, a retarget or a body fed on stdin.
type Forge interface {
	Create(ctx context.Context, args []string, body string) (int, string, error)
	Edit(ctx context.Context, args []string, stdin string) error
}

// Writer runs a jj write; jj.Client.Apply is one.
type Writer func(ctx context.Context, args []string) (string, error)

// Result is what a submit did: each step's pull request number, and the URLs of the ones it opened.
type Result struct {
	Numbers map[string]int
	Opened  []string
	Ran     int
}

// Run carries plan out, command by command, stopping at the first that fails. What ran before it
// stays done: a push cannot be undone, and running the submit again picks up where this one stopped,
// because the next plan is read from what the forge then says.
func Run(ctx context.Context, plan Plan, write Writer, forge Forge) (Result, error) {
	res := Result{Numbers: Numbers(plan.Steps)}

	for _, c := range plan.Commands {
		if err := runOne(ctx, plan, c, write, forge, &res); err != nil {
			return res, fmt.Errorf("%s: %w", c.Line(), err)
		}

		res.Ran++
	}

	return res, nil
}

func runOne(ctx context.Context, plan Plan, c Command, write Writer, forge Forge, res *Result) error {
	if c.Kind == KindBookmark || c.Kind == KindPush {
		_, err := write(ctx, c.Args)
		if err != nil && !errors.Is(err, jj.ErrNothingChanged) {
			return err
		}

		return nil
	}

	s := &plan.Steps[c.Step]

	switch c.Kind {
	case KindCreate:
		n, url, err := forge.Create(ctx, c.Args, s.Change.Body)
		if err != nil {
			return fmt.Errorf("opening %s's pull request: %w", s.Bookmark, err)
		}

		res.Numbers[s.Bookmark] = n
		res.Opened = append(res.Opened, url)
	case KindRetarget:
		if err := forge.Edit(ctx, c.Args, ""); err != nil {
			return fmt.Errorf("retargeting %s: %w", s.Bookmark, err)
		}
	case KindBody:
		if err := forge.Edit(ctx, c.Args, gh.BodyStdin(Body(plan, c.Step, res.Numbers))); err != nil {
			return fmt.Errorf("writing %s's stack section: %w", s.Bookmark, err)
		}
	case KindBookmark, KindPush:
	}

	return nil
}
