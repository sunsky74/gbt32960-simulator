package engine

// 0x80/0x81 下行真实帧闭环深测:平台侧组帧 → 协议库解码 → 生产解析路径
// (downlinkInfo/ParseDownlink),证明「时间 6B + 总数 + 参数」偏移解析在真实
// 线上帧上成立(2016 表B.9/B.5、表B.13/B.9;2025 表B.5/B.9)。
// 以及 0x80 应答的数据单元形态(表B.10/B.6:返回查询参数时间 6B + 总数 + 项)。

import (
	"bytes"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

const deepVIN = "LSV00000000000001"

// buildDeepFrame 用生产组帧路径(commandFor + frameMessage)造一帧。
func buildDeepFrame(t *testing.T, cmd byte, respType types.ResponseType, payload []byte) []byte {
	t.Helper()
	rt := commandFor(api.V2016, cmd)
	if rt == nil {
		t.Fatalf("commandFor(0x%02X) = nil", cmd)
	}
	msg := frameMessage(api.V2016, deepVIN, cmd, rt, respType, rawBody{v: api.V2016, b: payload})
	raw, err := msg.Bytes()
	if err != nil {
		t.Fatalf("组帧 0x%02X: %v", cmd, err)
	}
	return raw
}

// decodeDeepFrame 按接收路径解码(与 rx.go handleFrame 同构)。
func decodeDeepFrame(t *testing.T, raw []byte) *frame.ProtocolMessage {
	t.Helper()
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("协议库解码: %v", err)
	}
	pm := msg.(*frame.ProtocolMessage)
	_ = pm.DecodePayload() // 尽力解码,失败不影响帧级数据单元
	return pm
}

// TestDeepDownlinkParamQueryRealFrame 0x80 参数查询真实帧:
// 数据单元 = 查询时间(6B) + 总数(1B=3) + ID[0x01,0x03,0x7F](表B.9/B.5),
// 生产解析必须只取 3 个 ID,时间与总数不得混入。
func TestDeepDownlinkParamQueryRealFrame(t *testing.T) {
	payload := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00, 3, 0x01, 0x03, 0x7F}
	raw := buildDeepFrame(t, 0x80, types.ResponseCommand, payload)
	if raw[2] != 0x80 || raw[3] != byte(types.ResponseCommand) {
		t.Fatalf("线上 cmd/resp = %02X/%02X, want 80/FE", raw[2], raw[3])
	}

	pm := decodeDeepFrame(t, raw)
	if !bytes.Equal(pm.RawBytes, payload) {
		t.Fatalf("解码数据单元 = %x, want %x(帧-体往返不一致)", pm.RawBytes, payload)
	}

	info := downlinkInfo(raw[2], pm) // 生产路径:rx.go handleFrame → downlinkInfo
	if info == nil || info.Kind != "query" {
		t.Fatalf("downlinkInfo = %+v", info)
	}
	want := []int{0x01, 0x03, 0x7F}
	if len(info.ParamIDs) != len(want) {
		t.Fatalf("ParamIDs = %v, want %v(时间/总数混入?)", info.ParamIDs, want)
	}
	for i, id := range want {
		if info.ParamIDs[i] != id {
			t.Fatalf("ParamIDs = %v, want %v", info.ParamIDs, want)
		}
	}
}

// TestDeepDownlinkParamSetupRealFrame 0x81 参数设置真实帧:
// 数据单元 = 设置时间(6B) + 总数(1B=2) + (ID 1B + 值长度 2B + 值)×N(表B.13/B.9)。
func TestDeepDownlinkParamSetupRealFrame(t *testing.T) {
	// 项1: id=0x01 len=2 值 0x0030;项2: id=0x03 len=1 值 0x05
	payload := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00, 2,
		0x01, 0x00, 0x02, 0x00, 0x30,
		0x03, 0x00, 0x01, 0x05}
	raw := buildDeepFrame(t, 0x81, types.ResponseCommand, payload)
	pm := decodeDeepFrame(t, raw)

	info := downlinkInfo(raw[2], pm)
	if info == nil || info.Kind != "setup" {
		t.Fatalf("downlinkInfo = %+v", info)
	}
	if len(info.Params) != 2 {
		t.Fatalf("Params = %+v, want 2 项(时间/总数不得计入)", info.Params)
	}
	if p := info.Params[0]; p.ID != 0x01 || p.Length != 2 || p.Hex != "0030" {
		t.Fatalf("Params[0] = %+v, want id=1 len=2 hex=0030", p)
	}
	if p := info.Params[1]; p.ID != 0x03 || p.Length != 1 || p.Hex != "05" {
		t.Fatalf("Params[1] = %+v, want id=3 len=1 hex=05", p)
	}
}

// TestDeepParamQueryResponseFrameLevel 0x80 应答帧级断言(表B.10/B.6):
// 数据单元以 6B 返回查询参数时间开头,随后总数与 (ID+值)×N;
// 经库解码回读后逐字节一致。
func TestDeepParamQueryResponseFrameLevel(t *testing.T) {
	at := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	rows := []ParamResponseRow{
		{ID: 0x01, Hex: "0ee8"}, // u16 = 3816
		{ID: 0x03, Hex: "05"},
	}
	payload, err := BuildParamQueryResponse(rows, at)
	if err != nil {
		t.Fatalf("BuildParamQueryResponse: %v", err)
	}

	wantTime := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00} // 表5:年-2000/月/日/时/分/秒
	if !bytes.HasPrefix(payload, wantTime) {
		t.Fatalf("数据单元未以 6B 时间开头: %x", payload)
	}
	if payload[6] != 2 {
		t.Fatalf("参数总数 = %d, want 2", payload[6])
	}
	wantRest := []byte{0x01, 0x0e, 0xe8, 0x03, 0x05} // (ID+值)×2
	if !bytes.Equal(payload[7:], wantRest) {
		t.Fatalf("ID/值序列 = %x, want %x", payload[7:], wantRest)
	}

	// 帧级:命令帧头 + 成功应答标志 + 库解码回读一致。
	raw := buildDeepFrame(t, 0x80, types.ResponseSuccess, payload)
	pm := decodeDeepFrame(t, raw)
	if pm.ResponseType != types.ResponseSuccess {
		t.Fatalf("应答标志 = 0x%02X, want 0x01", pm.ResponseType)
	}
	if !bytes.Equal(pm.RawBytes, payload) {
		t.Fatalf("回读数据单元 = %x, want %x", pm.RawBytes, payload)
	}
}

// TestDeepDownlinkCountBeyondList 总数大于实际 ID 列表:按实际字节截断,不得越界。
func TestDeepDownlinkCountBeyondList(t *testing.T) {
	payload := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00, 9, 0x01, 0x03} // 声明 9 项仅给 2
	info := ParseDownlink(0x80, payload)
	if info == nil || len(info.ParamIDs) != 2 {
		t.Fatalf("ParamIDs = %+v, want 截断为 2 项", info)
	}
}
