package runscan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func rec(k Kind, api, to, ch, path string, line int, quote string) Record {
	return Record{Kind: k, API: api, To: Target{Name: to}, Channel: ch, Evidence: Site{path, line, quote}, Confidence: "syntactic", Via: "catalog"}
}

func mod(id, path string) model.Unit {
	return model.Unit{ID: model.ID(id), Kind: model.Module, Anchors: []model.Anchor{{Path: path}}}
}

func show(cs []Candidate) string {
	var b strings.Builder
	for _, c := range cs {
		fmt.Fprintf(&b, "%v %s %s>%s %s %s %s", c.Stored, c.Fact, c.Owner, c.Other, c.API, c.Entry, c.Channel)
		for _, s := range c.Sites {
			fmt.Fprintf(&b, " %s:%d", s.Path, s.Line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func want(t *testing.T, cs []Candidate, lines string) {
	t.Helper()
	if got := show(cs); strings.TrimSpace(got) != strings.TrimSpace(lines) {
		t.Errorf("candidates:\n%s\nwant:\n%s", got, strings.TrimSpace(lines))
	}
}

var pools = []model.Unit{mod("MOD-0001", "lib"), mod("MOD-0002", "lib/pool"), mod("MOD-0003", "app")}

func TestASpawnIsAnExecCandidateOwnedByTheUnitAnchoringItsCode(t *testing.T) {
	cs := Candidates([]Record{
		rec(Spawn, "pthread_create", "worker", "", "lib/pool/pool.c", 10, "pthread_create(&t, NULL, worker, ctx);"),
		rec(Spawn, "go", "", "", "cmd/main.go", 3, "go serve()"),
	}, pools)
	want(t, cs, `
false EXEC > go   cmd/main.go:3
false EXEC MOD-0002> pthread_create worker  lib/pool/pool.c:10`)
}

func TestASendAndARecvOnOneChannelPairOnlyInsideOneOwner(t *testing.T) {
	cs := Candidates([]Record{
		rec(Send, "cond_signal", "", "popCond", "lib/pool/pool.c", 20, "cond_signal(&ctx->popCond);"),
		rec(Recv, "cond_wait", "", "popCond", "lib/pool/pool.c", 40, "cond_wait(&ctx->popCond, &m);"),
		rec(Send, ".send", "", "tx", "app/a.rs", 5, "tx.send(e);"),
		rec(Recv, ".recv", "", "tx", "lib/b.rs", 9, "tx.recv();"),
	}, pools)
	want(t, cs, `false posts_to MOD-0002>MOD-0002 cond_signal  popCond lib/pool/pool.c:20 lib/pool/pool.c:40`)
}

func TestASemanticSendAndRecvPairAcrossOwners(t *testing.T) {
	s := rec(Send, ".send", "", "events", "app/a.rs", 5, "tx.send(e);")
	r := rec(Recv, ".recv", "", "events", "lib/b.rs", 9, "rx.recv();")
	s.Confidence, r.Confidence = "semantic", "semantic"
	want(t, Candidates([]Record{s, r}, pools), `false posts_to MOD-0003>MOD-0001 .send  events app/a.rs:5 lib/b.rs:9`)
}

func TestARegistrationIsARunsOnAndAnImplAProvides(t *testing.T) {
	units := append(pools, model.Unit{ID: "IF-0001", Kind: model.Interface, Name: model.Fact{Value: "obs_source_info"}, Anchors: []model.Anchor{{Path: "lib/obs.h", Symbol: "obs_source_info"}}})
	cs := Candidates([]Record{
		rec(RegisterCallback, "signal_handler_connect", "on_frame", "", "app/a.c", 7, "signal_handler_connect(sh, \"frame\", on_frame, d);"),
		rec(Impl, "struct initializer", "obs_source_info", "", "app/src.c", 30, "struct obs_source_info my_source = {"),
	}, units)
	want(t, cs, `
false runs_on MOD-0003> signal_handler_connect on_frame  app/a.c:7
false provides MOD-0003>IF-0001 struct initializer obs_source_info  app/src.c:30`)
}

func TestACandidateIsStoredOnceTheStateCitesEverySite(t *testing.T) {
	exec := model.Unit{ID: "EXEC-0001", Kind: model.Exec, Evidence: []model.Evidence{{Path: "lib/pool/pool.c", Lines: "10", Quote: "pthread_create"}}}
	posts := mod("MOD-0002", "lib/pool")
	posts.Relations = []model.Relation{{To: "EXEC-0001", Kind: model.PostsTo, Evidence: []model.Evidence{
		{Path: "lib/pool/pool.c", Quote: "cond_signal(&ctx->popCond);"},
	}}}
	rs := []Record{
		rec(Spawn, "pthread_create", "worker", "", "lib/pool/pool.c", 10, "pthread_create(&t, NULL, worker, ctx);"),
		rec(Send, "cond_signal", "", "popCond", "lib/pool/pool.c", 20, "cond_signal(&ctx->popCond);"),
		rec(Recv, "cond_wait", "", "popCond", "lib/pool/pool.c", 40, "cond_wait(&ctx->popCond, &m);"),
	}
	units := []model.Unit{mod("MOD-0001", "lib"), posts, exec}
	want(t, Candidates(rs, units), `
true EXEC MOD-0002> pthread_create worker  lib/pool/pool.c:10
false posts_to MOD-0002>MOD-0002 cond_signal  popCond lib/pool/pool.c:20 lib/pool/pool.c:40`)

	posts.Relations[0].Evidence = append(posts.Relations[0].Evidence, model.Evidence{Path: "lib/pool/pool.c", Lines: "39-41", Quote: "cond_wait"})
	units[1] = posts
	if cs := Candidates(rs, units); !cs[1].Stored {
		t.Errorf("a posts_to quoting both ends should hold the pair:\n%s", show(cs))
	}
}
