package main

import (
	"fmt"
	"io"
	"os"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// textModule is the module abbd understand stores a text for. A stored text
// whose unit is not one is an orphan: no command can restamp it.
func textModule(units []model.Unit, id model.ID) (model.Unit, bool) {
	u, ok := presentUnit(units, id)
	return u, ok && u.Kind == model.Module && u.Parent == ""
}

// understand stores what pstack's how wrote about a module, stamped with the
// module's code as it is now, so the screen can show the text beside the
// drawing and mark it STALE once the code moves on.
func (e env) understand(id model.ID, file string) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	u, ok := textModule(units, id)
	if !ok {
		return fmt.Errorf("%s is not a top-level module", id)
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
	text := store.CleanText(string(b))
	if text == "" {
		return fmt.Errorf("the text is empty; pass pstack how's Overview and Key Concepts")
	}
	digest := e.st.ModuleDigest(units, id)
	old, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	// Restamping the same words onto newer code would mark a stale reading
	// fresh without anyone reading the code again.
	if prev, ok := old[id]; ok && prev.Digest != digest && prev.Text == text {
		return fmt.Errorf("%s's code changed since this text was written; run pstack how on it again and store the new text", id)
	}
	if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest, Text: text}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "understood %s %s, INFERRED, against code %s\n", id, u.Name.Value, digest)
	return nil
}
