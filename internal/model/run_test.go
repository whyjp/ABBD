package model

import (
	"strings"
	"testing"
)

func TestARunReadsAsAShapeAndACount(t *testing.T) {
	for in, want := range map[string]Run{
		"tick 1":      {Tick, One},
		"wait N":      {Wait, Many},
		" steal pool": {Steal, Pool},
		"ready 1":     {Ready, One},
	} {
		got, err := ParseRun(in)
		if err != nil || got != want {
			t.Errorf("ParseRun(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if r, _ := ParseRun("complete N"); r.String() != "complete N" || r.Words() != "COMPLETE · N" {
		t.Errorf("complete N says %q and shows %q", r.String(), r.Words())
	}
}

func TestAnUnknownRunWordNamesTheWordsABBDKnows(t *testing.T) {
	for in, lists := range map[string]string{
		"spin 1":   "tick, wait, ready, complete, steal or inline",
		"tick 2":   "1, N or pool",
		"tick":     "the count one of 1, N or pool",
		"tick 1 x": "the shape is one of tick",
	} {
		_, err := ParseRun(in)
		if err == nil || !strings.Contains(err.Error(), lists) {
			t.Errorf("ParseRun(%q) = %v; want it to list %q", in, err, lists)
		}
	}
}

func execUnit(id ID, kind Kind, run string) Unit {
	u := Unit{ID: id, Kind: kind, Name: Said(Supplement, "video thread"), Anchors: []Anchor{{Path: "libobs/obs-video.c"}}, Evidence: cite}
	if run != "" {
		f := Said(Supplement, run)
		u.Run = &f
	}
	return u
}

func TestOnlyAnExecutionContextCarriesAValidRun(t *testing.T) {
	if err := execUnit("EXEC-0001", Exec, "tick 1").Validate(); err != nil {
		t.Fatalf("an EXEC with run tick 1: %v", err)
	}
	if err := execUnit("MOD-0001", Module, "tick 1").Validate(); err == nil || !strings.Contains(err.Error(), "only an execution context") {
		t.Errorf("a module with a run: %v", err)
	}
	if err := execUnit("EXEC-0001", Exec, "loop 1").Validate(); err == nil || !strings.Contains(err.Error(), "unknown shape") {
		t.Errorf("an EXEC with run loop 1: %v", err)
	}
}

func TestRunTimeRelationsJoinOnlyAnExecutionContext(t *testing.T) {
	for _, c := range []struct {
		kind     RelationKind
		from, to Kind
		ok       bool
	}{
		{RunsOn, Module, Exec, true},
		{RunsOn, Feature, Exec, true},
		{RunsOn, Exec, Exec, true},
		{RunsOn, Interface, Exec, false},
		{RunsOn, Module, Module, false},
		{PostsTo, Module, Exec, true},
		{PostsTo, Exec, Exec, true},
		{PostsTo, Feature, Exec, false},
		{PostsTo, Module, Interface, false},
		{DependsOn, Exec, Module, true},
		{Provides, Exec, Interface, false},
	} {
		err := c.kind.Joins(c.from, c.to)
		if (err == nil) != c.ok {
			t.Errorf("%s %s %s: %v, want ok=%v", c.from, c.kind, c.to, err, c.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "runs_on an EXEC") {
			t.Errorf("the refusal does not name the run-time kinds: %v", err)
		}
	}
}

func TestAPostsToQuotesTheProducerAndTheConsumerWhateverItsWriter(t *testing.T) {
	two := []Evidence{{Path: "a.c", Lines: "1", Quote: "push"}, {Path: "b.c", Lines: "2", Quote: "pop"}}
	for _, w := range []Writer{Script, Supplement, Reader, Human} {
		u := execUnit("MOD-0001", Module, "")
		r := Relation{To: "EXEC-0001", Kind: PostsTo, Writer: w, Via: "queue", Evidence: two[:1]}
		if w == Human {
			r.Source = "conversation"
		}
		u.Relations = []Relation{r}
		if err := u.Validate(); err == nil || !strings.Contains(err.Error(), "2 or more evidence items") {
			t.Errorf("a %s's posts_to with one quote: %v", w, err)
		}
		u.Relations[0].Evidence = two
		if err := u.Validate(); err != nil {
			t.Errorf("a %s's posts_to with two quotes: %v", w, err)
		}
	}
}
