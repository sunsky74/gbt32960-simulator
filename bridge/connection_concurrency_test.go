package bridge

// connection_concurrency_test.go:Connect 并发互斥(单飞)回归测试。
//
// 缺陷背景:Connect 无任何同步,Wails 为每次绑定 RPC 起独立 goroutine,
// 前端又存在「异步表单校验窗口」内的二次触发路径。两个并发 Connect 都会
// 读到同一个旧客户端(或 nil)并各自 replaceClient,后者覆盖前者,导致先
// 创建的客户端沦为孤儿:TCP 长连保活、事件持续污染共享 Bus、同一 VIN 上
// 可能互相重连,UI 也无法将其断开。
//
// 修复:Connect 入口用 connectMu.TryLock 拒绝进行中的第二次调用;
// 本测试用「延迟应答」假平台拉长首个 Connect 的在途窗口,确定性地复现。
//
// 假平台为手写(而非复用 internal/servermode):这里需要精确控制 0x01 的
// 应答时序以制造并发窗口,servermode 会立即应答,无法复现。回复帧按
// GB/T 32960 头部手工组帧——internal/engine 的 replyFrameRaw 仅包内可见。

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/framing"
)

const connConcurrencyVIN = "LSV00000000000001"

// delayedPlatform 本地假平台:统计 accept 次数;0x01 登录延迟 200ms 应答,
// 其余命令(0x04 登出、0x08 校时等)立即成功应答。
type delayedPlatform struct {
	ln       net.Listener
	accepted atomic.Int64
	done     chan struct{}
}

func startDelayedPlatform(t *testing.T) *delayedPlatform {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口: %v", err)
	}
	p := &delayedPlatform{ln: ln, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			p.accepted.Add(1)
			go p.handle(conn)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-p.done
	})
	return p
}

func (p *delayedPlatform) port() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *delayedPlatform) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)
	for {
		raw, err := fr.Next()
		if err != nil {
			return
		}
		cmd := raw[2]
		if cmd == 0x01 {
			time.Sleep(200 * time.Millisecond) // 制造 Connect 在途窗口
		}
		if _, err := conn.Write(connAckFrame(cmd)); err != nil {
			return
		}
		if cmd == 0x04 {
			return
		}
	}
}

// connAckFrame 手工组成功应答帧:## + cmd + 响应 0x01 + VIN(17 字节) +
// 加密 0x01 + 长度 0x0000 + BCC(从 cmd 起逐字节异或)。
func connAckFrame(cmd byte) []byte {
	head := []byte{0x23, 0x23, cmd, 0x01}
	head = append(head, []byte(connConcurrencyVIN)...)
	head = append(head, 0x01, 0x00, 0x00)
	bcc := byte(0)
	for _, b := range head[2:] {
		bcc ^= b
	}
	return append(head, bcc)
}

// TestConnConcurrentConnectRejected 首个 Connect 在途(0x01 延迟应答)时,
// 第二次 Connect 必须立即被拒绝且不影响首个连接;拒绝不泄漏锁,断开后
// 再次 Connect 必须成功。
func TestConnConcurrentConnectRejected(t *testing.T) {
	tempHome(t) // Connect → SaveConfig 会持久化,隔离到临时目录
	platform := startDelayedPlatform(t)
	cs := NewConnectionService(NewRuntime())
	t.Cleanup(func() { _ = cs.Disconnect() })

	cfg1 := *DefaultConnectionConfig()
	cfg1.Name = "c1"
	cfg1.Host, cfg1.Port = "127.0.0.1", platform.port()
	cfg2 := cfg1
	cfg2.Name = "c2"

	// 1) 首个 Connect 在后台发起,停在 0x01 延迟应答窗口内。
	//    等待拨号成功(accept 计数 ≥1)证明其已持 connectMu,避免固定 sleep
	//    在高负载 CI 上因调度抖动误判(预提交评审 F1)。
	errCh := make(chan error, 1)
	go func() { errCh <- cs.Connect(cfg1) }()
	inFlightDeadline := time.Now().Add(2 * time.Second)
	for platform.accepted.Load() < 1 {
		if time.Now().After(inFlightDeadline) {
			t.Fatal("首个 Connect 未在 2s 内建立 TCP 连接")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// 2) 第二次 Connect 应被立即拒绝(不接受新 TCP 连接、不覆盖客户端)
	start := time.Now()
	err2 := cs.Connect(cfg2)
	elapsed := time.Since(start)
	if err2 == nil || !strings.Contains(err2.Error(), "连接进行中") {
		t.Fatalf("连接在途时的第二次 Connect 应被拒绝并提示「连接进行中」, got err=%v", err2)
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("第二次 Connect 应立即返回,耗时 %v", elapsed)
	}
	// 2b) 被拒调用必须零写入:激活档案仍是首个 Connect 保存的 c1
	//     (防未来把守卫挪到 SaveConfig 之后的回归)
	if got, err := cs.GetConfig(); err != nil || got.Name != "c1" {
		t.Fatalf("被拒调用不应写入配置: name=%v err=%v, want c1", got, err)
	}

	// 3) 首个 Connect 不被第二次调用取消,正常登录成功
	if err1 := <-errCh; err1 != nil {
		t.Fatalf("首个 Connect 不应被并发的第二次调用取消, got err=%v", err1)
	}

	// 4) 平台只应收到 1 次 TCP 连接(孤儿客户端不会出现)
	if got := platform.accepted.Load(); got != 1 {
		t.Fatalf("平台应只收到 1 次 TCP 连接, got %d", got)
	}

	// 5) 当前客户端在线
	current := cs.rt.CurrentClient()
	if current == nil {
		t.Fatal("首个 Connect 成功后 CurrentClient 不应为 nil")
	}
	if got := current.State(); got != engine.StateOnline {
		t.Fatalf("CurrentClient.State() = %q, want online", got)
	}

	// 6) 拒绝不泄漏锁:断开后新建连接必须成功
	if err := cs.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if err := cs.Connect(cfg1); err != nil {
		t.Fatalf("被拒绝后锁未释放,后续 Connect 失败: %v", err)
	}
	if got := platform.accepted.Load(); got != 2 {
		t.Fatalf("重连后平台 accept 次数 = %d, want 2", got)
	}
}
