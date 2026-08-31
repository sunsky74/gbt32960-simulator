package ext

import (
	"strings"
	"testing"

	"gbt32960-simulator/internal/schema"
)

func TestEncodeNumericGolden(t *testing.T) {
	fields := []FieldSpec{
		{Key: "u8v", Label: "a", Type: "u8"},
		{Key: "u16v", Label: "b", Type: "u16", Scale: ptrf(0.1)},
		{Key: "i16v", Label: "c", Type: "i16", Offset: ptrf(40)},
		{Key: "u32v", Label: "d", Type: "u32", Scale: ptrf(0.01)},
		{Key: "f32v", Label: "e", Type: "f32"},
	}
	row := schema.RowValue{"u8v": 80, "u16v": 3.3, "i16v": 25, "u32v": 12.34, "f32v": 1.5}
	got, err := EncodeFields(fields, row)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// 50 | 0021 | fff1 | 000004d2 | 3fc00000
	const want = "500021fff1000004d23fc00000"
	if hexStr(t, got) != want {
		t.Fatalf("got %s want %s", hexStr(t, got), want)
	}
}

func TestEncodeNumericErrors(t *testing.T) {
	cases := []struct {
		name   string
		field  FieldSpec
		value  any
		wantIn string
	}{
		{"换算非整数", FieldSpec{Key: "v", Label: "v", Type: "u16", Scale: ptrf(0.1)}, 3.35, "不是整数线值"},
		{"线值超范围", FieldSpec{Key: "v", Label: "v", Type: "u8"}, 300, "超出 u8 范围"},
		{"负值进无符号", FieldSpec{Key: "v", Label: "v", Type: "u8"}, -1, "超出 u8 范围"},
		{"值非数值", FieldSpec{Key: "v", Label: "v", Type: "u8"}, "abc", "不是数值"},
		{"f32 带 scale 拒绝", FieldSpec{Key: "v", Label: "v", Type: "f32", Scale: ptrf(2)}, 1.5, "f32 不支持"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EncodeFields([]FieldSpec{tc.field}, schema.RowValue{"v": tc.value})
			if err == nil || !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("err = %v, want contains %q", err, tc.wantIn)
			}
		})
	}
	t.Run("缺少值", func(t *testing.T) {
		_, err := EncodeFields([]FieldSpec{{Key: "v", Label: "v", Type: "u8"}}, schema.RowValue{})
		if err == nil || !strings.Contains(err.Error(), "缺少值") {
			t.Fatalf("err = %v", err)
		}
	})
}

func hexStr(t *testing.T, b []byte) string {
	t.Helper()
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, hexDigits[x>>4], hexDigits[x&0x0F])
	}
	return string(out)
}
