package main

import (
	"fmt"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/store"
)

// screenInput loads what the blueprint shows, with the working tree it needs
// to say whether the state is current.
func (e env) screenInput() (screen.Input, error) {
	in, err := screen.Load(e.st)
	if err != nil {
		return in, err
	}
	in.Tree, _ = gitx.Snapshot(e.root, store.Dir)
	return in, nil
}

// publish renders the blueprint from the .abbd inputs. graph.json carries the
// watermark, so it is written last.
func (e env) publish() error {
	in, err := e.screenInput()
	if err != nil || len(in.Units) == 0 {
		return err
	}
	files, err := screen.Files(screen.Build(in))
	if err != nil {
		return err
	}
	for _, name := range []string{screen.ExplorerFile, screen.GraphFile} {
		if err := e.st.SaveGenerated(name, files[name]); err != nil {
			return err
		}
	}
	return nil
}

func (e env) printFreshness() error {
	in, err := e.screenInput()
	if err != nil || len(in.Units) == 0 {
		return err
	}
	fmt.Fprintln(e.stdout, "blueprint:", in.GeneratedFreshness())
	switch in.ActualFreshness() {
	case screen.Stale:
		fmt.Fprintf(e.stdout, "state: STALE (synced at %s; the code changed since; abbd sync re-extracts)\n", in.Synced.Revision)
	default:
		fmt.Fprintf(e.stdout, "state: %s (synced at %s)\n", in.ActualFreshness(), in.Synced.Revision)
	}
	return nil
}
