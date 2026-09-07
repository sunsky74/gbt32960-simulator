package servermode

// 扩展命令注册表端到端:注册规则后请求帧获得回显应答、应答帧不回帧(防回环)、
// 下发通道产生 TX 遥测。协议语义全部由注册规则注入,引擎不感知具体命令。

import (
	"bytes"
	"net"
	"testing"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

func readFrame(t *testing.T, c net.Conn) []byte {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	fr := framing.NewFrameReader(c)
	raw, err := fr.Next()
	if err != nil {
		t.Fatalf("读帧: %v", err)
	}
	return raw
}

func TestExtCmdEchoReplyFlow(t *testing.T) {
	ResetExtCmds()
	t.Cleanup(ResetExtCmds)
	RegisterExtCmd(0x90, ExtCmdRule{
		Label: "私有命令 0x90", Reply: true, Echo: true, RespType: types.ResponseSuccess,
	})

	fixed := time.Now()
	hooks, c := newCollector(fixed)
	srv := startTestServer(t, func() Config { c := DefaultConfig("127.0.0.1:0"); c.IdleEnabled = false; return c }(), hooks)
	conn := dial(t, srv)
	defer conn.Close()

	// 登入
	if _, err := conn.Write(assemble(0x01, types.ResponseCommand, vin17, []byte(itICCID))); err != nil {
		t.Fatal(err)
	}
	if got := readFrame(t, conn); got[3] != types.ResponseSuccess.Code() {
		t.Fatalf("登入应答标志 = 0x%02X", got[3])
	}

	// 扩展命令请求帧(0xFE 标志 + body 回显语义) → 应回 success + 同 body
	req := assemble(0x90, types.ResponseCommand, vin17, []byte{0xA1})
	if _, err := conn.Write(req); err != nil {
		t.Fatal(err)
	}
	want := assemble(0x90, types.ResponseSuccess, vin17, []byte{0xA1})
	if got := readFrame(t, conn); !bytes.Equal(got, want) {
		t.Fatalf("扩展命令回显应答:\n got %x\nwant %x", got, want)
	}

	// 应答帧(0x01 标志)不回帧:写后短暂窗口内无数据可读
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := conn.Write(assemble(0x90, types.ResponseSuccess, vin17, []byte{0x01})); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	if n, err := conn.Read(buf); err == nil {
		t.Fatalf("应答帧不应触发回帧,却收到 %x", buf[:n])
	}
	_ = conn.SetReadDeadline(time.Time{})

	// RX 帧事件应使用注册的显示名
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, f := range c.frames {
			if f.Dir == DirRX && f.Cmd == "私有命令 0x90" {
				return true
			}
		}
		return false
	})
}

func TestExtCmdLabelOnlyNoReply(t *testing.T) {
	ResetExtCmds()
	t.Cleanup(ResetExtCmds)
	RegisterExtCmd(0x91, ExtCmdRule{Label: "仅显示"}) // 无应答规则

	d := decodeFrame(assemble(0x91, types.ResponseCommand, vin17, nil))
	if d.Kind != KindNormal {
		t.Fatalf("已注册命令帧不应判 unknown: %s", d.Kind)
	}
}

func TestWriteFrameVIN(t *testing.T) {
	ResetExtCmds()
	t.Cleanup(ResetExtCmds)
	fixed := time.Now()
	hooks, c := newCollector(fixed)
	srv := startTestServer(t, func() Config { c := DefaultConfig("127.0.0.1:0"); c.IdleEnabled = false; return c }(), hooks)
	conn := dial(t, srv)
	defer conn.Close()

	if _, err := conn.Write(assemble(0x01, types.ResponseCommand, vin17, []byte(itICCID))); err != nil {
		t.Fatal(err)
	}
	if got := readFrame(t, conn); got[3] != types.ResponseSuccess.Code() {
		t.Fatalf("登入应答标志 = 0x%02X", got[3])
	}

	// 未登入 VIN 下发应报错
	if err := srv.WriteFrameVIN("UNKNOWN0000000000X", 0x90, nil, ""); err == nil {
		t.Fatal("离线 VIN 下发应报错")
	}

	raw, err := BuildFrame(api.V2016, vin17, 0x90, types.ResponseCommand, []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.WriteFrameVIN(vin17, 0x90, raw, "平台下发"); err != nil {
		t.Fatalf("下发: %v", err)
	}
	// 车端应收到下发帧(0xFE 标志 + body 0x01)
	want := assemble(0x90, types.ResponseCommand, vin17, []byte{0x01})
	if got := readFrame(t, conn); !bytes.Equal(got, want) {
		t.Fatalf("下发帧:\n got %x\nwant %x", got, want)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, f := range c.frames {
			if f.Dir == DirTX && f.Cmd == "0x90" {
				return true
			}
		}
		return false
	})
}
