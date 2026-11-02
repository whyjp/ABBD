package topology

import "github.com/whyjp/abbd/internal/model"

// EntryUse is one live relation that narrows to an entry of a contract: a
// consumer of a route, a writer or a reader of a field.
type EntryUse struct {
	From   model.ID
	Kind   model.RelationKind
	Origin model.Origin
}

// EntryUses lists, by entry key, the live relations that use each entry of
// the contract, in ID order.
func EntryUses(units []model.Unit, contract model.ID) map[string][]EntryUse {
	out := map[string][]EntryUse{}
	for _, l := range Links(units) {
		if l.To == contract {
			for _, k := range l.Entries {
				out[k] = append(out[k], EntryUse{From: l.From, Kind: l.Kind, Origin: l.Origin})
			}
		}
	}
	return out
}
