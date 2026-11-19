package screen

import (
	"slices"
	"sort"
)

// A view's lines are routed by a loop of measure, judge, act and measure
// again. A router offers the actions: each makes one move from a state, the
// layout a view's lines are laid on and the modes lay lays them in, to
// another, and lays the lines from it into a drawing whose cost it measures.
// A policy calls the actions, reads what each drawing measures, and keeps one.
// Laying never writes the state it lays from, so no drawing has to be put
// back, and every drawing carries the moves that led from the router's start
// to its state: replaying them draws it again.
//
// The built-in policy (builtin) keeps what the routing always kept. Moves,
// options and measures are plain values, so a policy that is an agent
// calling the same actions one at a time can read and log them.

// state is what a view's lines are laid from: a layout and the modes.
type state struct {
	from layout
	opts layOpts
	// path is the moves that led here from the router's start.
	path []move
}

// A move is one action called by name with its argument, as a log keeps it
// and a caller names it.
type move struct {
	Action string `json:"action"`
	// On says whether a mode is turned on (apart, share, drops, tweak,
	// early).
	On bool `json:"on,omitempty"`
	// Lines are the lines bend keeps on their buses.
	Lines []int `json:"lines,omitempty"`
	// Tracks are the tracks hold holds each crossing at or under: gap,
	// owner, side and track.
	Tracks [][4]int `json:"tracks,omitempty"`
}

// The actions a router offers.
const (
	// The modes lay draws in (layOpts).
	actApart = "apart"
	actShare = "share"
	actDrops = "drops"
	actBend  = "bend"
	actTweak = "tweak"
	actEarly = "early"
	// straighten moves the chains onto the span their lines' ends share.
	actStraighten = "straighten"
	// hold holds every crossing at or under the track it took in another
	// drawing.
	actHold = "hold"
)

// laid is one drawing of a view's lines: the state it was laid from, the
// layout lay left, and what it drew.
type laid struct {
	state
	at layout
	r  routing
	// weighed and costed are its measures, once read.
	weighed *weight
	costed  *cost
}

// Measure is what a drawing costs a reader, in the measures a policy reads:
// weigh's and costOf's (weigh.go), and what the drawing says could be drawn
// otherwise.
type Measure struct {
	Weigh   *Weight `json:"weigh,omitempty"`
	CostOf  *Cost   `json:"cost,omitempty"`
	Shares  bool    `json:"shares,omitempty"`
	Dropped bool    `json:"dropped,omitempty"`
	Bent    []int   `json:"bent,omitempty"`
	Tweaked bool    `json:"tweaked,omitempty"`
	Joined  bool    `json:"joined,omitempty"`
}

// Weight is a weight as a policy outside the package reads it.
type Weight struct {
	Touches   int `json:"touches"`
	Crossings int `json:"crossings"`
	Braid     int `json:"braid"`
	Turns     int `json:"turns"`
}

func (w weight) export() *Weight { return &Weight{w.touches, w.crossings, w.braid, w.turns} }

// Cost is a cost as a policy outside the package reads it, in beats' order.
type Cost struct {
	Touches   int  `json:"touches"`
	Breaks    int  `json:"breaks"`
	Hard      int  `json:"hard"`
	Seen      int  `json:"seen"`
	Turned    int  `json:"turned"`
	Over      int  `json:"over"`
	Crossings int  `json:"crossings"`
	Steps     int  `json:"steps"`
	Over2     int  `json:"over2"`
	Braid     int  `json:"braid"`
	Climb     int  `json:"climb"`
	Far       int  `json:"far"`
	Climbs    bool `json:"climbs,omitempty"`
}

func (c cost) export() *Cost {
	return &Cost{c.touches, c.breaks, c.hard, c.seen, c.turned, c.over, c.crossings, c.steps, c.over2, c.braid, c.climb, c.far, c.climbs}
}

// routeStep is one drawing a router laid, as its log keeps it: the moves that led
// to it, the modes it was laid in, and what it measured as the policy read
// it.
type routeStep struct {
	Path    []move  `json:"path"`
	Opts    layOpts `json:"opts"`
	Measure Measure `json:"measure"`
}

// router lays one view's lines for a policy and logs what it laid.
type router struct {
	reqs  []wireReq
	start state
	log   []*routeStep
	// logged is each drawing's line in the log.
	logged map[*laid]*routeStep
}

func newRouter(l layout, reqs []wireReq) *router {
	return &router{reqs: reqs, start: state{from: l}, logged: map[*laid]*routeStep{}}
}

// call makes move m from st. It says false where the move has nothing to
// do: straighten moves no chain.
func (rt *router) call(st state, m move) (state, bool) {
	next := state{from: st.from, opts: st.opts, path: append(slices.Clone(st.path), m)}
	switch m.Action {
	case actApart:
		next.opts.Apart = m.On
	case actShare:
		next.opts.Shared = m.On
	case actDrops:
		next.opts.Drops, next.opts.Bent = m.On, nil
		if m.On {
			next.opts.Bent = map[int]bool{}
		}
	case actBend:
		next.opts.Bent = map[int]bool{}
		for _, i := range m.Lines {
			next.opts.Bent[i] = true
		}
	case actTweak:
		next.opts.Tweak = m.On
	case actEarly:
		next.opts.Early = m.On
	case actStraighten:
		next.from = st.from.keep()
		if len(rt.reqs) > shareMost || !next.from.straighten(rt.reqs) {
			return st, false
		}
	case actHold:
		next.from.held = map[[3]int]int{}
		for _, t := range m.Tracks {
			next.from.held[[3]int{t[0], t[1], t[2]}] = t[3]
		}
	default:
		return st, false
	}
	return next, true
}

// lay lays the lines from st into a drawing, on a copy of its layout.
func (rt *router) lay(st state) *laid {
	at := st.from.keep()
	r := at.lay(rt.reqs, st.opts)
	at.held = nil
	s := &laid{state: st, at: at, r: r}
	t := &routeStep{Path: st.path, Opts: st.opts, Measure: Measure{Shares: r.shares, Dropped: r.dropped, Bent: r.bent, Tweaked: r.tweaked, Joined: r.joined}}
	rt.log = append(rt.log, t)
	rt.logged[s] = t
	return s
}

// weigh is weigh's measure of s, read once.
func (rt *router) weigh(s *laid) weight {
	if s.weighed == nil {
		c := weigh(s.r, rt.reqs)
		s.weighed = &c
		rt.logged[s].Measure.Weigh = c.export()
	}
	return *s.weighed
}

// cost is costOf's measure of s, read once.
func (rt *router) cost(s *laid) cost {
	if s.costed == nil {
		c := costOf(s.r, rt.reqs)
		s.costed = &c
		rt.logged[s].Measure.CostOf = c.export()
	}
	return *s.costed
}

// replay makes the moves of path from the router's start, in order, and
// lays the lines from where they lead.
func (rt *router) replay(path []move) (*laid, bool) {
	st := rt.start
	for _, m := range path {
		var ok bool
		if st, ok = rt.call(st, m); !ok {
			return nil, false
		}
	}
	return rt.lay(st), true
}

// A policy routes a view through a router's actions and keeps one drawing.
type policy interface {
	route(rt *router) *laid
}

// builtin is the policy routing has always kept. Its choices are the modes
// it lays a view both ways in, outermost first: each way is laid with every
// later choice made, and the first way is kept unless it leaves another way
// to try and that reads better. Then, where straighten moves a chain, the
// view is drawn again with the chains moved, kept where weigh reads it
// better, and else once more with every crossing held at or under its first
// track: a line that drops straight leaves its track empty rather than pull
// the tracks below it up, which would shorten the steps beneath into jogs.
type builtin struct{}

// choice is a mode builtin lays a view both ways in.
type choice struct {
	action string
	// first is the way laid first, and the one kept where open says there
	// is no other.
	first bool
	open  func(rt *router, first *laid) bool
	// keeps says the first way reads better than the other.
	keeps func(rt *router, first, other *laid) bool
}

var choices = []choice{
	// Lines between modules that use each other, bundled, read both ways
	// and join every pair of their far ends (rule 18), so they are drawn
	// apart, on buses of their own pair, unless that runs a line out of its
	// way past rule 15 or back past rule 13, which bundling spares. Only
	// where bundling would join such places is there anything to draw apart
	// (bundlesPairs).
	{actApart, false,
		func(rt *router, _ *laid) bool { return bundlesPairs(rt.reqs) },
		func(rt *router, bundled, apart *laid) bool { return rt.cost(apart).breaks > rt.cost(bundled).breaks }},
	// Where lines could share arrival buses, the shared buses are kept only
	// where they spare the reader turns and cost nothing else, as weigh
	// measures it. weigh compares every pair of strands, so a view with
	// more than shareMost requests keeps the plain drawing rather than pay
	// for that.
	{actShare, false,
		func(rt *router, plain *laid) bool { return plain.r.shares && len(rt.reqs) <= shareMost },
		func(rt *router, plain, shared *laid) bool { return !rt.weigh(shared).beats(rt.weigh(plain)) }},
	// Lines that can drop straight leave their buses, each that then does
	// not stand straight kept on them (bend), since such a line leaves or
	// reaches its box beside its bundle (rule 4); that is kept where weigh
	// reads it better than every line bundled.
	{actDrops, true,
		func(_ *router, s *laid) bool { return s.r.dropped },
		func(rt *router, dropped, bundled *laid) bool { return rt.weigh(dropped).beats(rt.weigh(bundled)) }},
	// Lines that would cross over the box they reach and come back are set
	// straight onto their arrival trunks, kept where that costs the reader
	// less than the plain drawing.
	{actTweak, true,
		func(_ *router, s *laid) bool { return s.r.tweaked },
		func(rt *router, tweaked, plain *laid) bool { return rt.cost(tweaked).beats(rt.cost(plain)) }},
	// A line that steps off its departure trunk leads its arrival bus's
	// drop (lay's early), kept where weigh reads it better.
	{actEarly, true,
		func(_ *router, s *laid) bool { return s.r.joined },
		func(rt *router, joined, plain *laid) bool { return rt.weigh(joined).beats(rt.weigh(plain)) }},
}

func (p builtin) route(rt *router) *laid {
	r := p.choose(rt, rt.start, 0)
	moved, ok := rt.call(rt.start, move{Action: actStraighten})
	if !ok {
		return r
	}
	for _, held := range [][][4]int{nil, tracksOf(r.r.tracks)} {
		st := moved
		if held != nil {
			st, _ = rt.call(moved, move{Action: actHold, Tracks: held})
		}
		if s := p.choose(rt, st, 0); rt.weigh(s).beats(rt.weigh(r)) {
			return s
		}
	}
	return r
}

// choose makes choices[k:] from st.
func (p builtin) choose(rt *router, st state, k int) *laid {
	if k == len(choices) {
		return rt.lay(st)
	}
	c := choices[k]
	first := p.way(rt, st, k, c.first)
	if !c.open(rt, first) {
		return first
	}
	if other := p.way(rt, st, k, !c.first); !c.keeps(rt, first, other) {
		return other
	}
	return first
}

// way lays st with choices[k] turned on or off and every later choice
// made. Turned on, drops keeps on their buses every line it took off them
// that did not stand straight, laying the view again until no more do.
func (p builtin) way(rt *router, st state, k int, on bool) *laid {
	c := choices[k]
	st, _ = rt.call(st, move{Action: c.action, On: on})
	s := p.choose(rt, st, k+1)
	if c.action != actDrops || !on {
		return s
	}
	bent := map[int]bool{}
	for {
		grew := false
		for _, i := range s.r.bent {
			grew = grew || !bent[i]
			bent[i] = true
		}
		if !grew {
			return s
		}
		next, _ := rt.call(st, move{Action: actBend, Lines: keysOf(bent)})
		s = p.choose(rt, next, k+1)
	}
}

// tracksOf is tracks as hold takes them, in order.
func tracksOf(tracks map[[3]int]int) [][4]int {
	out := [][4]int{}
	for k, t := range tracks {
		out = append(out, [4]int{k[0], k[1], k[2], t})
	}
	sort.Slice(out, func(i, j int) bool { return slices.Compare(out[i][:], out[j][:]) < 0 })
	return out
}

// keysOf is the lines set in m, in order.
func keysOf(m map[int]bool) []int {
	var out []int
	for i, ok := range m {
		if ok {
			out = append(out, i)
		}
	}
	slices.Sort(out)
	return out
}
