package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// textModule is the module abbd understand stores a text for: any present
// module, at any depth, whose text describes its own subtree. A stored text
// whose unit is not one is an orphan: no command can restamp it.
func textModule(units []model.Unit, id model.ID) (model.Unit, bool) {
	u, ok := presentUnit(units, id)
	return u, ok && u.Kind == model.Module
}

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
	u, ok := textModule(units, id)
	if !ok {
		return fmt.Errorf("%s is not a module", id)
	}
	old, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	digest, code := e.st.Stamp(units, id)
	prev, had := old[id]
	if e.restamp {
		if err := e.mayRestamp(units, prev, had, digest); err != nil {
			return err
		}
		// Code stays what the text was written against, so a file that
		// left the module can come back unchanged.
		prev.Digest = digest
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
	text := store.CleanText(string(b))
	if text == "" {
		return fmt.Errorf("the text is empty; pass pstack how's Overview and Key Concepts")
	}
	// Restamping the same words onto newer code would mark a stale reading
	// fresh without anyone reading the code again.
	if had && prev.Digest != digest && prev.Text == text {
		return fmt.Errorf("%s's code changed since this text was written; run pstack how on it again and store the new text", id)
	}
	if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest, Format: store.CodeFormat, Code: code, Text: text}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "understood %s %s, INFERRED, against code %s\n", id, u.Name.Value, digest)
	readText(e.stdout, id, text)
	return nil
}

// mayRestamp allows a restamp only when every file of the module now is one
// the text was written against, unchanged, and none it was written against
// is gone, so an extraction that moved units is the only thing that moved.
func (e env) mayRestamp(units []model.Unit, prev store.Understanding, had bool, digest string) error {
	id := prev.Unit
	switch {
	case !had:
		return fmt.Errorf("there is no text to restamp; store one with abbd understand MOD-ID FILE")
	case prev.Digest == digest:
		return fmt.Errorf("%s's text is current; nothing to restamp", id)
	case prev.Format != store.CodeFormat:
		return fmt.Errorf("%s's text was stored before abbd recorded the code a text is written against; run pstack how on it again and store the new text", id)
	}
	changed := e.st.Changed(units, id, prev.Code)
	if len(changed) == 0 {
		return nil
	}
	if len(changed) > 5 {
		changed = append(changed[:5], fmt.Sprintf("and %d more", len(changed)-5))
	}
	return fmt.Errorf("%s's code changed since this text was written (%s); run pstack how on it again and store the new text", id, strings.Join(changed, ", "))
}
