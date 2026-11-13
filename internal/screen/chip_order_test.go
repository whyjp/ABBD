package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Two callers in one column use a module's contracts, one from above and
// one from below, so both its chips lead to one x. A chip keeps its place
// on the edge when its caller's line gains a contract: issue #246, where
// the swap ran the lower line on beside the module's own outgoing bus.
func TestAChipKeepsItsPlaceWhenItsLineGainsAContract(t *testing.T) {
	order := func(gains bool) []string {
		lower := []model.Relation{rel("IF-0002", model.Consumes)}
		if gains {
			lower = append(lower, rel("IF-0001", model.Consumes))
		}
		d := build([]model.Unit{
			called("MOD-0001", "cli", rel("MOD-0003", model.DependsOn), rel("IF-0001", model.Provides), rel("IF-0002", model.Provides), rel("IF-0003", model.Provides)),
			called("MOD-0002", "setup", rel("IF-0001", model.Consumes), rel("IF-0003", model.Consumes)),
			called("MOD-0003", "skill", lower...),
			called("IF-0001", "a"), called("IF-0002", "b"), called("IF-0003", "c"),
		}).Overview
		var setup, skill Port
		for _, p := range d.Ports {
			switch {
			case !p.Exit:
			case strings.HasSuffix(p.Title, ": a, c"):
				setup = p
			default:
				skill = p
			}
		}
		if setup.X < skill.X {
			return []string{"setup", "skill"}
		}
		return []string{"skill", "setup"}
	}
	was, now := order(false), order(true)
	if strings.Join(was, " ") != strings.Join(now, " ") {
		t.Errorf("the cli's chips stood %v, and %v once skill's line gained a contract", was, now)
	}
}
