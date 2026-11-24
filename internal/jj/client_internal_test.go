package jj

import (
	"errors"
	"strings"
	"testing"
)

// jj prints progress, warnings, and hints ahead of the line that says what went wrong, so the first
// line of stderr is often not the cause. A fetch from a missing remote put "Fetching from Git
// remotes" in the footer and hid the error on line two.
func TestWrapJJError_ShowsTheErrorLineNotTheFirstLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name: "progress before the error",
			stderr: "Fetching from Git remotes: origin\n" +
				"Error: Could not find repository at '/nonexistent/remote.git'\n",
			want: "jj git fetch: Could not find repository at '/nonexistent/remote.git'",
		},
		{
			name: "warning and hint before the error",
			stderr: "Warning: Name and email not configured.\nHint: To configure, run:\n  jj config set\n" +
				"Error: Revision `nonexist` doesn't exist\n",
			want: "jj git fetch: Revision `nonexist` doesn't exist",
		},
		{
			name: "argument error keeps its caret detail",
			stderr: "error: invalid value 'my topic' for '<NAMES>...': " +
				"Failed to parse bookmark name: Syntax error\n\n" +
				"Caused by:  --> 1:3\n  |\n1 | my topic\n  |   ^---\n  |\n  = expected <EOI>\n",
			want: "jj git fetch: invalid value 'my topic' for '<NAMES>...': " +
				"Failed to parse bookmark name: Syntax error (expected <EOI>)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			args := []string{"git", "fetch", "--all-remotes"}
			err := wrapJJError(args, tc.stderr)

			if got := err.Error(); got != tc.want {
				t.Errorf("Error() = %q\nwant      %q", got, tc.want)
			}

			if !errors.Is(err, ErrCommandFailed) {
				t.Error("the sentinel is lost")
			}

			full := FullOutput(err)
			kept := strings.Contains(full, strings.TrimSpace(tc.stderr))
			if !kept || !strings.HasPrefix(full, "jj git fetch --all-remotes") {
				t.Errorf("FullOutput lost the command or jj's text:\n%s", full)
			}
		})
	}
}
