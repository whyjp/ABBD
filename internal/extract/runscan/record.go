// Package runscan finds where code starts threads, tasks, pools and loops,
// and where it hands work between them, from a catalog of each language's
// spawn and channel calls. What it finds are candidates: a reader confirms
// each, gives it a run shape and stores it with quotes.
package runscan

import (
	"encoding/json"
	"io"
)

// Kind is what a record says the code does at its site.
type Kind string

const (
	// Spawn starts an execution context: a thread, a task, a pool, a runtime
	// or an event loop.
	Spawn Kind = "spawn"
	// Send hands work or data to a channel, a queue or a signal.
	Send Kind = "send"
	// Recv takes work or data from one.
	Recv Kind = "recv"
	// RegisterCallback hands a function to a context that later calls it.
	RegisterCallback Kind = "register_callback"
	// Impl implements a contract a host calls through.
	Impl Kind = "impl"
)

// Site is the line a record rests on, quoted verbatim.
type Site struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Quote string `json:"quote"`
}

// Record is one site a front-end found. The catalog scanner and the
// language front-ends share this shape, so a semantic record can confirm or
// replace a syntactic one at the same site.
type Record struct {
	Kind Kind   `json:"kind"`
	API  string `json:"api"`
	// Module is the unit that owns the site, once the records are mapped
	// onto the state, and "" before or where no unit anchors the code.
	Module string `json:"module"`
	// From is the code that acts, when the front-end knows it.
	From string `json:"from"`
	// To is the function a spawn or a registration runs, or the contract an
	// impl implements, when the call names it.
	To Target `json:"to"`
	// Channel names what a send or a recv goes through, as the code spells it.
	Channel  string   `json:"channel"`
	Evidence Site     `json:"evidence"`
	Configs  []string `json:"configs"`
	// Confidence is syntactic for the catalog and semantic for a front-end
	// that resolved the symbol.
	Confidence string `json:"confidence"`
	Via        string `json:"via"`
}

// Target is what a record points at: its name as the call spells it, and
// its definition as path:line when one definition of that name stands in
// the scanned code. An unresolved target is {}.
type Target struct {
	Name string `json:"name,omitempty"`
	Def  string `json:"def,omitempty"`
}

// WriteJSON prints records as JSON lines.
func WriteJSON(w io.Writer, rs []Record) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, r := range rs {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}
