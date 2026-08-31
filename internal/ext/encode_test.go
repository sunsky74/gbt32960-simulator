package ext

import (
	"encoding/json"
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

func TestEncodeBits(t *testing.T) {
	f2 := FieldSpec{Key: "flags", Label: "f", Type: "bits", Bits: []BitSpec{{Index: 0}, {Index: 1}}}
	got, err := encodeField(f2, schema.RowValue{"flags": map[string]any{"bit0": true}})
	if err != nil || hexStr(t, got) != "01" {
		t.Fatalf("got %s err %v", hexStr(t, got), err)
	}
	f9 := FieldSpec{Key: "flags", Label: "f", Type: "bits", Bits: []BitSpec{{Index: 9}}}
	got, err = encodeField(f9, schema.RowValue{"flags": map[string]any{"bit9": true}})
	if err != nil || hexStr(t, got) != "0002" {
		t.Fatalf("got %s err %v(第 9 位应占 2 字节)", hexStr(t, got), err)
	}
}

func TestEncodeBytesField(t *testing.T) {
	f := FieldSpec{Key: "sn", Label: "s", Type: "bytes", Length: 2}
	got, err := encodeField(f, schema.RowValue{"sn": "a1b2"})
	if err != nil || hexStr(t, got) != "a1b2" {
		t.Fatalf("got %s err %v", hexStr(t, got), err)
	}
	if _, err := encodeField(f, schema.RowValue{"sn": "a1"}); err == nil || !strings.Contains(err.Error(), "长度") {
		t.Fatalf("长度不匹配应报错, err=%v", err)
	}
	if _, err := encodeField(f, schema.RowValue{"sn": "zz"}); err == nil || !strings.Contains(err.Error(), "hex") {
		t.Fatalf("非法 hex 应报错, err=%v", err)
	}
}

func demoUnit() AppendUnit {
	var p Pack
	if err := jsonUnmarshalDemo(&p); err != nil {
		panic(err)
	}
	return p.Realtime.AppendUnits[0]
}

func TestEncodeUnitTLV(t *testing.T) {
	u := demoUnit()
	row := schema.RowValue{
		"soc2": 80, "packVolt": 3.3, "temp": 25,
		"flags": map[string]any{"bit0": true}, "sn": "a1b2",
	}
	got, err := EncodeUnit(u, row)
	if err != nil {
		t.Fatalf("encode unit: %v", err)
	}
	// 80 | 0008 | 50 0021 fff1 01 a1b2
	const want = "800008500021fff101a1b2"
	if hexStr(t, got) != want {
		t.Fatalf("got %s want %s", hexStr(t, got), want)
	}
}

func jsonUnmarshalDemo(p *Pack) error { return json.Unmarshal([]byte(demoPackJSON), p) }
