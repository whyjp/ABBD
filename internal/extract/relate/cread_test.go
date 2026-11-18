package relate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract/lsp"
)

func TestCBaseReadsOnlyClassHeads(t *testing.T) {
	for _, c := range []struct {
		name string
		src  []string
		want bool
	}{
		{"one line", []string{"class Circle : public ^Shape {"}, true},
		{"second base", []string{"class C : public A, public ^B {"}, true},
		{"head over lines", []string{"class Circle", "    : public", "      ^Shape {"}, true},
		{"head after a closed block", []string{"struct A {};", "struct B : ^Shape {"}, true},
		{"parameter after a struct parameter", []string{"void f(struct S a, ^Foo b);"}, false},
		{"parameter on the next line", []string{"void f(struct S a,", "       ^Foo b);"}, false},
		{"parameter after class on the previous line", []string{"int g(class T t,", "      ^Foo b) {"}, false},
	} {
		line := len(c.src) - 1
		col := strings.Index(c.src[line], "^")
		c.src[line] = strings.Replace(c.src[line], "^", "", 1)
		got := cBase.MatchString(cHead(c.src, lsp.Position{Line: line, Character: col}))
		if got != c.want {
			t.Errorf("%s: base = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCInitNamesSkipCommentsAndGatedLines(t *testing.T) {
	src := []string{
		"static const struct ops o = {",
		"    /* never_block() is called later */",
		"    open_fn, /* trailing close_fn */",
		"    // line_comment_fn",
		"    .name = \"str_fn\",",
		"#if EXTRA",
		"    gated_fn,",
		"#endif",
		"    /* multi",
		"       line_block_fn */ last_fn,",
		"    open_fn,",
		"};",
		"after_fn();",
	}
	gated := func(l int) bool { return l >= 5 && l <= 7 }
	var got []string
	for _, id := range cInitNames(src, 0, len(src[0]), gated) {
		got = append(got, id.name)
	}
	if want := []string{"open_fn", "last_fn"}; !slices.Equal(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

func TestCFilesSkipVendoredAndHiddenDirs(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"src/a.c", "src/a.h", "vendor/v.c", "third_party/t.c", "Build/b.c", "node_modules/n.c", ".git/g.c", "src/vendor/x.c"} {
		abs := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := cFiles(tree{root: root}, owners{code: []held{{path: ".", ref: "mod-all"}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"src/a.c", "src/a.h"}; !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestParallelReturnsTheLowestFailingIndex(t *testing.T) {
	failing := map[int]bool{3: true, 17: true, 41: true, 90: true}
	for range 200 {
		err := parallel(100, func(i int) error {
			if failing[i] {
				return errors.New(strings.Repeat("e", i))
			}
			return nil
		})
		if err == nil || err.Error() != strings.Repeat("e", 3) {
			t.Fatalf("error = %v, want the one of index 3", err)
		}
	}
	if err := parallel(10, func(int) error { return nil }); err != nil {
		t.Errorf("error = %v, want none", err)
	}
}
