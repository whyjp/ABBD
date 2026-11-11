package screen

import "testing"

// On orca's src/ the mobile app and mobile/transport each send a line to
// shared types along the same pair of buses, and both step from the one to
// the other at the pair's x. Drawn as two paths on one track, the one on
// top hid the other, so the pointer never reached its line. Lines of one
// kind whose own parts run the same way share one path, which names them
// all.
func TestTwoLinesOnOneTrackShareOnePath(t *testing.T) {
	t.Parallel()
	for _, ins := range fixture(t, "orca").Insides {
		if ins.ID != "MOD-0098" {
			continue
		}
		seen := map[[2]string]string{}
		for _, w := range ins.Drawing.Wires {
			if w.Net != 0 {
				continue
			}
			k := [2]string{w.D, w.Kind}
			if title, ok := seen[k]; ok {
				t.Errorf("%s drawn twice: %q and %q", w.D, title, w.Title)
			}
			seen[k] = w.Title
		}
		return
	}
	t.Fatal("no inside MOD-0098")
}
