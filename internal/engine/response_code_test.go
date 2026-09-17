package engine

import (
	"testing"

	"github.com/sunsky74/gb32960/api"
)

// TestResponseCodes 锁定应答码列表与表4一致:
// 2016 = 0x01~0x03;2025 = 0x01~0x07(0x02 文案两版不同)。
func TestResponseCodes(t *testing.T) {
	c16 := ResponseCodes(api.V2016)
	if len(c16) != 3 || c16[0].Code != 0x01 || c16[1].Code != 0x02 || c16[2].Code != 0x03 {
		t.Fatalf("2016 应答码 = %+v", c16)
	}
	if c16[1].Label != "错误(设置未成功)" {
		t.Fatalf("2016 0x02 文案 = %q", c16[1].Label)
	}

	c25 := ResponseCodes(api.V2025)
	if len(c25) != 7 {
		t.Fatalf("2025 应答码条目 = %d, want 7", len(c25))
	}
	want := []struct {
		code  int
		label string
	}{
		{0x01, "成功"}, {0x02, "其他错误"}, {0x03, "VIN重复"}, {0x04, "VIN不存在"},
		{0x05, "验签错误"}, {0x06, "数据结构错误"}, {0x07, "解密错误"},
	}
	for i, w := range want {
		if c25[i].Code != w.code || c25[i].Label != w.label {
			t.Fatalf("2025 应答码[%d] = %+v, want %+v", i, c25[i], w)
		}
	}
}
