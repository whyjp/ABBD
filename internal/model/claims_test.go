package model

import (
	"math/rand"
	"testing"
)

var cite = []Evidence{{Path: "README.md", Lines: "1", Quote: "x"}}

func mod(id ID, claims ...ParentClaim) Unit {
	u := Unit{ID: id, Kind: Module, Name: Said(Script, string(id)), Anchors: []Anchor{{Path: string(id)}}}
	for _, c := range claims {
		u.PutParent(c)
	}
	return u
}

func by(w Writer, to ID) ParentClaim {
	c := ParentClaim{To: to, Writer: w}
	switch w.Origin() {
	case Inferred:
		c.Evidence = cite
	case UserStated:
		c.Source = "conversation 2026-11-02"
	}
	return c
}

func parentOf(units []Unit, id ID) (ID, Origin) {
	Resolve(units)
	for _, u := range units {
		if u.ID == id {
			return u.Parent, u.ParentOrigin
		}
	}
	return "", ""
}

func TestEveryWriterHasAnOrigin(t *testing.T) {
	for f, ws := range Precedence {
		for _, w := range ws {
			if w.Origin() == "" {
				t.Errorf("%s lists writer %q, which has no origin", f, w)
			}
		}
	}
}

// 2404d01: a reader's parent was shown as the script's. The origin now
// comes from the claim that won, whichever writer that is.
func TestTheShownParentCarriesItsWritersOrigin(t *testing.T) {
	for _, tc := range []struct {
		claims []ParentClaim
		want   ID
		origin Origin
	}{
		{[]ParentClaim{by(Reader, "MOD-0002")}, "MOD-0002", Inferred},
		{[]ParentClaim{by(Reader, "MOD-0002"), by(Script, "MOD-0003")}, "MOD-0003", Extracted},
		{[]ParentClaim{by(Human, "MOD-0002"), by(Script, "MOD-0003")}, "MOD-0002", UserStated},
		{[]ParentClaim{by(Supplement, "MOD-0002"), by(Reader, "MOD-0003")}, "MOD-0003", Inferred},
		{[]ParentClaim{by(Group, "MOD-0002"), by(Script, "MOD-0003")}, "MOD-0002", Extracted},
	} {
		units := []Unit{mod("MOD-0001", tc.claims...), mod("MOD-0002"), mod("MOD-0003")}
		if p, o := parentOf(units, "MOD-0001"); p != tc.want || o != tc.origin {
			t.Errorf("%+v: shown %s (%s), want %s (%s)", tc.claims, p, o, tc.want, tc.origin)
		}
	}
}

// f0d043b: a unit vanished when its parent went absent. It now falls to its
// next claim, and the claim on the absent parent stays stored.
func TestAUnitFallsToItsNextParentWhenItsParentGoes(t *testing.T) {
	gone := mod("MOD-0002")
	gone.SetAbsent(NotReported)
	units := []Unit{mod("MOD-0001", by(Human, "MOD-0002"), by(Reader, "MOD-0003")), gone, mod("MOD-0003")}
	if p, _ := parentOf(units, "MOD-0001"); p != "MOD-0003" {
		t.Errorf("shown parent %s, want the reader's MOD-0003", p)
	}
	if _, ok := units[0].Claim(Human); !ok {
		t.Errorf("the claim on the absent parent was deleted")
	}
}

// c05d076: cycle repair dropped parents off the cycle. Only a claim that
// closes a cycle is skipped, the script's tree is accepted first, and
// nothing is deleted.
func TestOnlyTheClaimClosingACycleIsSkipped(t *testing.T) {
	units := []Unit{
		mod("MOD-0001", by(Reader, "MOD-0004")),
		mod("MOD-0002", by(Reader, "MOD-0003")),
		mod("MOD-0003", by(Script, "MOD-0002")),
		mod("MOD-0004"),
	}
	Resolve(units)
	if c := ParentCycle(units); c != "" {
		t.Fatalf("cycle at %s", c)
	}
	want := map[ID]ID{"MOD-0001": "MOD-0004", "MOD-0002": "", "MOD-0003": "MOD-0002"}
	for _, u := range units[:3] {
		if u.Parent != want[u.ID] {
			t.Errorf("%s inside %q, want %q", u.ID, u.Parent, want[u.ID])
		}
	}
	if _, ok := units[1].Claim(Reader); !ok {
		t.Errorf("the skipped claim was deleted")
	}
}

// Between a reader and the supplement, the claim whose evidence still holds
// wins; otherwise the table decides. A dead claim never shows over another.
func TestALiveClaimBeatsADriftedOneBetweenReaderAndSupplement(t *testing.T) {
	drifted := []Evidence{{Path: "a.go", Quote: "x", Status: Drifted, Since: "r1"}}
	dead := []Evidence{{Path: "a.go", Quote: "x", Status: Dead, Reason: "a.go is gone"}}
	claim := func(w Writer, es []Evidence) Relation {
		return Relation{To: "MOD-0002", Kind: DependsOn, Writer: w, Via: "call", Evidence: es}
	}
	for _, tc := range []struct {
		name   string
		claims []Relation
		want   Writer
	}{
		{"reader over supplement", []Relation{claim(Supplement, cite), claim(Reader, cite)}, Reader},
		{"live supplement over drifted reader", []Relation{claim(Reader, drifted), claim(Supplement, cite)}, Supplement},
		{"drifted reader over dead supplement", []Relation{claim(Supplement, dead), claim(Reader, drifted)}, Reader},
		{"script over all", []Relation{claim(Reader, cite), {To: "MOD-0002", Kind: DependsOn, Writer: Script}}, Script},
		{"human over a reader", []Relation{claim(Reader, cite), {To: "MOD-0002", Kind: DependsOn, Writer: Human, Source: "pr"}}, Human},
	} {
		u := Unit{ID: "MOD-0001", Kind: Module, Name: Said(Script, "a"), Anchors: []Anchor{{Path: "a"}}}
		for _, r := range tc.claims {
			u.PutRelation(r)
		}
		if len(u.Relations) != 1 || u.Relations[0].Writer != tc.want || len(u.Hidden) != len(tc.claims)-1 {
			t.Errorf("%s: shown %+v, hidden %d", tc.name, u.Relations, len(u.Hidden))
		}
		u.DropRelations(func(r Relation) bool { return r.Writer == tc.want })
		if len(u.Relations) != 1 {
			t.Errorf("%s: no claim shows once the winner goes", tc.name)
		}
	}
}

// A mask hides a lower claim only while that claim's content is what was
// retracted, and never hides the script's.
func TestAMaskIsBoundToTheMaskedContent(t *testing.T) {
	r := Relation{To: "MOD-0002", Kind: DependsOn, Writer: Supplement, Via: "call", Evidence: cite}
	u := Unit{ID: "MOD-0001", Kind: Module, Name: Said(Script, "a"), Anchors: []Anchor{{Path: "a"}}}
	u.PutRelation(r)
	if !u.PutMask(Mask{Target: RelationTarget(r.Kind, r.To), Writer: Reader, Digest: RelationDigest(r), Reason: "wrong"}) {
		t.Fatal("the first mask was not added")
	}
	if u.PutMask(Mask{Target: RelationTarget(r.Kind, r.To), Writer: Reader, Digest: RelationDigest(r), Reason: "again"}) {
		t.Error("the same mask was added twice")
	}
	u.resolve()
	if len(u.Relations) != 0 || len(u.Hidden) != 1 {
		t.Fatalf("the masked claim shows: %+v", u.Relations)
	}
	changed := r
	changed.Via = "data"
	u.PutRelation(changed)
	if len(u.Relations) != 1 || u.Relations[0].Via != "data" {
		t.Errorf("a claim whose content changed stays hidden: %+v", u.Relations)
	}
	if Hides(Reader, Script) || Hides(Human, Script) || Hides(Reader, Human) {
		t.Error("a mask may hide only lower INFERRED writers")
	}
}

// One tree: whatever claims random units make, resolved parents never cycle
// and every present unit's parent is present.
func TestResolvedParentsAlwaysFormATree(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	writers := []Writer{Script, Group, Human, Reader, Supplement}
	for round := 0; round < 300; round++ {
		n := 2 + rng.Intn(8)
		units := make([]Unit, n)
		for i := range units {
			units[i] = mod(FormatID(Module, i+1))
		}
		for i := range units {
			for _, w := range writers {
				if rng.Intn(3) == 0 {
					to := FormatID(Module, 1+rng.Intn(n))
					if to != units[i].ID {
						units[i].PutParent(by(w, to))
					}
				}
			}
			if rng.Intn(6) == 0 {
				units[i].SetAbsent(NotReported)
			}
		}
		Resolve(units)
		if c := ParentCycle(units); c != "" {
			t.Fatalf("round %d: cycle at %s", round, c)
		}
		present := map[ID]bool{}
		for _, u := range Present(units) {
			present[u.ID] = true
		}
		for _, u := range Present(units) {
			if u.Parent != "" && !present[u.Parent] {
				t.Fatalf("round %d: %s inside absent %s", round, u.ID, u.Parent)
			}
			if err := u.Validate(); err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
}
