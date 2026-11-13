package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/extract/relate"
)

// relateExtraction prints an extraction with the code-level relations the
// bundled relaters read between its units. The extraction is what the
// command after -- prints, or stdin without one. It reads only.
func (e env) relateExtraction(args []string) error {
	var specs []frontend.Spec
	for _, a := range args {
		s, err := relate.ParseSpec(a)
		if err != nil {
			return err
		}
		specs = append(specs, s)
	}
	var in io.Reader = e.stdin
	if len(e.argv) > 0 {
		out, err := e.runScript(e.argv)
		if err != nil {
			return err
		}
		in = bytes.NewReader(out)
	}
	var doc extract.Doc
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("the extraction to relate: %w", err)
	}
	doc, unread, err := relate.Relate(e.root, cacheDir(), doc, specs)
	if err != nil {
		return err
	}
	for _, u := range unread {
		if u.Gated {
			fmt.Fprintf(e.stderr, "relate: %s: %d files hold code %s, so the relations in it are missing:\n", u.Relater, len(u.Paths), u.Why)
		} else {
			fmt.Fprintf(e.stderr, "relate: %s: %d files were not read, %s:\n", u.Relater, len(u.Paths), u.Why)
		}
		for _, p := range u.Paths {
			fmt.Fprintf(e.stderr, "  %s\n", p)
		}
	}
	return doc.Encode(e.stdout)
}
