package engine

import (
	"testing"

	"github.com/sunsky74/gb32960/api"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

func TestParseParamQuery(t *testing.T) {
	info := ParseDownlink(0x80, []byte{0x01, 0x03, 0x7F})
	if info == nil || info.Kind != "query" {
		t.Fatalf("info = %+v", info)
	}
	if len(info.ParamIDs) != 3 || info.ParamIDs[0] != 1 || info.ParamIDs[2] != 0x7F {
		t.Fatalf("param ids = %v", info.ParamIDs)
	}
}

func TestParseParamSetup(t *testing.T) {
	// count=2, (id=1,len=2,val=0x0030), (id=3,len=1,val=0x05)
	payload := []byte{2, 1, 0, 2, 0x00, 0x30, 3, 0, 1, 0x05}
	info := ParseDownlink(0x81, payload)
	if info == nil || len(info.Params) != 2 {
		t.Fatalf("info = %+v", info)
	}
	if info.Params[0].ID != 1 || info.Params[0].Length != 2 || info.Params[0].Hex != "0030" {
		t.Fatalf("p0 = %+v", info.Params[0])
	}
	if info.Params[1].ID != 3 || info.Params[1].Hex != "05" {
		t.Fatalf("p1 = %+v", info.Params[1])
	}
}

func TestParseRemoteControl(t *testing.T) {
	// 表21头: 时间26/08/27 10:00:00 + 流水号 0x1234 + 命令数1 + 信息类型0x0A + 信息体 2 字节
	header := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00, 0x12, 0x34, 0x01, 0x0a}
	body := []byte{0xDE, 0xAD}
	info := ParseDownlink(0x8A, append(header, body...))
	if info == nil || info.Kind != "remote" {
		t.Fatalf("info = %+v", info)
	}
	if info.CommandTime != "2026-08-27 10:00:00" {
		t.Errorf("time = %s", info.CommandTime)
	}
	if info.SerialNumber != 0x1234 || info.CommandCount != 1 || info.InfoTypeFlag != 0x0a {
		t.Errorf("header fields = %+v", info)
	}
	if info.BodyHex != "dead" || info.HeaderHex != utils.BytesToHex(header) {
		t.Errorf("hex fields: %s / %s", info.HeaderHex, info.BodyHex)
	}
}

func TestBuildParamQueryResponse(t *testing.T) {
	rows := []ParamResponseRow{
		{ID: 1, Hex: "0ee8"}, // u16 1000
		{ID: 3, Hex: "05"},
	}
	got, err := BuildParamQueryResponse(rows)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{2, 1, 0x0E, 0xE8, 3, 0x05}
	if string(got) != string(want) {
		t.Fatalf("got %x want %x", got, want)
	}
	if _, err := BuildParamQueryResponse([]ParamResponseRow{{ID: 1, Hex: "zz"}}); err == nil {
		t.Fatal("invalid hex should fail")
	}
}

func TestBuildRemoteSecondLayer(t *testing.T) {
	header := "1a081b0a00001234010a"
	got, err := BuildRemoteSecondLayer(header, "dead")
	if err != nil {
		t.Fatal(err)
	}
	want := "1a081b0a00001234010adead"
	if utils.BytesToHex(got) != want {
		t.Fatalf("got %s want %s", utils.BytesToHex(got), want)
	}
	if _, err := BuildRemoteSecondLayer("1234", "00"); err == nil {
		t.Fatal("short header should fail")
	}
}

// TestRespondAckFrameBytes 验证应答帧结构:同 cmd、应答标志生效、空载荷、BCC 正确。
// 私有远控 0x8A 落在库枚举的 0x83~0xBF 预留区间,commandFor 必须给出精确命令字节。
func TestRespondAckFrameBytes(t *testing.T) {
	rt := commandFor(api.V2016, 0x8A)
	body := rawBody{v: api.V2016, b: nil}
	msg := frameMessage(api.V2016, "LSV00000000000001", 0x8A, rt, types.ResponseSuccess, body)
	raw, err := msg.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	dec, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	pm := dec.(*frame.ProtocolMessage)
	if pm.ResponseType != types.ResponseSuccess {
		t.Errorf("resp = %02X want 01", pm.ResponseType)
	}
	if raw[2] != 0x8A {
		t.Errorf("wire cmd byte = %02X want 8A (区间折叠 bug)", raw[2])
	}
	if pm.PayloadLength != 0 {
		t.Errorf("payload len = %d", pm.PayloadLength)
	}
}
