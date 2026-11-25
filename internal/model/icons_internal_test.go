package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/framecheck"
	"github.com/kyleking/aragonite/tui/icons"

	"github.com/kyleking/jojo/internal/tuitest"
)

// iconCases are the resolutions the legend's icons row is pinned with: each source, and auto both
// recognizing a terminal and not.
//
//nolint:gochecknoglobals // a fixture
var iconCases = map[string]icons.Resolution{
	"wezterm": {Mode: icons.Nerd, Source: icons.FromAuto, Detail: "WezTerm"},
	"unknown": {Mode: icons.Unicode, Source: icons.FromAuto},
	"env":     {Mode: icons.None, Source: icons.FromEnv, Detail: "off"},
	"setting": {Mode: icons.Nerd, Source: icons.FromSetting, Detail: "nerd"},
}

// The legend says in the frame's last row which icon set the file list draws, why, and how to
// change it, in one row at every width, and the log's footer comes back once it closes.
func TestHelp_NamesTheIconSet(t *testing.T) {
	t.Parallel()

	for name, why := range iconCases {
		for _, size := range [][2]int{{80, 24}, {100, 30}} {
			t.Run(fmt.Sprintf("%s/%dx%d", name, size[0], size[1]), func(t *testing.T) {
				t.Parallel()

				m := iconModel(t, &why, size)
				m, _ = pressOne(t, m, "?")

				frame := m.render()
				lines := strings.Split(tuitest.StripANSI(frame), "\n")

				if last := lines[len(lines)-1]; !strings.HasPrefix(last, "icons  "+why.Mode.String()) {
					t.Errorf("the legend's last row is %q, want the icon set", last)
				}

				assertFits(t, frame, size[0], size[1], 0)
				framecheck.Require(t, name, frame, framecheck.MaxWidth(size[0]), framecheck.Graph())
				tuitest.AssertGolden(t, frame)

				m, _ = pressOne(t, m, "?")
				if strings.Contains(tuitest.StripANSI(m.render()), "icons  ") {
					t.Error("the icons row stayed once the legend closed")
				}
			})
		}
	}
}

// The icons row fits any width: the change is said in the longest wording that fits and cut after
// that, and the set itself is cut last.
func TestIconsLine_FitsEveryWidth(t *testing.T) {
	t.Parallel()

	for name, why := range iconCases {
		for _, width := range []int{12, 30, 52, 80, 100, 160} {
			t.Run(fmt.Sprintf("%s/%d", name, width), func(t *testing.T) {
				t.Parallel()

				m := testModel(t, nil)
				m.icons, m.width = &why, width

				line := tuitest.StripANSI(m.iconsLine())
				if w := lipgloss.Width(line); w > width {
					t.Errorf("%q is %d wide, over %d", line, w, width)
				}

				if !strings.HasPrefix(line, "icons  ") {
					t.Errorf("%q does not start with the label", line)
				}

				if width >= 80 && !strings.Contains(line, why.String()+" · ") {
					t.Errorf("%q at %d cuts the set's reason", line, width)
				}
			})
		}
	}
}

// iconModel is the stack fixture at size, its legend naming why as the icon set.
func iconModel(t *testing.T, why *icons.Resolution, size [2]int) Model {
	t.Helper()

	m := testModel(t, stackRows())
	m.icons = why

	return asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))
}
