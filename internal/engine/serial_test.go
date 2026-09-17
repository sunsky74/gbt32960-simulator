package engine

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"

	"gbt32960-simulator/internal/framing"
)

// frameSerial 取帧数据单元中的流水号(数据采集时间 6B 之后的 2B,大端):
// 表6/表20/表21/表22/表29/表30 均为「时间 6B + 流水号 2B」开头。
func frameSerial(raw []byte) int {
	return int(raw[24+6])<<8 | int(raw[24+7])
}

// TestLoginLogoutSerialSame 车辆登入/登出必须使用同一流水号(表20),
// 且同链接首个登入的流水号从 1 起(表6「从 1 开始循环累加」)。
func TestLoginLogoutSerialSame(t *testing.T) {
	var mu sync.Mutex
	var login, logout int
	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			switch raw[2] {
			case 0x01:
				mu.Lock()
				login = frameSerial(raw)
				mu.Unlock()
				_, _ = conn.Write(replyFrameRaw(0x01, 0x01, nil))
			case 0x04:
				mu.Lock()
				logout = frameSerial(raw)
				mu.Unlock()
				_, _ = conn.Write(replyFrameRaw(0x04, 0x01, nil))
				return
			}
		}
	})

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		LoginTimeout: 2 * time.Second, LoginRetries: 1,
	}, nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	c.Disconnect()

	mu.Lock()
	defer mu.Unlock()
	if login != 1 {
		t.Fatalf("首个登入流水号 = %d, want 1", login)
	}
	if logout != login {
		t.Fatalf("登出流水号 = %d, want 与登入一致(%d)", logout, login)
	}
}

// TestPlatformAndVehicleSerialPerVIN 平台级联同一链接存在两个身份
// (0x05 平台 VIN + 0x01 车辆 VIN):流水号按 VIN 独立从 1 递增,
// 且 0x06/0x04 分别复用各自登入的流水号(表22/表30)。
func TestPlatformAndVehicleSerialPerVIN(t *testing.T) {
	var mu sync.Mutex
	serials := map[byte]int{}
	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			switch raw[2] {
			case 0x05, 0x01, 0x06, 0x04:
				mu.Lock()
				serials[raw[2]] = frameSerial(raw)
				mu.Unlock()
				_, _ = conn.Write(replyFrameRaw(raw[2], 0x01, nil))
			}
		}
	})

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
		PlatformMode: true, PlatformVIN: "PLT00000000000001",
		PlatformUser: "user", PlatformPass: "pass",
		LoginTimeout: 2 * time.Second, LoginRetries: 1,
	}, nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	c.Disconnect()

	mu.Lock()
	defer mu.Unlock()
	if serials[0x05] != 1 || serials[0x01] != 1 {
		t.Fatalf("平台/车辆首登流水号 = %d/%d, want 1/1(按 VIN 独立递增)", serials[0x05], serials[0x01])
	}
	if serials[0x06] != serials[0x05] {
		t.Fatalf("平台登出流水号 = %d, want 与平台登入一致(%d)", serials[0x06], serials[0x05])
	}
	if serials[0x04] != serials[0x01] {
		t.Fatalf("车辆登出流水号 = %d, want 与车辆登入一致(%d)", serials[0x04], serials[0x01])
	}
}
