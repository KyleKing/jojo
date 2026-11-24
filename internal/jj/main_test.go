package jj_test

import (
	"testing"

	"github.com/kyleking/jojo/internal/hermetic"
)

// TestMain runs the tests in a home of their own, failing the run if one reaches the real one.
func TestMain(m *testing.M) { hermetic.Main(m) }
