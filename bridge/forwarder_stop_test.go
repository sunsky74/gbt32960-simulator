package bridge

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
)

// TestForwarderStopsOnContextCancel 验证 Forwarder 的优雅停止:
// ctx 取消后 Start 返回并关闭 done;此后到达的事件不再镜像进导出缓冲。
// 注意:本用例只在 done 关闭后才 Emit——测试环境没有真实 wails 上下文,
// 若 Start 运行期间触发批量推送,runtime.EventsEmit 会 log.Fatalf 终止进程。
// 本用例是停止语义的冒烟测试,"取消瞬间积压批次被镜像"的路径
// 由 TestForwarderMirrorsBacklogOnCancel 覆盖(注入 emit 后方可确定性断言)。
func TestForwarderStopsOnContextCancel(t *testing.T) {
	rt := NewRuntime()
	f := NewForwarder(rt)
	ctx, cancel := context.WithCancel(context.Background())
	go f.Start(ctx)

	cancel()
	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Forwarder.Start 未在 ctx 取消后 2s 内退出")
	}

	rt.Bus().Emit(engine.Event{Kind: engine.EventConn, Message: "after stop"})
	time.Sleep(150 * time.Millisecond)
	if got := len(f.Snapshot(nil)); got != 0 {
		t.Fatalf("停止后事件仍被镜像: len = %d, want 0", got)
	}
}

// TestForwarderMirrorsBacklogOnCancel 验证取消瞬间已积压的批次被镜像进导出缓冲,
// 但不再向前端推送:注入 emit 计数器与拉长的批窗口,避免与取消竞速。
func TestForwarderMirrorsBacklogOnCancel(t *testing.T) {
	rt := NewRuntime()
	f := NewForwarder(rt)
	f.batchWindow = 2 * time.Second
	var calls int32
	f.emit = func(context.Context, string, ...any) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	go f.Start(ctx)
	// Bus.Emit 只投递给已有订阅者:先等 Start 完成订阅,事件才会进入通道积压。
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 3; i++ {
		rt.Bus().Emit(engine.Event{Kind: engine.EventTx, Message: "backlog"})
	}
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Forwarder.Start 未在 ctx 取消后 2s 内退出")
	}
	if got := len(f.Snapshot(nil)); got != 3 {
		t.Fatalf("取消时积压批次未镜像: len = %d, want 3", got)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("取消后仍向前端推送: calls = %d, want 0", got)
	}
}
