package main

import (
	"fmt"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/store"
)

// readOnly commands never publish, so status can still see a STALE file.
var readOnly = map[string]bool{"status": true, "turn show": true, "serve": true}

// screenInput loads what the screen shows, with the working tree it needs to
// say whether the actual topology is current.
func (e env) screenInput() (screen.Input, error) {
	in, err := screen.Load(e.st)
	if err != nil {
		return in, err
	}
	in.Tree, _ = gitx.Snapshot(e.root, store.Dir)
	return in, nil
}

// publish regenerates .abbd/generated/ from the unit Markdown. graph.json
// carries the watermark, so it is written last.
func (e env) publish() error {
	in, err := screen.Load(e.st)
	if err != nil || len(in.Units) == 0 {
		return err
	}
	files, err := screen.Files(screen.Build(in))
	if err != nil {
		return err
	}
	for _, name := range []string{screen.ExplorerFile, screen.QueueFile, screen.GraphFile} {
		if err := e.st.SaveGenerated(name, files[name]); err != nil {
			return err
		}
	}
	return nil
}

// published rereads the generated files and fails unless they are CURRENT.
func (e env) published() error {
	if err := e.publish(); err != nil {
		return err
	}
	in, err := screen.Load(e.st)
	if err != nil {
		return err
	}
	if f := in.GeneratedFreshness(); f != screen.Fresh {
		return fmt.Errorf(".abbd/generated/ is %s after publishing", f)
	}
	return nil
}

func (e env) printFreshness() error {
	in, err := e.screenInput()
	if err != nil || len(in.Units) == 0 {
		return err
	}
	fmt.Fprintln(e.stdout, "generated:", in.GeneratedFreshness())
	switch in.ActualFreshness() {
	case screen.Stale:
		fmt.Fprintf(e.stdout, "actual topology: STALE (extracted at %s, working tree %s; abbd turn diff or abbd mode on re-extracts)\n", in.Synced.Fingerprint, in.Tree)
	default:
		fmt.Fprintln(e.stdout, "actual topology:", in.ActualFreshness())
	}
	return nil
}
