package engine

import (
	"strings"
	"testing"

	"github.com/sunsky74/gb32960/api"
)

// TestParamSpecs 锁定参数定义表与文档一致:
// 2025 表B.8(0x02 = 1 字节 1~30s;0x03 = 1 字节 0~1s);
// 2016 表B.12(0x02 = 2 字节 1~600s;0x03 = 2 字节 0~60000ms;0x01 计量单元 1ms)。
func TestParamSpecs(t *testing.T) {
	v25 := ParamSpecs(api.V2025)
	if len(v25) != 16 {
		t.Fatalf("2025 参数条目 = %d, want 16", len(v25))
	}
	for i, s := range v25 {
		if s.ID != i+1 {
			t.Fatalf("第 %d 条 ID = 0x%02X, want 0x%02X", i, s.ID, i+1)
		}
	}
	if s := v25[0]; s.Length != 2 || s.RawMax == nil || *s.RawMax != 60000 {
		t.Fatalf("2025 0x01 = %+v", s)
	}
	if s := v25[1]; s.Type != "u8" || s.Length != 1 || s.RawMin == nil || *s.RawMin != 1 || *s.RawMax != 30 {
		t.Fatalf("2025 0x02 = %+v", s)
	}
	if s := v25[2]; s.Length != 1 || *s.RawMax != 1 {
		t.Fatalf("2025 0x03 = %+v", s)
	}
	// 表B.8:0x05/0x0E 为可变长(bytes),长度由 0x04/0x0D 决定
	if s := v25[4]; s.Type != "bytes" || s.Length != 0 {
		t.Fatalf("2025 0x05 = %+v", s)
	}
	if s := v25[13]; s.Type != "bytes" || s.Length != 0 {
		t.Fatalf("2025 0x0E = %+v", s)
	}
	// 0x10 枚举取值:0x01 是 / 0x02 否 / 0xFE 异常 / 0xFF 无效
	if s := v25[15]; len(s.Options) != 4 || s.Options[0].Value != 1 || s.Options[3].Value != 0xFF {
		t.Fatalf("2025 0x10 = %+v", s)
	}

	v16 := ParamSpecs(api.V2016)
	if s := v16[1]; s.Type != "u16" || s.Length != 2 || s.RawMax == nil || *s.RawMax != 600 {
		t.Fatalf("2016 0x02 = %+v", s)
	}
	if s := v16[2]; s.Type != "u16" || s.Length != 2 || *s.RawMax != 60000 {
		t.Fatalf("2016 0x03 = %+v", s)
	}
	if !strings.Contains(v16[0].Note, "1ms") {
		t.Fatalf("2016 0x01 note = %q", v16[0].Note)
	}
}
