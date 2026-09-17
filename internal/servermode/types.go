// Package servermode 实现 GB/T 32960 平台侧接收服务(被动接收器):
// 接收车端连接、自动应答、事件推送。与 internal/engine(客户端引擎)零耦合。
package servermode

import (
	"time"

	"gbt32960-simulator/internal/signature"
)

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
	// Platform:该会话由 0x05 平台登入建立(企业平台链路),区别于车辆直连。
	Platform bool `json:"platform,omitempty"`
}

type FrameEvent struct {
	Time    time.Time `json:"time"`
	VIN     string    `json:"vin"`
	Cmd     string    `json:"cmd"`
	Hex     string    `json:"hex"`
	Summary string    `json:"summary"`
	Kind    FrameKind `json:"kind"`
	// Unauthed: 2016 帧来自未登入连接且非登入命令本身(0x01/0x05 是合法的
	// 鉴权流程,不标)。前端据此在控制台加「未登入」标记。
	Unauthed bool `json:"unauthed,omitempty"`
	// Dir: rx 收到 / tx 服务端应答(缺省 rx,向后兼容)。
	Dir string `json:"dir,omitempty"`
	// Platform:帧来自平台链路连接(0x05 登入建立),前端据此标注链路类型。
	Platform bool `json:"platform,omitempty"`
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
	// Platform:平台链路会话(0x05 登入建立)。
	Platform bool `json:"platform,omitempty"`
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
	// MaxVinsPerConn 单连接可注册的车辆 VIN 数上限(平台链路的 0x05 平台标识不计数)。
	MaxVinsPerConn int
	// LogLines 导出环形缓冲保留行数。
	LogLines    int
	IdleEnabled bool
	IdleTimeout time.Duration
	// SignatureVerifier 2025 车端签名(表8)的外部验证器;nil = 只解码不校验。
	SignatureVerifier signature.Verifier
}

func DefaultConfig(addr string) Config {
	return Config{
		Addr: addr, MaxConns: 64, MaxFrameBytes: 8192,
		MaxVinsPerConn: 128, LogLines: 500,
		IdleEnabled: true, IdleTimeout: 60 * time.Second,
	}
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
