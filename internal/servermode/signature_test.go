package servermode

import (
	"bytes"
	"strings"
	"testing"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/signature"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	sigrt "github.com/sunsky74/gb32960/model/gbt2025/realtime"
)

// TestDecodeV2025Signature 覆盖 2025 车端签名(表8)在服务端接收链路的解码与验签钩子:
// 帧解码后应能取出 R/S/SignData,且外部验证器可经 signature.Verifier 注入。
func TestDecodeV2025Signature(t *testing.T) {
	r, s := []byte{0xAA, 0xBB}, []byte{0xCC, 0xDD}
	body := &mdl.RealTimeV2025Data{
		BeanTime: model.BeanTime{Year: 26, Month: 1, Day: 2, Hour: 3, Minute: 4, Second: 5},
		VehicleSignature: &sigrt.VehicleSignature{
			Type: 1, RLength: len(r), RValue: r, SLength: len(s), SValue: s,
		},
	}
	raw, _, err := engine.BuildFrame(api.V2025, vin17, 0x02, body)
	if err != nil {
		t.Fatal(err)
	}
	d := decodeFrame(raw)
	if d.Err != nil || d.Version != api.V2025 || d.PM == nil {
		t.Fatalf("decode = %+v", d)
	}
	sig := signature.FromBody(d.PM.Payload)
	if sig == nil || sig.Type != 1 || !bytes.Equal(sig.RValue, r) || !bytes.Equal(sig.SValue, s) {
		t.Fatalf("signature = %+v", sig)
	}
	// SignData 应覆盖数据采集时间首字节至签名段前一个字节(表8);RawBytes 即数据单元
	payload := d.PM.RawBytes
	if len(sig.SignData) >= len(payload) || !bytes.HasPrefix(payload, sig.SignData) || payload[len(sig.SignData)] != 0xFF {
		t.Fatalf("SignData 范围错误: len=%d payload=%d", len(sig.SignData), len(payload))
	}

	desc := signature.Describe(signature.VerifierFunc(func(st byte, data, rv, sv []byte) error {
		if st != 1 || !bytes.Equal(rv, r) || !bytes.Equal(sv, s) || !bytes.Equal(data, sig.SignData) {
			t.Errorf("verify 参数不符: type=%d r=%x s=%x data=%dB", st, rv, sv, len(data))
		}
		return nil
	}), sig)
	if !strings.Contains(desc, "验签通过") {
		t.Errorf("desc = %q", desc)
	}
}
