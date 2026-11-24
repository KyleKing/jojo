// Package stack turns a stack of jj changes into stacked pull requests: one bookmark per change,
// each pull request based on the bookmark below it and the bottom one on trunk. Plan works out what
// a submit would do from what jj and the forge already say, as the exact jj and gh commands, and Run
// carries the plan out. Nothing here draws; the model shows the plan and asks before Run.
package stack

import (
	"regexp"
	"slices"
	"strings"

	"github.com/kyleking/aragonite/trail"

	"github.com/kyleking/jojo/internal/gh"
)

// DefaultTemplate names a bookmark for a change that has none: jj's own `jj git push -c` name.
const DefaultTemplate = "push-{change}"

// Change is one revision of the stack, parent first, with what a plan needs to know about it.
type Change struct {
	// ID is the change id as the log prints it, LongID the one a generated bookmark is named from.
	ID     string
	LongID string
	// Subject is the description's first line, the pull request's title; Body is the rest.
	Subject string
	Body    string
	// Bookmark is the local bookmark already on the change, "" when it has none.
	Bookmark string
	// Pushed says the remote already holds the bookmark at this commit, so a push would move nothing.
	Pushed      bool
	Empty       bool
	Immutable   bool
	Conflict    bool
	Divergent   bool
	WorkingCopy bool
}

// Undescribed reports a change with no subject, which jj refuses to push.
func (c Change) Undescribed() bool { return strings.TrimSpace(c.Subject) == "" }

// Trim drops what a stack carries but nobody submits: an empty, undescribed working copy at the top,
// which is where jj leaves @ after a commit. It answers what was dropped.
func Trim(changes []Change) ([]Change, []Change) {
	n := len(changes)
	if n > 0 {
		top := changes[n-1]
		if top.WorkingCopy && top.Empty && top.Undescribed() {
			return changes[:n-1], changes[n-1:]
		}
	}

	return changes, nil
}

// Refusals are the reasons a stack cannot be submitted, one per change that blocks it, empty when it
// can. Each names the change and the fix.
func Refusals(changes []Change) []string {
	if len(changes) == 0 {
		return []string{"nothing to submit: the stack holds no change above trunk"}
	}

	var out []string

	for _, c := range changes {
		switch {
		case c.Immutable:
			out = append(out, c.ID+" is immutable: it is already on trunk or protected")
		case c.Conflict:
			out = append(out, c.ID+" is conflicted: x resolves it")
		case c.Divergent:
			out = append(out, c.ID+" is divergent: X converges it")
		case c.Empty:
			out = append(out, c.ID+" is empty: a pull request needs a change; a abandons it")
		case c.Undescribed():
			out = append(out, c.ID+" has no description: d describes it")
		}
	}

	return out
}

// Action is what a submit does to one change's pull request.
type Action int

// The actions, in the order a plan's legend explains them.
const (
	// Create opens a pull request for a change that has none.
	Create Action = iota
	// Update pushes the change to the pull request it already has.
	Update
	// Retarget pushes it and moves the pull request onto a new base.
	Retarget
	// Unchanged has nothing to push and nothing to move.
	Unchanged
	// Merged is a change whose pull request already merged: left alone.
	Merged
)

// String is the action as a plan row says it.
func (a Action) String() string {
	return [...]string{"new", "update", "retarget", "up to date", "merged"}[a]
}

// Step is one change of a plan.
type Step struct {
	PR       *gh.PR
	Bookmark string
	Base     string
	// WasBase is the base a retarget moves the pull request off.
	WasBase     string
	Change      Change
	Action      Action
	NewBookmark bool
	Push        bool
}

// Kind is which of a submit's commands a Command is.
type Kind int

// Commands run in this order: bookmarks, one push, pull requests bottom up, retargets, then bodies,
// which wait for every number so each can link the others.
const (
	KindBookmark Kind = iota
	KindPush
	KindCreate
	KindRetarget
	KindBody
)

// Command is one jj or gh call of a plan.
type Command struct {
	Args []string
	// Step is the index of the step the command serves, -1 for the push, which serves them all.
	Step int
	Kind Kind
}

// Tool is the program the command runs.
func (c Command) Tool() string {
	if c.Kind == KindBookmark || c.Kind == KindPush {
		return "jj"
	}

	return "gh"
}

// Line is the command as a shell line. A gh call whose body goes on stdin says so, since the body is
// written at run time once every number in the stack is known.
func (c Command) Line() string {
	line := trail.Shell(append([]string{c.Tool()}, c.Args...))
	if c.Kind == KindCreate || c.Kind == KindBody {
		line += " <<< body"
	}

	return line
}

// Plan is what a submit will do.
type Plan struct {
	Trunk    string
	Steps    []Step
	Commands []Command
	// Skipped are changes the stack carries that the plan leaves out (an empty working copy on top).
	Skipped []Change
	// Notes are what the reader should know before running it: a pull request left behind by a change
	// that left the stack, or a merged one under what is still open.
	Notes []string
	Draft bool
}

// Input is what Build reads: the stack parent first, the forge's pull request per bookmark, the
// branch trunk() is on, the bookmark name template, and whether new pull requests open as drafts.
type Input struct {
	PRs      map[string]*gh.PR
	Trunk    string
	Template string
	Changes  []Change
	Skipped  []Change
	Draft    bool
}

// Build works out the plan. It writes nothing.
func Build(in Input) Plan {
	plan := Plan{Trunk: in.Trunk, Draft: in.Draft, Skipped: in.Skipped}
	base := in.Trunk
	inStack := map[string]bool{in.Trunk: true}

	for _, c := range in.Changes {
		step := Step{Change: c, Bookmark: c.Bookmark}
		if step.Bookmark == "" {
			step.Bookmark, step.NewBookmark = BookmarkName(in.Template, c), true
		}

		inStack[step.Bookmark] = true
		step.PR = in.PRs[step.Bookmark]
		step.Base = base
		step.Push = step.NewBookmark || !c.Pushed

		switch {
		case step.PR != nil && step.PR.State == gh.StateMerged:
			step.Action, step.Push = Merged, false
		case step.PR == nil || step.PR.State == gh.StateClosed:
			step.Action = Create
		case step.PR.BaseRefName != base:
			step.Action, step.WasBase = Retarget, step.PR.BaseRefName
		case step.Push:
			step.Action = Update
		default:
			step.Action = Unchanged
		}

		if step.Action != Merged {
			base = step.Bookmark
		}

		plan.Steps = append(plan.Steps, step)
	}

	plan.Notes = notes(plan, inStack)
	plan.Commands = commands(plan)

	return plan
}

// notes are what Build saw that the reader should act on themselves.
func notes(plan Plan, inStack map[string]bool) []string {
	var out []string

	merged := false

	for k := range plan.Steps {
		s := &plan.Steps[k]
		if s.Action == Merged {
			merged = true

			out = append(out, "#"+itoa(s.PR.Number)+" ("+s.Change.ID+") merged: rebase what is above it onto trunk "+
				"(r -s onto trunk()) before the next submit, or its diff repeats the merged change")
		}

		if s.Action == Retarget && !inStack[s.WasBase] {
			out = append(out, "#"+itoa(s.PR.Number)+" was based on "+s.WasBase+", which is no longer in the stack: "+
				"its pull request stays open until you close it")
		}
	}

	if merged && plan.Steps[0].Action != Merged {
		out = append(out, "a merged pull request sits above an open one; the stack's order changed since it merged")
	}

	return out
}

// commands are the calls that carry a plan out, in the order Run makes them.
func commands(plan Plan) []Command {
	var (
		out    []Command
		pushes []string
	)

	creates := false

	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.NewBookmark {
			out = append(out, Command{Kind: KindBookmark, Step: i, Args: BookmarkArgs(s.Bookmark, s.Change.LongID)})
		}

		if s.Push {
			pushes = append(pushes, s.Bookmark)
		}

		creates = creates || s.Action == Create
	}

	if len(pushes) > 0 {
		out = append(out, Command{Kind: KindPush, Step: -1, Args: PushArgs(pushes)})
	}

	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.Action == Create {
			out = append(out, Command{Kind: KindCreate, Step: i, Args: gh.CreateArgs(createOptions(s, plan.Draft))})
		}
	}

	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.Action == Retarget {
			out = append(out, Command{Kind: KindRetarget, Step: i, Args: gh.RetargetArgs(s.Bookmark, s.Base)})
		}
	}

	numbers := Numbers(plan.Steps)

	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.Action == Merged {
			continue
		}

		// A new pull request changes every number the others link, so each body is rewritten; with
		// none, only a body whose stack section would read differently is.
		if creates || normalized(Body(plan, i, numbers)) != normalized(s.PR.Body) {
			out = append(out, Command{Kind: KindBody, Step: i, Args: gh.BodyArgs(s.Bookmark)})
		}
	}

	return out
}

// createOptions is the gh pr create a step runs.
func createOptions(s *Step, draft bool) gh.CreateOptions {
	return gh.CreateOptions{Head: s.Bookmark, Base: s.Base, Title: s.Change.Subject, Draft: draft}
}

// BookmarkArgs is the jj call that names a change for its pull request.
func BookmarkArgs(name, rev string) []string { return []string{"bookmark", "create", name, "-r", rev} }

// PushArgs is the one jj call that pushes every bookmark of the stack that moved. Naming a bookmark
// creates and tracks it on the remote, and a moved one is force-pushed with a lease.
func PushArgs(names []string) []string {
	args := []string{"git", "push"} //nolint:prealloc // two words, then a flag and a name per bookmark
	for _, n := range names {
		args = append(args, "--bookmark", n)
	}

	return args
}

// Numbers is the pull request number of each step's bookmark that has one.
func Numbers(steps []Step) map[string]int {
	out := map[string]int{}

	for k := range steps {
		s := &steps[k]
		if s.PR != nil && s.Action != Create {
			out[s.Bookmark] = s.PR.Number
		}
	}

	return out
}

var unsafeRef = regexp.MustCompile(`[^a-z0-9]+`)

// slugMax keeps a bookmark named from a subject short enough to read in a branch list.
const slugMax = 40

// BookmarkName fills template for c: {change} is its change id, {subject} its subject as a slug.
func BookmarkName(template string, c Change) string {
	if template == "" {
		template = DefaultTemplate
	}

	slug := strings.Trim(unsafeRef.ReplaceAllString(strings.ToLower(c.Subject), "-"), "-")
	if len(slug) > slugMax {
		slug = strings.TrimRight(slug[:slugMax], "-")
	}

	return strings.NewReplacer("{change}", c.LongID, "{subject}", slug).Replace(template)
}

// Open is the steps whose pull requests a submit leaves open, bottom first.
func (p Plan) Open() []Step {
	return slices.DeleteFunc(slices.Clone(p.Steps), func(s Step) bool { return s.Action == Merged })
}

// Writes reports whether the plan changes anything.
func (p Plan) Writes() bool { return len(p.Commands) > 0 }
