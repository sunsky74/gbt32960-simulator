package signature

import (
	"errors"
	"testing"

	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	sigrt "github.com/sunsky74/gb32960/model/gbt2025/realtime"
)

func TestDescribe(t *testing.T) {
	sig := &sigrt.VehicleSignature{
		Type: 1, RLength: 2, RValue: []byte{0xAA, 0xBB}, SLength: 2, SValue: []byte{0xCC, 0xDD},
		SignData: []byte{0x01, 0x02},
	}

	var gotType byte
	var gotData, gotR, gotS []byte
	desc := Describe(VerifierFunc(func(st byte, data, r, s []byte) error {
		gotType, gotData, gotR, gotS = st, data, r, s
		return nil
	}), sig)
	if desc != "车端签名 SM2 (R 2B/S 2B),验签通过" {
		t.Errorf("desc = %q", desc)
	}
	if gotType != 1 || len(gotData) != 2 || len(gotR) != 2 || len(gotS) != 2 {
		t.Errorf("verify 参数不符: type=%d data=%dB r=%dB s=%dB", gotType, len(gotData), len(gotR), len(gotS))
	}

	fail := Describe(VerifierFunc(func(byte, []byte, []byte, []byte) error { return errors.New("坏签名") }), sig)
	if fail != "车端签名 SM2 (R 2B/S 2B),验签失败: 坏签名" {
		t.Errorf("失败描述 = %q", fail)
	}

	if got := Describe(nil, sig); got != "车端签名 SM2 (R 2B/S 2B),未配置验证器" {
		t.Errorf("无验证器描述 = %q", got)
	}
}

func TestTypeName(t *testing.T) {
	for _, c := range []struct {
		in   byte
		want string
	}{{0x01, "SM2"}, {0x02, "RSA"}, {0x03, "ECC"}, {0x09, "其他(0x09)"}} {
		if got := TypeName(c.in); got != c.want {
			t.Errorf("TypeName(0x%02X) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFromBody(t *testing.T) {
	sig := &sigrt.VehicleSignature{Type: 3}
	if got := FromBody(&mdl.RealTimeV2025Data{VehicleSignature: sig}); got != sig {
		t.Error("应从 2025 实时体取出签名")
	}
	if got := FromBody(&mdl.RealTimeV2025Data{}); got != nil {
		t.Error("无签名段应返回 nil")
	}
}
