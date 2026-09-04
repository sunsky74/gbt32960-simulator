// Package servermode 实现 GB/T 32960 平台侧接收服务(被动接收器):
// 接收车端连接、自动应答、事件推送。与 internal/engine(客户端引擎)零耦合。
package servermode

import "time"

type FrameKind string

const (
	KindNormal    FrameKind = "normal"    // 普通帧
	KindUnknown   FrameKind = "unknown"   // 未知命令字
	KindEncrypted FrameKind = "encrypted" // 加密帧(不支持解密)
)

// 帧方向(TX = 服务端发出的应答)。
const (
	DirRX string = "rx"
	DirTX string = "tx"
)

// 事件结构一律带 json tag:wails EventsEmit 序列化后前端按小写字段消费。
type Status struct {
	Running    bool   `json:"running"`
	ListenAddr string `json:"listenAddr"`
}

type SessionEvent struct {
	VIN      string    `json:"vin"`
	Peer     string    `json:"peer"`
	Online   bool      `json:"online"`
	LastSeen time.Time `json:"lastSeen"`
}

type FrameEvent struct {
	Time    time.Time `json:"time"`
	VIN     string    `json:"vin"`
	Cmd     string    `json:"cmd"`
	Hex     string    `json:"hex"`
	Summary string    `json:"summary"`
	Kind    FrameKind `json:"kind"`
	// Unauthed: 2016 帧来自未登入连接且非登入命令本身(0x01 是合法的鉴权流程,
	// 不标)。前端据此在控制台加「未登入」标记。
	Unauthed bool `json:"unauthed,omitempty"`
	// Dir: rx 收到 / tx 服务端应答(缺省 rx,向后兼容)。
	Dir string `json:"dir,omitempty"`
}

type WarnEvent struct {
	Note string `json:"note"`
	Hex  string `json:"hex"`
}

// Session(Sessions 快照)同样带 tag 供前端会话表消费。
type Session struct {
	VIN      string    `json:"vin"`
	Peer     string    `json:"peer"`
	LoginAt  time.Time `json:"loginAt"`
	LastSeen time.Time `json:"lastSeen"`
	RxCount  int       `json:"rxCount"`
	TxCount  int       `json:"txCount"`
}

// Hooks 是 servermode 对外的唯一事件出口与时间源;bridge 层注入 EventsEmit,
// 测试注入采集切片与固定时钟。
type Hooks struct {
	OnStatus  func(Status)
	OnSession func(SessionEvent)
	OnFrame   func(FrameEvent)
	OnWarn    func(WarnEvent)
	Now       func() time.Time
}

type Config struct {
	Addr          string
	MaxConns      int
	MaxFrameBytes int
	IdleEnabled   bool
	IdleTimeout   time.Duration
}

func DefaultConfig(addr string) Config {
	return Config{Addr: addr, MaxConns: 64, MaxFrameBytes: 8192, IdleEnabled: true, IdleTimeout: 60 * time.Second}
}

func normalizeHooks(h Hooks) Hooks {
	if h.OnStatus == nil {
		h.OnStatus = func(Status) {}
	}
	if h.OnSession == nil {
		h.OnSession = func(SessionEvent) {}
	}
	if h.OnFrame == nil {
		h.OnFrame = func(FrameEvent) {}
	}
	if h.OnWarn == nil {
		h.OnWarn = func(WarnEvent) {}
	}
	if h.Now == nil {
		h.Now = time.Now
	}
	return h
}
