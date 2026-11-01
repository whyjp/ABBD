package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// understand stores what pstack's how wrote about a module, stamped with the
// module's code as it is now, so the screen can show the text beside the
// drawing and mark it STALE once the code moves on.
func (e env) understand(id model.ID, file string) error {
	v, err := e.load()
	if err != nil {
		return err
	}
	u, ok := presentUnit(v, id)
	if !ok || u.Kind != model.Module || u.Parent != "" {
		return fmt.Errorf("%s is not a top-level module of the actual topology", id)
	}
	var b []byte
	if file == "-" {
		b, err = io.ReadAll(e.stdin)
	} else {
		b, err = os.ReadFile(file)
	}
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return fmt.Errorf("the text is empty; pass pstack how's Overview and Key Concepts")
	}
	digest := e.st.ModuleDigest(v.units, id)
	old, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	// Restamping the same words onto newer code would mark a stale reading
	// fresh without anyone reading the code again.
	if prev, ok := old[id]; ok && prev.Digest != digest && strings.TrimSpace(prev.Text) == text {
		return fmt.Errorf("%s's code changed since this text was written; run pstack how on it again and store the new text", id)
	}
	by := "outside a turn"
	if t, err := e.st.LatestTurn(); err != nil {
		return err
	} else if t != nil && t.State.Resumable() {
		by = "agent in " + t.ID
	}
	if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, By: by, Digest: digest, Text: text}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "understood %s %s, INFERRED, against code %s\n", id, u.Name.Value, digest)
	return nil
}
