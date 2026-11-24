package fileview_test

import (
	"os"
	"testing"
)

// TestMain pins the icon set the goldens were drawn with, so an ARAGONITE_ICONS in the environment
// running the tests cannot redraw them.
func TestMain(m *testing.M) {
	if err := os.Setenv("ARAGONITE_ICONS", "unicode"); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}
