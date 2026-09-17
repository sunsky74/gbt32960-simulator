package parser

import (
	"testing"

	"github.com/sunsky74/gb32960/api"
)

// 本次哨兵测试用例使用的固定采集时间(表5:6×BYTE 十进制)。
var bean6 = []byte{0x1a, 0x08, 0x1b, 0x0a, 0x00, 0x00}

// vehicleTLV 一帧合法的 0x01 整车数据 TLV(20 字节),用于验证哨兵之后仍能对齐解析。
var vehicleTLV = append([]byte{0x01},
	0x01, 0x01, 0x01, // 车辆状态/充电状态/运行模式
	0x00, 0x64, // 车速
	0x00, 0x00, 0x00, 0x01, // 累计里程
	0x00, 0x64, // 总电压
	0x03, 0xE8, // 总电流
	0x64,       // SOC
	0x01,       // DC/DC 状态
	0x0F,       // 挡位(停车P)
	0x00, 0x64, // 绝缘电阻
	0x00, 0x00, // 加速踏板/制动踏板
)

// parseTLV 以 0x02 报文体解析 TLV 序列(不带帧头/BCC,直接走 parsePayload)。
func parseTLV(t *testing.T, parts ...[]byte) []Field {
	t.Helper()
	p := append([]byte{}, bean6...)
	for _, part := range parts {
		p = append(p, part...)
	}
	return parsePayload(api.V2016, 0x02, p, nil, func(string) {}, func(ByteIssue) {})
}

func fieldByName(fs []Field, name string) *Field {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}

// TestAlarmCountSentinelKeepsAlignment 报警 N1=0xFE(异常)/0xFF(无效)不得吞掉后续 TLV
// (表17:故障总数 0~252,0xFE 异常/0xFF 无效)。
func TestAlarmCountSentinelKeepsAlignment(t *testing.T) {
	cases := []struct {
		count   byte
		wantTxt string
	}{
		{0xFE, "异常(0xFE):本组无有效列表"},
		{0xFF, "无效(0xFF):本组无有效列表"},
	}
	for _, c := range cases {
		// 报警 TLV:最高报警等级 1B + 通用报警标志 4B + N1 哨兵 + N2/N3/N4 各 1B(计数结构固定)
		fs := parseTLV(t, []byte{0x07, 0x00, 0x00, 0x00, 0x00, 0x00, c.count, 0x00, 0x00, 0x00}, vehicleTLV)
		if f := fieldByName(fs, "可充电储能装置故障总数"); f == nil || f.Translate != c.wantTxt {
			t.Fatalf("N1=0x%02X 字段 = %+v", c.count, f)
		}
		if fieldByName(fs, "车辆状态") == nil {
			t.Fatalf("N1=0x%02X 后整车 TLV 应继续解析", c.count)
		}
	}
}

// TestStorageSubsystemCountSentinel 储能子系统个数哨兵(表B.5/B.7:1~250,0xFE 异常/0xFF 无效)。
func TestStorageSubsystemCountSentinel(t *testing.T) {
	fs := parseTLV(t, []byte{0x08, 0xFE}, vehicleTLV)
	if f := fieldByName(fs, "电压数据子系统个数"); f == nil || f.Translate != "异常(0xFE):本组无有效列表" {
		t.Fatalf("0x08 子系统个数 = %+v", f)
	}
	if fieldByName(fs, "车辆状态") == nil {
		t.Fatal("0x08 哨兵后整车 TLV 应继续解析")
	}

	fs = parseTLV(t, []byte{0x09, 0xFF}, vehicleTLV)
	if f := fieldByName(fs, "温度数据子系统个数"); f == nil || f.Translate != "无效(0xFF):本组无有效列表" {
		t.Fatalf("0x09 子系统个数 = %+v", f)
	}
	if fieldByName(fs, "车辆状态") == nil {
		t.Fatal("0x09 哨兵后整车 TLV 应继续解析")
	}
}

// TestProbeCountSentinel 2 字节计数哨兵(表12/表B.8:0xFFFE 异常/0xFFFF 无效)。
func TestProbeCountSentinel(t *testing.T) {
	// 0x09:1 个子系统 + 子系统号 + 温度探针个数=0xFFFE
	fs := parseTLV(t, []byte{0x09, 0x01, 0x01, 0xFF, 0xFE}, vehicleTLV)
	if f := fieldByName(fs, "温度1·温度探针个数"); f == nil || f.Translate != "异常(0xFFFE):本组无有效列表" {
		t.Fatalf("探针个数(0xFFFE) = %+v", f)
	}
	if fieldByName(fs, "车辆状态") == nil {
		t.Fatal("0xFFFE 哨兵后整车 TLV 应继续解析")
	}

	// 0x03:电压/电流/消耗率 6B + 温度探针总数=0xFFFF + 氢系统字段 10B
	fs = parseTLV(t, []byte{
		0x03, 0x00, 0x64, 0x00, 0x64, 0x00, 0x64, 0xFF, 0xFF,
		0x00, 0x64, 0x01, 0x00, 0x64, 0x01, 0x00, 0x64, 0x01, 0x01,
	}, vehicleTLV)
	if f := fieldByName(fs, "温度探针总数"); f == nil || f.Translate != "无效(0xFFFF):本组无有效列表" {
		t.Fatalf("燃料电池探针总数 = %+v", f)
	}
	if fieldByName(fs, "高压 DC/DC 状态") == nil {
		t.Fatal("0xFFFF 哨兵后同组后续字段应继续解析")
	}
	if fieldByName(fs, "车辆状态") == nil {
		t.Fatal("0xFFFF 哨兵后整车 TLV 应继续解析")
	}
}
