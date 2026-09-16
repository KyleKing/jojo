package help_test

import (
	"errors"
	"testing"

	"github.com/kyleking/jojo/internal/components/help"
)

func keymap(t *testing.T, overrides map[string]string) help.Keymap {
	t.Helper()

	keys, err := help.NewKeymap(overrides)
	if err != nil {
		t.Fatalf("NewKeymap(%v): %v", overrides, err)
	}

	return keys
}

// A rebind has to reach the handlers and the legend from one place, or the help overlay teaches a
// key that does nothing.
func TestRebindMovesTheActionAndTheLegendTogether(t *testing.T) {
	t.Parallel()

	keys := keymap(t, map[string]string{"describe": "M"})

	if got := keys.Resolve("M"); got != "d" {
		t.Errorf("Resolve(M) = %q, want the describe handler's key", got)
	}

	if got := keys.Resolve("x"); got != "x" {
		t.Errorf("Resolve(x) = %q, want the key itself when nothing rebinds it", got)
	}

	var found bool

	for _, b := range keys.Runnable() {
		if b.Action == "describe" {
			found = b.Run == "M" && b.Key == "M"
		}
	}

	if !found {
		t.Error("the legend still shows describe on its default key")
	}
}

func TestRebindRefusesWhatItCannotHonor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		overrides map[string]string
		want      error
		name      string
	}{
		{
			name:      "an action the keymap does not have",
			overrides: map[string]string{"descr1be": "M"},
			want:      help.ErrUnknownAction,
		},
		{
			name:      "two actions on one key",
			overrides: map[string]string{"describe": "M", "squash": "M"},
			want:      help.ErrDuplicateKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := help.NewKeymap(tc.overrides); !errors.Is(err, tc.want) {
				t.Errorf("NewKeymap(%v) = %v, want %v", tc.overrides, err, tc.want)
			}
		})
	}
}

// Every runnable binding needs a name to rebind it by, or the setting cannot reach it.
func TestEveryRunnableBindingIsNameable(t *testing.T) {
	t.Parallel()

	for _, b := range keymap(t, nil).Runnable() {
		if b.Action == "" {
			t.Errorf("the binding on %q has no action name", b.Run)
		}
	}
}

// A rebind moves a key. Leaving the old one working would make the legend a lie and would hide the
// collision when an action is moved onto a key something else already holds.
func TestRebindMovesTheKeyRatherThanAddingOne(t *testing.T) {
	t.Parallel()

	keys := keymap(t, map[string]string{"describe": "M"})

	if got := keys.Resolve("d"); got == "d" {
		t.Error("the old describe key still reaches the describe handler")
	}

	if _, err := help.NewKeymap(map[string]string{"describe": "s"}); !errors.Is(err, help.ErrDuplicateKey) {
		t.Error("moving describe onto squash's key was allowed")
	}
}

// Swapping two actions is the one rebind where every key stays held, so it has to be legal.
func TestRebindAllowsASwap(t *testing.T) {
	t.Parallel()

	keys := keymap(t, map[string]string{"describe": "s", "squash": "d"})

	if got := keys.Resolve("s"); got != "d" {
		t.Errorf("Resolve(s) = %q, want the describe handler", got)
	}

	if got := keys.Resolve("d"); got != "s" {
		t.Errorf("Resolve(d) = %q, want the squash handler", got)
	}
}
