package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// quotedAt writes "return nil" at each of lines and "edited" at edit.
// Every other line differs, so no two places read the same lines around.
func quotedAt(edit int, lines ...int) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		switch {
		case i == edit:
			b.WriteString("edited\n")
		case slices.Contains(lines, i):
			b.WriteString("\treturn nil\n")
		default:
			fmt.Fprintf(&b, "line %d\n", i)
		}
	}
	return b.String()
}

// placesProject's app holds two units that quote "return nil" in
// lib/util.go, at lines a and b.
func placesProject(t *testing.T, a, b int) *project {
	t.Helper()
	p, exe := bare(t)
	p.code("app/main.go", "package app\n")
	p.code("lib/util.go", quotedAt(0, a, b))
	p.extract(`{"units":[{"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go"}]}]}`)
	rules := fmt.Sprintf(`{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go"}]},
 {"key":"a","kind":"CFG","name":"early exit","parent":"app","evidence":[{"path":"lib/util.go","lines":"%d","quote":"return nil"}]},
 {"key":"b","kind":"CFG","name":"late exit","parent":"app","evidence":[{"path":"lib/util.go","lines":"%d","quote":"return nil"}]}
]}`, a, b)
	p.setup(0, "synced", "--supplement", p.file(t, "rules.json", rules), "--", exe)
	return p
}

// Two places within each other's slack each keep to their own lines, so an
// edit only the second place's lines reach stales the text.
func TestNearbyPlacesOfOneQuoteCountApart(t *testing.T) {
	p := placesProject(t, 10, 12)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app exits early.\n"))
	p.resync()
	p.want(0, "lib/util.go:12", "evidence")
	p.code("lib/util.go", quotedAt(17, 10, 12))
	p.resync()
	p.stale("1")
}

// When the unit citing the first place leaves, the place left is code the
// text was written against, unchanged, so a restamp is allowed.
func TestRestampIsAllowedAfterTheFirstPlaceLeaves(t *testing.T) {
	p := placesProject(t, 7, 25)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app exits early.\n"))
	p.want(0, "retracted", "retract", "CFG-0001", "--reason", "not an exit")
	p.resync()
	p.stale("1")
	p.want(0, "restamped MOD-0001", "understand", "MOD-0001", "--restamp")
	p.stale("0")
}

// The place left still has to read as it did: edited, it refuses a restamp.
func TestRestampIsRefusedWhenThePlaceLeftChanged(t *testing.T) {
	p := placesProject(t, 7, 25)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app exits early.\n"))
	p.want(0, "retracted", "retract", "CFG-0001", "--reason", "not an exit")
	p.code("lib/util.go", quotedAt(27, 7, 25))
	p.resync()
	p.want(1, "lib/util.go#quote-", "understand", "MOD-0001", "--restamp")
	p.stale("1")
}
