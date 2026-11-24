package model

import (
	"os"
	"testing"

	"github.com/kyleking/aragonite/hermetic"
)

// TestMain pins the icon set the goldens were drawn with, so an ARAGONITE_ICONS in the environment
// running the tests cannot redraw them, and runs the tests in a home of their own, failing the run if
// one reaches the real one.
func TestMain(m *testing.M) {
	if err := os.Setenv("ARAGONITE_ICONS", "unicode"); err != nil {
		panic(err)
	}

	hermetic.Main(m, hermetic.App("jojo"))
}
