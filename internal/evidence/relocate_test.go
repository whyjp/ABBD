package evidence

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
)

// repo makes a git working tree, so a quote has committable files to move to.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return root
}

func write(t *testing.T, root, p, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const charge = "client.Charge(ctx, order.Amount, order.Currency)"

func TestAQuoteNeverMovesIntoATest(t *testing.T) {
	for _, p := range []string{"pay/service_test.go", "pay/tests/service.go", "pay/test/service.go", "pay/__tests__/service.go"} {
		root := repo(t)
		write(t, root, "pay/service.go", "package pay\n// the call is gone\n")
		write(t, root, p, "package pay\nfunc TestX() {\n\t"+charge+"\n}\n")
		units := []model.Unit{{ID: "MOD-0001", Kind: model.Module, Anchors: []model.Anchor{{Path: "pay"}}, Relations: []model.Relation{{
			To: "MOD-0002", Kind: model.DependsOn, Origin: model.Inferred,
			Evidence: []model.Evidence{{Path: "pay/service.go", Lines: "2", Quote: charge}},
		}}}}
		if got := New(root).Apply(units)[0].Relations[0]; !got.Dead {
			t.Errorf("%s keeps the coupling alive: evidence=%+v", p, got.Evidence)
		}
	}
}

// The state's fingerprint leaves ignored files out, so they must not decide
// whether a quote lives: the same fingerprint gives the same state.
func TestAnIgnoredBuildOutputNeverChangesTheState(t *testing.T) {
	root := repo(t)
	write(t, root, ".gitignore", "dist/\n")
	write(t, root, "src/app.js", "// the call is gone\n")
	e := model.Evidence{Path: "src/app.js", Lines: "1", Quote: charge}
	everywhere := []model.Anchor{{Path: "."}}

	before, err := gitx.Snapshot(root, ".abbd")
	if err != nil {
		t.Fatal(err)
	}
	clean := New(root).Check(e, everywhere)
	write(t, root, "dist/app.js", charge+"\n")
	after, err := gitx.Snapshot(root, ".abbd")
	if err != nil {
		t.Fatal(err)
	}
	built := New(root).Check(e, everywhere)
	if before != after || clean.Live || built.Live {
		t.Errorf("fingerprint %s then %s; live=%v without dist/, live=%v (path %q) with it", before, after, clean.Live, built.Live, built.Path)
	}
}

// A vendored or generated copy lies outside the unit that cited the quote.
func TestAQuoteMovesOnlyWithinItsOwnersAnchors(t *testing.T) {
	for _, dir := range []string{"third_party/lib", "target/debug/build", ".venv/lib/site-packages/x", "build/gen"} {
		root := repo(t)
		write(t, root, "src/a.go", "package a\n")
		write(t, root, dir+"/copy.go", charge+"\n")
		if r := New(root).Check(model.Evidence{Path: "src/a.go", Quote: charge}, []model.Anchor{{Path: "src"}}); r.Live {
			t.Errorf("%s: quote followed into %q", dir, r.Path)
		}
	}
	root := repo(t)
	write(t, root, "src/a.go", "package a\n")
	write(t, root, "src/inner/b.go", charge+"\n")
	write(t, root, "other/c.go", "package c\n")
	if r := New(root).Check(model.Evidence{Path: "src/a.go", Quote: charge}, []model.Anchor{{Path: "src"}}); !r.Live || r.Path != "src/inner/b.go" {
		t.Errorf("a quote under its owner's directory anchor should follow: %+v", r)
	}
	if r := New(root).Check(model.Evidence{Path: "src/a.go", Quote: charge}, []model.Anchor{{Path: "src/a.go"}, {Path: "other/c.go"}}); r.Live {
		t.Errorf("a file anchor covers only its own file: %+v", r)
	}
}
