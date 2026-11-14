package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/keyusage"
)

func TestKeysLandUnderTheirContextAndTypedTextIsNotLogged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rec := keyusage.Open(UsageApp, keyusage.WithDir(dir), keyusage.WithHost("test"))

	m := testModel(t, twoRows())
	m.usage = rec

	next, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = asModel(t, next)

	m.prompt.Open("describe", "")

	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = asModel(t, next)

	if !m.prompt.IsOpen() {
		t.Fatal("the prompt closed, so the key never reached it")
	}

	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	u, err := keyusage.Load(dir, UsageKeymap(m.keys))
	if err != nil {
		t.Fatal(err)
	}

	if got := u.Stat(ctxLog, "G").Count; got != 1 {
		t.Errorf("G in the log pane was counted %d times, want 1", got)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("got %d log files, want 1", len(files))
	}

	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}

	if lines := strings.Count(string(raw), "\n"); lines != 1 {
		t.Errorf("the log has %d lines, want only the log pane's key:\n%s", lines, raw)
	}
}
