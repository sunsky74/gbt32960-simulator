package servermode

import (
	"bytes"
	"testing"
	"time"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

// xorBCC 测试内手写异或(不走 utils.CalcBCC,保证 golden 独立于生产路径,评审 m6):
// BCC = 帧去掉起始符 2B 与末位 BCC 后逐字节异或。
func xorBCC(b []byte) byte {
	var x byte
	for _, v := range b {
		x ^= v
	}
	return x
}

// assemble 独立拼装期望帧。加密字节 = 0x01(协议"不加密"线上值,评审 B1)。
func assemble(cmd byte, resp types.ResponseType, vin string, body []byte) []byte {
	out := []byte{0x23, 0x23, cmd, resp.Code()}
	out = append(out, []byte(vin)...)
	out = append(out, 0x01, byte(len(body)>>8), byte(len(body)))
	out = append(out, body...)
	return append(out, xorBCC(out[2:]))
}

func TestBuildReplyLoginSuccess(t *testing.T) {
	got, err := buildReply(api.V2016, vin17, 0x01, types.ResponseSuccess, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x01, types.ResponseSuccess, vin17, nil); !bytes.Equal(got, want) {
		t.Fatalf("login success reply:\n got %x\nwant %x", got, want)
	}
	// 双保险(评审 B1 回归锚):应答帧必须能被协议库自身 Decode 接受——
	// 车端引擎 readLoop 用的正是这个解码器,解不了等于应答无效。
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(got))
	if err != nil {
		t.Fatalf("应答帧被协议库拒收(加密字节/BCC 错误): %v", err)
	}
	if pm := msg.(*frame.ProtocolMessage); pm.ResponseType != types.ResponseSuccess {
		t.Fatalf("回读应答标志 = 0x%02X", pm.ResponseType)
	}
}

func TestBuildReplyLoginFailed(t *testing.T) {
	got, err := buildReply(api.V2016, vin17, 0x01, types.ResponseFailed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x01, types.ResponseFailed, vin17, nil); !bytes.Equal(got, want) {
		t.Fatalf("login failed reply:\n got %x\nwant %x", got, want)
	}
}

func TestBuildReplyHeartbeat(t *testing.T) {
	got, err := buildReply(api.V2016, vin17, 0x07, types.ResponseSuccess, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x07, types.ResponseSuccess, vin17, nil); !bytes.Equal(got, want) {
		t.Fatalf("heartbeat reply:\n got %x\nwant %x", got, want)
	}
}

func TestBuildReplyClock(t *testing.T) {
	fixed := time.Date(2026, 9, 3, 10, 30, 5, 0, time.Local)
	body := clockBody(fixed)
	if want := ext.EncodeBeanTime(fixed); !bytes.Equal(body, want[:]) {
		t.Fatalf("clock body = %x, want %x", body, want)
	}
	got, err := buildReply(api.V2016, vin17, 0x08, types.ResponseSuccess, body)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x08, types.ResponseSuccess, vin17, body); !bytes.Equal(got, want) {
		t.Fatalf("clock reply:\n got %x\nwant %x", got, want)
	}
}
