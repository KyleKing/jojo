package model

import "testing"

// The log lists newest first, so the checked revisions arrive newest first too and a range built in
// that order searches backwards and finds nothing.
func TestBisectRangeRunsOldestToNewest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		want    string
		checked []string
		ok      bool
	}{
		{name: "two checked", checked: []string{"newer", "older"}, want: "older::newer", ok: true},
		{name: "one checked", checked: []string{"only"}, want: "", ok: false},
		{name: "three checked", checked: []string{"a", "b", "c"}, want: "", ok: false},
		{name: "none checked", checked: nil, want: "", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := bisectRange(tc.checked)
			if ok != tc.ok || got != tc.want {
				t.Errorf("bisectRange(%v) = %q, %v, want %q, %v", tc.checked, got, ok, tc.want, tc.ok)
			}
		})
	}
}
