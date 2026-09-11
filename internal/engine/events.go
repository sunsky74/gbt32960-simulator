package engine

import (
	"sync"
	"time"
)

// EventKind 控制台事件类型。
type EventKind string

const (
	// EventConn 连接生命周期事件(连接/断开/TLS 握手/重连)。
	EventConn EventKind = "conn"
	// EventTx 发出的协议帧。
	EventTx EventKind = "tx"
	// EventRx 收到的协议帧。
	EventRx EventKind = "rx"
	// EventError 引擎错误。
	EventError EventKind = "error"
)

// Event 是推送给前端控制台的统一事件。Tx/Rx 帧带 hex 与解码摘要。
type Event struct {
	Time    time.Time `json:"time"`
	Kind    EventKind `json:"kind"`
	Message string    `json:"message"`           // conn/error 的人类可读文本
	Cmd     string    `json:"cmd,omitempty"`     // 如 "0x01 VEHICLE_LOGIN"
	Hex     string    `json:"hex,omitempty"`     // tx/rx 完整帧 hex
	Decoded any       `json:"decoded,omitempty"` // Decoded 由 bridge 转发层填充(tx/rx 帧的中文键解析树);引擎自身不再赋值
	Bytes   int       `json:"bytes,omitempty"`   // 帧字节数
	// Downlink 非空表示这是平台下行的 0x80/0x81/0x82/0x8A 命令,前端据此展示"应答"入口。
	Downlink *DownlinkInfo `json:"downlink,omitempty"`
}

// Bus 是引擎对外的唯一事件出口。订阅者读取 channel,满则丢弃最旧
// 订阅者(由 bridge 层做批量转发,保证引擎永不阻塞)。
type Bus struct {
	mu          sync.Mutex
	subscribers []chan Event
}

// NewBus 创建事件总线。
func NewBus() *Bus { return &Bus{} }

// Subscribe 返回一个带缓冲的事件订阅通道。cancel 后不再接收。
func (b *Bus) Subscribe(bufSize int) (<-chan Event, func()) {
	ch := make(chan Event, bufSize)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, s := range b.subscribers {
			if s == ch {
				b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

// Emit 向所有订阅者非阻塞推送事件;缓冲满时丢弃该事件(控制台允许丢帧)。
func (b *Bus) Emit(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default: // 订阅者阻塞则丢弃,引擎不能被 UI 拖死
		}
	}
}
