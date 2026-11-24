package readmark_test

import (
	"testing"

	"github.com/kyleking/jojo/internal/readmark"
)

func TestStrip_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want  string
		strip readmark.Strip
	}{
		{"····", readmark.Strip{}},
		{"▱▱▱▱", readmark.Strip{Total: 5}},
		{"▰▱▱▱", readmark.Strip{Read: 1, Total: 50}},
		{"▰▰▰▱", readmark.Strip{Read: 49, Total: 50}},
		{"▰▰▰▰", readmark.Strip{Read: 3, Total: 3}},
	}

	for _, tc := range cases {
		if got := tc.strip.String(); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc.strip, got, tc.want)
		}
	}
}

func TestStrip_Label(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want  string
		strip readmark.Strip
	}{
		{"", readmark.Strip{}},
		{"◌5", readmark.Strip{Total: 5}},
		{"◌1", readmark.Strip{Read: 49, Total: 50}},
		{"", readmark.Strip{Read: 3, Total: 3}},
	}

	for _, tc := range cases {
		if got := tc.strip.Label(); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc.strip, got, tc.want)
		}
	}
}
