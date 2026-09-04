package servermode

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// 采集 Hooks:事件收进切片供断言。回调运行在连接 goroutine、断言运行在测试
// goroutine——必须加锁(评审 M5,-race 门禁)。
type collector struct {
	mu       sync.Mutex
	status   []Status
	sessions []SessionEvent
	frames   []FrameEvent
	warns    []WarnEvent
	now      time.Time
}

func newCollector(fixed time.Time) (Hooks, *collector) {
	c := &collector{now: fixed}
	return normalizeHooks(Hooks{
		OnStatus:  func(s Status) { c.mu.Lock(); c.status = append(c.status, s); c.mu.Unlock() },
		OnSession: func(e SessionEvent) { c.mu.Lock(); c.sessions = append(c.sessions, e); c.mu.Unlock() },
		OnFrame:   func(e FrameEvent) { c.mu.Lock(); c.frames = append(c.frames, e); c.mu.Unlock() },
		OnWarn:    func(e WarnEvent) { c.mu.Lock(); c.warns = append(c.warns, e); c.mu.Unlock() },
		Now:       func() time.Time { return c.now },
	}), c
}

// waitFor 轮询断言辅助:fn 返回 true 前最多等 timeout(fn 自行持锁)。
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("条件在 %v 内未满足", timeout)
}

func startTestServer(t *testing.T, cfg Config, hooks Hooks) *Server {
	t.Helper()
	srv := New(cfg, hooks)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

func dial(t *testing.T, srv *Server) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", srv.Status().ListenAddr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func TestDefaultAddrLoopback(t *testing.T) {
	cfg := DefaultConfig("127.0.0.1:0")
	if len(cfg.Addr) < 9 || cfg.Addr[:9] != "127.0.0.1" {
		t.Fatalf("默认地址必须环回: %s", cfg.Addr)
	}
}

func TestStartIdempotentAndPortInUse(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("重复 Start 应幂等: %v", err)
	}
	srv2 := New(DefaultConfig(srv.Status().ListenAddr), h)
	defer func() { _ = srv2.Stop() }()
	if err := srv2.Start(context.Background()); err == nil {
		t.Fatal("端口占用应报错")
	}
}

func TestFrameTooLargeDroppedConnAlive(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer conn.Close()
	// 声明 payload=9000(>8KB)的伪帧头 + 紧随一帧合法登入。
	// framing 层限长立即丢弃伪头并重同步(不阻塞等 9025 字节凑齐,评审 B4)
	hdr := append([]byte{0x23, 0x23, 0x07, 0xFE}, []byte(vin17)...)
	hdr = append(hdr, 0x00, 0x23, 0x28) // len=9000
	if _, err := conn.Write(append(hdr, loginFrame(t)...)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if w.Note == "帧超长丢弃(>8KB)" {
				return true
			}
		}
		return false
	})
	// 重同步成功:伪帧后的合法登入仍被处理(帧事件含 0x01)
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, f := range c.frames {
			if f.Cmd == "0x01" {
				return true
			}
		}
		return false
	})
}

func TestIdleEnabledCloses(t *testing.T) {
	h, c := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.IdleTimeout = 200 * time.Millisecond
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	waitFor(t, 2*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if strings.HasPrefix(w.Note, "空闲超时关闭") {
				return true
			}
		}
		return false
	})
}

func TestIdleDisabledKeeps(t *testing.T) {
	h, _ := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.IdleEnabled = false
	cfg.IdleTimeout = 100 * time.Millisecond
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	time.Sleep(400 * time.Millisecond)
	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal("空闲检测关闭时连接不应被剔除")
	}
}

func TestIdleUpdateRuntime(t *testing.T) {
	h, c := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.IdleEnabled = false // 初始关闭:连接挂 150ms 不被剔
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	time.Sleep(150 * time.Millisecond)
	// 运行中开启(200ms)→ UpdateIdle 逐连接重设 deadline,静默连接在 ~200ms 后
	// 被服务端关闭(评审 M1:靠 poke 生效,不再依赖帧到达)。断言关闭发生在
	// 客户端 2s 读超时**之前**(即确系服务端关闭,非本地超时)。
	srv.UpdateIdle(true, 200*time.Millisecond)
	start := time.Now()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	if err == nil {
		t.Fatal("运行中开启空闲检测后连接应被关闭")
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("连接由客户端读超时关闭而非服务端空闲剔除(耗时 %v)", elapsed)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if strings.HasPrefix(w.Note, "空闲超时关闭") {
				return true
			}
		}
		return false
	})
}

func TestMaxConns(t *testing.T) {
	h, _ := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.MaxConns = 2
	srv := startTestServer(t, cfg, h)
	c1, c2, c3 := dial(t, srv), dial(t, srv), dial(t, srv)
	defer c1.Close()
	defer c2.Close()
	time.Sleep(200 * time.Millisecond)
	// 第 3 个连接应被立即关闭:读到 EOF
	c3.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := c3.Read(buf); err == nil {
		t.Fatal("超限连接应被关闭")
	}
	c3.Close()
}

func TestStopDeadline(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	done := make(chan error, 1)
	go func() { done <- srv.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 超过 2s")
	}
}

func TestRingBufferExport(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	_, _ = conn.Write(loginFrame(t))
	time.Sleep(200 * time.Millisecond)
	conn.Close()
	lines := srv.ExportLines()
	if len(lines) == 0 {
		t.Fatal("导出不应为空")
	}
	// 行格式: [时间] [VIN] [命令] [hex] —— VIN 以 [VIN] 子串形式出现在行内。
	// (计划原切片断言 l[1:len(wantPrefix)+1] 恒比较 19B vs 17B 恒 false,
	// 与裁决行格式矛盾,按意图修正为包含性检查。)
	want := "[" + vin17 + "]"
	found := false
	for _, l := range lines {
		if strings.Contains(l, want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("导出行缺少 VIN: %q", lines)
	}
}

func TestUnauthedFrameMarked(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer conn.Close()

	// 登入前发心跳:帧事件必须带未登入标记(0x07 应答被 authed 门禁拦截)
	if _, err := conn.Write(heartbeatFrame(t)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, f := range c.frames {
			if f.Cmd == "0x07" && f.Unauthed {
				return true
			}
		}
		return false
	})

	// 登入成功后同连接再发心跳:不再标记
	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, s := range c.sessions {
			if s.Online {
				return true
			}
		}
		return false
	})
	if _, err := conn.Write(heartbeatFrame(t)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		count := 0
		for _, f := range c.frames {
			if f.Cmd == "0x07" {
				count++
			}
		}
		return count >= 2
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	// 按时间序断言:第 1 条心跳(登入前)带标记,其后(登入后)不带
	var hb []FrameEvent
	for _, f := range c.frames {
		if f.Cmd == "0x07" {
			hb = append(hb, f)
		}
	}
	if len(hb) < 2 {
		t.Fatalf("心跳帧不足 2 条: %d", len(hb))
	}
	if !hb[0].Unauthed {
		t.Fatal("登入前的心跳应带未登入标记")
	}
	for _, f := range hb[1:] {
		if f.Unauthed {
			t.Fatalf("登入后的心跳不应带未登入标记: %+v", f)
		}
	}
	// 登入帧本身(0x01)永不标记
	for _, f := range c.frames {
		if f.Cmd == "0x01" && f.Unauthed {
			t.Fatal("登入帧是合法鉴权流程,不应标记未登入")
		}
	}
}

func TestTxEventAndCounters(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer conn.Close()

	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal(err)
	}
	// 登入应答产生 TX 事件
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, f := range c.frames {
			if f.Dir == DirTX && f.Cmd == "0x01" && f.Summary == "应答 成功(0x01)" {
				return true
			}
		}
		return false
	})
	// 会话快照:RX=1(登入帧)、TX=1(登入应答)
	snap := srv.Sessions()
	if len(snap) != 1 || snap[0].RxCount != 1 || snap[0].TxCount != 1 {
		t.Fatalf("会话计数异常: %+v", snap)
	}
}
