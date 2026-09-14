package bridge

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/parser"
)

// DecodeEvent 为 tx/rx 帧事件补上中文键保序解析树;其余事件原样返回。
// 解析失败时保持 Decoded 为空,不记日志不 panic——控制台展示容错优先。
func DecodeEvent(e engine.Event) engine.Event {
	if (e.Kind != engine.EventTx && e.Kind != engine.EventRx) || e.Hex == "" {
		return e
	}
	res, err := parser.Parse(e.Hex)
	if err != nil {
		return e
	}
	e.Decoded = res.Tree
	return e
}

// Forwarder 订阅共享事件总线,按 100ms 窗口批量推送给前端,
// 同时把事件镜像进环形缓冲供导出使用。
type Forwarder struct {
	rt *Runtime

	bufMu  sync.Mutex
	buffer []engine.Event // 环形缓冲,超出容量丢弃最旧
	cap    int            // 导出缓冲容量(SetCap 运行中可调)

	// emit 前端推送函数,默认 runtime.EventsEmit;测试可注入以确定性验证取消路径。
	emit func(ctx context.Context, eventName string, optionalData ...any)
	// batchWindow 批窗口时长(默认 100ms;测试可注入拉长以避免与取消竞速)。
	batchWindow time.Duration

	// done 在 Start 完全停止(转发循环退出 + 总线退订)后关闭,
	// 供调用方确认停止完成(当前由测试消费;shutdown 可选择等待)。
	done     chan struct{}
	doneOnce sync.Once
}

const defaultExportBufferCap = 50000

// NewForwarder 创建转发器。
func NewForwarder(rt *Runtime) *Forwarder {
	return &Forwarder{
		rt:          rt,
		buffer:      make([]engine.Event, 0, 1024),
		cap:         defaultExportBufferCap,
		emit:        runtime.EventsEmit,
		batchWindow: 100 * time.Millisecond,
		done:        make(chan struct{}),
	}
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
// 返回前关闭 done,供调用方确认已停止。
func (f *Forwarder) Start(ctx context.Context) {
	events, stop := f.rt.Bus().Subscribe(512)
	// defer 后进先出:先注册 done 关闭、后退订,执行时退订在前、done 在后——
	// done 关闭即代表转发循环已退出且总线订阅已解除。
	defer f.doneOnce.Do(func() { close(f.done) })
	defer stop()

	for {
		batch := make([]engine.Event, 0, 64)
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			batch = append(batch, DecodeEvent(e))
		}

		timer := time.NewTimer(f.batchWindow)
	drain:
		for len(batch) < 128 {
			select {
			case e, ok := <-events:
				if !ok {
					break drain
				}
				batch = append(batch, DecodeEvent(e))
			case <-timer.C:
				break drain
			case <-ctx.Done():
				timer.Stop()
				// 关闭中不再向前端推送,但已积压的批量仍镜像进导出缓冲,
				// 供退出前导出(前端随后即关闭,emit 已无意义)。
				for _, e := range batch {
					f.mirror(e)
				}
				return
			}
		}
		timer.Stop()
		for _, e := range batch {
			f.mirror(e)
		}
		// 取消瞬间(定时器先于 ctx.Done 就绪)只镜像不推送:shutdown 时前端正在关闭。
		if ctx.Err() != nil {
			return
		}
		f.emit(ctx, "console:events", batch)
	}
}
