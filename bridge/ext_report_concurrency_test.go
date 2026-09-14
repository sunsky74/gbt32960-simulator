package bridge

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestStartExtReportConcurrentNoOrphanTickers 回归 ⑧:并发 startExtReport 不得
// 因 stop(close)→register(写入)两步竞态覆盖 stop channel 而泄漏孤儿 ticker。
// 每个 key 任一时刻最多只允许一个存活 ticker;stop 后必须全部退出。
func TestStartExtReportConcurrentNoOrphanTickers(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)

	time.Sleep(50 * time.Millisecond) // 等此前测试遗留的 goroutine 落地
	base := runtime.NumGoroutine()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ms.startExtReport("extData09", time.Second)
		}()
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond) // 给被 close 的 ticker 退出时间

	if n := runtime.NumGoroutine(); n > base+4 {
		t.Fatalf("并发 startExtReport 泄漏孤儿 ticker: goroutines=%d base=%d (上限 base+4)", n, base)
	}

	// 幸存 ticker(恰好 1 个)也必须能被显式停止。
	ms.stopExtReport("extData09")
	deadline := time.Now().Add(2 * time.Second)
	for {
		n := runtime.NumGoroutine()
		if n <= base+1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stopExtReport 后仍有 ticker 未退出: goroutines=%d base=%d", n, base)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
