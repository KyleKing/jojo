package tuitest

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// Size is one width by height frame an overlay is asked to render into.
type Size struct {
	Width, Height int
}

// String renders a Size as "WxH", which is what a subtest and a failure
// message both want.
func (s Size) String() string {
	return fmt.Sprintf("%dx%d", s.Width, s.Height)
}

// The grid OverlaySizes and HintSizes draw from: the floor most terminals
// give, a run of increasingly awkward smaller frames down to the point where
// a modal's own border and padding no longer fit, and one size larger than
// any real terminal.
const (
	floorWidth, floorHeight   = 80, 24
	looseWidth, looseHeight   = 60, 20
	tightWidth, tightHeight   = 40, 12
	narrowWidth, narrowHeight = 30, 8
	thinWidth, thinHeight     = 20, 5
	edgeWidth, edgeHeight     = 6, 4
	pointWidth, pointHeight   = 1, 1
	hugeWidth, hugeHeight     = 200, 60
)

// OverlaySizes is the grid of terminal sizes every full-screen overlay in
// this app has to render sanely into.
func OverlaySizes() []Size {
	return []Size{
		{Width: floorWidth, Height: floorHeight},
		{Width: looseWidth, Height: looseHeight},
		{Width: tightWidth, Height: tightHeight},
		{Width: narrowWidth, Height: narrowHeight},
		{Width: thinWidth, Height: thinHeight},
		{Width: edgeWidth, Height: edgeHeight},
		{Width: pointWidth, Height: pointHeight},
		{Width: hugeWidth, Height: hugeHeight},
	}
}

// HintSizes is the subset of OverlaySizes tall enough that a key hint row is
// never the line overlay.Center's own vertical elision drops, so a caller
// checking that a hint line degrades by width alone can ignore height.
func HintSizes() []Size {
	return []Size{
		{Width: floorWidth, Height: floorHeight},
		{Width: looseWidth, Height: looseHeight},
		{Width: tightWidth, Height: tightHeight},
		{Width: hugeWidth, Height: hugeHeight},
	}
}

// AssertOverlayInvariants renders view at every size OverlaySizes returns and
// checks what every full-screen overlay in this app promises: it never
// panics, no rendered line exceeds the requested width, the block never
// exceeds the requested height, and it never draws a blank frame.
func AssertOverlayInvariants(t *testing.T, view func(width, height int) string) {
	t.Helper()

	for _, size := range OverlaySizes() {
		t.Run(size.String(), func(t *testing.T) {
			t.Helper()
			assertFitsFrame(t, size.Width, size.Height, renderSafely(t, size, view))
		})
	}
}

func renderSafely(t *testing.T, size Size, view func(width, height int) string) string {
	t.Helper()

	var frame string

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("View(%s) panicked: %v", size, r)
			}
		}()

		frame = view(size.Width, size.Height)
	}()

	return frame
}

// AssertLineFits fails when line, measured in display cells rather than
// bytes, is wider than width: the invariant every row of an overlay frame
// and every row of a plain single-line view (such as search's) promises.
func AssertLineFits(t *testing.T, width int, line string) {
	t.Helper()

	if w := lipgloss.Width(StripANSI(line)); w > width {
		t.Errorf("line is %d cells wide, want at most %d: %q", w, width, line)
	}
}

func assertFitsFrame(t *testing.T, width, height int, frame string) {
	t.Helper()

	lines := strings.Split(StripANSI(frame), "\n")
	if len(lines) > height {
		t.Errorf("frame is %d rows tall, want at most %d", len(lines), height)
	}

	for _, line := range lines {
		AssertLineFits(t, width, line)
	}

	if strings.TrimSpace(strings.Join(lines, "")) == "" {
		t.Error("frame drew nothing")
	}
}
