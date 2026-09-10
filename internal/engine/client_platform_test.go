package engine

import (
	"context"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	_ "github.com/sunsky74/gb32960/codec/all"
)

// platformRecorder 记录帧序的假目标平台:按 cmd 应答,捕获原始帧供断言。
type platformRecorder struct {
	ln     net.Listener
	mu     sync.Mutex
	frames [][]byte
	closed chan struct{}
}

func startPlatformRecorder(t *testing.T) *platformRecorder {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	pr := &platformRecorder{ln: ln, closed: make(chan struct{})}
	go pr.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		<-pr.closed
	})
	return pr
}

func (pr *platformRecorder) port() int { return pr.ln.Addr().(*net.TCPAddr).Port }

func (pr *platformRecorder) cmds() []byte {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	out := make([]byte, len(pr.frames))
	for i, f := range pr.frames {
		out[i] = f[2]
	}
	return out
}

func (pr *platformRecorder) frameOf(cmd byte) []byte {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	for _, f := range pr.frames {
		if f[2] == cmd {
			return f
		}
	}
	return nil
}

func (pr *platformRecorder) serve() {
	defer close(pr.closed)
	for {
		conn, err := pr.ln.Accept()
		if err != nil {
			return
		}
		go pr.handle(conn)
	}
}

func (pr *platformRecorder) reply(conn net.Conn, cmd byte, resp byte) {
	head := []byte{0x23, 0x23, cmd, resp}
	head = append(head, make([]byte, 17)...) // 应答帧 VIN 与匹配无关
	head = append(head, 0x01, 0x00, 0x00)
	bcc := byte(0)
	for _, b := range head[2:] {
		bcc ^= b
	}
	_, _ = conn.Write(append(head, bcc))
}

func (pr *platformRecorder) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)
	for {
		raw, err := fr.Next()
		if err != nil {
			return
		}
		pr.mu.Lock()
		pr.frames = append(pr.frames, raw)
		pr.mu.Unlock()
		switch raw[2] {
		case 0x06:
			pr.reply(conn, 0x06, 0x01)
			return // 平台登出后才关连接(级联:0x04 后连接仍存活)
		default:
			pr.reply(conn, raw[2], 0x01)
		}
	}
}

func platformTestOptions(port int) Options {
	return Options{
		Host: "127.0.0.1", Port: port, Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		PlatformMode:      true,
		PlatformVIN:       "PLT00000000000001",
		PlatformUser:      "entuser",
		PlatformPass:      "entpass",
		HeartbeatInterval: 10 * time.Second,
		LoginTimeout:      2 * time.Second,
		LoginRetries:      2,
	}
}

func TestPlatformModeFrameOrder(t *testing.T) {
	pr := startPlatformRecorder(t)
	client := NewClient(platformTestOptions(pr.port()), NewBus())

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if s := client.State(); s != StateOnline {
		t.Fatalf("state = %s", s)
	}

	// 首帧必须是 0x05 平台登入,其次 0x01 车辆登入
	cmds := pr.cmds()
	if len(cmds) < 2 || cmds[0] != 0x05 || cmds[1] != 0x01 {
		t.Fatalf("frame order = % x, want 05 01 ...", cmds)
	}

	// 0x05 帧:帧头 VIN = 平台标识;体含账号(12B 空格填充)
	f := pr.frameOf(0x05)
	if string(f[4:21]) != "PLT00000000000001" {
		t.Errorf("0x05 frame VIN = %q", string(f[4:21]))
	}
	bodyHex := hex.EncodeToString(f[24 : len(f)-1])
	if !strings.Contains(bodyHex, hex.EncodeToString([]byte("entuser"))) {
		t.Errorf("0x05 body missing username: %s", bodyHex)
	}

	// 0x01 帧:帧头 VIN = 车辆 VIN(数据转发保持车辆身份)
	f = pr.frameOf(0x01)
	if string(f[4:21]) != "LSV00000000000001" {
		t.Errorf("0x01 frame VIN = %q", string(f[4:21]))
	}

	// 断开:0x04 车辆登出 → 0x06 平台登出
	client.Disconnect()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cmds = pr.cmds()
		if len(cmds) >= 4 && cmds[len(cmds)-2] == 0x04 && cmds[len(cmds)-1] == 0x06 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cmds = pr.cmds()
	if len(cmds) < 4 || cmds[len(cmds)-2] != 0x04 || cmds[len(cmds)-1] != 0x06 {
		t.Fatalf("disconnect order = % x, want ... 04 06", cmds)
	}
	if f := pr.frameOf(0x06); string(f[4:21]) != "PLT00000000000001" {
		t.Errorf("0x06 frame VIN = %q", string(f[4:21]))
	}
}

func TestPlatformLoginRejected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		fr := framing.NewFrameReader(conn)
		raw, err := fr.Next()
		if err != nil {
			return
		}
		// 应答 0x05 = 0x02 错误
		head := []byte{0x23, 0x23, raw[2], 0x02}
		head = append(head, make([]byte, 17)...)
		head = append(head, 0x01, 0x00, 0x00)
		bcc := byte(0)
		for _, b := range head[2:] {
			bcc ^= b
		}
		_, _ = conn.Write(append(head, bcc))
	}()

	opts := platformTestOptions(ln.Addr().(*net.TCPAddr).Port)
	client := NewClient(opts, NewBus())
	err = client.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "平台拒绝登入") {
		t.Fatalf("err = %v, want 平台拒绝登入", err)
	}
	if s := client.State(); s != StateIdle {
		t.Fatalf("state after reject = %s", s)
	}
}

func TestPlatformModeHeartbeatUsesPlatformVIN(t *testing.T) {
	pr := startPlatformRecorder(t)
	opts := platformTestOptions(pr.port())
	opts.HeartbeatInterval = 100 * time.Millisecond
	client := NewClient(opts, NewBus())
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var hb []byte
	for time.Now().Before(deadline) {
		if hb = pr.frameOf(0x07); hb != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if hb == nil {
		t.Fatal("heartbeat frame not received")
	}
	if string(hb[4:21]) != "PLT00000000000001" {
		t.Errorf("heartbeat VIN = %q, want platform VIN", string(hb[4:21]))
	}
	client.Disconnect()
}
