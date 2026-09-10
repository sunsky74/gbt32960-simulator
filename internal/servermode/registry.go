package servermode

import (
	"sort"
	"sync"
	"time"
)

// Session 结构定义见 types.go(Task 2,带 json tag)。

// Registry VIN→会话索引;putIfAbsent 语义:后到登入被拒,原会话不受影响。
type Registry struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func NewRegistry() *Registry { return &Registry{sessions: map[string]Session{}} }

// Register 登记会话(putIfAbsent:后到登入被拒,原会话不受影响)。
// platform 标记平台链路会话(0x05 平台登入建立)。
func (r *Registry) Register(vin, peer string, now time.Time, platform bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[vin]; exists {
		return false
	}
	r.sessions[vin] = Session{VIN: vin, Peer: peer, LoginAt: now, LastSeen: now, Platform: platform}
	return true
}

func (r *Registry) Touch(vin string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[vin]; ok {
		s.LastSeen = now
		r.sessions[vin] = s
	}
}

// Count 累加会话的 RX/TX 帧计数;会话不存在时为 no-op(未登入连接的帧不计数)。
// 最后活跃时间由 Touch 维护,二者职责分离。
func (r *Registry) Count(vin string, rx, tx int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[vin]; ok {
		s.RxCount += rx
		s.TxCount += tx
		r.sessions[vin] = s
	}
}

func (r *Registry) Remove(vin string) (Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[vin]
	if ok {
		delete(r.sessions, vin)
	}
	return s, ok
}

func (r *Registry) Snapshot() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VIN < out[j].VIN })
	return out
}
