package servermode

import (
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/types"
)

// connVinsLen 返回唯一活动连接已注册的 VIN 数(仅单连接场景使用)。
// 锁序与 WriteFrameVIN 一致:Server.mu → conn.idMu。
func connVinsLen(t *testing.T, srv *Server) int {
	t.Helper()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.conns) != 1 {
		t.Fatalf("活动连接数 = %d, want 1", len(srv.conns))
	}
	for c := range srv.conns {
		c.idMu.Lock()
		n := len(c.vins)
		c.idMu.Unlock()
		return n
	}
	return -1
}

func TestLoginGuardDirectConnRejectsRelogin(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer conn.Close()
	fr := framing.NewFrameReader(conn)

	// Given 车辆直连连接,When 首次 0x01 登入,Then 成功应答
	sendReq(t, conn, vin17, 0x01, nil)
	if cmd, resp, vin := readAck(t, fr); cmd != 0x01 || resp != byte(types.ResponseSuccess) || vin != vin17 {
		t.Fatalf("首次登入应答 = %02X/%02X/%q", cmd, resp, vin)
	}

	// When 同连接换 VIN 再次 0x01 登入,Then 失败应答 + 重复登入告警,原状态不变
	sendReq(t, conn, vinA17, 0x01, nil)
	if cmd, resp, vin := readAck(t, fr); cmd != 0x01 || resp != byte(types.ResponseFailed) || vin != vinA17 {
		t.Fatalf("重复登入应答 = %02X/%02X/%q, want 01/02/%q", cmd, resp, vin, vinA17)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if w.Note == "连接已登入,拒绝重复登入: "+vinA17 {
				return true
			}
		}
		return false
	})
	if snap := srv.Sessions(); len(snap) != 1 || snap[0].VIN != vin17 {
		t.Fatalf("会话快照 = %+v, want 仅 %s", snap, vin17)
	}
	if n := connVinsLen(t, srv); n != 1 {
		t.Fatalf("连接 vins 长度 = %d, want 1", n)
	}
}

func TestLoginGuardPlatformVinsLimit(t *testing.T) {
	h, c := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.MaxVinsPerConn = 2
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	fr := framing.NewFrameReader(conn)

	// Given 上限 2 的平台链路,When 0x05 + 车辆 A + 车辆 B 依次登入,Then 全部成功
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

	// When 车辆 C 超限登入,Then 失败应答 + 指定告警,C 不注册
	const vinC17 = "CCCCCCCCCCCCCCCC3"
	sendReq(t, conn, vinC17, 0x01, nil)
	if cmd, resp, vin := readAck(t, fr); cmd != 0x01 || resp != byte(types.ResponseFailed) || vin != vinC17 {
		t.Fatalf("超限登入应答 = %02X/%02X/%q, want 01/02/%q", cmd, resp, vin, vinC17)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if w.Note == "平台链路 VIN 数超限(上限 2),拒绝登入: "+vinC17 {
				return true
			}
		}
		return false
	})
	if snap := srv.Sessions(); len(snap) != 3 {
		t.Fatalf("会话快照 = %+v, want 平台标识+A+B 共 3 个", snap)
	}
	if n := connVinsLen(t, srv); n != 3 {
		t.Fatalf("连接 vins 长度 = %d, want 3", n)
	}
}
