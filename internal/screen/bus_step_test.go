package screen

import (
	"math/rand"
	"testing"
)

// A line that steps off its departure track onto a track of its own steps
// between the two trunks, never past the departure trunk, where its track
// would hook back under it.
func TestAStepOffADepartureTrackStaysBetweenTheTrunks(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for k := 0; k < 2000; k++ {
		at := 100 + r.Intn(600)
		d := at + (2+r.Intn(120))*(1-2*r.Intn(2))
		taken := map[int]bool{}
		for x := min(at, d) - 300; x <= max(at, d)+300; x++ {
			taken[x] = r.Intn(3) == 0
		}
		via := viaX(at, d, func(x int) bool { return !taken[x] })
		if (via-at)*(via-d) >= 0 {
			t.Fatalf("trunk at %d, departure trunk at %d: steps at %d", at, d, via)
		}
	}
	for _, c := range []struct{ t, d, want int }{
		{646, 630, 638},
		{241, 249, 245},
		{241, 305, 257},
	} {
		if got := viaX(c.t, c.d, func(int) bool { return true }); got != c.want {
			t.Errorf("trunk at %d, departure trunk at %d: steps at %d, want %d", c.t, c.d, got, c.want)
		}
	}
}

// A line that rides its departure trunk into its last gap steps onto its
// arrival track at an x no other line uses whenever one lies within 32
// lanes, the nearest past its end or behind the trunk when none lies
// between them.
func TestAStepOntoAnArrivalTrackTakesAClearX(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for k := 0; k < 2000; k++ {
		base := 400 + r.Intn(200)
		end := base + lane*(r.Intn(25)-12)
		taken := map[int]bool{}
		for x := 0; x < 1000; x++ {
			taken[x] = r.Intn(8) > 0
		}
		free := func(x int) bool { return !taken[x] }
		clear := false
		for x := min(base, end) - 32*lane; x <= max(base, end)+32*lane; x += lane {
			clear = clear || free(x)
		}
		if x := stepX(base, end, free); clear && !free(x) {
			t.Fatalf("trunk at %d, end at %d: steps at %d, which another line uses", base, end, x)
		}
	}
	taken := func(lo, hi int) func(int) bool { return func(x int) bool { return x < lo || x > hi } }
	if got := stepX(500, 540, taken(480, 560)); got != 564 {
		t.Errorf("past its end: steps at %d, want 564", got)
	}
	if got := stepX(500, 540, taken(500, 540)); got != 548 {
		t.Errorf("nearest past its end: steps at %d, want 548", got)
	}
	if got := stepX(500, 540, taken(0, 2000)); got != 500 {
		t.Errorf("nothing clear: steps at %d, want the trunk's 500", got)
	}
}

// The lines of trunk 0 bound for arrival bus 1 may step down on the trunk
// only when every place on that bus reaches every place the trunk leads to
// below. T rides bus 1 too and does not reach D, so the join would show a
// relation from T to D that is not there.
func TestAJoinThatShowsAMissingRelationIsHarmful(t *testing.T) {
	e := func(n string) end { return end{node: n} }
	reqs := []wireReq{{from: e("S"), to: e("A")}, {from: e("S"), to: e("D")}, {from: e("T"), to: e("A")}}
	plans := []plan{{dep: 0, arr: 1, ride: 0, gaps: []int{2}}, {dep: 0, arr: -1, ride: 1, gaps: []int{2, 4}}, {dep: -1, arr: 1, gaps: []int{2}}}
	buses := []*bus{{members: []int{0, 1}}, {members: []int{0, 2}, arrive: true}}
	reach := map[[2]string]bool{{"S", "A"}: true, {"S", "D"}: true, {"T", "A"}: true}
	if joinsHarmless(reach, reqs, plans, buses, 0, 1, 2) {
		t.Fatal("want the join refused: T does not reach D")
	}
	reach[[2]string{"T", "D"}] = true
	if !joinsHarmless(reach, reqs, plans, buses, 0, 1, 2) {
		t.Fatal("want the join allowed once T reaches D")
	}
}
