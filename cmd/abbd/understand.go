package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// understand stores what pstack's how wrote about a module, stamped with the
// module's code as it is now, so the screen can show the text beside the
// drawing and mark it STALE once the code moves on. With --restamp it stamps
// the stored text again instead, when only what the module holds moved.
func (e env) understand(id model.ID, args []string) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	if e.restamp != (len(args) == 0) {
		return fmt.Errorf("pass a FILE with the text, or --restamp without one")
	}
	u, ok := presentUnit(units, id)
	if !ok || u.Kind != model.Module || u.Parent != "" {
		return fmt.Errorf("%s is not a top-level module", id)
	}
	old, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	digest := e.st.ModuleDigest(units, id)
	tree, err := gitx.Snapshot(e.root, store.Dir)
	if err != nil {
		return fmt.Errorf("snapshotting the working tree: %w", err)
	}
	prev, had := old[id]
	if e.restamp {
		if err := e.mayRestamp(units, prev, had, digest, tree); err != nil {
			return err
		}
		prev.Digest, prev.Tree = digest, tree
		if err := e.st.SaveUnderstanding(prev); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "restamped %s %s against code %s; its code is unchanged since the text was written\n", id, u.Name.Value, digest)
		return nil
	}
	var b []byte
	if args[0] == "-" {
		b, err = io.ReadAll(e.stdin)
	} else {
		b, err = os.ReadFile(args[0])
	}
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return fmt.Errorf("the text is empty; pass pstack how's Overview and Key Concepts")
	}
	// Restamping the same words onto newer code would mark a stale reading
	// fresh without anyone reading the code again.
	if had && prev.Digest != digest && strings.TrimSpace(prev.Text) == text {
		return fmt.Errorf("%s's code changed since this text was written; run pstack how on it again and store the new text", id)
	}
	if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest, Tree: tree, Text: text}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "understood %s %s, INFERRED, against code %s\n", id, u.Name.Value, digest)
	return nil
}

// mayRestamp allows a restamp only when no file the module's digest reads
// now changed between the tree the text was written at and this one, so an
// extraction that moved units in or out is the only thing that moved.
func (e env) mayRestamp(units []model.Unit, prev store.Understanding, had bool, digest, tree string) error {
	id := prev.Unit
	switch {
	case !had:
		return fmt.Errorf("there is no text to restamp; store one with abbd understand MOD-ID FILE")
	case prev.Digest == digest:
		return fmt.Errorf("%s's text is current; nothing to restamp", id)
	case prev.Tree == "" || tree == "":
		return fmt.Errorf("%s's text records no tree to compare with; run pstack how on it again and store the new text", id)
	}
	changed, ok, err := gitx.Changed(e.root, prev.Tree, tree)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("the tree %s's text was written at is gone; run pstack how on it again and store the new text", id)
	}
	var touched []string
	for _, p := range e.st.ModulePaths(units, id) {
		if slices.Contains(changed, p) {
			touched = append(touched, p)
		}
	}
	if len(touched) > 0 {
		if len(touched) > 5 {
			touched = append(touched[:5], fmt.Sprintf("and %d more", len(touched)-5))
		}
		return fmt.Errorf("%s's code changed since this text was written (%s); run pstack how on it again and store the new text", id, strings.Join(touched, ", "))
	}
	return nil
}
