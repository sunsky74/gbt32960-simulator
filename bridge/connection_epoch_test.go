package bridge

// connection_epoch_test.go:Connect 窗口内落地的 Disconnect 必须取消本次连接。
//
// 缺陷背景:Connect 先对旧客户端 Disconnect(旧平台不应答 0x04 时最长阻塞
// ~500ms),之后才 TLS 构建/重建客户端/拨号。Disconnect RPC 若恰好落在该窗口,
// 它作用于「目标将已被替换」的旧客户端(空操作),新客户端随后照常建链——
// 用户的断开意图丢失,UI 回到 online。
//
// 修复:ConnectionService.disconnectEpoch 断开纪元。Disconnect 先将纪元 +1;
// Connect 在拨号前与登录成功后各校验一次,发现纪元前进即取消(后者覆盖
// 「校验 1 与建链完成」之间的残余微窗口)。
//
// silentLogoutPlatform 手写假平台(与 connection_concurrency_test.go 的
// delayedPlatform 同构):除 0x04 外全部立即成功应答;0x04 只记录不回应、
// 不关连接——把旧客户端 Disconnect 的登出等待拉满 500ms,使「窗口内
// Disconnect」可确定性命中。回复帧复用同文件手工组帧的 connAckFrame。

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
)

// silentLogoutPlatform 假平台:除 0x04 外全部立即成功应答;0x04 永不应答。
type silentLogoutPlatform struct {
	ln         net.Listener
	accepted   atomic.Int64 // 接受的 TCP 连接数
	logoutRecv atomic.Int64 // 收到的 0x04 帧数(窗口同步点)
	done       chan struct{}
}

func startSilentLogoutPlatform(t *testing.T) *silentLogoutPlatform {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口: %v", err)
	}
	p := &silentLogoutPlatform{ln: ln, done: make(chan struct{})}
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

func (p *silentLogoutPlatform) port() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *silentLogoutPlatform) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)
	for {
		raw, err := fr.Next()
		if err != nil {
			return
		}
		cmd := raw[2]
		if cmd == 0x04 {
			p.logoutRecv.Add(1)
			continue // 永不应答:客户端自行等待 500ms 超时后关闭
		}
		if _, err := conn.Write(connAckFrame(cmd)); err != nil {
			return
		}
	}
}

func TestDisconnectDuringConnectWindow(t *testing.T) {
	tempHome(t) // Connect → SaveConfig 会持久化,隔离到临时目录
	platform := startSilentLogoutPlatform(t)
	rt := NewRuntime()
	cs := NewConnectionService(rt)
	t.Cleanup(func() { _ = cs.Disconnect() })

	cfg := *DefaultConnectionConfig()
	cfg.Name = "epoch"
	cfg.Host, cfg.Port = "127.0.0.1", platform.port()

	// 1) 首个连接上线
	if err := cs.Connect(cfg); err != nil {
		t.Fatalf("首次 Connect: %v", err)
	}
	if got := cs.State(); got != "online" {
		t.Fatalf("首次 Connect 后 State = %q, want online", got)
	}

	// 2) 后台发起第二次 Connect;因本平台不应答 0x04,它会阻塞在旧客户端的
	//    登出等待(最长 ~500ms),窗口即由此拉开。
	errCh := make(chan error, 1)
	go func() { errCh <- cs.Connect(cfg) }()

	// 以平台收到 0x04 为同步点:证明 Connect#2 已进入窗口(而非调度抖动),
	// 且此刻距其登出等待超时尚有近 500ms。
	deadline := time.Now().Add(2 * time.Second)
	for platform.logoutRecv.Load() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("Connect#2 未在 2s 内发出旧客户端的登出报文(窗口未打开)")
		}
		time.Sleep(2 * time.Millisecond)
	}

	// 3) Disconnect 落在窗口内(自身可能阻塞至其登出等待结束,可接受)
	if err := cs.Disconnect(); err != nil {
		t.Fatalf("窗口内 Disconnect: %v", err)
	}

	// 4) Connect#2 必须被取消:不拨号、不回到 online
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("窗口内落地的 Disconnect 应取消 Connect#2, got nil(缺陷重现)")
		}
		if !strings.Contains(err.Error(), "连接已取消") {
			t.Fatalf("Connect#2 错误 = %q, want 含「连接已取消」", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Connect#2 未在 3s 内返回")
	}
	if got := platform.accepted.Load(); got != 1 {
		t.Fatalf("被取消的 Connect#2 不应拨号,平台 accept = %d, want 1", got)
	}
	if got := cs.State(); got == "online" {
		t.Fatal("窗口内 Disconnect 后不应回到 online")
	}
}
