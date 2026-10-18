package heartbeat

type dedupe struct {
	seen map[string]struct{}
}

func newDedupe() *dedupe { return &dedupe{seen: map[string]struct{}{}} }

func (d *dedupe) firstSeen(id string) bool {
	if _, ok := d.seen[id]; ok {
		return false
	}
	d.seen[id] = struct{}{}
	return true
}
