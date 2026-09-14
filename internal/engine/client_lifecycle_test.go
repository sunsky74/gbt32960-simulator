package engine

import (
	"context"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/types"

	"gbt32960-simulator/internal/framing"
)

// ---------------------------------------------------------------- 生命周期/并发回归假平台

// lifecyclePlatform 多连接假平台:每条连接交给 onConn 处理,统计接受
// 总数与仍打开的连接数(后者用于连接泄漏断言)。
type lifecyclePlatform struct {
	ln       net.Listener
	accepted atomic.Int64
	open     atomic.Int64
	mu       sync.Mutex
	conns    []net.Conn
	closed   chan struct{}
	onConn   func(conn net.Conn, n int64)
}

func startLifecyclePlatform(t *testing.T, onConn func(conn net.Conn, n int64)) *lifecyclePlatform {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := &lifecyclePlatform{ln: ln, closed: make(chan struct{}), onConn: onConn}
	go p.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		<-p.closed
		// 兜底关闭所有已接受连接,避免测试间互相污染
		p.mu.Lock()
		for _, conn := range p.conns {
			_ = conn.Close()
		}
		p.mu.Unlock()
	})
	return p
}

func (p *lifecyclePlatform) port() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *lifecyclePlatform) serve() {
	defer close(p.closed)
	for {
		conn, err := p.ln.Accept()
		if err != nil {
			return
		}
		n := p.accepted.Add(1)
		p.open.Add(1)
		p.mu.Lock()
		p.conns = append(p.conns, conn)
		p.mu.Unlock()
		go func() {
			defer func() {
				p.open.Add(-1)
				_ = conn.Close()
			}()
			p.onConn(conn, n)
		}()
	}
}

// ---------------------------------------------------------------- T-B1: SetAutoReport 协程泄漏

// TestTB1SetAutoReportGoroutineLeak 反复重设周期上报后,旧上报协程必须
// 随 stop 信号退出;禁用后协程数应回到基线附近(旧实现每轮泄漏 1 个)。
func TestTB1SetAutoReportGoroutineLeak(t *testing.T) {
	c := NewClient(Options{Version: api.V2016, VIN: "LSV00000000000001"}, nil)

	time.Sleep(50 * time.Millisecond) // 等待运行时/测试框架协程稳定
	baseline := runtime.NumGoroutine()

	for i := 0; i < 40; i++ {
		c.SetAutoReport(time.Hour, func() error { return nil })
	}
	c.SetAutoReport(0, nil)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+3 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("周期上报协程泄漏: baseline=%d current=%d", baseline, runtime.NumGoroutine())
}

// ---------------------------------------------------------------- T-B2: 重连后心跳去重

// TestTB2ReconnectHeartbeatDedup 自动重连成功后只允许存在一个心跳循环:
// 旧实现重连会再起一个 heartbeatLoop,两个循环同时发心跳。
func TestTB2ReconnectHeartbeatDedup(t *testing.T) {
	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			switch raw[2] {
			case 0x01:
				_, _ = conn.Write(replyFrameRaw(0x01, 0x01, nil))
				if n == 1 {
					// 首连登录成功后平台主动断开,触发客户端自动重连
					time.Sleep(30 * time.Millisecond)
					return
				}
			case 0x04:
				_, _ = conn.Write(replyFrameRaw(0x04, 0x01, nil))
				return
			}
		}
	})

	bus := NewBus()
	events, stop := bus.Subscribe(4096)
	defer stop()

	var hbCount atomic.Int64
	var counting atomic.Bool
	go func() {
		for e := range events {
			if counting.Load() && e.Kind == EventTx && strings.HasPrefix(e.Cmd, "0x07") {
				hbCount.Add(1)
			}
		}
	}()

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		HeartbeatInterval: 80 * time.Millisecond,
		LoginTimeout:      2 * time.Second,
		LoginRetries:      1,
		AutoReconnect:     true,
		ReconnectDelay:    50 * time.Millisecond,
	}, bus)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	// 等到重连完成(接受数 >= 2 且再次 Online)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fp.accepted.Load() >= 2 && c.State() == StateOnline {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n, s := fp.accepted.Load(), c.State(); n < 2 || s != StateOnline {
		t.Fatalf("自动重连未完成: accepted=%d state=%s", n, s)
	}

	time.Sleep(100 * time.Millisecond) // 让重连后的心跳循环进入稳态
	hbCount.Store(0)
	counting.Store(true)
	time.Sleep(400 * time.Millisecond)
	counting.Store(false)

	n := hbCount.Load()
	if n < 1 {
		t.Fatalf("重连后 400ms 内无心跳")
	}
	// 80ms 间隔 400ms 窗口期望约 5 条;存在两个心跳循环时约 10 条
	if n > 6 {
		t.Fatalf("重连后心跳重复发送: 400ms 内 %d 条 (want ~5, <=6)", n)
	}
}

// ---------------------------------------------------------------- T-B3: Disconnect 与 Connect 竞态

// TestTB3DisconnectDuringConnect 登录应答迟到 + 用户在登录等待期 Disconnect:
// Connect 必须返回错误,且最终回到 IDLE,不得在断开后宣告 ONLINE。
// 旧实现中登出等待(平台不应答 0x04)会拖到 500ms,迟到的 0x01 应答
// 可抢先满足 waitAck → Connect 返回 nil。
func TestTB3DisconnectDuringConnect(t *testing.T) {
	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			if raw[2] == 0x01 {
				// 登录应答延迟 120ms;其余帧不应答且保持连接,
				// 保证旧实现的登出等待拖满 500ms。
				time.AfterFunc(120*time.Millisecond, func() {
					_, _ = conn.Write(replyFrameRaw(0x01, 0x01, nil))
				})
			}
		}
	})

	const iters = 10
	bad := 0
	for i := 0; i < iters; i++ {
		c := NewClient(Options{
			Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
			VIN: "LSV00000000000001", ICCID: "89860000000000000001",
			LoginTimeout: 800 * time.Millisecond,
			LoginRetries: 1,
		}, nil)

		errCh := make(chan error, 1)
		go func() { errCh <- c.Connect(context.Background()) }()
		time.Sleep(40 * time.Millisecond)
		c.Disconnect()

		var err error
		select {
		case err = <-errCh:
		case <-time.After(3 * time.Second):
			t.Fatalf("iter %d: Connect 未返回", i)
		}
		if err == nil {
			bad++
			t.Errorf("iter %d: Disconnect 后 Connect 返回 nil,应为错误", i)
		}
		if s := c.State(); s != StateIdle {
			t.Errorf("iter %d: state = %s, want idle", i, s)
		}
	}
	if bad > 0 {
		t.Fatalf("%d/%d 次 Connect 在 Disconnect 后仍报告成功", bad, iters)
	}
}

// ---------------------------------------------------------------- T-B4: 迟到应答清理

// TestTB4StaleAckDrained Connect 前遗留的上一会话 0x01 成功应答不得
// 满足本次登录(平台不应答,登录必须超时失败)。
func TestTB4StaleAckDrained(t *testing.T) {
	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			if _, err := fr.Next(); err != nil {
				return
			}
		}
	})

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		LoginTimeout: 300 * time.Millisecond,
		LoginRetries: 1,
	}, nil)

	// 模拟上一会话迟到的登录成功应答
	c.acks <- ackMsg{cmd: 0x01, resp: types.ResponseSuccess}

	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("Connect 不应被上一会话的迟到应答满足,应返回错误")
	}
	if s := c.State(); s != StateIdle {
		t.Fatalf("state = %s, want idle", s)
	}
}

// ---------------------------------------------------------------- T-B5: 失败重连的连接泄漏

// TestTB5FailedReconnectConnCleanup 若平台持续拒绝登录(不应答),每次
// 失败的重连尝试都必须自行清理连接;Disconnect 后平台不应还有打开连接。
func TestTB5FailedReconnectConnCleanup(t *testing.T) {
	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			if n == 1 && raw[2] == 0x01 {
				_, _ = conn.Write(replyFrameRaw(0x01, 0x01, nil))
				time.Sleep(20 * time.Millisecond)
				return // 平台主动断开首连,触发客户端自动重连
			}
			// 后续连接只收不答:登录必然超时
		}
	})

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		LoginTimeout:   150 * time.Millisecond,
		LoginRetries:   1,
		AutoReconnect:  true,
		ReconnectDelay: 50 * time.Millisecond,
	}, nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// 等到至少 3 次接受(首连 + ≥2 次失败重连,保证旧实现至少遗留 1 条连接)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if fp.accepted.Load() >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := fp.accepted.Load(); n < 3 {
		t.Fatalf("失败重连尝试不足: accepted=%d", n)
	}
	time.Sleep(100 * time.Millisecond) // 让当前尝试处于明确的中间态

	c.Disconnect()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fp.open.Load() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("失败重连泄漏连接: open=%d accepted=%d", fp.open.Load(), fp.accepted.Load())
}
