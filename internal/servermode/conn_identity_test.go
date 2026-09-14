package servermode

// conn 身份字段(vin/vins/authed/platform)并发回归测试:
//   T-C1 平台链路多 VIN 定向下发——WriteFrameVIN 必须命中任意曾登入的 VIN,
//        而非仅最后登入者(conn.go 旧实现只比较 c.vin)。
//   T-C2 -race 回归——本连接 goroutine 写入身份字段的同时,bridge goroutine
//        (此处由测试 goroutine 模拟)经 WriteFrameVIN/writeFrame 读取。

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

// readFrameHeader 带 2s 读超时读一帧,返回 (cmd, resp, vin 帧头)。
func readFrameHeader(t *testing.T, conn net.Conn, fr *framing.FrameReader) (byte, byte, string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	raw, err := fr.Next()
	if err != nil {
		t.Fatalf("读取下发帧: %v", err)
	}
	if len(raw) < 21 {
		t.Fatalf("下发帧长度 %d,不足帧头 21 字节", len(raw))
	}
	return raw[2], raw[3], string(raw[4:21])
}

// TestTC1MultiVINTargeting:同一平台链路连接上登录 0x05(P) → 0x01(A) → 0x01(B)
// 后,A 已非最后登入者(c.vin==B)。修复前 WriteFrameVIN("A") 返回
// 「车辆 A 不在线或未登入」;修复后须命中该连接并写出 VIN 头为 A 的帧。
func TestTC1MultiVINTargeting(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer conn.Close()
	fr := framing.NewFrameReader(conn)

	// Given 一条 TCP 连接:0x05 平台登入 + 车辆 A、B 依次登入,应答均成功
	sendReq(t, conn, pltVIN17, 0x05, []byte{1, 2, 3})
	if cmd, resp, vin := readAck(t, fr); cmd != 0x05 || resp != byte(types.ResponseSuccess) || vin != pltVIN17 {
		t.Fatalf("平台登入应答 = %02X/%02X/%q, want 05/01/%q", cmd, resp, vin, pltVIN17)
	}
	for _, vin := range []string{vinA17, vinB17} {
		sendReq(t, conn, vin, 0x01, nil)
		if cmd, resp, got := readAck(t, fr); cmd != 0x01 || resp != byte(types.ResponseSuccess) || got != vin {
			t.Fatalf("车辆 %s 登入应答 = %02X/%02X/%q, want 01/01/%q", vin, cmd, resp, got, vin)
		}
	}

	// Then 对 A(非最后登入者)与 B 分别下发:均须返回 nil 且写入帧头 VIN 正确
	for _, vin := range []string{vinA17, vinB17} {
		payload, err := buildReply(api.V2016, vin, 0x80, types.ResponseCommand, []byte{0xAA})
		if err != nil {
			t.Fatalf("构造 %s 下发帧: %v", vin, err)
		}
		if err := srv.WriteFrameVIN(vin, 0x80, payload, "test"); err != nil {
			t.Fatalf("WriteFrameVIN(%s) 应命中多 VIN 平台连接: %v", vin, err)
		}
		cmd, resp, got := readFrameHeader(t, conn, fr)
		if cmd != 0x80 || resp != byte(types.ResponseCommand) || got != vin {
			t.Fatalf("下发帧头 = %02X/%02X/%q, want 80/%02X/%q", cmd, resp, got, byte(types.ResponseCommand), vin)
		}
	}

	// And 不存在的 VIN 仍返回原错误文案
	const ghost = "ZZZZZZZZZZZZZZZZZ"
	payload, err := buildReply(api.V2016, ghost, 0x80, types.ResponseCommand, nil)
	if err != nil {
		t.Fatalf("构造 ghost 下发帧: %v", err)
	}
	err = srv.WriteFrameVIN(ghost, 0x80, payload, "test")
	if err == nil {
		t.Fatal("不存在的 VIN 应返回错误")
	}
	if want := "车辆 " + ghost + " 不在线或未登入"; err.Error() != want {
		t.Fatalf("错误文案 = %q, want %q", err.Error(), want)
	}
}

// TestTC2IdentityRaceRegression:一条连接持续登入新 VIN + 心跳(conn goroutine
// 写 vin/vins/authed/platform),同时另一 goroutine 循环 WriteFrameVIN(读 c.vin
// 与 writeFrame 内的 c.platform)。-race 下修复前必报数据竞争,修复后干净。
// 前置同步登入 A/B,保证请求 A/B 时命中 writeFrame 路径(旧实现读 c.platform)。
func TestTC2IdentityRaceRegression(t *testing.T) {
	h, _ := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.MaxVinsPerConn = 10000 // 允许持续登入新 VIN
	cfg.IdleEnabled = false    // 连接保持打开,测试自控生命周期
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	fr := framing.NewFrameReader(conn)

	// 前置:平台 P + 车辆 A、B 同步登入并读走应答(确定性,不依赖时序)
	sendReq(t, conn, pltVIN17, 0x05, []byte{1, 2, 3})
	if cmd, resp, vin := readAck(t, fr); cmd != 0x05 || resp != byte(types.ResponseSuccess) || vin != pltVIN17 {
		t.Fatalf("平台登入应答 = %02X/%02X/%q", cmd, resp, vin)
	}
	for _, vin := range []string{vinA17, vinB17} {
		sendReq(t, conn, vin, 0x01, nil)
		if cmd, resp, got := readAck(t, fr); cmd != 0x01 || resp != byte(types.ResponseSuccess) || got != vin {
			t.Fatalf("车辆 %s 登入应答 = %02X/%02X/%q", vin, cmd, resp, got)
		}
	}

	payload, err := buildReply(api.V2016, pltVIN17, 0x80, types.ResponseCommand, []byte{0x01})
	if err != nil {
		t.Fatalf("构造下发帧: %v", err)
	}

	// When 写入 goroutine 持续发送登入(唯一新 VIN)+ 心跳,驱动身份字段并发写
	started := make(chan struct{})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(started)
		for i := 0; i < 600; i++ {
			select {
			case <-stop:
				return
			default:
			}
			vin := fmt.Sprintf("L%016d", i)
			raw, ferr := buildReply(api.V2016, vin, 0x01, types.ResponseCommand, nil)
			if ferr != nil {
				return
			}
			if _, werr := conn.Write(raw); werr != nil {
				return
			}
			hb, ferr := buildReply(api.V2016, vin, 0x07, types.ResponseCommand, nil)
			if ferr != nil {
				return
			}
			if _, werr := conn.Write(hb); werr != nil {
				return
			}
		}
	}()

	// And 同时循环下发:A/B 必命中(writeFrame 读平台标记),持续 800 轮
	<-started
	for i := 0; i < 800; i++ {
		for _, vin := range []string{pltVIN17, vinA17, vinB17} {
			_ = srv.WriteFrameVIN(vin, 0x80, payload, "race")
		}
	}
	close(stop)
	wg.Wait()
}
