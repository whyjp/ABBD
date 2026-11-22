package model

import (
	"slices"
	"strings"
	"testing"
)

func use(to ID, via Via) Relation { return Relation{To: to, Kind: DependsOn, Via: via} }

// inside is a module MOD-0100 of five boxes a to e, an outside module
// MOD-0200 and a contract IF-0001 that box a provides and box b uses. relations
// gives each box's uses by index into ids.
func inside(rels map[ID][]Relation) []Unit {
	ids := []ID{"MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104", "MOD-0105"}
	us := []Unit{
		{ID: "MOD-0100", Kind: Module, Anchors: []Anchor{{Path: "m"}}},
		{ID: "MOD-0200", Kind: Module, Anchors: []Anchor{{Path: "o"}}, Relations: rels["MOD-0200"]},
		{ID: "IF-0001", Kind: Interface, Parent: "MOD-0101", Anchors: []Anchor{{Path: "m/a.go", Symbol: "API"}}},
	}
	for _, id := range ids {
		us = append(us, Unit{ID: id, Kind: Module, Parent: "MOD-0100", Anchors: []Anchor{{Path: "m/" + string(id)}}, Relations: rels[id]})
	}
	return us
}

func group(as string, ids ...ID) Layer {
	l := layer(string(ids[0]), ids...)
	l.As = as
	return l
}

func gapTexts(c Composition, layers []Layer, units []Unit) []string {
	var out []string
	for _, g := range c.Gaps(layers, units) {
		out = append(out, g.Text)
	}
	return out
}

func mutual(ids ...ID) map[ID][]Relation {
	rels := map[ID][]Relation{}
	for _, a := range ids {
		for _, b := range ids {
			if a != b {
				rels[a] = append(rels[a], use(b, "call"))
			}
		}
	}
	return rels
}

// A clique holds only while every pair uses each other both ways, by
// relations that say how they arise.
func TestACliqueIsCompleteOnRelationsThatSayTheirVia(t *testing.T) {
	core := Composition{View: "MOD-0100", Kind: KindCore}
	g := []Layer{group(AsClique, "MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104"), group(AsHub, "MOD-0105")}
	four := mutual("MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104")
	for _, id := range []ID{"MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104"} {
		four["MOD-0105"] = append(four["MOD-0105"], use(id, "call"))
		four[id] = append(four[id], use("MOD-0105", "call"))
	}
	if got := gapTexts(core, g, inside(four)); len(got) != 0 {
		t.Fatalf("a complete clique and a hub have gaps %q", got)
	}
	missing := mutual("MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104")
	missing["MOD-0104"] = missing["MOD-0104"][1:]
	got := gapTexts(core, g[:1], inside(missing))
	if len(got) != 1 || !strings.Contains(got[0], "all use one another") || strings.Contains(got[0], "no via") {
		t.Errorf("a clique missing one use: %q", got)
	}
	viaLess := mutual("MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104")
	for id, rs := range viaLess {
		for i := range rs {
			rs[i].Via = ""
		}
		viaLess[id] = rs
	}
	got = gapTexts(core, g[:1], inside(viaLess))
	if len(got) != 1 || !strings.Contains(got[0], "only relations with no via say so") {
		t.Errorf("a clique of relations with no via: %q", got)
	}
}

// A cycle closes when each box reaches every other along the uses among them.
func TestACycleClosesOrItIsAChain(t *testing.T) {
	core := Composition{View: "MOD-0100", Kind: KindCore}
	ring := map[ID][]Relation{"MOD-0101": {use("MOD-0102", "call")}, "MOD-0102": {use("MOD-0103", "call")}, "MOD-0103": {use("MOD-0101", "call")}}
	chain := map[ID][]Relation{"MOD-0101": {use("MOD-0102", "call")}, "MOD-0102": {use("MOD-0103", "call")}}
	g := []Layer{group(AsCycle, "MOD-0101", "MOD-0102", "MOD-0103")}
	if got := gapTexts(core, g, inside(ring)); len(got) != 0 {
		t.Errorf("a ring has gaps %q", got)
	}
	if got := gapTexts(core, g, inside(chain)); len(got) != 1 || !strings.Contains(got[0], "close a cycle") {
		t.Errorf("a chain closes no cycle: %q", got)
	}
}

// A hub is one box used both ways with half the view's other boxes.
func TestAHubIsUsedWithHalfTheViewsBoxes(t *testing.T) {
	core := Composition{View: "MOD-0100", Kind: KindCore}
	g := []Layer{group(AsHub, "MOD-0101")}
	rels := mutual("MOD-0101", "MOD-0102", "MOD-0103")
	if got := gapTexts(core, g, inside(rels)); len(got) != 0 {
		t.Errorf("a box used with two of four others has gaps %q", got)
	}
	rels = mutual("MOD-0101", "MOD-0102")
	if got := gapTexts(core, g, inside(rels)); len(got) != 1 {
		t.Errorf("a box used with one of four others: %q", got)
	}
}

// A boundary box reaches outside the module by a relation that says its via,
// in either direction.
func TestABoundaryBoxReachesOutsideTheModule(t *testing.T) {
	core := Composition{View: "MOD-0100", Kind: KindCore}
	g := []Layer{group(AsBoundary, "MOD-0101", "MOD-0102")}
	rels := map[ID][]Relation{"MOD-0101": {use("MOD-0200", "call")}, "MOD-0200": {use("MOD-0102", "import")}}
	if got := gapTexts(core, g, inside(rels)); len(got) != 0 {
		t.Errorf("boxes reaching out and used from outside have gaps %q", got)
	}
	rels["MOD-0200"] = nil
	if got := gapTexts(core, g, inside(rels)); len(got) != 1 || !strings.Contains(got[0], "MOD-0102") {
		t.Errorf("a box with no relation outside: %q", got)
	}
	if got := gapTexts(Composition{View: OverviewView, Kind: KindCore}, g, inside(rels)); len(got) == 0 {
		t.Errorf("a boundary of the overview is no claim a module's relations can bear out")
	}
}

// A promise is a contract the group provides and another group of the record
// uses; a family's siblings are each tied to the contract they keep.
func TestPromisesAreProvidedByTheGroupAndUsedByAnother(t *testing.T) {
	layers := Composition{View: "MOD-0100", Kind: KindLayers}
	provider := group(AsLayer, "MOD-0101")
	provider.Promises = []ID{"IF-0001"}
	user := group(AsLayer, "MOD-0102")
	rels := map[ID][]Relation{"MOD-0101": {{To: "IF-0001", Kind: Provides}}, "MOD-0102": {{To: "IF-0001", Kind: Consumes, Via: "call"}}}
	us := inside(rels)
	if got := gapTexts(layers, []Layer{provider, user}, us); len(got) != 0 {
		t.Errorf("a provided and used promise has gaps %q", got)
	}
	if got := gapTexts(layers, []Layer{provider, group(AsLayer, "MOD-0103")}, us); len(got) != 1 || !strings.Contains(got[0], "no unit of another layer uses IF-0001") {
		t.Errorf("a promise nobody else uses: %q", got)
	}
	if got := gapTexts(layers, []Layer{group(AsLayer, "MOD-0103"), withPromise(group(AsLayer, "MOD-0102"), "IF-0001")}, us); len(got) == 0 || !strings.Contains(got[0], "provides IF-0001") {
		t.Errorf("a promise the group does not provide: %q", got)
	}
	family := Composition{View: "MOD-0100", Kind: KindFamily}
	sib := withPromise(group(AsMembers, "MOD-0101", "MOD-0102", "MOD-0103"), "IF-0001")
	if got := gapTexts(family, []Layer{sib}, us); len(got) != 1 || !strings.Contains(got[0], "MOD-0103 neither provides nor uses") {
		t.Errorf("a sibling tied to no promise: %q", got)
	}
}

func withPromise(l Layer, ids ...ID) Layer { l.Promises = ids; return l }

// A face is a contract a unit inside the view provides.
func TestAFaceIsProvidedFromInsideTheView(t *testing.T) {
	c := Composition{View: "MOD-0100", Kind: KindLayers, Faces: []ID{"IF-0001"}}
	us := inside(map[ID][]Relation{"MOD-0101": {{To: "IF-0001", Kind: Provides}}})
	if got := gapTexts(c, []Layer{group(AsLayer, "MOD-0101")}, us); len(got) != 0 {
		t.Errorf("a provided face has gaps %q", got)
	}
	if got := gapTexts(c, []Layer{group(AsLayer, "MOD-0101")}, inside(nil)); len(got) != 1 || !strings.Contains(got[0], "no unit inside MOD-0100 provides IF-0001") {
		t.Errorf("a face nobody provides: %q", got)
	}
}

// A group's units stand inside the view the record composes, and the view is
// a module in the state.
func TestAGroupStandsInsideTheView(t *testing.T) {
	c := Composition{View: "MOD-0100", Kind: KindLayers}
	if got := gapTexts(c, []Layer{group(AsLayer, "MOD-0200")}, inside(nil)); len(got) != 1 || !strings.Contains(got[0], "is not inside MOD-0100") {
		t.Errorf("a unit outside the view: %q", got)
	}
	if got := gapTexts(Composition{View: "MOD-0999"}, []Layer{group(AsLayer, "MOD-0101")}, inside(nil)); len(got) != 1 || !strings.Contains(got[0], "no module in the state") {
		t.Errorf("a view that is no module: %q", got)
	}
	if got := gapTexts(Composition{}, []Layer{group(AsLayer, "MOD-0200")}, inside(nil)); len(got) != 0 {
		t.Errorf("a stack that composes no view has gaps %q", got)
	}
}

func TestACompositionRecordIsWellFormed(t *testing.T) {
	cl := group(AsClique, "MOD-0101", "MOD-0102")
	for _, tc := range []struct {
		name   string
		c      Composition
		layers []Layer
		want   string
	}{
		{"plain stack", Composition{}, []Layer{layer("a", "MOD-0101")}, ""},
		{"unknown kind", Composition{Kind: "tree"}, []Layer{layer("a", "MOD-0101")}, "kind is layers"},
		{"as of another kind", Composition{Kind: KindLayers}, []Layer{cl}, "which a stack of kind layers does not hold"},
		{"core needs an as", Composition{Kind: KindCore}, []Layer{layer("a", "MOD-0101")}, "says what it is"},
		{"hub is one box", Composition{Kind: KindCore}, []Layer{group(AsHub, "MOD-0101", "MOD-0102")}, "which is one box"},
		{"clique takes two", Composition{Kind: KindCore}, []Layer{group(AsClique, "MOD-0101")}, "two units or more"},
		{"primary is stages", Composition{Kind: KindCore, Primary: true}, []Layer{cl}, "primary flow"},
		{"primary stages", Composition{Kind: KindStages, Primary: true}, []Layer{layer("a", "MOD-0101")}, ""},
		{"view a unit", Composition{View: "IF-0001"}, []Layer{layer("a", "MOD-0101")}, "view is overview or the ID of a module"},
		{"overview", Composition{View: OverviewView}, []Layer{layer("a", "MOD-0101")}, ""},
		{"promise is a contract", Composition{Kind: KindLayers}, []Layer{withPromise(layer("a", "MOD-0101"), "MOD-0102")}, "no contract"},
		{"family promises", Composition{Kind: KindFamily}, []Layer{group(AsMembers, "MOD-0101")}, "names no promise"},
		{"faces need a view", Composition{Faces: []ID{"IF-0001"}}, []Layer{layer("a", "MOD-0101")}, "say which view"},
	} {
		err := tc.c.Validate(tc.layers)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want one saying %q", tc.name, err, tc.want)
		}
	}
}

func TestAGroupAddressIsStackKeyDotN(t *testing.T) {
	k, n, err := ParseGroupAddress(GroupAddress("core-main", 3))
	if err != nil || k != "core-main" || n != 3 {
		t.Errorf("round trip gives %q %d %v", k, n, err)
	}
	for _, bad := range []string{"flow-a.1", "stack-a", "stack-a.0", "stack-a.01", "stack-a.x", "stack-A.1", "stack-.1", "stack-a.1.2"} {
		if _, _, err := ParseGroupAddress(bad); err == nil {
			t.Errorf("%q reads as a group address", bad)
		}
	}
	if !slices.Contains([]string{"stack-a.1"}, GroupAddress("a", 1)) {
		t.Errorf("group address = %q", GroupAddress("a", 1))
	}
}

// A use with no via bears out no claim: a promise used only by a via-less use
// is not used, a sibling tied to its contract only by one is not tied, and a
// box reaching only an absent unit is no boundary.
func TestAUseWithNoViaBearsOutNoClaim(t *testing.T) {
	layers := Composition{View: "MOD-0100", Kind: KindLayers}
	provider := withPromise(group(AsLayer, "MOD-0101"), "IF-0001")
	user := group(AsLayer, "MOD-0102")
	rels := map[ID][]Relation{"MOD-0101": {{To: "IF-0001", Kind: Provides}}, "MOD-0102": {{To: "IF-0001", Kind: Consumes}}}
	if got := gapTexts(layers, []Layer{provider, user}, inside(rels)); len(got) != 1 || !strings.Contains(got[0], "no unit of another layer uses IF-0001") {
		t.Errorf("a promise used by a use with no via: %q", got)
	}
	rels["MOD-0102"][0].Via = "call"
	if got := gapTexts(layers, []Layer{provider, user}, inside(rels)); len(got) != 0 {
		t.Errorf("a promise used by a use that says its via: %q", got)
	}
	family := Composition{View: "MOD-0100", Kind: KindFamily}
	sib := withPromise(group(AsMembers, "MOD-0101", "MOD-0102"), "IF-0001")
	rels["MOD-0102"][0].Via = ""
	if got := gapTexts(family, []Layer{sib}, inside(rels)); len(got) != 1 || !strings.Contains(got[0], "MOD-0102 neither provides nor uses") {
		t.Errorf("a sibling tied by a use with no via: %q", got)
	}
	core := Composition{View: "MOD-0100", Kind: KindCore}
	gone := map[ID][]Relation{"MOD-0101": {use("MOD-0999", "call")}}
	if got := gapTexts(core, []Layer{group(AsBoundary, "MOD-0101")}, inside(gone)); len(got) != 1 {
		t.Errorf("a box reaching an absent unit is a boundary: %q", got)
	}
}

// A plain stack has no composition gap, the stack marked the primary flow
// may say its stages without a kind, and a stored record the code cannot
// read as a composition is a gap, never a panic.
func TestAPlainStackHasNoCompositionGapAndABadRecordNoPanic(t *testing.T) {
	us := inside(nil)
	if got := (Composition{}).Gaps([]Layer{layer("a", "MOD-0101"), layer("b", "MOD-0102")}, us); got != nil {
		t.Errorf("a plain stack has gaps %v", got)
	}
	stages := group(AsStage, "MOD-0101")
	if got := gapTexts(Composition{Primary: true}, []Layer{stages}, us); len(got) != 0 {
		t.Errorf("a primary stack's stage has gaps %q", got)
	}
	for _, c := range []Composition{{Kind: "tree"}, {Kind: KindCore}, {View: "nowhere"}, {Kind: KindFamily}} {
		if got := c.Gaps([]Layer{layer("a", "MOD-0101")}, us); len(got) != 1 || !strings.Contains(got[0].Text, "no composition") {
			t.Errorf("%+v: gaps %v, want one saying the record is no composition", c, got)
		}
		_ = c.AsOf(layer("a", "MOD-0101"))
	}
}

// A group's facts are the largest cycle its boxes make among themselves and
// how many of its boxes reach outside the module.
func TestGroupFactsCountTheCycleAndTheBoxesThatReachOutside(t *testing.T) {
	rels := mutual("MOD-0101", "MOD-0102", "MOD-0103")
	rels["MOD-0101"] = append(rels["MOD-0101"], use("MOD-0200", "call"))
	rels["MOD-0104"] = []Relation{use("MOD-0101", "call")}
	us := inside(rels)
	cycle, boundary := GroupFacts(us, []ID{"MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104"}, "MOD-0100")
	if cycle != 3 || boundary != 1 {
		t.Errorf("cycle %d, boundary %d; want 3 and 1", cycle, boundary)
	}
	if cycle, _ := GroupFacts(us, []ID{"MOD-0104", "MOD-0105"}, "MOD-0100"); cycle != 0 {
		t.Errorf("two boxes with no cycle between them: %d", cycle)
	}
}

// A phrase's symbols are few, one name each as the code spells it, none
// twice, only with words, and a reader's each stand in a cited quote.
func TestAPhrasesSymbolsAreNamesTheCodeHolds(t *testing.T) {
	quote := []Evidence{{Path: "a.c", Quote: "uv_async_send(&wq_async)"}}
	for _, tc := range []struct {
		name    string
		symbols []string
		text    string
		cited   bool
		want    string
	}{
		{"none", nil, "", true, ""},
		{"held", []string{"uv_async_send", "wq_async"}, "완료 알림", true, ""},
		{"absent from the quotes", []string{"uv__work_done"}, "완료 알림", true, "stands in none of the quotes"},
		{"absent but not a reader's", []string{"uv__work_done"}, "완료 알림", false, ""},
		{"too many", []string{"a", "b", "c", "d"}, "x", false, "at most 3"},
		{"a bracket", []string{"f()"}, "x", false, "one name as the code spells it"},
		{"a space", []string{"two words"}, "x", false, "one name as the code spells it"},
		{"twice", []string{"a", "a"}, "x", false, "named twice"},
		{"without words", []string{"a"}, "", false, "give the phrase"},
		{"scoped name", []string{"Foo::bar", "a.b#c"}, "x", false, ""},
		{"a name inside a longer one", []string{"uv_async"}, "x", true, "stands in none of the quotes"},
	} {
		err := ValidateSymbols(tc.symbols, tc.text, quote, tc.cited)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}
