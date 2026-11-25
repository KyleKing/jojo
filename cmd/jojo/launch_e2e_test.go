package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selectedRow is the row of the log the cursor is on: the selection is a background laid under the
// row, so its gutter cell is the one with a background. -1 when no row shows one.
func (s *screen) selectedRow(rows int) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	for y := range rows {
		if cell := s.term.CellAt(0, y); cell != nil && cell.Style.Bg != nil {
			return y
		}
	}

	return -1
}

// shows reports whether text is anywhere on the terminal.
func (s *screen) shows(text string) bool {
	return strings.Contains(strings.Join(s.frame(), "\n"), text)
}

// firstLaunch is env with the coach's ledger removed, so jojo opens on its welcome card. Its home
// is the subtest's own, since dismissing the card writes the ledger.
func firstLaunch(t *testing.T) []string {
	t.Helper()

	env := isolated(t)

	// The last of a repeated variable is the one the child sees, and isolated's comes last.
	state := ""

	for _, kv := range env {
		if dir, ok := strings.CutPrefix(kv, "XDG_STATE_HOME="); ok {
			state = dir
		}
	}

	if err := os.Remove(filepath.Join(state, "jojo", "coach.json")); err != nil {
		t.Fatal(err)
	}

	return env
}

// TestLaunch_FirstKeyActs pins the first key of a session. A key typed as the command is entered
// reaches jojo before the log has loaded, and before that, while the terminal is being asked for its
// background; neither may drop it. It runs against a terminal that answers capability queries and
// one that answers none, since a silent terminal holds the background question open for its whole
// wait. On a first launch the welcome card is up and the first key's act is to close it, which is
// the card's design, so there it is the second key that moves the cursor.
func TestLaunch_FirstKeyActs(t *testing.T) {
	t.Parallel()

	env := isolated(t)
	dir := historyRepo(t, env, 8)

	for _, sz := range []size{{80, 24}, {120, 40}} {
		for _, silent := range []bool{false, true} {
			for _, early := range []bool{true, false} {
				for _, welcomed := range []bool{true, false} {
					name := fmt.Sprintf("%dx%d/silent=%t/typed-ahead=%t/welcomed=%t",
						sz.cols, sz.rows, silent, early, welcomed)

					t.Run(name, func(t *testing.T) {
						t.Parallel()
						firstKeyActs(t, sz, dir, silent, early, welcomed)
					})
				}
			}
		}
	}
}

func firstKeyActs(t *testing.T, sz size, dir string, silent, early, welcomed bool) {
	t.Helper()

	// A home per run: jojo writes the coach's ledger on the way out, and two sessions sharing one
	// race on its temp file.
	env := isolated(t)
	if !welcomed {
		env = firstLaunch(t)
	}

	typed := ""
	if early {
		typed = "j"
	}

	sc := start(t, sz, dir, env, silent, typed)

	if !early {
		// The welcome card covers the log at a small size, so a first launch waits on the card.
		if welcomed {
			sc.awaitFrame("the cursor on @ in the log", func([]string) bool {
				return sc.shows("commit number 1") && sc.selectedRow(sz.rows) == 0
			})
		} else {
			sc.awaitFrame("the welcome card", func([]string) bool { return sc.shows("Welcome to jojo") })
		}

		sc.press("j")
	}

	if !welcomed {
		sc.awaitFrame("the first key to close the welcome card", func([]string) bool {
			return !sc.shows("Welcome to jojo") && sc.selectedRow(sz.rows) == 0
		})
		sc.press("j")
	}

	sc.awaitFrame("j to move the cursor off @ onto the newest commit", func([]string) bool {
		return sc.selectedRow(sz.rows) == 1
	})

	sc.quit()
}
