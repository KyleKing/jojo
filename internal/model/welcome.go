package model

import (
	"strings"
	"time"

	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
)

// newWelcome is the first-run card: what jojo is for and the core jj loop a first session starts
// with, every one a key on the log the card is drawn over, labeled from the keymap so a rebind changes
// the card. Reading and submitting come later, from the help and the coach, once the loop is in hand.
func newWelcome(km help.Keymap) *coach.Welcome {
	w := &coach.Welcome{
		Title: "Welcome to jojo",
		Lead:  "Your jj log, a key per verb: the cursor's revision is what each one acts on.",
		Starters: coach.StartersFrom(km.Keys(), keys.ActionDescribe, help.ActionNew, help.ActionSquash,
			help.ActionRebase, keys.ActionUndo),
	}
	w.HelpKeysFrom(km.Keys())

	return w
}

// welcomeKey answers a key while the card is up: any key takes it down and the dismissal is saved at
// once, since a short first session never reaches the idle lull the ledger is otherwise written in.
// It reports whether the key goes on to act, which only the card's own help and palette keys do. Any
// other key only closes it, so a first keystroke like a or S cannot abandon or split by accident.
func (m *Model) welcomeKey(key string, now time.Time) bool {
	if !m.welcome.Showing() {
		return true
	}

	ledger := m.coach.Ledger()

	pass := m.welcome.Key(ledger, key, now)
	if err := ledger.Save(); err != nil {
		m.message = "could not save the coach's notes: " + err.Error()
	}

	return pass
}

// withWelcome lays the card over every row but the footer, or where the screen is too small for it,
// puts its one-line form in the footer. The palette, once open, has the screen to itself.
func (m Model) withWelcome(screen string) string {
	if !m.welcome.Showing() || m.pal.IsOpen() {
		return screen
	}

	lines := strings.Split(screen, "\n")
	last := len(lines) - footerRows

	if last < 1 {
		return screen
	}

	if !coach.WelcomeFits(m.width, last) {
		lines[last] = " " + m.welcome.Line(m.skin.Skin, m.width-1)

		return strings.Join(lines, "\n")
	}

	return strings.Join(append(m.welcome.Over(m.skin.Skin, lines[:last], m.width, last), lines[last]), "\n")
}
