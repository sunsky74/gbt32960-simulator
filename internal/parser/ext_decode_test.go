package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
)

func pf(v float64) *float64 { return &v }

func parserTestPack() *ext.Pack {
	return &ext.Pack{
		Meta: ext.Meta{ID: "parser-test", Label: "解析测试包", BaseVersion: "2016"},
		Realtime: ext.Realtime{AppendUnits: []ext.AppendUnit{{
			Key: "telemetry", Title: "私有遥测", UnitCode: 0x80, Fields: []ext.FieldSpec{
				{Key: "soc2", Label: "SOC2", Type: "u8", Unit: "%"},
				{Key: "volt", Label: "包电压", Type: "u16", Scale: pf(0.1), Unit: "V"},
				{Key: "temp", Label: "温度", Type: "i8", Offset: pf(40), Unit: "°C"},
				{Key: "flags", Label: "状态位", Type: "bits", Bits: []ext.BitSpec{
					{Index: 0, Label: "锁车"}, {Index: 1, Label: "限速"},
				}},
				{Key: "sn", Label: "序列号", Type: "bytes", Length: 2},
				{Key: "rest", Label: "尾部", Type: "tail"},
			},
		}}},
	}
}

// buildRealtimeFrame 构造 0x02 实时帧:头 + payload(时间 + TLV 列表) + BCC。
func buildRealtimeFrame(payload []byte) string {
	body := []byte{0x23, 0x23, 0x02, 0xFE}
	body = append(body, []byte("PARSERTESTUNIT001")...)
	body = append(body, 0x01, byte(len(payload)>>8), byte(len(payload)))
	body = append(body, payload...)
	var bcc byte
	for _, b := range body[2:] {
		bcc ^= b
	}
	body = append(body, bcc)
	return fmt.Sprintf("%X", body)
}

func TestParseWithPackRoundTrip(t *testing.T) {
	pack := parserTestPack()
	unit := pack.Realtime.AppendUnits[0]
	row := schema.RowValue{
		"soc2": 88, "volt": 12.3, "temp": -40,
		"flags": map[string]any{"bit0": true, "bit1": false},
		"sn":    "1a2b", "rest": "aabb",
	}
	tlv, err := ext.EncodeUnit(unit, row)
	if err != nil {
		t.Fatal(err)
	}
	// 追加一个包内未定义的单元(0x81),验证通用展示且解析继续
	payload := append([]byte{0x1A, 0x09, 0x03, 0x0B, 0x10, 0x1E}, tlv...)
	payload = append(payload, 0x81, 0x00, 0x02, 0xCA, 0xFE)

	r, err := ParseWithPack(buildRealtimeFrame(payload), pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings = %v", r.Warnings)
	}

	flag := findField(r.Fields, "数据类型标志 (TLV)")
	if flag == nil || flag.Translate != "自定义·私有遥测" {
		t.Errorf("TLV 标志行 = %+v", flag)
	}
	cases := []struct{ name, raw, phys, trans, unit string }{
		{"SOC2", "88", "88", "-", "%"},
		{"包电压", "123", "12.3", "-", "V"},
		{"温度", "-80", "-40", "-", "°C"},
		{"状态位", "0x01", "-", "开: 锁车", ""},
		{"序列号", "1a2b", "-", "-", ""},
		{"尾部", "aabb", "-", "-", ""},
	}
	for _, c := range cases {
		f := findField(r.Fields, c.name)
		if f == nil {
			t.Fatalf("缺少字段 %s", c.name)
		}
		if f.RawValue != c.raw || f.OffsetVal != c.phys || f.Translate != c.trans || f.Unit != c.unit {
			t.Errorf("%s = raw %q phys %q trans %q unit %q (want %q/%q/%q/%q)", c.name,
				f.RawValue, f.OffsetVal, f.Translate, f.Unit, c.raw, c.phys, c.trans, c.unit)
		}
	}
	// 未匹配单元(0x81)按通用展示
	if f := findField(r.Fields, "自定义数据单元 0x81 数据"); f == nil || f.RawValue != "cafe" {
		t.Errorf("未匹配单元展示 = %+v", f)
	}
}

// TestParseWithPackAllFieldKindsRoundTrip 覆盖 DSL 全部字段类型:
// 数值类(u8/i8/u16/i16/u32/i32 带 scale/offset 变体)、f32、bits(2B/4B)、bytes、tail。
// 由 ext.EncodeUnit 编码 → 解析器解码,断言线值(原始展示)与物理值(浮点容差)与输入一致。
func TestParseWithPackAllFieldKindsRoundTrip(t *testing.T) {
	fields := []ext.FieldSpec{
		{Key: "u8", Label: "U8", Type: "u8"},
		{Key: "u8s", Label: "U8缩放", Type: "u8", Scale: pf(0.5)},
		{Key: "i8", Label: "I8偏移", Type: "i8", Offset: pf(40)},
		{Key: "u16", Label: "U16缩放", Type: "u16", Scale: pf(0.1)},
		{Key: "i16", Label: "I16缩放偏移", Type: "i16", Scale: pf(2), Offset: pf(-100)},
		{Key: "u32", Label: "U32缩放", Type: "u32", Scale: pf(0.001)},
		{Key: "i32", Label: "I32", Type: "i32"},
		{Key: "f32", Label: "F32", Type: "f32"},
		{Key: "bits", Label: "位组", Type: "bits", Bits: []ext.BitSpec{
			{Index: 0, Label: "锁车"}, {Index: 9, Label: "限速"},
		}},
		{Key: "bits4", Label: "位组4B", Type: "bits", Bits: []ext.BitSpec{
			{Index: 0, Label: "上电"}, {Index: 20, Label: "预留"},
		}},
		{Key: "bytes", Label: "定长", Type: "bytes", Length: 3},
		{Key: "tail", Label: "尾部", Type: "tail"},
	}
	unit := ext.AppendUnit{Key: "allkinds", Title: "全类型遥测", UnitCode: 0x82, Fields: fields}
	row := schema.RowValue{
		"u8": 255, "u8s": 20, "i8": -40,
		"u16": 12.3, "i16": 150, "u32": 123456.789, "i32": -2000000000,
		"f32":   3.14159,
		"bits":  map[string]any{"bit0": true, "bit9": true},
		"bits4": map[string]any{"bit0": false, "bit20": true},
		"bytes": "1a2b3c", "tail": "deadbeef",
	}

	tlv, err := ext.EncodeUnit(unit, row)
	if err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{0x1A, 0x09, 0x03, 0x0B, 0x10, 0x1E}, tlv...)
	pack := &ext.Pack{
		Meta:     ext.Meta{ID: "allkinds", Label: "全类型测试包", BaseVersion: "2016"},
		Realtime: ext.Realtime{AppendUnits: []ext.AppendUnit{unit}},
	}
	r, err := ParseWithPack(buildRealtimeFrame(payload), pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings = %v", r.Warnings)
	}
	if len(r.Issues) != 0 {
		t.Fatalf("issues = %v", r.Issues)
	}

	numeric := []struct {
		name, raw string
		want      float64
	}{
		{"U8", "255", 255},
		{"U8缩放", "40", 20},
		{"I8偏移", "-80", -40},
		{"U16缩放", "123", 12.3},
		{"I16缩放偏移", "125", 150},
		{"U32缩放", "123456789", 123456.789},
		{"I32", "-2000000000", -2000000000},
	}
	for _, c := range numeric {
		f := findField(r.Fields, c.name)
		if f == nil {
			t.Fatalf("缺少字段 %s", c.name)
		}
		if f.RawValue != c.raw {
			t.Errorf("%s 线值 = %q, want %q", c.name, f.RawValue, c.raw)
		}
		got, err := strconv.ParseFloat(f.OffsetVal, 64)
		if err != nil {
			t.Errorf("%s 物理值 %q 不是数值: %v", c.name, f.OffsetVal, err)
			continue
		}
		if math.Abs(got-c.want) > 1e-3 {
			t.Errorf("%s 物理值 = %v, want %v", c.name, got, c.want)
		}
	}

	f32f := findField(r.Fields, "F32")
	if f32f == nil {
		t.Fatal("缺少字段 F32")
	}
	if want := fmt.Sprintf("0x%08X", math.Float32bits(float32(3.14159))); f32f.RawValue != want {
		t.Errorf("F32 线值 = %q, want %q", f32f.RawValue, want)
	}
	if got, err := strconv.ParseFloat(f32f.OffsetVal, 64); err != nil || math.Abs(got-3.14159) > 1e-3 {
		t.Errorf("F32 物理值 = %q (err=%v), want ≈3.14159", f32f.OffsetVal, err)
	}

	if f := findField(r.Fields, "位组"); f == nil || f.RawValue != "0x0102" || f.Translate != "开: 锁车 / 限速" {
		t.Errorf("位组 = %+v", f)
	}
	if f := findField(r.Fields, "位组4B"); f == nil || f.RawValue != "0x00001000" || f.Translate != "开: 预留" {
		t.Errorf("位组4B = %+v", f)
	}
	if f := findField(r.Fields, "定长"); f == nil || f.RawValue != "1a2b3c" || f.Length != 3 {
		t.Errorf("定长 = %+v", f)
	}
	if f := findField(r.Fields, "尾部"); f == nil || f.RawValue != "deadbeef" || f.Length != 4 {
		t.Errorf("尾部 = %+v", f)
	}
}

func TestParseCustomUnitWithoutPack(t *testing.T) {
	payload := []byte{0x1A, 0x09, 0x03, 0x0B, 0x10, 0x1E, 0x80, 0x00, 0x02, 0x12, 0x34}
	r, err := Parse(buildRealtimeFrame(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("自定义单元不应再触发'未知 TLV 类型'告警: %v", r.Warnings)
	}
	if f := findField(r.Fields, "自定义数据单元 0x80 数据"); f == nil || f.RawValue != "1234" {
		t.Errorf("通用展示 = %+v", f)
	}
	flag := findField(r.Fields, "数据类型标志 (TLV)")
	if flag == nil || flag.Translate != "自定义数据单元" {
		t.Errorf("TLV 标志行 = %+v", flag)
	}
}

func TestParseWithPackVersionMismatch(t *testing.T) {
	pack := parserTestPack()
	pack.Meta.BaseVersion = "2025"
	payload := []byte{0x1A, 0x09, 0x03, 0x0B, 0x10, 0x1E, 0x80, 0x00, 0x02, 0x12, 0x34}
	r, err := ParseWithPack(buildRealtimeFrame(payload), pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "基准版本") {
		t.Errorf("warnings = %v", r.Warnings)
	}
	if f := findField(r.Fields, "自定义数据单元 0x80 数据"); f == nil {
		t.Error("版本不匹配时应退化为通用展示")
	}
}

// TestParseWithPackUnitMismatch 单元数据与协议定义不符:每单元仅一条汇总告警,
// 并记录异常字节区间(Issues)供字节视图微红高亮。
func TestParseWithPackUnitMismatch(t *testing.T) {
	pack := parserTestPack()
	// 固定字段 7 字节(u8+u16+i8+bits+bytes2)+ tail,只发 5 字节 → 序列号/尾部短缺
	payload := append([]byte{0x1A, 0x09, 0x03, 0x0B, 0x10, 0x1E}, 0x80, 0x00, 0x05, 0x58, 0x00, 0x7B, 0xB0, 0x01)

	r, err := ParseWithPack(buildRealtimeFrame(payload), pack)
	if err != nil {
		t.Fatal(err)
	}
	var truncWarns, otherWarns int
	for _, w := range r.Warnings {
		if strings.Contains(w, "字段未解析") {
			truncWarns++
		} else {
			otherWarns++
		}
	}
	if truncWarns != 1 {
		t.Fatalf("截断应汇总为 1 条告警,实际 %d 条: %v", truncWarns, r.Warnings)
	}
	if otherWarns != 0 {
		t.Fatalf("不应有其他告警: %v", r.Warnings)
	}
	// 修复后:空 tail 不计为已解析字段,少 2 字节 → 恰好 2 个未解析字段
	if !strings.Contains(r.Warnings[0], "2 个字段未解析:序列号、尾部") {
		t.Errorf("汇总告警应列出未解析字段: %q", r.Warnings[0])
	}
	if len(r.Issues) != 1 {
		t.Fatalf("Issues 应为 1 条,实际 %d", len(r.Issues))
	}
	// 单元 TLV 起始 = 24(头) + 6(时间) = 30,区间 [30, 30+3+5)
	iss := r.Issues[0]
	if iss.Start != 30 || iss.End != 38 {
		t.Errorf("issue 区间 = [%d,%d),want [30,38)", iss.Start, iss.End)
	}
	if iss.Note == "" {
		t.Error("issue 应带说明")
	}
}
