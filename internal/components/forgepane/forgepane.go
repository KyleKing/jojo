// Package forgepane renders pull request state, review decision, mergeability,
// the CI rollup, and ahead/behind for the bookmark at or near @, polling in
// the background per internal/forge and never fetching from View.
package forgepane

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/display"
	aforge "github.com/kyleking/aragonite/forge"
	"github.com/kyleking/aragonite/tui/glyph"
	"github.com/kyleking/aragonite/tui/region"

	"github.com/kyleking/jojo/internal/forge"
)

// fetchTimeout bounds one poll so a hung gh call cannot stall the pane
// indefinitely; the next tick tries again regardless.
const fetchTimeout = 20 * time.Second

// Styles is the vocabulary the pane renders in. The caller owns it, so this
// package never learns a palette.
type Styles struct {
	Region region.Styles
	OK     lipgloss.Style
	Warn   lipgloss.Style
	Err    lipgloss.Style
	Dim    lipgloss.Style
}

// FetchedMsg carries the result of one poll or manual refresh.
type FetchedMsg struct {
	Status *forge.Status
	Err    error
}

// tickMsg drives the background poll loop; it never leaves this package.
type tickMsg time.Time

// state is the three UI states the requirements call for, rendered
// distinctly rather than collapsed into one spinner.
type state int

const (
	stateStale state = iota
	stateFresh
	stateFetching
)

// Model is the forge pane. The parent routes size, focus, bookmark changes,
// and key presses to it and renders View in its place.
type Model struct {
	// ctx carries what every poll runs under: the program's lifetime and the gh runner the parent
	// routes the pane's calls through (forge/github.WithRunner).
	ctx       context.Context //nolint:containedctx // the pane's polls outlive no call of their own.
	styles    Styles
	detection forge.Detection
	poller    *forge.Poller
	bookmark  string
	fetching  bool
	width     int
	height    int
}

// New detects repoPath's forge and returns a pane watching bookmark on it.
// No network call happens until Init's command runs.
func New(ctx context.Context, styles Styles, repoPath, bookmark string, opts ...forge.Option) Model {
	detection := forge.Detect(ctx, repoPath)

	m := Model{ctx: ctx, styles: styles, detection: detection, bookmark: bookmark}

	if detection.Supported() {
		allOpts := append([]forge.Option{forge.WithDiskCache(forge.UserDiskCache())}, opts...)
		m.poller = forge.NewPoller(detection.Provider, repoPath, detection.RemoteID, bookmark, allOpts...)
	}

	return m
}

// Init kicks off the pane's first fetch immediately, whatever the initial
// focus state, once it has a bookmark to ask about.
func (m *Model) Init() tea.Cmd {
	// With no bookmark yet there is nothing to ask; SetBookmark fetches once the log names one.
	if m.poller == nil || m.bookmark == "" {
		return nil
	}

	return m.fetchCmd()
}

// Update advances the poll loop and handles the pane's own refresh key. The
// parent routes every other key elsewhere.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tickMsg:
		if m.poller == nil || m.fetching {
			return nil
		}

		return m.fetchCmd()

	case FetchedMsg:
		m.fetching = false

		return m.scheduleNext()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+l" && m.poller != nil && !m.fetching {
			return m.fetchCmd()
		}
	}

	return nil
}

// Lookup is a function answering what the pane's caches hold for a bookmark, safe to call off the
// update loop and never reaching the network: "#12 open · CI passing", or "" when nothing is known.
// It is nil when the repository has no supported forge.
func (m *Model) Lookup() func(bookmark string) string {
	poller := m.poller
	if poller == nil {
		return nil
	}

	return func(bookmark string) string {
		status, ok := poller.Known(bookmark)
		if !ok || status == nil || status.PR == nil {
			return ""
		}

		return Summary(*status)
	}
}

// Summary is a pull request and its CI in a few words, the way a row has room for them.
func Summary(status forge.Status) string {
	pr := status.PR
	text := fmt.Sprintf("#%d %s", pr.Number, strings.ToLower(display.PRStatusDisplay(*pr)))

	if checks := display.ChecksSummary(status.Checks); checks != "" && checks != display.EmDash {
		text += " · CI " + checks
	}

	return text
}

// SetSize records the space the pane renders into.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
}

// SetFocused tells the pane whether it is the one the user is looking at,
// resuming the poll loop when focus returns to it.
func (m *Model) SetFocused(focused bool) tea.Cmd {
	if m.poller == nil {
		return nil
	}

	wasFocused := m.poller.Focused()
	m.poller.SetFocused(focused)

	if focused && !wasFocused && !m.fetching {
		return m.scheduleNext()
	}

	return nil
}

// SetBookmark points the pane at a different bookmark, showing whatever is
// cached for it immediately and fetching a fresh value in the background.
func (m *Model) SetBookmark(bookmark string) tea.Cmd {
	if m.poller == nil || bookmark == m.bookmark || bookmark == "" {
		return nil
	}

	m.bookmark = bookmark
	m.poller.SetBookmark(bookmark)

	if m.fetching {
		return nil
	}

	return m.fetchCmd()
}

func (m *Model) fetchCmd() tea.Cmd {
	m.fetching = true
	poller, parent := m.poller, m.ctx

	if parent == nil {
		parent = context.Background()
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, fetchTimeout)
		defer cancel()

		status, err := poller.Fetch(ctx)

		return FetchedMsg{Status: status, Err: err}
	}
}

func (m *Model) scheduleNext() tea.Cmd {
	if m.poller == nil {
		return nil
	}

	interval, ok := m.poller.Interval()
	if !ok {
		return nil
	}

	return tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) state() state {
	switch {
	case m.fetching:
		return stateFetching
	case m.poller != nil && m.poller.FetchedLive():
		return stateFresh
	default:
		return stateStale
	}
}

func stateCaption(s state) string {
	switch s {
	case stateFresh:
		return "live"
	case stateFetching:
		return "fetching…"
	default:
		return "cached"
	}
}

// View renders the pane. It returns exactly height lines so panes around it
// never shift, matching region.Region's own contract.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	return strings.Join(m.render().Render(m.styles.Region, m.width, m.height), "\n")
}

func (m Model) render() region.Region {
	title := "Forge"
	if m.detection.Host != "" {
		title = "Forge (" + m.detection.Host + ")"
	}

	if !m.detection.Supported() {
		return region.Region{Title: title, Body: []string{unsupportedMessage(m.detection)}}
	}

	status := m.poller.Last()
	caption := stateCaption(m.state())

	body := []string{}
	if err := m.poller.LastErr(); err != nil {
		body = append(body, m.styles.Err.Render(err.Error()))
	}

	if status == nil || status.PR == nil {
		body = append([]string{"no pull request for " + m.bookmark}, body...)

		return region.Region{Title: title, Body: body, Caption: caption}
	}

	return region.Region{
		Title:       title,
		Head:        facts(*status),
		HeadColumns: headColumns,
		Body:        body,
		Caption:     caption,
	}
}

// headColumns is how many label/value pairs the region lays out per line.
const headColumns = 2

func facts(status forge.Status) []region.Fact {
	pr := status.PR

	return []region.Fact{
		{Label: "PR", Value: fmt.Sprintf("#%d %s", pr.Number, glyph.Settle(pr.Title))},
		{Label: "Status", Value: display.PRStatusDisplay(*pr)},
		{Label: "Review", Value: reviewValue(pr)},
		{Label: "Mergeable", Value: emptyDash(pr.Mergeable)},
		{Label: "CI", Value: display.ChecksSummary(status.Checks)},
		{Label: "Behind base", Value: yesNo(status.Behind)},
	}
}

func reviewValue(pr *aforge.PullRequest) string {
	mark := display.PRReviewGlyph(pr)
	if mark == "" {
		return display.PRReviewStatus(*pr)
	}

	return display.PRReviewStatus(*pr) + " " + mark
}

func emptyDash(s string) string {
	if s == "" {
		return display.EmDash
	}

	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}

	return "no"
}

func unsupportedMessage(d forge.Detection) string {
	if d.Host == "" {
		return "no git remote to detect a forge from"
	}

	return "no forge integration for " + d.Host
}
