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
// such as "void (*)(void *data)", or a trait method, such as "fn build(&self)".
var slot = regexp.MustCompile(`\(\s*\*|^(pub(\([^)]*\))?\s+)?(async\s+)?(unsafe\s+)?fn\b`)

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
// provider and two or more units that consume them via call, by contract.
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
		if p, c := providers[u.ID], callers[u.ID]; plugPoint(u) && len(p) == 1 && len(c) >= 2 {
			out = append(out, Reversed{Contract: u.ID, Name: u.Name.Value, Provider: p[0], Callers: slices.Sorted(slices.Values(c))})
		}
	}
	return out
}
