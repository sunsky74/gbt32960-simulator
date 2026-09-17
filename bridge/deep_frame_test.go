package bridge

// 组帧-解码闭环深测:bridge 组装报文体(schema.Assemble + 扩展/签名拼装)
// → engine.BuildFrame → 协议库 ProtocolCodec.Decode,对 V2016 与 V2025
// 各断言关键字段;2025 带签名帧额外断言 0xFF TLV(表8)位于数据单元末尾、
// SignData 为数据单元前缀、R/S 与配置一致。
// 另含 Preview → internal/parser 的字段级闭环(UI 预览与解析器口径一致)。

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/parser"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/utils"
)

const deepVINBridge = "LSV00000000000001"

// deepDecode 解码一帧并按接收路径尽力解载荷。
func deepDecode(t *testing.T, raw []byte) *frame.ProtocolMessage {
	t.Helper()
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("协议库解码: %v", err)
	}
	pm := msg.(*frame.ProtocolMessage)
	_ = pm.DecodePayload()
	return pm
}

// deepXorBCC 独立重算 BCC(不依赖库/生产代码,锚定线上字节)。
func deepXorBCC(b []byte) byte {
	var x byte
	for _, v := range b {
		x ^= v
	}
	return x
}

// TestDeepFrameRoundTripV2016 2016 实时报文闭环:
// 起点 6B 数据采集时间 → 关键帧头字段 → 载荷长度一致 → parser 字段级解析零告警。
func TestDeepFrameRoundTripV2016(t *testing.T) {
	at := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	rt := newExtRT(t, false)
	ms := NewMessageService(rt)
	// 构造会读取用户目录 message.json 并回填组快照:此处显式注入 2016 默认组,
	// 保证断言与宿主机环境无关。
	rt.SetGroups(ms.DefaultGroups("2016").ToMap())

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatalf("assembleBody: %v", err)
	}
	raw, name, err := engine.BuildFrame(api.V2016, deepVINBridge, 0x02, body)
	if err != nil {
		t.Fatalf("BuildFrame: %v", err)
	}

	// 帧头:起始符 ##、命令 0x02、命令包标志 0xFE、BCC 独立可验
	if raw[0] != 0x23 || raw[1] != 0x23 || raw[2] != 0x02 || raw[3] != 0xFE {
		t.Fatalf("帧头 = %02X%02X cmd=%02X resp=%02X", raw[0], raw[1], raw[2], raw[3])
	}
	if got := deepXorBCC(raw[2 : len(raw)-1]); got != raw[len(raw)-1] {
		t.Fatalf("BCC = %02X, 独立计算 %02X", raw[len(raw)-1], got)
	}
	if !strings.HasPrefix(name, "0x02") {
		t.Fatalf("cmdName = %q", name)
	}

	// 库解码回读
	pm := deepDecode(t, raw)
	if pm.Version != api.V2016 || pm.VIN != deepVINBridge {
		t.Fatalf("回读 version/vin = %v/%q", pm.Version, pm.VIN)
	}
	if pm.PayloadLength != len(raw)-25 || len(pm.RawBytes) != len(raw)-25 {
		t.Fatalf("载荷长度 = %d/%d, want %d", pm.PayloadLength, len(pm.RawBytes), len(raw)-25)
	}
	if len(pm.RawBytes) < 6 {
		t.Fatalf("载荷过短: %d", len(pm.RawBytes))
	}
	// 表4/表5:数据单元以 6B 数据采集时间开头
	wantBean := []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00}
	if !bytes.HasPrefix(pm.RawBytes, wantBean) {
		t.Fatalf("数据采集时间 = %x, want %x", pm.RawBytes[:6], wantBean)
	}

	// parser 闭环:同一 hex 的字段级解析与帧解码一致,且无 BCC/长度告警
	res, err := parser.Parse(utils.BytesToHex(raw))
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	if res.Version != "V2016" || res.VIN != deepVINBridge {
		t.Fatalf("解析 version/vin = %s/%q", res.Version, res.VIN)
	}
	if res.TotalBytes != len(raw) || res.PayloadLen != len(raw)-25 {
		t.Fatalf("解析字节数 = %d/%d, want %d/%d", res.TotalBytes, res.PayloadLen, len(raw), len(raw)-25)
	}
	if !strings.HasPrefix(res.Command, "0x02") {
		t.Fatalf("解析命令 = %q", res.Command)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "BCC") || strings.Contains(w, "长度字段") {
			t.Fatalf("解析告警: %v", res.Warnings)
		}
	}
}

// TestDeepFrameRoundTripV2025Signature 2025 带签名实时报文闭环(表8):
// 0xFF 签名 TLV 必须是数据单元最末段,SignData 覆盖数据采集时间首字节至
// 签名段前一个字节;R/S 与连接配置一致。
func TestDeepFrameRoundTripV2025Signature(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rt := newExtRT(t, false)
	rt.SetConnCfg(&ConnectionConfig{Version: "2025", SignatureType: 1, SignatureR: "aabb", SignatureS: "ccdd"})
	ms := NewMessageService(rt)
	rt.SetGroups(ms.DefaultGroups("2025").ToMap()) // 显式注入,隔离宿主机持久化配置

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatalf("assembleBody: %v", err)
	}
	raw, _, err := engine.BuildFrame(api.V2025, deepVINBridge, 0x02, body)
	if err != nil {
		t.Fatalf("BuildFrame: %v", err)
	}
	if raw[0] != 0x24 || raw[1] != 0x24 || raw[2] != 0x02 {
		t.Fatalf("V2025 帧头 = %02X%02X cmd=%02X", raw[0], raw[1], raw[2])
	}

	pm := deepDecode(t, raw)
	if pm.Version != api.V2025 {
		t.Fatalf("回读版本 = %v", pm.Version)
	}
	// 数据单元起点:6B 数据采集时间(2026-01-02 03:04:05)
	wantBean := []byte{0x1a, 0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.HasPrefix(pm.RawBytes, wantBean) {
		t.Fatalf("数据采集时间 = %x, want %x", pm.RawBytes[:6], wantBean)
	}

	sig := decodeV2025Signature(t, body) // 复用 bridge/signature_test.go 辅助(含覆盖范围断言)
	if sig == nil {
		t.Fatal("配置签名后应携带 0xFF 签名段")
	}

	// 表8 线格式:0xFF + 类型(1B) + R 长度(2B) + R + S 长度(2B) + S
	wantTail := []byte{0xFF, 0x01, 0x00, 0x02, 0xAA, 0xBB, 0x00, 0x02, 0xCC, 0xDD}
	payload := pm.RawBytes
	if !bytes.HasSuffix(payload, wantTail) {
		t.Fatalf("签名 TLV 不在数据单元末尾: 尾部 = %x, want %x", payload[len(payload)-len(wantTail):], wantTail)
	}
	// SignData 是数据单元前缀,且签名段恰好紧随其后直至单元末(无多余字节)
	if !bytes.HasPrefix(payload, sig.SignData) {
		t.Fatalf("SignData 不是数据单元前缀: signdata=%d payload=%d", len(sig.SignData), len(payload))
	}
	if len(sig.SignData)+len(wantTail) != len(payload) {
		t.Fatalf("SignData 未覆盖至签名段前一个字节: signdata=%d tlv=%d payload=%d",
			len(sig.SignData), len(wantTail), len(payload))
	}
	if sig.Type != 0x01 || sig.RLength != 2 || sig.SLength != 2 ||
		!bytes.Equal(sig.RValue, []byte{0xAA, 0xBB}) || !bytes.Equal(sig.SValue, []byte{0xCC, 0xDD}) {
		t.Fatalf("签名字段 = type=%d r=%x s=%x", sig.Type, sig.RValue, sig.SValue)
	}
}

// TestDeepPreviewParseLoopV2016 UI 预览 → 解析器闭环:预览 hex 可被 parser
// 完整解析,版本/VIN/命令/载荷长度一致且无结构告警。
func TestDeepPreviewParseLoopV2016(t *testing.T) {
	rt := newExtRT(t, false)
	cfg := DefaultConnectionConfig()
	cfg.Version = "2016"
	cfg.VIN = deepVINBridge
	rt.SetConnCfg(cfg)
	ms := NewMessageService(rt)
	rt.SetGroups(ms.DefaultGroups("2016").ToMap()) // 显式注入,隔离宿主机持久化配置

	pv, err := ms.Preview()
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !strings.HasPrefix(pv.Cmd, "0x02") || pv.Len < 25 {
		t.Fatalf("预览 = %+v", pv)
	}
	res, err := parser.Parse(pv.Hex)
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	if res.Version != "V2016" || res.VIN != cfg.VIN {
		t.Fatalf("解析 version/vin = %s/%q, want V2016/%q", res.Version, res.VIN, cfg.VIN)
	}
	if res.TotalBytes != pv.Len || res.PayloadLen != pv.Len-25 {
		t.Fatalf("解析字节数 = %d/%d, 预览 %d", res.TotalBytes, res.PayloadLen, pv.Len)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "BCC") || strings.Contains(w, "长度字段") {
			t.Fatalf("预览帧解析告警: %v", res.Warnings)
		}
	}
}
