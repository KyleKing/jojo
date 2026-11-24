package diffpane_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/ui"
)

// benchDiff is a git diff of that many Go files, each one hunk of the given number of lines, which is
// the shape a docked diff lands with: code the lexer has to read before a row can be colored.
func benchDiff(files, lines int) string {
	var b strings.Builder

	for f := range files {
		path := fmt.Sprintf("internal/pkg%d/file%d.go", f, f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 0000000..1111111 100644\n", path, path)
		fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", path, path)
		fmt.Fprintf(&b, "@@ -1,%d +1,%d @@ func handler%d()\n", lines, lines, f)

		for i := range lines {
			switch i % 3 {
			case 0:
				fmt.Fprintf(&b, " \tvalue%d := compute(%d, \"label %d\") // keep\n", i, i, i)
			case 1:
				fmt.Fprintf(&b, "-\tif err := check%d(ctx, value); err != nil { return err }\n", i)
			default:
				fmt.Fprintf(&b, "+\tif err := verify%d(ctx, value, %d); err != nil { return wrap(err) }\n", i, i)
			}
		}
	}

	return b.String()
}

// benchSizes are a revision touching a little and one touching a lot.
var benchSizes = []struct {
	name         string
	files, lines int
}{
	{"small", 2, 30},
	{"large", 40, 120},
}

// BenchmarkDock_Show times what a revision landing in the dock costs the update loop: parsing and
// drawing the rows the pane scrolls through. Every cursor rest on a new revision pays it.
func BenchmarkDock_Show(b *testing.B) {
	for _, size := range benchSizes {
		text := benchDiff(size.files, size.lines)

		b.Run(size.name, func(b *testing.B) {
			m := diffpane.NewDocked(ui.New(theme.Macchiato()))

			b.ReportAllocs()

			for i := range b.N {
				m.Show(fmt.Sprintf("diff %d", i), text, 100, "")
			}
		})
	}
}

// BenchmarkDock_Frame times one frame of the docked pane once a diff is showing.
func BenchmarkDock_Frame(b *testing.B) {
	for _, size := range benchSizes {
		text := benchDiff(size.files, size.lines)

		b.Run(size.name, func(b *testing.B) {
			m := diffpane.NewDocked(ui.New(theme.Macchiato()))
			m.Show("diff", text, 100, "")

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				_ = m.DockView(102, 40, false)
			}
		})
	}
}

// BenchmarkDock_ShowRead times what the update loop pays for a revision the dock read in the
// background, which draws it there too: in color when that fit the budget, else plain with the colors
// following. Paint itself runs off the loop, so it is drawn once, outside the timer.
func BenchmarkDock_ShowRead(b *testing.B) {
	for _, size := range benchSizes {
		text := benchDiff(size.files, size.lines)
		paint := diffpane.NewDocked(ui.New(theme.Macchiato())).Painter(100).Paint(text)

		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()

			for i := range b.N {
				m := diffpane.NewDocked(ui.New(theme.Macchiato()))
				_ = m.Accept(paint)
				m.Show(fmt.Sprintf("diff %d", i), text, 100, "")
			}
		})
	}
}
