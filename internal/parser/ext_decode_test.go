package parser

import (
	"fmt"
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
	// 定义 9 字节固定宽(u8+u16+i8+bits+bytes2+tail),只发 5 字节 → 尾部字段短缺
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
	if !strings.Contains(r.Warnings[0], "1 个字段未解析:序列号") {
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
