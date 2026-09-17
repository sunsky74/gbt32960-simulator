package parser

import (
	"fmt"
	"strings"
	"testing"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/api"
)

// 本文件补齐 internal/parser 的深层覆盖:
//   - 帧级异常行为:非法 BCC(表 2)、长度字段不符(表 2)、未知命令(表 3)、
//     未知 TLV 类型(表 8)、字段截断(半字段);
//   - 0x01 整车组 20 字节逐字段 Offset/Length(表 9 / 表 B.4)、
//     附录 A.1 挡位 bit5/bit4/nibble(表 A.1)、表 15 定位状态位 bit0~2;
//   - V2025 帧(start '$$')必须走 V2025 桩,不得按 2016 表解析;
//   - 自定义 TLV 单元边界:未注册 unit、空 pack、截断单元体;
//   - 报警计数 N1~N4 与最高报警等级、表 B.4 SOC/踏板的 0xFE/0xFF 哨兵(sentinel_test.go 仅覆盖计数)。
//
// 断言锁定修复后的解析器行为;凡本轮修复翻转的期望均以"修复后:"注释标注。

/* ---------------------------------------------------------------- 构造辅助 */

// frameHex 构造一帧:2B 起始符 + 命令标识 + 应答标志 + 17B VIN + 加密方式 0x01 +
// 2B 声明长度 + payload + BCC。declared 独立于 len(payload),用于构造长度字段不符的帧。
// BCC 范围与 parser.calcBCC(raw[2:len-1]) 一致(表 2:命令单元首字节到校验码前一字节)。
func frameHex(start [2]byte, cmd, resp byte, declared int, payload []byte) string {
	body := []byte{start[0], start[1], cmd, resp}
	body = append(body, []byte("PARSERTESTUNIT001")...)
	body = append(body, 0x01, byte(declared>>8), byte(declared))
	body = append(body, payload...)
	return fmt.Sprintf("%X", append(body, calcBCC(body[2:])))
}

// vehicleBody20 20 字节整车数据体(不含 TLV 标志),顺序与长度按表 B.4。
// 注:docs/standard/2016.md 表 9(正文)末尾 2 字节记「预留 2 WORD」,表 B.4 记为
// 「加速踏板行程值 1 + 制动踏板状态 1」;两表总长同为 20 字节、偏移一致,
// parser 采用表 B.4 的字段命名(见 fields_v2016.go parseTLVGroup case 0x01)。
var vehicleBody20 = []byte{
	0x02, 0x02, 0x02, // 车辆状态/充电状态/运行模式
	0x00, 0xC8, // 车速 200 → 20.0 km/h
	0x00, 0x0F, 0x42, 0x40, // 累计里程 1000000 → 100000.0 km
	0x00, 0x64, // 总电压 100 → 10.0 V
	0x27, 0x10, // 总电流 10000 → 表 9 偏移量 1000 A:I = raw/10−1000 = 0.0 A
	0x32,       // SOC 50
	0x01,       // DC/DC 工作
	0x0F,       // 挡位 停车P挡
	0x00, 0x64, // 绝缘电阻 100 kΩ
	0x64, // 加速踏板行程值 100%
	0x65, // 制动踏板状态 101=制动有效(表 B.4)
}

// vehicleTLVWithGear 复制整车数据体并把挡位字节(体偏移 15)替换为 g。
func vehicleTLVWithGear(g byte) []byte {
	b := append([]byte{}, vehicleBody20...)
	b[15] = g
	return append([]byte{0x01}, b...)
}

// parseTLVWarnings 同 sentinel_test.go 的 parseTLV,额外返回告警列表。
func parseTLVWarnings(t *testing.T, parts ...[]byte) ([]Field, []string) {
	t.Helper()
	p := append([]byte{}, bean6...)
	for _, part := range parts {
		p = append(p, part...)
	}
	var warns []string
	fs := parsePayload(api.V2016, 0x02, p, nil, func(w string) { warns = append(warns, w) }, func(ByteIssue) {})
	return fs, warns
}

/* ---------------------------------------------------------------- 0x01 整车组 */

// TestVehicleGroupFieldOffsets 表 9/表 B.4:整车数据 20 字节逐字段 Offset/Length。
// 数据采集时间(表 5,6×BYTE 十进制)在载荷最前,TLV 标志位于 24+6=30。
func TestVehicleGroupFieldOffsets(t *testing.T) {
	fs := parseTLV(t, append([]byte{0x01}, vehicleBody20...))

	want := []struct {
		name    string
		off, ln int
		rawHex  string
	}{
		{"数据采集时间", 24, 6, "1a081b0a0000"},
		{"数据类型标志 (TLV)", 30, 1, "01"},
		{"车辆状态", 31, 1, "02"},
		{"充电状态", 32, 1, "02"},
		{"运行模式", 33, 1, "02"},
		{"车速", 34, 2, "00c8"},
		{"累计里程", 36, 4, "000f4240"},
		{"总电压", 40, 2, "0064"},
		{"总电流", 42, 2, "2710"},
		{"SOC", 44, 1, "32"},
		{"DC/DC 状态", 45, 1, "01"},
		{"档位", 46, 1, "0f"}, // parser 用字「档位」,表 A.1 用「挡位」
		{"绝缘电阻", 47, 2, "0064"},
		{"加速踏板行程值", 49, 1, "64"},
		{"制动踏板状态", 50, 1, "65"},
	}
	if len(fs) != len(want) {
		t.Fatalf("字段数 = %d,期望 %d: %+v", len(fs), len(want), fs)
	}
	for i, w := range want {
		f := fs[i]
		if f.Name != w.name || f.Offset != w.off || f.Length != w.ln || f.RawHex != w.rawHex {
			t.Errorf("[%d] = {%s off=%d len=%d hex=%s},期望 {%s off=%d len=%d hex=%s}",
				i, f.Name, f.Offset, f.Length, f.RawHex, w.name, w.off, w.ln, w.rawHex)
		}
	}

	// 物理值抽查(表 B.4 + codec 换算):×0.1 的车速/里程/电压;总电流含 +1000 A 偏移
	for name, wantVal := range map[string]string{
		"车速": "20", "累计里程": "100000", "总电压": "10", "总电流": "0",
	} {
		if f := fieldByName(fs, name); f == nil || f.OffsetVal != wantVal {
			t.Errorf("%s 物理值 = %+v,期望 %s", name, f, wantVal)
		}
	}
}

// TestGearByteBitsA1 表 A.1:bit5 驱动力、bit4 制动力、bit3~0 挡位码
// (0x0 空挡、0x1~0x6 为 1~6 挡、0xD 倒挡、0xE 自动D挡、0xF 停车P挡);bit7/6 预留。
func TestGearByteBitsA1(t *testing.T) {
	cases := []struct {
		gear byte
		want string
	}{
		{0x00, "空挡"},
		{0x01, "1挡"},
		{0x06, "6挡"},
		{0x0D, "倒挡"},
		{0x0E, "自动D挡"},
		{0x0F, "停车P挡"},
		{0x0C, "预留(0xC)"},      // A.1 未定义码:现状给兜底文案(parseGear)
		{0x2F, "驱动力+停车P挡"},     // bit5
		{0x1D, "制动力+倒挡"},       // bit4
		{0x3F, "驱动力+制动力+停车P挡"}, // bit5+bit4
		{0xCF, "停车P挡"},         // bit7/6 预留位置位:挡位码翻译不受影响,另发预留位告警(TestGearReservedBitsWarning)
	}
	for _, c := range cases {
		fs := parseTLV(t, vehicleTLVWithGear(c.gear))
		f := fieldByName(fs, "档位")
		if f == nil {
			t.Fatalf("挡位 0x%02X:缺少档位字段", c.gear)
		}
		if f.Offset != 46 || f.Length != 1 {
			t.Errorf("挡位 0x%02X:off=%d len=%d,期望 46/1", c.gear, f.Offset, f.Length)
		}
		if f.Translate != c.want {
			t.Errorf("挡位 0x%02X 翻译 = %q,期望 %q", c.gear, f.Translate, c.want)
		}
	}
}

// TestGearReservedBitsWarning 表 A.1:Bit7/Bit6 为预留位,要求用 0 表示。
// 修复前 0xCF(预留位置位)被静默忽略;修复后发 1 条预留位告警,且不影响挡位码翻译。
func TestGearReservedBitsWarning(t *testing.T) {
	fs, warns := parseTLVWarnings(t, vehicleTLVWithGear(0xCF))
	if f := fieldByName(fs, "档位"); f == nil || f.Translate != "停车P挡" {
		t.Errorf("档位 0xCF = %+v,期望仍翻译为 停车P挡", f)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "预留位(Bit7/Bit6)非零") {
		t.Errorf("warnings = %v,期望 1 条预留位告警", warns)
	}
	// 预留位为 0(仅 bit5/bit4 置位)时不告警
	if _, warns := parseTLVWarnings(t, vehicleTLVWithGear(0x3F)); len(warns) != 0 {
		t.Errorf("0x3F warnings = %v,期望无告警", warns)
	}
}

// TestLocationStatusBits 表 14/表 15:bit0 有效位(0 有效/1 无效)、bit1 南北纬、bit2 东西经;
// bit3~7 保留。经/纬度 4B,以 10^6 度为单位。
func TestLocationStatusBits(t *testing.T) {
	cases := []struct {
		status  byte
		want    string
		lonName string
		latName string
	}{
		{0x00, "有效 / 北纬 / 东经", "经度(东经)", "纬度(北纬)"},
		{0x01, "无效 / 北纬 / 东经", "经度(东经)", "纬度(北纬)"},
		{0x02, "有效 / 南纬 / 东经", "经度(东经)", "纬度(南纬)"},
		{0x04, "有效 / 北纬 / 西经", "经度(西经)", "纬度(北纬)"},
		{0x07, "无效 / 南纬 / 西经", "经度(西经)", "纬度(南纬)"},
		{0x08, "有效 / 北纬 / 东经", "经度(东经)", "纬度(北纬)"}, // bit3 保留位被忽略(现状)
	}
	for _, c := range cases {
		fs := parseTLV(t, []byte{0x05, c.status, 0x01, 0x02, 0x03, 0x04, 0x00, 0x0F, 0x42, 0x40})
		if f := fieldByName(fs, "定位状态"); f == nil || f.Offset != 31 || f.Length != 1 || f.Translate != c.want {
			t.Errorf("定位状态 0x%02X = %+v,期望 off=31 len=1 translate=%q", c.status, f, c.want)
		}
		// 0x01020304 = 16909060 → 16.909060°;0x000F4240 = 1000000 → 1.000000°
		if f := fieldByName(fs, c.lonName); f == nil || f.Offset != 32 || f.Length != 4 || f.OffsetVal != "16.909060" {
			t.Errorf("经度 0x%02X = %+v,期望 %s off=32 len=4 val=16.909060", c.status, f, c.lonName)
		}
		if f := fieldByName(fs, c.latName); f == nil || f.Offset != 36 || f.Length != 4 || f.OffsetVal != "1.000000" {
			t.Errorf("纬度 0x%02X = %+v,期望 %s off=36 len=4 val=1.000000", c.status, f, c.latName)
		}
	}
}

/* ---------------------------------------------------------------- 报警哨兵 N1~N4 */

// TestAlarmAllCountSentinels 表 17:N1~N4 均为 1 BYTE,有效值 0~252,0xFE 异常/0xFF 无效。
// 哨兵按 0 项处理且不得吞掉本组其余计数与后续 TLV(sentinel_test.go 仅覆盖 N1)。
func TestAlarmAllCountSentinels(t *testing.T) {
	names := []string{"可充电储能装置故障总数", "驱动电机故障总数", "发动机故障总数", "其他故障总数"}
	sentinels := []struct {
		v    byte
		want string
	}{
		{0xFE, "异常(0xFE):本组无有效列表"},
		{0xFF, "无效(0xFF):本组无有效列表"},
	}
	for idx, name := range names {
		for _, s := range sentinels {
			counts := []byte{0, 0, 0, 0}
			counts[idx] = s.v
			tlv := append([]byte{0x07, 0x00, 0x00, 0x00, 0x00, 0x00}, counts...)
			fs := parseTLV(t, tlv, vehicleTLV)

			if f := fieldByName(fs, name); f == nil || f.Translate != s.want {
				t.Errorf("N%d(%s)=0x%02X 翻译 = %+v,期望 %q", idx+1, name, s.v, f, s.want)
			}
			for j, other := range names {
				if j == idx {
					continue
				}
				if fieldByName(fs, other) == nil {
					t.Errorf("N%d=0x%02X 后 %s 缺失(哨兵吞掉了后续计数)", idx+1, s.v, other)
				}
			}
			if fieldByName(fs, "车辆状态") == nil {
				t.Errorf("N%d=0x%02X 后整车 TLV 应继续解析", idx+1, s.v)
			}
		}
	}
}

// TestAlarmFaultListAndLevel 表 17:故障代码列表 4×N DWORD;最高报警等级 0~3,0xFE 异常/0xFF 无效。
func TestAlarmFaultListAndLevel(t *testing.T) {
	fs := parseTLV(t, []byte{
		0x07, 0x02, 0x00, 0x00, 0x00, 0x00, // 最高报警等级 2 / 通用报警标志
		0x01, 0x11, 0x22, 0x33, 0x44, // N1=1 + 一个故障码
		0x00, 0x00, 0x00, // N2/N3/N4 = 0
	}, vehicleTLV)

	// 0~3 数值仍按原样展示(Translate "-"),仅哨兵值写入语义
	if f := fieldByName(fs, "最高报警等级"); f == nil || f.RawValue != "2" || f.Translate != "-" {
		t.Errorf("最高报警等级 = %+v", f)
	}
	f := fieldByName(fs, "可充电储能装置故障代码[1]")
	if f == nil || f.Offset != 37 || f.Length != 4 || f.RawValue != "0x11223344" || f.OffsetVal != "287454020" {
		t.Errorf("N1 故障码[1] = %+v,期望 off=37 len=4 raw=0x11223344 val=287454020", f)
	}
	if fieldByName(fs, "车辆状态") == nil {
		t.Error("报警组之后整车 TLV 应继续解析")
	}

	// 等级哨兵(表 17:0xFE 异常/0xFF 无效):修复后 Translate 标注哨兵语义(修复前恒为 "-")
	for _, s := range []struct {
		v    byte
		want string
	}{
		{0xFE, "异常(0xFE)"},
		{0xFF, "无效(0xFF)"},
	} {
		fs = parseTLV(t, []byte{0x07, s.v, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, vehicleTLV)
		if f := fieldByName(fs, "最高报警等级"); f == nil || f.RawValue != fmt.Sprint(s.v) || f.Translate != s.want {
			t.Errorf("最高报警等级 0x%02X = %+v,期望 Translate %q", s.v, f, s.want)
		}
	}
}

// TestPedalSentinelsB4 表 B.4:加速踏板行程值/制动踏板状态 0~100,
// 0xFE 异常/0xFF 无效;修复前哨兵仅裸展示 RawValue 254/255、Translate 恒为 "-"。
func TestPedalSentinelsB4(t *testing.T) {
	for _, c := range []struct {
		name string
		idx  int
		v    byte
		want string
	}{
		{"加速踏板行程值", 18, 0xFE, "异常(0xFE)"},
		{"加速踏板行程值", 18, 0xFF, "无效(0xFF)"},
		{"制动踏板状态", 19, 0xFE, "异常(0xFE)"},
		{"制动踏板状态", 19, 0xFF, "无效(0xFF)"},
	} {
		body := append([]byte{}, vehicleBody20...)
		body[c.idx] = c.v
		fs := parseTLV(t, append([]byte{0x01}, body...))
		if f := fieldByName(fs, c.name); f == nil || f.RawValue != fmt.Sprint(c.v) || f.Translate != c.want {
			t.Errorf("%s 0x%02X = %+v,期望 Translate %q", c.name, c.v, f, c.want)
		}
	}
}

// TestSOCSentinelsB4 docs/standard/2016.md 表 B.4 SOC 行:有效值 0~100,
// 0xFE 异常/0xFF 无效;有效值维持原展示(RawValue 数值、Translate "-")。
func TestSOCSentinelsB4(t *testing.T) {
	const idx = 13 // 整车数据体(表 B.4)内 SOC 的字节下标
	for _, c := range []struct {
		v    byte
		want string // Translate;有效值为 "-"
	}{
		{0xFE, "异常(0xFE)"},
		{0xFF, "无效(0xFF)"},
		{0x32, "-"}, // 50%,修复前即此展示
	} {
		body := append([]byte{}, vehicleBody20...)
		body[idx] = c.v
		fs := parseTLV(t, append([]byte{0x01}, body...))
		if f := fieldByName(fs, "SOC"); f == nil || f.RawValue != fmt.Sprint(c.v) || f.Translate != c.want {
			t.Errorf("SOC 0x%02X = %+v,期望 RawValue %q / Translate %q", c.v, f, fmt.Sprint(c.v), c.want)
		}
	}
}

/* ---------------------------------------------------------------- V2025 桩 */

// TestV2025PayloadIsStubbedNotParsedAs2016 $$ 起始的 V2025 帧走 parsePayload 的 V2025 桩
// (fields_v2016.go:16-22):数据单元不逐字段解析,更不得按 2016 表把字节错认成 车辆状态 等。
func TestV2025PayloadIsStubbedNotParsedAs2016(t *testing.T) {
	// 载荷刻意构造成"合法的 2016 整车组"(标志 0x01 + 20 字节体)
	payload := append([]byte{0x01}, vehicleBody20...)
	r, err := Parse(frameHex([2]byte{0x24, 0x24}, 0x02, 0xFE, len(payload), payload))
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "V2025" || r.VersionByte != "$$" {
		t.Fatalf("版本 = %s/%s,期望 V2025/$$", r.Version, r.VersionByte)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings = %v", r.Warnings)
	}
	// 头 6 行 + V2025 桩 1 行 + BCC 1 行
	if len(r.Fields) != 8 {
		t.Errorf("字段数 = %d,期望 8: %+v", len(r.Fields), r.Fields)
	}
	stub := fieldByName(r.Fields, "数据单元 (V2025)")
	if stub == nil {
		t.Fatal("缺少 数据单元 (V2025) 桩行")
	}
	if stub.Offset != 24 || stub.Length != len(payload) || stub.RawHex != strings.ToLower(fmt.Sprintf("%X", payload)) {
		t.Errorf("桩行 = %+v", stub)
	}
	if !strings.Contains(stub.Translate, "V2025") {
		t.Errorf("桩行提示语 = %q,期望包含 V2025", stub.Translate)
	}
	if fieldByName(r.Fields, "车辆状态") != nil || fieldByName(r.Fields, "数据类型标志 (TLV)") != nil {
		t.Error("V2025 帧不得按 2016 表解析出 车辆状态/TLV 标志")
	}
	n := 0
	for _, f := range r.Fields {
		if f.Offset >= 24 && f.Name != "校验码 BCC" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("载荷字段行 = %d,期望仅 1 行桩", n)
	}
}

// TestV2025GoldenStub 生产 V2025 金标准帧同样只产出桩行(不依赖手工构造)。
func TestV2025GoldenStub(t *testing.T) {
	r, err := Parse(loadHex(t, "prod_realtime_v2025_01.hex"))
	if err != nil {
		t.Fatal(err)
	}
	if stub := fieldByName(r.Fields, "数据单元 (V2025)"); stub == nil || stub.Offset != 24 {
		t.Fatalf("金标准 V2025 桩行 = %+v", stub)
	}
	if fieldByName(r.Fields, "车辆状态") != nil || fieldByName(r.Fields, "数据类型标志 (TLV)") != nil {
		t.Error("金标准 V2025 帧不得按 2016 表解析")
	}
	if len(r.Fields) != 8 {
		t.Errorf("字段数 = %d,期望 8", len(r.Fields))
	}
}

/* ---------------------------------------------------------------- 帧级异常 */

// TestFrameBCCMismatch 表 2:BCC 为异或校验(覆盖命令单元首字节~校验码前一字节)。
// 翻转末字节:解析仍成功,恰好 1 条 BCC 告警,校验码行给出计算值,载荷照常解析。
func TestFrameBCCMismatch(t *testing.T) {
	payload := append(append([]byte{}, bean6...), append([]byte{0x01}, vehicleBody20...)...)
	raw := mustHex(t, buildRealtimeFrame(payload))
	raw[len(raw)-1] ^= 0xFF
	r, err := Parse(fmt.Sprintf("%X", raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "BCC 校验不符") {
		t.Fatalf("warnings = %v,期望恰好 1 条 BCC 告警", r.Warnings)
	}
	if f := fieldByName(r.Fields, "校验码 BCC"); f == nil || !strings.HasPrefix(f.Translate, "不符(计算值 ") {
		t.Errorf("校验码行 = %+v", f)
	}
	if fieldByName(r.Fields, "车辆状态") == nil {
		t.Error("BCC 不符不应阻止载荷解析")
	}
}

// TestFramePayloadLengthMismatch 表 2:数据单元长度"是数据单元的总字节数"(0~65531)。
// 声明长度与实际不符只告警、不中断;修复后声明外多余字节的丢弃数写入告警文案。
func TestFramePayloadLengthMismatch(t *testing.T) {
	t.Run("声明长度小于实际", func(t *testing.T) {
		payload := append(append([]byte{}, bean6...), 0xAA) // 第 7 字节在声明长度之外
		r, err := Parse(frameHex([2]byte{0x23, 0x23}, 0x02, 0xFE, 6, payload))
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "长度字段(6)与实际数据单元(7 字节)不一致") ||
			!strings.Contains(r.Warnings[0], "超出声明长度的 1 字节已忽略") {
			t.Fatalf("warnings = %v,期望含丢弃 1 字节的说明", r.Warnings)
		}
		if fieldByName(r.Fields, "数据采集时间") == nil {
			t.Error("声明长度内的 6 字节时间应正常解析")
		}
		if fieldByName(r.Fields, "未知类型数据") != nil {
			t.Error("声明长度之外的字节不应进入解析")
		}
	})
	t.Run("声明长度大于实际", func(t *testing.T) {
		r, err := Parse(frameHex([2]byte{0x23, 0x23}, 0x02, 0xFE, 10, bean6))
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) != 2 {
			t.Fatalf("warnings = %v,期望 2 条(不一致 + 截断)", r.Warnings)
		}
		if !strings.Contains(r.Warnings[0], "长度字段(10)与实际数据单元(6 字节)不一致") ||
			!strings.Contains(r.Warnings[1], "报文被截断:按实际剩余字节解析") {
			t.Errorf("warnings = %v", r.Warnings)
		}
		if fieldByName(r.Fields, "数据采集时间") == nil {
			t.Error("按实际剩余字节仍应解析出时间字段")
		}
	})
}

// TestFrameUnknownCommand 表 3:0x00 不属于任何已定义命令编码
// (库 types.CommandV2016ByCode(0x00) 返回 nil)。
// 现状:命令标识行翻译为 未知命令;数据单元整段按"未细分的命令"展示,不产生告警。
func TestFrameUnknownCommand(t *testing.T) {
	r, err := Parse(frameHex([2]byte{0x23, 0x23}, 0x00, 0xFE, 2, []byte{0xAA, 0xBB}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Command != "0x00 未知命令" {
		t.Errorf("Command = %q,期望 0x00 未知命令", r.Command)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v,期望无告警(现状)", r.Warnings)
	}
	f := fieldByName(r.Fields, "数据单元(未细分的命令)")
	if f == nil || f.Offset != 24 || f.Length != 2 || f.RawValue != "aabb" {
		t.Errorf("数据单元行 = %+v", f)
	}
}

// TestPayloadUnknownTLVType 表 8 信息类型标志:标志 0x0A~0x7F 不在已定义组内。
// 标志行 Translate 兜底为 未知(0xXX)(修复前为空串,不显示"未知"字样)。
// 标志后仍有数据:一条"未知 TLV 类型"告警后,剩余字节整体作为 未知类型数据 吞掉
// (其后合法 TLV 不再解析)。
// 修复后行为:标志为数据单元最后一个字节、其数据 0 字节时,不产出 0 长度空行,
// 改发一条 take() 风格的截断告警(点名该未知标志字节)。
func TestPayloadUnknownTLVType(t *testing.T) {
	t.Run("标志后仍有数据", func(t *testing.T) {
		for _, flag := range []byte{0x0A, 0x7F} {
			p := append(append([]byte{}, bean6...), flag, 0xAA)
			p = append(p, vehicleTLV...)
			var warns []string
			fs := parsePayload(api.V2016, 0x02, p, nil, func(w string) { warns = append(warns, w) }, func(ByteIssue) {})

			var tag *Field
			for i := range fs {
				if fs[i].Name == "数据类型标志 (TLV)" {
					tag = &fs[i]
					break
				}
			}
			if want := fmt.Sprintf("未知(0x%02X)", flag); tag == nil || tag.Translate != want {
				t.Errorf("0x%02X 标志行 = %+v,期望 Translate %q", flag, tag, want)
			}
			wantWarn := fmt.Sprintf("未知 TLV 类型 0x%02X,剩余数据按原始字节展示", flag)
			if len(warns) != 1 || warns[0] != wantWarn {
				t.Errorf("0x%02X warnings = %v,期望 [%s]", flag, warns, wantWarn)
			}
			// 剩余 = 0xAA(1) + vehicleTLV(21) = 22 字节,起点 = 24+6(时间)+1(标志) = 31
			if f := fieldByName(fs, "未知类型数据"); f == nil || f.Offset != 31 || f.Length != 22 {
				t.Errorf("0x%02X 未知类型数据 = %+v,期望 off=31 len=22", flag, f)
			}
			if fieldByName(fs, "车辆状态") != nil {
				t.Errorf("0x%02X 未知 TLV 之后的合法 TLV 现行不解析", flag)
			}
		}
	})
	t.Run("标志为最后一个字节", func(t *testing.T) {
		for _, flag := range []byte{0x0A, 0x7F} {
			p := append(append([]byte{}, bean6...), flag)
			var warns []string
			fs := parsePayload(api.V2016, 0x02, p, nil, func(w string) { warns = append(warns, w) }, func(ByteIssue) {})

			if f := fieldByName(fs, "未知类型数据"); f != nil {
				t.Errorf("0x%02X 标志后无数据,不应产出空行: %+v", flag, f)
			}
			for _, f := range fs {
				if f.Length == 0 {
					t.Errorf("0x%02X 出现 0 长度字段行: %+v", flag, f)
				}
			}
			wantWarn := fmt.Sprintf("报文在字段 %q 处被截断(还需 1 字节,剩余 0)",
				fmt.Sprintf("未知类型数据 (0x%02X)", flag))
			if len(warns) != 1 || warns[0] != wantWarn {
				t.Errorf("0x%02X warnings = %v,期望 [%s]", flag, warns, wantWarn)
			}
		}
	})
}

// TestPayloadTruncationHalfField 表 5/表 9:字段字节不足(半字段)。
// TLV 组:每个失败字段一条 take() 截断告警,该字段不产出,后续字段继续尝试,不 panic。
// 固定布局(0x01/0x04,修复后):首次截断即停,只发一条截断告警,不逐字段刷屏。
func TestPayloadTruncationHalfField(t *testing.T) {
	t.Run("登入时间只有 3 字节", func(t *testing.T) {
		var warns []string
		fs := parsePayload(api.V2016, 0x01, []byte{0x1a, 0x08, 0x1b}, nil,
			func(w string) { warns = append(warns, w) }, func(ByteIssue) {})
		if len(fs) != 0 {
			t.Errorf("字段 = %+v,期望 0 行", fs)
		}
		// 修复后:固定布局首次截断即停,只告警一次(修复前逐字段共 5 条:时间/流水号/ICCID/子系统数/编码长度)
		if len(warns) != 1 {
			t.Errorf("告警数 = %d %v,期望 1(仅首个截断字段)", len(warns), warns)
		}
		want := `字段 "数据采集时间" 处被截断(还需 6 字节,剩余 3)`
		if len(warns) == 0 || !strings.Contains(warns[0], want) {
			t.Errorf("warnings = %v,期望首条含 [%s]", warns, want)
		}
	})
	t.Run("登出时间只有 3 字节", func(t *testing.T) {
		var warns []string
		fs := parsePayload(api.V2016, 0x04, []byte{0x1a, 0x08, 0x1b}, nil,
			func(w string) { warns = append(warns, w) }, func(ByteIssue) {})
		if len(fs) != 0 {
			t.Errorf("字段 = %+v,期望 0 行", fs)
		}
		// 修复后:0x04 同 0x01,只告警一次(修复前为时间+流水号 2 条)
		if len(warns) != 1 || !strings.Contains(warns[0], `字段 "数据采集时间" 处被截断`) {
			t.Errorf("warnings = %v,期望 1 条且指认 数据采集时间", warns)
		}
	})
	t.Run("整车组末字段缺 1 字节", func(t *testing.T) {
		fs, warns := parseTLVWarnings(t, vehicleTLV[:len(vehicleTLV)-1])
		want := `字段 "制动踏板状态" 处被截断(还需 1 字节,剩余 0)`
		if len(warns) != 1 || !strings.Contains(warns[0], want) {
			t.Errorf("warnings = %v,期望 [%s]", warns, want)
		}
		if fieldByName(fs, "加速踏板行程值") == nil {
			t.Error("截断前的 加速踏板行程值 应已产出")
		}
		if fieldByName(fs, "制动踏板状态") != nil {
			t.Error("截断字段不应产出")
		}
	})
	t.Run("整车组在车速处截断", func(t *testing.T) {
		// 标志 + 3 个枚举后即断:车速起 10 个字段各产生一条截断告警
		fs, warns := parseTLVWarnings(t, append([]byte{0x01}, 0x01, 0x01, 0x01))
		if len(warns) != 10 {
			t.Errorf("告警数 = %d %v,期望 10(车速~制动踏板逐字段)", len(warns), warns)
		}
		if !strings.Contains(warns[0], `字段 "车速" 处被截断(还需 2 字节,剩余 0)`) {
			t.Errorf("首条告警 = %q,期望指认 车速", warns[0])
		}
		if fieldByName(fs, "运行模式") == nil || fieldByName(fs, "车速") != nil {
			t.Errorf("截断点前后字段 = %+v", fs)
		}
	})
}

/* ---------------------------------------------------------------- 自定义 TLV 边界 */

// TestCustomUnitTruncatedLengthField 自定义单元:unitCode 后的 2 字节长度字段本身被截断。
// 修复后:告警字段名覆盖为单元长度字段(修复前停留在上一字段"数据采集时间"),不产出单元数据行。
func TestCustomUnitTruncatedLengthField(t *testing.T) {
	payload := append(append([]byte{}, bean6...), 0x80, 0x00) // 长度字段只有 1 字节
	r, err := ParseWithPack(buildRealtimeFrame(payload), parserTestPack())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "处被截断") {
		t.Fatalf("warnings = %v,期望 1 条截断告警", r.Warnings)
	}
	if !strings.Contains(r.Warnings[0], `"自定义·私有遥测 (0x80) 长度字段"`) {
		t.Errorf("告警 = %q,期望指认单元长度字段,而非上一字段", r.Warnings[0])
	}
	for _, f := range r.Fields {
		if strings.Contains(f.Name, "私有遥测") {
			t.Errorf("长度字段截断不应产出单元数据行: %+v", f)
		}
	}
}

// TestCustomUnitDeclaredLenExceedsRemaining 自定义单元:长度字段声明 16 字节,实际只剩 2 字节。
// 现状:告警"超出剩余字节,按剩余解析"→ 按单元定义尽力解析 → 缺口汇总为 1 条告警并记 Issues。
func TestCustomUnitDeclaredLenExceedsRemaining(t *testing.T) {
	payload := append(append([]byte{}, bean6...), 0x80, 0x00, 0x10, 0x12, 0x34)
	r, err := ParseWithPack(buildRealtimeFrame(payload), parserTestPack())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 2 {
		t.Fatalf("warnings = %v,期望 2 条", r.Warnings)
	}
	if !strings.Contains(r.Warnings[0], "长度字段(16)超出剩余字节(2),按剩余解析") {
		t.Errorf("warnings[0] = %q", r.Warnings[0])
	}
	// 修复后:空 tail 不计为已解析字段,缺口 5 字节与 5 个未解析字段口径一致
	if !strings.Contains(r.Warnings[1], "比协议定义(7 字节)少 5 字节") || !strings.Contains(r.Warnings[1], "5 个字段未解析:包电压、温度、状态位、序列号、尾部") {
		t.Errorf("warnings[1] = %q", r.Warnings[1])
	}
	if f := fieldByName(r.Fields, "SOC2"); f == nil || f.RawValue != "18" {
		t.Errorf("剩余 2 字节内的 SOC2 应解析 = %+v", f)
	}
	if len(r.Issues) != 1 {
		t.Errorf("Issues = %+v,期望 1 条", r.Issues)
	}
}

// TestCustomUnitUnregisteredCodeWithPack 包内未注册的 unitCode(0x81,包仅定义 0x80):
// 长度字段内数据整体按原始字节通用展示,不告警,后续 TLV 继续解析。
func TestCustomUnitUnregisteredCodeWithPack(t *testing.T) {
	payload := append(append([]byte{}, bean6...), 0x81, 0x00, 0x01, 0xAB)
	payload = append(payload, vehicleTLV...)
	r, err := ParseWithPack(buildRealtimeFrame(payload), parserTestPack())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings = %v", r.Warnings)
	}
	if f := fieldByName(r.Fields, "自定义数据单元 0x81 数据"); f == nil || f.RawValue != "ab" || f.Length != 1 {
		t.Errorf("未注册单元展示 = %+v", f)
	}
	if fieldByName(r.Fields, "车辆状态") == nil {
		t.Error("未注册单元之后整车 TLV 应继续解析")
	}
}

// TestCustomUnitEmptyPack 空 pack(有 Meta、无任何单元定义):自定义单元退化为通用展示,
// 不告警、不越界,后续 TLV 继续解析。
func TestCustomUnitEmptyPack(t *testing.T) {
	pack := &ext.Pack{Meta: ext.Meta{ID: "empty", Label: "空包", BaseVersion: "2016"}}
	payload := append(append([]byte{}, bean6...), 0x80, 0x00, 0x02, 0x12, 0x34)
	payload = append(payload, vehicleTLV...)
	r, err := ParseWithPack(buildRealtimeFrame(payload), pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings = %v", r.Warnings)
	}
	if f := fieldByName(r.Fields, "自定义数据单元 0x80 数据"); f == nil || f.RawValue != "1234" {
		t.Errorf("空包通用展示 = %+v", f)
	}
	if fieldByName(r.Fields, "车辆状态") == nil {
		t.Error("空包之后整车 TLV 应继续解析")
	}
}
