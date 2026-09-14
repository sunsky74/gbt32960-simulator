package servermode

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// TestAcceptErrorBackoff 回归 ⑪:持续 accept 错误必须带 50ms 退避重试,
// 不得在 ctx 存活期间紧循环刷 warn(log 洪泛 + CPU 空转)。
func TestAcceptErrorBackoff(t *testing.T) {
	var warns atomic.Int64
	srv := New(DefaultConfig("127.0.0.1:0"), Hooks{
		Now:    time.Now,
		OnWarn: func(WarnEvent) { warns.Add(1) },
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		srv.acceptLoop(ln, ctx)
		close(done)
	}()

	_ = ln.Close() // 此后 Accept 持续失败;ctx 保持存活
	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel 后 acceptLoop 未退出(goroutine 泄漏)")
	}

	if n := warns.Load(); n > 20 {
		t.Fatalf("accept 错误无退避:120ms 内 warn %d 次(上限 20)", n)
	}
}
