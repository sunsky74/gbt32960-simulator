package engine

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/utils"

	"gbt32960-simulator/internal/framing"
)

// TestRemoteDownlinkAndAck 端到端:假平台登录后下发 0x8A 远控命令,
// 模拟器事件应携带解析后的下行信息;RespondAck 发出的第一层 ACK
// 必须被平台收到且命令字节精确为 0x8A(防区间折叠回归)。
func TestRemoteDownlinkAndAck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	downlinkReceived := make(chan []byte, 4)
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
			cmd, resp := raw[2], raw[3]
			switch {
			case cmd == 0x01 && resp == 0xFE:
				_, _ = conn.Write(replyFrameRaw(0x01, 0x01, nil))
				// 登录成功后平台主动下发远控命令(表21头 + 信息体)
				payload := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00, 0x12, 0x34, 0x01, 0x0a, 0xde, 0xad}
				_, _ = conn.Write(replyFrameRaw(0x8A, 0xFE, payload))
			case cmd == 0x8A:
				select {
				case downlinkReceived <- raw:
				default:
				}
			case cmd == 0x04:
				return
			}
		}
	}()

	bus := NewBus()
	events, stop := bus.Subscribe(256)
	defer stop()
	client := NewClient(Options{
		Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		LoginTimeout: 2 * time.Second, HeartbeatInterval: 0,
	}, bus)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect()

	// 等待下行 0x8A 事件并断言解析结果
	var dl *DownlinkInfo
	deadline := time.After(3 * time.Second)
	for dl == nil {
		select {
		case e := <-events:
			if e.Downlink != nil {
				dl = e.Downlink
			}
		case <-deadline:
			t.Fatal("downlink event not received")
		}
	}
	if dl.Cmd != 0x8A || dl.SerialNumber != 0x1234 || dl.InfoTypeFlag != 0x0a {
		t.Fatalf("downlink parse wrong: %+v", dl)
	}
	if dl.BodyHex != "dead" {
		t.Errorf("body hex = %s", dl.BodyHex)
	}

	// 第一层 ACK
	if err := client.RespondAck(0x8A, 0x01); err != nil {
		t.Fatalf("respond ack: %v", err)
	}
	select {
	case raw := <-downlinkReceived:
		if raw[2] != 0x8A || raw[3] != 0x01 {
			t.Fatalf("ack frame cmd/resp = %02X/%02X", raw[2], raw[3])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ack not received by fake platform")
	}

	// 第二层业务应答
	if err := client.RespondRemoteSecondLayer(dl.HeaderHex, dl.BodyHex); err != nil {
		t.Fatalf("second layer: %v", err)
	}
	select {
	case raw := <-downlinkReceived:
		if raw[2] != 0x8A || raw[3] != 0xFE {
			t.Fatalf("2nd frame cmd/resp = %02X/%02X", raw[2], raw[3])
		}
		payloadHex := utils.BytesToHex(raw[24 : len(raw)-1])
		if payloadHex != dl.HeaderHex+dl.BodyHex {
			t.Fatalf("2nd payload = %s", payloadHex)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("2nd layer ack not received")
	}
}

// replyFrameRaw 构造原始应答帧(测试用,不做完整模型解码)。
func replyFrameRaw(cmd byte, resp byte, payload []byte) []byte {
	head := []byte{0x23, 0x23, cmd, resp}
	head = append(head, []byte("LSV00000000000001")...)
	head = append(head, 0x01, byte(len(payload)>>8), byte(len(payload)))
	head = append(head, payload...)
	bcc := byte(0)
	for _, b := range head[2:] {
		bcc ^= b
	}
	return append(head, bcc)
}
