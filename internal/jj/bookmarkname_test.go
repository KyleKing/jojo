package jj_test

import (
	"errors"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// The prompt refuses what jj refuses (a name it cannot read as a revset symbol) and what jj keeps
// but cannot export to git, so the name is fixed in the prompt rather than after jj says no.
func TestCheckBookmarkName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"main", true},
		{"feat/parse-header", true},
		{"v1.2+build", true},
		{"push-qpvx_2", true},
		{"é", true},
		{"", false},
		{"  ", false},
		{"a b", false},
		{"a~b", false},
		{"a:b", false},
		{"a@origin", false},
		{"a*b", false},
		{"-x", false},
		{".x", false},
		{"a/.b", false},
		{"a..b", false},
		{"a//b", false},
		{"x/", false},
		{"x.", false},
		{"x.lock", false},
	} {
		err := jj.CheckBookmarkName(tc.name)
		if (err == nil) != tc.ok {
			t.Errorf("CheckBookmarkName(%q) = %v, want ok %v", tc.name, err, tc.ok)
		}

		if err != nil && !errors.Is(err, jj.ErrBookmarkName) {
			t.Errorf("CheckBookmarkName(%q) = %v, not an ErrBookmarkName", tc.name, err)
		}
	}
}
