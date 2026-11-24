package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/tuitest"
)

// ' labels the rows on screen and a label lands the cursor; ctrl+o goes back to where it left from,
// ctrl+i forward again, and tab right after ctrl+o walks forward too.
func TestLabelJump_LandsAndTheJumpListWalksBack(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "wkwyyxvq")
	m, _ = pressOne(t, m, "'")

	if !m.labelsShown() || !strings.Contains(tuitest.StripANSI(m.render()), "d koopqtrk") {
		t.Fatalf("' drew no labels:\n%s", tuitest.StripANSI(m.render()))
	}

	m, _ = pressOne(t, m, "d")
	if m.labelsShown() || m.log.Selected().ChangeID != "koopqtrk" {
		t.Fatalf("the label d landed on %s (labels up %v)", m.log.Selected().ChangeID, m.labelsShown())
	}

	m, _ = pressOne(t, m, "ctrl+o")
	if got := m.log.Selected().ChangeID; got != "wkwyyxvq" {
		t.Errorf("ctrl+o went to %s, want wkwyyxvq", got)
	}

	m, _ = pressOne(t, m, "tab")
	if got := m.log.Selected().ChangeID; got != "koopqtrk" {
		t.Errorf("tab right after ctrl+o went to %s, want koopqtrk", got)
	}

	m, _ = pressOne(t, m, "ctrl+o")
	m, _ = pressOne(t, m, "ctrl+i")

	if got := m.log.Selected().ChangeID; got != "koopqtrk" {
		t.Errorf("ctrl+i went to %s, want koopqtrk", got)
	}
}

// A key no label starts with puts the labels away and says so; ctrl+o with no jump says why.
func TestLabelJump_RefusalsSayWhy(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "wkwyyxvq")
	if _, msgs := pressOne(t, m, "ctrl+o"); !strings.Contains(said(msgs), "no jump to go back to yet") {
		t.Errorf("ctrl+o with no jumps said %q", said(msgs))
	}

	m, _ = pressOne(t, m, "'")
	m, msgs := pressOne(t, m, "z")

	if m.labelsShown() || !strings.Contains(said(msgs), "no label z") {
		t.Errorf("z left the labels up (%v) or said %q", m.labelsShown(), said(msgs))
	}
}

// In a picker a label moves the destination, and ]b and gg leave jumps the list remembers.
func TestLabelJump_InThePicker(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "wkwyyxvq")
	for _, k := range []string{"y", "'", "f"} {
		m, _ = pressOne(t, m, k)
	}

	if m.pick.dest != "twnsvwmr" || m.labelsShown() {
		t.Errorf("the label f in the picker chose %s", m.pick.dest)
	}

	if pairs := labelsFor(30); len(pairs) != 30 || pairs[0] != "aa" || pairs[29] != "sf" {
		t.Errorf("past 26 rows every label is a pair, got %v", pairs)
	}
}

// The labels over the log at both sizes.
func TestLabelJump_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m, _ := pressOne(t, verbFrame(t, size, "rnzxmskv"), "'")
			assertVerbFrame(t, m.render(), size)
		})
	}
}
