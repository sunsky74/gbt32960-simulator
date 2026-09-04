package servermode

import "sync"

// ring 环形缓冲:超出容量丢最旧。
type ring struct {
	mu   sync.Mutex
	cap  int
	vals []string
}

func newRing(capacity int) *ring { return &ring{cap: capacity} }

func (r *ring) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.vals) >= r.cap {
		r.vals = r.vals[len(r.vals)-r.cap+1:]
	}
	r.vals = append(r.vals, line)
}

func (r *ring) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.vals))
	copy(out, r.vals)
	return out
}

func (r *ring) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vals = r.vals[:0]
}
