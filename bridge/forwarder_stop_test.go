package bridge

import (
	"context"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
)

// TestForwarderStopsOnContextCancel 验证 Forwarder 的优雅停止:
// ctx 取消后 Start 返回并关闭 done;此后到达的事件不再镜像进导出缓冲。
// 注意:本用例只在 done 关闭后才 Emit——测试环境没有真实 wails 上下文,
// 若 Start 运行期间触发批量推送,runtime.EventsEmit 会 log.Fatalf 终止进程。
// 本用例是停止语义的冒烟测试,不覆盖"取消瞬间积压批次被镜像"的路径。
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
