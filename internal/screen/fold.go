package screen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A view whose units use each other round in a circle cannot read top to
// bottom: some line must climb back. When such a cycle is only part of a
// view, the view folds it into one box, and that box's own view draws the
// cycle. Folding decomposes; it never leaves a relation out. A cycle that
// fills most of its view has nothing to fold into, and the view draws its
// climbing lines dashed instead (rule 13).

// foldMin is the smallest cycle worth a box of its own. A cycle is folded
// only while it holds at most two thirds of the view's boxes, so the view
// keeps showing mostly real structure rather than one box and a remainder.
const foldMin = 3

// fold folds each tangled cycle among container's children. The fold is a
// unit of the drawing only: the store never holds it.
func (t *tree) fold(container model.ID) {
	kids := t.children[container]
	var units []model.ID
	for _, k := range kids {
		if !t.units[k].Kind.IsContract() {
			units = append(units, k)
		}
	}
	if len(units) <= foldMin {
		return
	}
	uses := map[model.ID]map[model.ID]bool{}
	for _, k := range units {
		uses[k] = map[model.ID]bool{}
		for _, s := range t.sources(k) {
			for _, r := range t.units[s].Relations {
				if _, live := t.units[r.To]; !live || r.Dead {
					continue
				}
				if to, ok := t.within(r.To, container); ok && to != k {
					uses[k][to] = true
				}
			}
		}
	}
	reach := func(from model.ID) map[model.ID]bool {
		seen := map[model.ID]bool{from: true}
		stack := []model.ID{from}
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for w := range uses[v] {
				if !seen[w] {
					seen[w] = true
					stack = append(stack, w)
				}
			}
		}
		return seen
	}
	reaches := map[model.ID]map[model.ID]bool{}
	for _, k := range units {
		reaches[k] = reach(k)
	}
	done := map[model.ID]bool{}
	n := 0
	for _, k := range units {
		if done[k] {
			continue
		}
		var cycle []model.ID
		for _, j := range units {
			if reaches[k][j] && reaches[j][k] {
				cycle = append(cycle, j)
				done[j] = true
			}
		}
		if len(cycle) < foldMin || 3*len(cycle) > 2*len(kids) {
			continue
		}
		n++
		id := model.ID(fmt.Sprintf("%s~%d", container, n))
		var names []string
		for _, c := range cycle {
			names = append(names, t.units[c].Name.Value)
			u := t.units[c]
			u.Parent = id
			t.units[c] = u
		}
		t.units[id] = model.Unit{
			ID: id, Kind: model.Module, Parent: container,
			Name:    model.Fact{Value: fmt.Sprintf("서로 쓰는 %d개", len(cycle)), Origin: model.Extracted},
			Purpose: &model.Fact{Value: "서로를 돌아가며 쓰는 단위: " + strings.Join(names, ", "), Origin: model.Extracted},
		}
		t.folds[id] = true
		t.children[id] = cycle
		t.children[container] = slices.DeleteFunc(t.children[container], func(c model.ID) bool { return slices.Contains(cycle, c) })
		t.children[container] = append(t.children[container], id)
		slices.Sort(t.children[container])
	}
}
