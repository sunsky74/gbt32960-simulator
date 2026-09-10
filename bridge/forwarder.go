package bridge

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/engine"
)

// Forwarder 订阅共享事件总线,按 100ms 窗口批量推送给前端,
// 同时把事件镜像进环形缓冲供导出使用。
type Forwarder struct {
	rt *Runtime

	bufMu  sync.Mutex
	buffer []engine.Event // 环形缓冲,超出容量丢弃最旧
	cap    int            // 导出缓冲容量(SetCap 运行中可调)
}

const defaultExportBufferCap = 50000

// NewForwarder 创建转发器。
func NewForwarder(rt *Runtime) *Forwarder {
	return &Forwarder{rt: rt, buffer: make([]engine.Event, 0, 1024), cap: defaultExportBufferCap}
}

// mirror 把事件写入导出缓冲(环形,超出容量丢最旧)。
func (f *Forwarder) mirror(e engine.Event) {
	f.bufMu.Lock()
	defer f.bufMu.Unlock()
	if len(f.buffer) >= f.cap {
		f.buffer = f.buffer[len(f.buffer)-f.cap+1:]
	}
	f.buffer = append(f.buffer, e)
}

// SetCap 运行中调整导出缓冲容量,超出的旧事件立即裁剪,仅保留最近 n 条。
func (f *Forwarder) SetCap(n int) {
	if n <= 0 {
		return // 容量非法忽略(调用方 SettingsService 已做范围校验)
	}
	f.bufMu.Lock()
	defer f.bufMu.Unlock()
	f.cap = n
	if len(f.buffer) > n {
		f.buffer = f.buffer[len(f.buffer)-n:]
	}
}

// Snapshot 返回过滤后的副本。kinds 为空返回全部。
func (f *Forwarder) Snapshot(kinds []string) []engine.Event {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	f.bufMu.Lock()
	defer f.bufMu.Unlock()
	out := make([]engine.Event, 0, len(f.buffer))
	for _, e := range f.buffer {
		if len(want) == 0 || want[string(e.Kind)] {
			out = append(out, e)
		}
	}
	return out
}

// Clear 清空导出缓冲。
func (f *Forwarder) Clear() {
	f.bufMu.Lock()
	defer f.bufMu.Unlock()
	f.buffer = f.buffer[:0]
}

// Start 阻塞转发直到 ctx 取消。应在 app startup 的 goroutine 中启动。
func (f *Forwarder) Start(ctx context.Context) {
	events, stop := f.rt.Bus().Subscribe(512)
	defer stop()

	for {
		batch := make([]engine.Event, 0, 64)
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			batch = append(batch, e)
		}

		timer := time.NewTimer(100 * time.Millisecond)
	drain:
		for len(batch) < 128 {
			select {
			case e, ok := <-events:
				if !ok {
					break drain
				}
				batch = append(batch, e)
			case <-timer.C:
				break drain
			case <-ctx.Done():
				timer.Stop()
				if len(batch) > 0 {
					runtime.EventsEmit(ctx, "console:events", batch)
				}
				return
			}
		}
		timer.Stop()
		for _, e := range batch {
			f.mirror(e)
		}
		runtime.EventsEmit(ctx, "console:events", batch)
	}
}
