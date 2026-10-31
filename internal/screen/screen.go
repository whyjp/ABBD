// Package screen builds what the human sees: the turn's changes first, then
// drift, trust, the confirm queue and the map. It reads the store and never
// writes the topology; the CLI owns every decision the page sends back.
package screen

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/whyjp/abbd/internal/confirm"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/topology"
	"github.com/whyjp/abbd/internal/turn"
)

// Input is everything the screen reads.
type Input struct {
	Units    []model.Unit
	Intended *topology.Intended
	// Turns are oldest first.
	Turns    []turn.Turn
	Requests map[string]string
	// Runs maps a verification run to its result.
	Runs    map[string]string
	ADRs    []store.ADR
	Project store.Project
	// Synced is the watermark of the last extraction; Tree is the working tree now.
	Synced store.State
	Tree   string
	Inputs string
	// Generated is the watermark the files in .abbd/generated/ carry, nil when missing.
	Generated *Watermark
}

// Watermark ties a derived file to the .abbd inputs and intended topology it came from.
type Watermark struct {
	Inputs   string `json:"inputs_hash"`
	Intended int    `json:"intended_revision"`
}

// Load reads the store. It leaves Tree empty: snapshotting the working tree
// costs a git call, and only a live view needs it.
func Load(st store.Store) (Input, error) {
	var in Input
	var err error
	if in.Units, err = st.LoadUnits(); err != nil {
		return in, err
	}
	if in.Intended, err = st.LoadIntended(); err != nil {
		return in, err
	}
	if in.Turns, err = st.LoadTurns(); err != nil {
		return in, err
	}
	in.Requests, in.Runs = map[string]string{}, map[string]string{}
	for _, t := range in.Turns {
		if text, err := st.Request(t.Requirement); err == nil {
			in.Requests[t.Requirement] = text
		}
		if r, err := st.LoadRun(t.Run); err == nil && r != nil {
			in.Runs[t.Run] = string(r.Result)
		}
	}
	if in.ADRs, err = st.LoadADRs(); err != nil {
		return in, err
	}
	if in.Project, err = st.LoadProject(); err != nil {
		return in, err
	}
	if in.Synced, err = st.LoadState(); err != nil {
		return in, err
	}
	if in.Inputs, err = st.InputsHash(); err != nil {
		return in, err
	}
	b, err := st.ReadGenerated(GraphFile)
	if err != nil {
		return in, err
	}
	if b != nil {
		var g struct{ Watermark Watermark }
		if json.Unmarshal(b, &g) == nil {
			in.Generated = &g.Watermark
		}
	}
	return in, nil
}

// Current is the watermark the generated files must carry right now.
func (in Input) Current() Watermark {
	w := Watermark{Inputs: in.Inputs}
	if in.Intended != nil {
		w.Intended = in.Intended.Revision
	}
	return w
}

type Freshness string

const (
	Fresh   Freshness = "CURRENT"
	Stale   Freshness = "STALE"
	Unknown Freshness = "UNKNOWN"
)

// GeneratedFreshness compares the generated files with the inputs they derive from.
func (in Input) GeneratedFreshness() Freshness {
	switch {
	case in.Generated == nil:
		return Unknown
	case *in.Generated != in.Current():
		return Stale
	}
	return Fresh
}

// ActualFreshness says whether the actual topology was extracted from the working tree as it is now.
func (in Input) ActualFreshness() Freshness {
	switch {
	case in.Tree == "" || in.Synced.Fingerprint == "":
		return Unknown
	case in.Tree != in.Synced.Fingerprint:
		return Stale
	}
	return Fresh
}

// Card is one grade C item waiting for a human.
type Card struct {
	Key    string
	Detail string
	// Question is true for a declared change asked before any edit.
	Question  bool
	Turn      string
	Rejection *turn.Rejection
}

type TurnView struct {
	turn.Turn
	Request  string
	Verdicts []string
	// Live is true while the turn can still take decisions.
	Live bool
	// Standing are the rejections no later approval replaced.
	Standing []turn.Rejection
	// Verified is the result of the turn's last verification run.
	Verified string
}

type Row struct {
	Key, Effect, Detail string
}

type QueueRow struct {
	confirm.Item
	Name string
	// Field is true for a purpose or responsibility, false for a coupling.
	Field bool
}

type Candidate struct {
	ID, Kind, Name, Purpose, Badge string
	Anchors                        []string
}

// Group is a run of rows under one heading. The page folds each group, so a
// large project's first screen stays short.
type Group[T any] struct {
	Title  string
	Design bool
	Rows   []T
}

// group gathers rows by title and orders the groups by order(title).
func group[T any](rows []T, title func(T) string, order func(string) string) []Group[T] {
	var out []Group[T]
	at := map[string]int{}
	for _, r := range rows {
		t := title(r)
		i, ok := at[t]
		if !ok {
			i, at[t] = len(out), len(out)
			out = append(out, Group[T]{Title: t})
		}
		out[i].Rows = append(out[i].Rows, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return order(out[i].Title) < order(out[j].Title) })
	return out
}

// byKind orders kinds as a reader meets them: modules, then contracts, then features.
func byKind(k string) string {
	return fmt.Sprintf("%d", slices.Index([]model.Kind{model.Module, model.Interface, model.Event, model.Data, model.Config, model.Feature}, model.Kind(k)))
}

// Promotion is a candidate a human may adopt as a design unit or keep below the line.
type Promotion struct {
	Key, ID, Kind, Name string
	// Design lists the design units it relates to.
	Design []string
}

type Fact struct {
	Value, Badge, Rejected string
}

type UnitRow struct {
	ID, Kind, Name, Signature string
	Design                    bool
	Purpose, Responsibility   *Fact
	Anchors                   []string
}

type Model struct {
	Phase      topology.Phase
	Pending    []Card
	Turn       *TurnView
	History    []TurnView
	Drift      []Row
	Promote    []Group[Promotion]
	Defaults   []store.ADR
	Queue      []QueueRow
	Align      []Candidate
	Units      []UnitRow
	Actual     Freshness
	Generated  Freshness
	Registered int
	// Realign offers the short realignment: drift a human acknowledges, outside a live turn.
	Realign bool
	// Waiting is true while a grade C change waits, which realignment must not pass.
	Waiting bool
	// Idle is true when no unfinished turn waits, so promotion decisions are open.
	Idle      bool
	Map       Map
	Extract   []string
	Verify    []string
	Watermark Watermark
	Synced    store.State
	Tree      string
}

// Build derives the screen from the store. It is deterministic: the same
// input gives the same model, so the generated files only change with the data.
func Build(in Input) Model {
	m := Model{
		Phase: topology.DerivePhase(in.Units, in.Intended), Actual: in.ActualFreshness(), Generated: in.GeneratedFreshness(),
		Extract: in.Project.Extract, Verify: in.Project.Verify, Watermark: in.Current(), Synced: in.Synced, Tree: in.Tree,
	}
	design := map[model.ID]bool{}
	var items []topology.Item
	if in.Intended != nil {
		for _, u := range in.Intended.Units {
			design[u.ID] = true
		}
		items = topology.Compare(*in.Intended, in.Units)
	}
	names := map[model.ID]string{}
	for _, u := range in.Units {
		names[u.ID] = u.Name.Value
	}

	if n := len(in.Turns); n > 0 {
		latest := in.Turns[n-1]
		tv := view(in, latest)
		if latest.State.Resumable() && in.Intended != nil {
			tv.Live = true
			vs := turn.Judge(*in.Intended, in.Units, latest.Approvals)
			for _, v := range vs {
				tv.Verdicts = append(tv.Verdicts, v.String())
			}
			m.Pending = pendingInTurn(latest, vs)
		}
		m.Turn = &tv
		for i := n - 2; i >= 0 && len(m.History) < 20; i-- {
			m.History = append(m.History, view(in, in.Turns[i]))
		}
	}
	if (m.Turn == nil || !m.Turn.Live) && in.Intended != nil {
		for _, it := range items {
			if it.Effect == topology.Blocked {
				c := Card{Key: it.Key(), Detail: detail(it)}
				if a := store.Rejection(in.ADRs, turn.Question(*in.Intended, it)); a != nil {
					c.Rejection = &turn.Rejection{Item: it.Key(), ADR: a.ID, Reason: a.Reason}
				}
				m.Pending = append(m.Pending, c)
			}
		}
	}
	m.Idle = m.Turn == nil || !m.Turn.Live
	for _, it := range items {
		if it.Effect != topology.Promote {
			m.Drift = append(m.Drift, Row{Key: it.Key(), Effect: string(it.Effect), Detail: detail(it)})
			m.Realign = m.Realign || (it.Effect == topology.Realign && m.Idle)
			m.Waiting = m.Waiting || it.Effect == topology.Blocked
		}
	}
	if in.Intended != nil {
		var rows []Promotion
		for _, c := range topology.Candidates(*in.Intended, in.Units, func(q string) bool { return store.Rejection(in.ADRs, q) != nil }) {
			p := Promotion{Key: topology.Item{Change: topology.UnitAdded, Unit: c.Unit}.Key(), ID: string(c.Unit), Kind: string(c.Kind), Name: names[c.Unit]}
			for _, id := range c.Design {
				p.Design = append(p.Design, string(id))
			}
			rows = append(rows, p)
		}
		m.Promote = group(rows, func(p Promotion) string { return p.Kind }, byKind)
	}
	for _, a := range in.ADRs {
		if a.Confirmation == model.Unconfirmed {
			m.Defaults = append(m.Defaults, a)
		}
	}
	for _, it := range confirm.Queue(in.Units, design) {
		m.Queue = append(m.Queue, QueueRow{Item: it, Name: names[it.Unit], Field: !isRelation(it.Key)})
	}
	present := topology.Present(in.Units)
	if m.Phase == topology.Bootstrapped {
		for _, u := range present {
			c := Candidate{ID: string(u.ID), Kind: string(u.Kind), Name: u.Name.Value, Anchors: anchors(u)}
			if u.Purpose != nil {
				c.Purpose, c.Badge = u.Purpose.Value, Badge(u.Purpose.Origin, u.Purpose.Confirmation)
			}
			m.Align = append(m.Align, c)
		}
	}
	for _, u := range present {
		m.Units = append(m.Units, UnitRow{
			ID: string(u.ID), Kind: string(u.Kind), Name: u.Name.Value, Signature: value(u.Signature), Design: design[u.ID],
			Purpose: fact(u.Purpose), Responsibility: fact(u.Responsibility), Anchors: anchors(u),
		})
		for _, r := range u.Relations {
			if r.Origin == model.UserStated || (r.Confirmation == model.Confirmed && r.Origin != model.Extracted) {
				m.Registered++
			}
		}
	}
	m.Map = Draw(in.Units, in.Intended, items, in.Synced.HighWater)
	return m
}

func view(in Input, t turn.Turn) TurnView {
	tv := TurnView{Turn: t, Request: in.Requests[t.Requirement], Verified: in.Runs[t.Run]}
	seen := map[string]bool{}
	for _, r := range t.Rejections {
		if !seen[r.Item] {
			seen[r.Item] = true
			if s := t.Rejected(r.Item); s != nil {
				tv.Standing = append(tv.Standing, *s)
			}
		}
	}
	return tv
}

// pendingInTurn lists the questions still declared and unanswered, then the
// blocked changes the gate sees, each once.
func pendingInTurn(t turn.Turn, vs []turn.Verdict) []Card {
	var out []Card
	seen := map[string]bool{}
	add := func(c Card) {
		if !seen[c.Key] {
			seen[c.Key] = true
			c.Turn, c.Rejection = t.ID, t.Rejected(c.Key)
			out = append(out, c)
		}
	}
	for _, q := range t.Questions {
		if slices.Contains(t.Delta.Changes, q) && t.Approved(q) == "" {
			add(Card{Key: q, Question: true, Detail: "편집 전에 선언함"})
		}
	}
	for _, v := range turn.Unallowed(vs) {
		add(Card{Key: v.Item.Key(), Detail: detail(v.Item)})
	}
	return out
}

func detail(it topology.Item) string {
	switch it.Change {
	case topology.ContractChanged:
		return fmt.Sprintf("시그니처 %q → %q", it.Old, it.New)
	case topology.NewRelation:
		return fmt.Sprintf("새 결합: %s %s %s", it.Edge.From, it.Edge.Kind, it.Edge.To)
	case topology.RemovedRelation:
		return fmt.Sprintf("사라진 결합: %s %s %s", it.Edge.From, it.Edge.Kind, it.Edge.To)
	case topology.UnitRemoved:
		return string(it.Unit) + "이(가) 코드에서 사라짐"
	}
	return string(it.Unit)
}

func isRelation(key string) bool {
	_, err := topology.ParseEdge(key)
	return err == nil
}

// Badge is the trust a value earns from its evidence axes: stated or
// confirmed by a human, inferred by an agent, or read from code.
func Badge(o model.Origin, c model.Confirmation) string {
	switch {
	case c == model.Rejected:
		return "stated, guess rejected"
	case o == model.UserStated:
		return "stated"
	case c == model.Confirmed:
		return "confirmed"
	case o == model.Inferred:
		return "inferred"
	case o == model.Adopted:
		return "adopted"
	}
	return "extracted"
}

func fact(f *model.Fact) *Fact {
	if f == nil {
		return nil
	}
	return &Fact{Value: f.Value, Badge: Badge(f.Origin, f.Confirmation), Rejected: f.Rejected}
}

func value(f *model.Fact) string {
	if f == nil {
		return ""
	}
	return f.Value
}

func anchors(u model.Unit) []string {
	var out []string
	for _, a := range u.Anchors {
		s := a.Path
		if a.Symbol != "" {
			s += " " + a.Symbol
		}
		out = append(out, s)
	}
	return out
}
