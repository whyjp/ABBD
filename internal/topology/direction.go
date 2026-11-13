package topology

import (
	"regexp"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Reversed is a plug-point contract whose relations look recorded the wrong
// way round: one unit provides it and several consume it by call. The
// modules that implement or register a plug point provide it, and the host
// that calls through it consumes it, so the one provider is likely the host
// and the callers the implementations.
type Reversed struct {
	Contract model.ID   `json:"contract"`
	Name     string     `json:"name"`
	Provider model.ID   `json:"provider"`
	Callers  []model.ID `json:"callers"`
}

// slot matches an entry shaped as a callback slot: a C function pointer,
// such as "void (*)(void *data)", a Rust fn pointer, such as "fn(&mut Ctx)",
// or a trait method, which takes self, such as "fn build(&self)". A free
// function takes no self: a plain call, not a slot.
var slot = regexp.MustCompile(`\(\s*\*|\bfn\s*\(|\bfn\s+\w+[^(]*\(\s*&?('\w+\s+)?(mut\s+)?self\b`)

// plugPoint reports whether u is an interface most of whose entries are
// callback slots a host calls through.
func plugPoint(u model.Unit) bool {
	if u.Kind != model.Interface || len(u.Entries) == 0 {
		return false
	}
	n := 0
	for _, e := range u.Entries {
		if slot.MatchString(e.Shape) {
			n++
		}
	}
	return 2*n > len(u.Entries)
}

// ReversedPlugPoints lists the plug-point contracts with exactly one
// provider and two or more other units that consume them via call, by
// contract. A host that provides a table and calls through it is not one of
// its callers.
func ReversedPlugPoints(units []model.Unit) []Reversed {
	present := model.Present(units)
	providers, callers := map[model.ID][]model.ID{}, map[model.ID][]model.ID{}
	for _, u := range present {
		for _, r := range u.Relations {
			switch {
			case r.Dead:
			case r.Kind == model.Provides:
				providers[r.To] = append(providers[r.To], u.ID)
			case r.Kind == model.Consumes && r.Via == "call":
				callers[r.To] = append(callers[r.To], u.ID)
			}
		}
	}
	var out []Reversed
	for _, u := range present {
		p := providers[u.ID]
		if !plugPoint(u) || len(p) != 1 {
			continue
		}
		if c := slices.DeleteFunc(slices.Clone(callers[u.ID]), func(id model.ID) bool { return id == p[0] }); len(c) >= 2 {
			out = append(out, Reversed{Contract: u.ID, Name: u.Name.Value, Provider: p[0], Callers: slices.Sorted(slices.Values(c))})
		}
	}
	return out
}
