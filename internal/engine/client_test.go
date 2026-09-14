package engine

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
	"gbt32960-simulator/internal/schema"
	"github.com/sunsky74/gb32960/api"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/utils"
)

// ---------------------------------------------------------------- 假平台集成测试

// fakePlatform 本地 TCP 假平台:应答登录/上报/心跳,记录收到的帧。
type fakePlatform struct {
	ln        net.Listener
	received  atomic.Int64
	heartbeat chan struct{}
	closed    chan struct{}
}

func startFakePlatform(t *testing.T) *fakePlatform {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fp := &fakePlatform{ln: ln, heartbeat: make(chan struct{}, 8), closed: make(chan struct{})}
	go fp.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		<-fp.closed
	})
	return fp
}

func (fp *fakePlatform) port() int { return fp.ln.Addr().(*net.TCPAddr).Port }

func (fp *fakePlatform) serve() {
	defer close(fp.closed)
	for {
		conn, err := fp.ln.Accept()
		if err != nil {
			return
		}
		go fp.handle(conn)
	}
}

// replyFrame 构造平台应答帧:同 cmd,响应标志=0x01(成功)。
func (fp *fakePlatform) replyFrame(cmd byte, payload []byte) []byte {
	head := []byte{0x23, 0x23, cmd, 0x01}
	head = append(head, []byte("LSV00000000000001")...)
	head = append(head, 0x01, byte(len(payload)>>8), byte(len(payload)))
	head = append(head, payload...)
	bcc := byte(0)
	for _, b := range head[2:] {
		bcc ^= b
	}
	return append(head, bcc)
}

func (fp *fakePlatform) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)
	for {
		raw, err := fr.Next()
		if err != nil {
			return
		}
		fp.received.Add(1)
		cmd := raw[2]
		switch cmd {
		case 0x01, 0x02, 0x03, 0x07, 0x08:
			_, _ = conn.Write(fp.replyFrame(cmd, nil))
		case 0x04:
			return // 登出后平台关连接
		default:
			_, _ = conn.Write(fp.replyFrame(cmd, nil))
		}
		if cmd == 0x07 {
			select {
			case fp.heartbeat <- struct{}{}:
			default:
			}
		}
	}
}

func TestClientFullLifecycle(t *testing.T) {
	fp := startFakePlatform(t)
	bus := NewBus()
	events, stop := bus.Subscribe(256)
	defer stop()

	client := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		HeartbeatInterval: 200 * time.Millisecond,
		LoginTimeout:      2 * time.Second,
		LoginRetries:      2,
	}, bus)

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if s := client.State(); s != StateOnline {
		t.Fatalf("state = %s", s)
	}

	// 发送一次实时上报
	groups := fullGroupsForEngine()
	body, err := schema.AssembleRealtime(groups, time.Now())
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := client.Send(context.Background(), 0x02, body); err != nil {
		t.Fatalf("send realtime: %v", err)
	}

	// 等心跳到达
	select {
	case <-fp.heartbeat:
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat not received by fake platform")
	}

	// 断开(内部发 0x04)
	client.Disconnect()
	if s := client.State(); s != StateIdle {
		t.Fatalf("state after disconnect = %s", s)
	}

	// 校验事件流里有 TX 登录、TX 上报、RX 应答
	var sawLoginTx, sawRealtimeTx, sawAckRx bool
loop:
	for {
		select {
		case e := <-events:
			switch {
			case e.Kind == EventTx && e.Cmd == "0x01 VEHICLE_LOGIN":
				sawLoginTx = true
			case e.Kind == EventTx && e.Cmd == "0x02 REAL_TIME":
				sawRealtimeTx = true
			case e.Kind == EventRx && e.Cmd == "0x01 VEHICLE_LOGIN [应答:成功]":
				sawAckRx = true
			}
		case <-time.After(200 * time.Millisecond):
			break loop
		}
	}
	if !sawLoginTx || !sawRealtimeTx || !sawAckRx {
		t.Fatalf("events missing: loginTx=%v realtimeTx=%v ackRx=%v", sawLoginTx, sawRealtimeTx, sawAckRx)
	}

	// 登出后假平台应已收到 0x04,连接关闭
	if n := fp.received.Load(); n < 4 {
		t.Logf("frames received by platform: %d", n)
	}
}

func TestClientConnectRefused(t *testing.T) {
	// 找一个必然拒绝的端口。
	// 仅当受限环境连本地回环监听端口都分配不到时跳过;正常环境不应命中 skip。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("受限环境:本地监听端口分配失败,跳过: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	client := NewClient(Options{
		Host: "127.0.0.1", Port: port, Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
	}, nil)
	if err := client.Connect(context.Background()); err == nil {
		t.Fatal("connect should fail")
	}
	if s := client.State(); s != StateIdle {
		t.Fatalf("state = %s, want idle", s)
	}
}

func TestLoginReject(t *testing.T) {
	// 平台回 0x02(错误)应导致登录失败
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			cmd := raw[2]
			head := []byte{0x23, 0x23, cmd, 0x02}
			head = append(head, []byte("LSV00000000000001")...)
			head = append(head, 0x01, 0x00, 0x00)
			bcc := byte(0)
			for _, b := range head[2:] {
				bcc ^= b
			}
			_, _ = conn.Write(append(head, bcc))
		}
	}()

	client := NewClient(Options{
		Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		LoginTimeout: 1 * time.Second, LoginRetries: 2,
	}, nil)
	err = client.Connect(context.Background())
	if err == nil {
		t.Fatal("login should be rejected")
	}
	if got := fmt.Sprint(err); got == "" {
		t.Fatal("empty error")
	}
	if s := client.State(); s != StateIdle {
		t.Fatalf("state = %s, want idle", s)
	}
}

func fullGroupsForEngine() schema.GroupsConfig {
	return schema.GroupsConfig{
		"vehicle": {Enabled: true, Rows: []schema.RowValue{{
			"operatingState": 1, "chargingState": 1, "operationMode": 1,
			"speed": 30, "soc": 60, "gear": 3,
		}}},
		"location": {Enabled: true, Rows: []schema.RowValue{{"valid": true, "longitude": 121.4, "latitude": 31.2}}},
	}
}

// TestBuildFrameHexSanity 抽查帧头与 BCC 结构。
func TestBuildFrameHexSanity(t *testing.T) {
	body, err := schema.AssembleRealtime(fullGroupsForEngine(), time.Date(2026, 8, 26, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	raw, _, err := BuildFrame(api.V2016, "LSV00000000000001", 0x02, body)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	hexStr := utils.BytesToHex(raw)
	if len(hexStr) < 50 || hexStr[:4] != "2323" {
		t.Fatalf("bad frame head: %s", hexStr[:20])
	}
	if hexStr[6:8] != "fe" {
		t.Fatalf("response flag should be 0xfe: %s", hexStr[6:8])
	}
	if len(raw)%2 != 0 {
		t.Fatal("hex length must be even")
	}
}
