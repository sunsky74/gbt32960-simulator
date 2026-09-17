package bridge

import (
	"bytes"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	sigrt "github.com/sunsky74/gb32960/model/gbt2025/realtime"
	"github.com/sunsky74/gb32960/utils"
)

// TestAssembleBodyV2025Signature 锁定 2025 实时报文的签名段(表8):
// 配置签名后报文体尾部携带 0xFF 签名 TLV,且 SignData 覆盖数据采集时间起至签名前;
// 签名类型为 0(不带)时不得出现签名段。
func TestAssembleBodyV2025Signature(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rt := newExtRT(t, false)
	rt.SetConnCfg(&ConnectionConfig{Version: "2025", SignatureType: 1, SignatureR: "aabb", SignatureS: "ccdd"})
	ms := NewMessageService(rt)

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	sig := decodeV2025Signature(t, body)
	if sig == nil {
		t.Fatal("配置签名后应携带签名段")
	}
	if sig.Type != 0x01 || !bytes.Equal(sig.RValue, []byte{0xAA, 0xBB}) || !bytes.Equal(sig.SValue, []byte{0xCC, 0xDD}) {
		t.Errorf("signature = %+v", sig)
	}
	if len(sig.SignData) == 0 {
		t.Error("SignData 不应为空(应覆盖签名段之前的全部字节)")
	}

	rt.SetConnCfg(&ConnectionConfig{Version: "2025"})
	unsigned, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeV2025Signature(t, unsigned); got != nil {
		t.Errorf("未配置签名时不应带签名段: %+v", got)
	}
}

// decodeV2025Signature 组帧→解码→取签名段,并校验 SignData 覆盖范围(表8)。
func decodeV2025Signature(t *testing.T, body model.MessageBody) *sigrt.VehicleSignature {
	t.Helper()
	raw, _, err := engine.BuildFrame(api.V2025, "LSV00000000000001", 0x02, body)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	pm := msg.(*frame.ProtocolMessage)
	if err := pm.DecodePayload(); err != nil {
		t.Fatal(err)
	}
	sig := pm.Payload.(*mdl.RealTimeV2025Data).VehicleSignature
	if sig == nil {
		return nil
	}
	// RawBytes 即数据单元(SignData 应覆盖其首字节至签名段前一个字节)
	payload := pm.RawBytes
	if len(sig.SignData) >= len(payload) || !bytes.HasPrefix(payload, sig.SignData) || payload[len(sig.SignData)] != 0xFF {
		t.Fatalf("SignData 覆盖范围错误: signdata=%d payload=%d", len(sig.SignData), len(payload))
	}
	return sig
}
