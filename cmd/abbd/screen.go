package main

import (
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

// publish renders the blueprint from the .abbd inputs, then records in the
// state which inputs it rendered.
func (e env) publish() error {
	in, err := e.screenInput()
	if err != nil || len(in.Units) == 0 {
		return err
	}
	files, err := screen.Files(screen.Build(in))
	if err != nil {
		return err
	}
	if err := e.st.SaveBlueprint(files); err != nil {
		return err
	}
	in.Synced.Blueprint = in.Inputs
	return e.st.SaveState(in.Synced)
}
