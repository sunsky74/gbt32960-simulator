package servermode

// 平台链路(企业平台级联)多车复用测试:0x05 平台登入后,同一条连接上
// 多个车辆登入/转发,验证 ACK 回显请求帧 VIN、会话平台标记、断开全量注销。

import (
	"net"
	"strconv"
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

const (
	pltVIN17 = "PLT00000000000009"
	vinA17   = "AAAAAAAAAAAAAAAA1"
	vinB17   = "BBBBBBBBBBBBBBBB2"
)

// sendReq 以请求帧形态(应答标志 0xFE)构造并发送一帧。
func sendReq(t *testing.T, conn net.Conn, vin string, cmd byte, body []byte) {
	t.Helper()
	raw, err := buildReply(api.V2016, vin, cmd, types.ResponseCommand, body)
	if err != nil {
		t.Fatalf("build %02X: %v", cmd, err)
	}
	if _, err := conn.Write(raw); err != nil {
		t.Fatalf("write %02X: %v", cmd, err)
	}
}

// readAck 读一帧并返回 (cmd, resp, vin)。
func readAck(t *testing.T, fr *framing.FrameReader) (byte, byte, string) {
	t.Helper()
	raw, err := fr.Next()
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	return raw[2], raw[3], string(raw[4:21])
}

func TestPlatformLinkMultiplexAckVIN(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	host, port := splitHostPort(t, srv.Status().ListenAddr)

	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)

	// 0x05 平台登入 → ACK 回显平台 VIN
	sendReq(t, conn, pltVIN17, 0x05, []byte{1, 2, 3})
	cmd, resp, vin := readAck(t, fr)
	if cmd != 0x05 || resp != byte(types.ResponseSuccess) || vin != pltVIN17 {
		t.Fatalf("platform login ack = %02X/%02X/%q", cmd, resp, vin)
	}

	// 车辆 A、B 依次登入(同一连接)
	sendReq(t, conn, vinA17, 0x01, nil)
	if cmd, resp, vin = readAck(t, fr); cmd != 0x01 || vin != vinA17 || resp != byte(types.ResponseSuccess) {
		t.Fatalf("vehicle A ack = %02X/%02X/%q", cmd, resp, vin)
	}
	sendReq(t, conn, vinB17, 0x01, nil)
	if cmd, resp, vin = readAck(t, fr); cmd != 0x01 || vin != vinB17 || resp != byte(types.ResponseSuccess) {
		t.Fatalf("vehicle B ack = %02X/%02X/%q", cmd, resp, vin)
	}

	// 关键断言:B 是最后登入者,A 的转发数据 ACK 必须回显 A 的 VIN
	sendReq(t, conn, vinA17, 0x02, []byte{9, 9})
	if cmd, resp, vin = readAck(t, fr); cmd != 0x02 || vin != vinA17 || resp != byte(types.ResponseSuccess) {
		t.Fatalf("data ack = %02X/%02X/%q, want 02/01/%q", cmd, resp, vin, vinA17)
	}

	// 会话:3 个,平台会话带 Platform 标记;平台链路上的帧事件同样带标记
	waitFor(t, 2*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		online := 0
		platform := false
		for _, s := range c.sessions {
			if s.Online {
				online++
				if s.Platform {
					platform = true
				}
			}
		}
		return online == 3 && platform
	})
	var frameMarked bool
	c.mu.Lock()
	for _, f := range c.frames {
		if f.Platform && f.VIN == pltVIN17 && f.Cmd == "0x05" {
			frameMarked = true
		}
	}
	c.mu.Unlock()
	if !frameMarked {
		t.Fatal("platform frame event not marked Platform")
	}

	// 断开:平台 + A + B 三个会话全部 offline(旧实现只下线最后一个 VIN)
	_ = conn.Close()
	waitFor(t, 2*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		offline := 0
		for _, s := range c.sessions {
			if !s.Online {
				offline++
			}
		}
		return offline >= 3 // 3 online + 3 offline 事件
	})
}
