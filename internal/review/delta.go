// Package review is the review surface of PRD v1.4 08B: structural deltas, a stable layout,
// a deterministic topology render and the item cards people approve or reject.
package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
)

// Structure is the projection of the SSOT that a reviewer reasons about. Code inside a module
// is not in it, so a turn that only edits module internals projects to the same Structure and
// yields zero deltas (AC-REV-05).
type Structure struct {
	Modules   map[string]ModuleNode
	Deps      map[string]DepEdge // "MOD-a->MOD-b"
	Contracts map[string]ContractNode
	Deploy    map[string]string // module ID -> sorted, comma-joined deploy units
	Features  map[string]FeatureNode
}

type ModuleNode struct {
	ID, Name, Path string
	Inferred       bool
}

type DepEdge struct {
	From, To  string
	Forbidden bool
}

type ContractNode struct {
	ID, Owner, Kind, Direction, Selector, Compat string
	Consumers                                    *bool
	Origin                                       model.Origin
	Inferred                                     bool
}

type FeatureNode struct {
	ID, Name string
	Owners   string // sorted, comma-joined module IDs
	Path     string // sorted, comma-joined contract IDs
	Inferred bool
}

func EdgeKey(from, to string) string { return from + "->" + to }

func Project(s *ssot.Snapshot) Structure {
	st := Structure{Modules: map[string]ModuleNode{}, Deps: map[string]DepEdge{}, Contracts: map[string]ContractNode{}, Deploy: map[string]string{}, Features: map[string]FeatureNode{}}
	for id, m := range s.Modules {
		if m.Status == model.ItemRemoved {
			continue
		}
		st.Modules[id] = ModuleNode{ID: id, Name: m.Name, Path: m.Path, Inferred: m.Responsibility.Pending()}
		for _, d := range m.DependsOn {
			st.Deps[EdgeKey(id, d)] = DepEdge{From: id, To: d, Forbidden: forbidden(m, d)}
		}
		var units []string
		for _, sv := range m.Serving {
			units = append(units, sv.DeployUnit)
		}
		sort.Strings(units)
		if len(units) > 0 {
			st.Deploy[id] = strings.Join(units, ",")
		}
		for _, c := range m.ExternalContracts {
			if c.Status == model.ItemRemoved {
				continue
			}
			st.Contracts[c.ID] = ContractNode{ID: c.ID, Owner: id, Kind: c.Kind, Direction: c.Direction, Selector: c.Selector,
				Compat: c.Compatibility.Value, Consumers: c.ExternalConsumers, Origin: c.Origin, Inferred: c.Compatibility.Pending()}
		}
	}
	for id, f := range s.Features {
		if f.Status == model.ItemRemoved {
			continue
		}
		owners := append([]string{}, f.OwnedBy...)
		path := append([]string{}, f.ExternalContracts...)
		sort.Strings(owners)
		sort.Strings(path)
		st.Features[id] = FeatureNode{ID: id, Name: f.Name, Owners: strings.Join(owners, ","), Path: strings.Join(path, ","), Inferred: f.Intent.Purpose.Pending()}
	}
	return st
}

// forbidden: an explicit forbidden list names it, or an allowed list exists and omits it.
func forbidden(m *model.Module, dep string) bool {
	for _, f := range m.DependencyPolicy.Forbidden {
		if f == dep {
			return true
		}
	}
	if len(m.DependencyPolicy.Allowed) == 0 {
		return false
	}
	for _, a := range m.DependencyPolicy.Allowed {
		if a == dep {
			return false
		}
	}
	return true
}

// tierTable is the default tier of each kind (PRD v1.4 08B section 4). Contract kinds escalate
// to C when the contract has external consumers; that rule lives in contractTier.
var tierTable = map[model.DeltaKind]model.Tier{
	model.ModuleAdded: model.TierB, model.ModuleRemoved: model.TierB, model.ModuleSplit: model.TierB, model.ModuleMerged: model.TierB,
	model.DependencyAdded: model.TierA, model.DependencyRemoved: model.TierA, model.ForbiddenDependency: model.TierB,
	model.ContractAdded: model.TierB, model.ContractRemoved: model.TierB, model.ContractCompatChange: model.TierB,
	model.OwnershipMoved: model.TierB, model.DeployMappingChanged: model.TierB, model.FeaturePathChanged: model.TierA,
}

func hasConsumers(c ContractNode) bool { return c.Consumers != nil && *c.Consumers }

// Diff lists the structural deltas from before to after, in a stable order, with tiers and the
// initial card status. chg names the turn; delta IDs are chg#n.
func Diff(before, after Structure, chg string) []model.StructuralDelta {
	var out []model.StructuralDelta
	add := func(d model.StructuralDelta) {
		if d.Tier == "" {
			d.Tier = tierTable[d.Kind]
		}
		out = append(out, d)
	}

	added, removed := map[string]bool{}, map[string]bool{}
	for id := range after.Modules {
		if _, ok := before.Modules[id]; !ok {
			added[id] = true
		}
	}
	for id := range before.Modules {
		if _, ok := after.Modules[id]; !ok {
			removed[id] = true
		}
	}
	splitMerge(before, after, added, removed, add)
	for _, id := range sorted(added) {
		m := after.Modules[id]
		add(model.StructuralDelta{Kind: model.ModuleAdded, Subject: id, Nodes: []string{id}, After: m.Path, Inferred: m.Inferred, Reason: "새 모듈"})
	}
	for _, id := range sorted(removed) {
		add(model.StructuralDelta{Kind: model.ModuleRemoved, Subject: id, Nodes: []string{id}, Before: before.Modules[id].Path, Reason: "모듈 삭제"})
	}

	// Edges of a module that appeared or disappeared belong to that module's card.
	touchesNewOrGone := func(e DepEdge) bool {
		return added[e.From] || added[e.To] || removed[e.From] || removed[e.To]
	}
	for _, k := range sortedKeys(after.Deps) {
		e := after.Deps[k]
		old, existed := before.Deps[k]
		switch {
		case e.Forbidden && (!existed || !old.Forbidden):
			add(model.StructuralDelta{Kind: model.ForbiddenDependency, Subject: k, Nodes: []string{e.From, e.To}, After: k, Reason: "금지 의존 위반"})
		case !existed && !touchesNewOrGone(e):
			add(model.StructuralDelta{Kind: model.DependencyAdded, Subject: k, Nodes: []string{e.From, e.To}, After: k, Reason: "허용 범위 안의 의존 추가"})
		}
	}
	for _, k := range sortedKeys(before.Deps) {
		e := before.Deps[k]
		if _, ok := after.Deps[k]; !ok && !touchesNewOrGone(e) {
			add(model.StructuralDelta{Kind: model.DependencyRemoved, Subject: k, Nodes: []string{e.From, e.To}, Before: k, Reason: "의존 제거"})
		}
	}

	for _, id := range sortedKeys(after.Contracts) {
		c := after.Contracts[id]
		old, existed := before.Contracts[id]
		if !existed {
			d := model.StructuralDelta{Kind: model.ContractAdded, Subject: id, Nodes: []string{id, c.Owner}, After: c.Direction + " " + c.Selector, Inferred: c.Inferred, Reason: "외부 계약 추가"}
			if hasConsumers(c) {
				d.Tier, d.Reason = model.TierC, "외부 소비자가 있는 계약 추가"
			}
			add(d)
			continue
		}
		if old.Owner != c.Owner {
			add(model.StructuralDelta{Kind: model.OwnershipMoved, Subject: id, Nodes: []string{id, old.Owner, c.Owner}, Before: old.Owner, After: c.Owner, Reason: "계약 소유 모듈 이동"})
		}
		breaking := old.Selector != c.Selector || old.Direction != c.Direction
		if breaking || old.Compat != c.Compat || hasConsumers(old) != hasConsumers(c) {
			d := model.StructuralDelta{Kind: model.ContractCompatChange, Subject: id, Nodes: []string{id, c.Owner},
				Before: contractLabel(old), After: contractLabel(c), Inferred: c.Inferred, Reason: "계약 호환성 변경"}
			if breaking {
				d.Reason = "breaking: selector 변경"
				if hasConsumers(old) || hasConsumers(c) {
					d.Tier, d.Reason = model.TierC, "외부 소비자가 있는 계약의 breaking change"
				}
			}
			add(d)
		}
	}
	for _, id := range sortedKeys(before.Contracts) {
		if _, ok := after.Contracts[id]; ok {
			continue
		}
		c := before.Contracts[id]
		d := model.StructuralDelta{Kind: model.ContractRemoved, Subject: id, Nodes: []string{id, c.Owner}, Before: c.Direction + " " + c.Selector, Reason: "외부 계약 삭제"}
		if hasConsumers(c) {
			d.Tier, d.Reason = model.TierC, "외부 소비자가 있는 계약 삭제"
		}
		add(d)
	}

	for _, id := range sortedKeys(after.Deploy) {
		if old, ok := before.Deploy[id]; ok && old != after.Deploy[id] && !added[id] {
			add(model.StructuralDelta{Kind: model.DeployMappingChanged, Subject: id, Nodes: []string{id}, Before: old, After: after.Deploy[id], Reason: "배포 단위 매핑 변경"})
		}
	}
	for _, id := range sortedKeys(before.Deploy) {
		if _, ok := after.Deploy[id]; !ok && !removed[id] {
			add(model.StructuralDelta{Kind: model.DeployMappingChanged, Subject: id, Nodes: []string{id}, Before: before.Deploy[id], Reason: "배포 단위에서 빠짐"})
		}
	}

	for _, id := range sortedKeys(after.Features) {
		f := after.Features[id]
		old, existed := before.Features[id]
		switch {
		case existed && old.Owners != "" && old.Owners != f.Owners:
			add(model.StructuralDelta{Kind: model.OwnershipMoved, Subject: id, Nodes: append([]string{id}, split(f.Owners)...), Before: old.Owners, After: f.Owners, Reason: "기능 소유 모듈 이동"})
		case !existed || old.Path != f.Path:
			add(model.StructuralDelta{Kind: model.FeaturePathChanged, Subject: id, Nodes: append([]string{id}, split(f.Path)...), Before: old.Path, After: f.Path, Inferred: f.Inferred, Reason: "기능 경로 변경"})
		}
	}
	for _, id := range sortedKeys(before.Features) {
		if _, ok := after.Features[id]; !ok {
			add(model.StructuralDelta{Kind: model.FeaturePathChanged, Subject: id, Nodes: []string{id}, Before: before.Features[id].Path, Reason: "기능 경로 사라짐"})
		}
	}

	for i := range out {
		out[i].ID = fmt.Sprintf("%s#%d", chg, i+1)
		out[i].Status = initialStatus(out[i].Tier)
	}
	return out
}

func initialStatus(t model.Tier) model.DeltaStatus {
	switch t {
	case model.TierA:
		return model.DeltaApplied
	case model.TierC:
		return model.DeltaPending
	default:
		return model.DeltaDefaultApplied
	}
}

// splitMerge recognizes one removed module whose path now holds two or more added modules
// (split), and two or more removed modules now under one added module's path (merge).
func splitMerge(before, after Structure, added, removed map[string]bool, add func(model.StructuralDelta)) {
	under := func(child, parent string) bool { return parent == "." || strings.HasPrefix(child, parent+"/") }
	for _, r := range sorted(removed) {
		var parts []string
		for _, a := range sorted(added) {
			if under(after.Modules[a].Path, before.Modules[r].Path) {
				parts = append(parts, a)
			}
		}
		if len(parts) >= 2 {
			add(model.StructuralDelta{Kind: model.ModuleSplit, Subject: r, Nodes: append([]string{r}, parts...), Before: before.Modules[r].Path, After: strings.Join(parts, ","), Reason: "모듈 분할"})
			delete(removed, r)
			for _, p := range parts {
				delete(added, p)
			}
		}
	}
	for _, a := range sorted(added) {
		var parts []string
		for _, r := range sorted(removed) {
			if under(before.Modules[r].Path, after.Modules[a].Path) {
				parts = append(parts, r)
			}
		}
		if len(parts) >= 2 {
			add(model.StructuralDelta{Kind: model.ModuleMerged, Subject: a, Nodes: append([]string{a}, parts...), Before: strings.Join(parts, ","), After: after.Modules[a].Path, Reason: "모듈 병합"})
			delete(added, a)
			for _, p := range parts {
				delete(removed, p)
			}
		}
	}
}

func contractLabel(c ContractNode) string {
	consumers := "consumers ?"
	if c.Consumers != nil {
		consumers = fmt.Sprintf("consumers %v", *c.Consumers)
	}
	return fmt.Sprintf("%s %s · %s · %s", c.Direction, c.Selector, c.Compat, consumers)
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func sorted(m map[string]bool) []string { return model.SortedKeys(m) }

func sortedKeys[V any](m map[string]V) []string { return model.SortedKeys(m) }

const BaseFile = "generated/turn-base.json"

// SaveBase records the structure at turn start; sync diffs against it, so meaning edits made
// during the turn (intent contract, intent module) show up as structural changes too.
func SaveBase(s *ssot.Snapshot) error {
	b, err := json.Marshal(Project(s))
	if err != nil {
		return err
	}
	p := ssot.Path(s.Root, BaseFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// LoadBase returns the saved turn-start structure, or the current projection when none exists.
func LoadBase(s *ssot.Snapshot) Structure {
	var st Structure
	if b, err := os.ReadFile(ssot.Path(s.Root, BaseFile)); err == nil && json.Unmarshal(b, &st) == nil && st.Modules != nil {
		return st
	}
	return Project(s)
}
