package parser

import (
	"fmt"
	"strings"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/utils"
)

// parsePayload 按命令解析数据单元,产出逐字段行。
func parsePayload(v api.GBTVersion, cmd byte, p []byte, warn warnFn) []Field {
	if v == api.V2025 {
		return []Field{{
			Offset: 24, Length: len(p), Name: "数据单元 (V2025)", Type: "bytes",
			RawHex: utils.BytesToHex(p), RawValue: fmt.Sprintf("%d 字节", len(p)),
			Translate: "V2025 数据单元逐字段解析将在后续版本提供",
		}}
	}
	w := &walker{p: p, warn: warn}
	switch cmd {
	case 0x01:
		w.beanTime()
		w.u16("登入流水号", "")
		w.ascii(20, "ICCID", "")
		cnt := w.u8("可充电储能子系统数", "")
		codeLen := w.u8("子系统编码长度", "")
		for i := 0; i < int(cnt.num); i++ {
			w.bytesF(int(codeLen.num), fmt.Sprintf("子系统编码[%d]", i+1), "ascii")
		}
	case 0x04:
		w.beanTime()
		w.u16("登出流水号", "")
	case 0x02, 0x03:
		w.beanTime()
		for w.remain() > 0 {
			flag := w.take(1)
			if flag == nil {
				return w.out
			}
			tlvName := tlvName(flag[0])
			w.out = append(w.out, Field{
				Offset: 24 + w.pos - 1, Length: 1, Name: "数据类型标志 (TLV)", Type: "u8",
				RawHex: utils.BytesToHex(flag), RawValue: fmt.Sprintf("0x%02X", flag[0]),
				OffsetVal: "-", Translate: tlvName,
			})
			parseTLVGroup(w, flag[0])
		}
	case 0x07, 0x08:
		// 心跳/校时:数据单元为空
	default:
		w.bytesF(len(p), "数据单元(未细分的命令)", "bytes")
	}
	return w.out
}

// walker 顺序走字节,自动记录 Offset/Length,越界告警不中断。
type walker struct {
	p    []byte
	pos  int
	out  []Field
	warn warnFn
}

type numField struct {
	num int64
}

func (w *walker) remain() int  { return len(w.p) - w.pos }
func (w *walker) take(n int) []byte {
	if w.pos+n > len(w.p) {
		w.warn(fmt.Sprintf("报文在字段 %q 处被截断(还需 %d 字节,剩余 %d)", pendingName, n, w.remain()))
		w.pos = len(w.p)
		return nil
	}
	b := w.p[w.pos : w.pos+n]
	w.pos += n
	return b
}

var pendingName string

func (w *walker) emit(name, typ string, raw []byte, rawVal, offsetVal, translate, unit string) {
	w.out = append(w.out, Field{
		Offset: 24 + w.pos - len(raw), Length: len(raw), Name: name, Type: typ,
		RawHex: utils.BytesToHex(raw), RawValue: rawVal,
		OffsetVal: offsetVal, Translate: translate, Unit: unit,
	})
}

func (w *walker) u8(name, unit string) numField {
	pendingName = name
	b := w.take(1)
	if b == nil {
		return numField{}
	}
	w.emit(name, "u8", b, fmt.Sprint(b[0]), "-", "-", unit)
	return numField{num: int64(b[0])}
}

func (w *walker) u16(name, unit string) numField {
	pendingName = name
	b := w.take(2)
	if b == nil {
		return numField{}
	}
	v := int64(b[0])<<8 | int64(b[1])
	w.emit(name, "u16", b, fmt.Sprint(v), "-", "-", unit)
	return numField{num: v}
}

func (w *walker) conv(name, unit string, width int, c *codec.ValueConverter) {
	pendingName = name
	b := w.take(width)
	if b == nil {
		return
	}
	var v int64
	switch width {
	case 1:
		v = int64(b[0])
	case 2:
		v = int64(b[0])<<8 | int64(b[1])
	case 4:
		v = int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3])
	}
	phys := "-"
	trans := "-"
	if c.ErrValue.IsInvalid(v) {
		trans = "无效值哨兵(原始值直读)"
		phys = fmt.Sprint(v)
	} else {
		phys = trimFloat(c.Decode(v))
	}
	w.emit(name, fmt.Sprintf("u%d", width), b, fmt.Sprint(v), phys, trans, unit)
}

func (w *walker) enumF(name string, labels map[byte]string) {
	pendingName = name
	b := w.take(1)
	if b == nil {
		return
	}
	label, ok := labels[b[0]]
	if !ok {
		label = fmt.Sprintf("未知(0x%02X)", b[0])
	}
	w.emit(name, "u8", b, fmt.Sprintf("0x%02X", b[0]), "-", label, "")
}

func (w *walker) ascii(n int, name, unit string) {
	pendingName = name
	b := w.take(n)
	if b == nil {
		return
	}
	w.emit(name, "ascii", b, printable(b), "-", "-", unit)
}

func (w *walker) bytesF(n int, name, typ string) {
	pendingName = name
	b := w.take(n)
	if b == nil {
		return
	}
	w.emit(name, typ, b, utils.BytesToHex(b), "-", "-", "")
}

func (w *walker) beanTime() {
	pendingName = "数据采集时间"
	b := w.take(6)
	if b == nil {
		return
	}
	trans := fmt.Sprintf("20%02d-%02d-%02d %02d:%02d:%02d", b[0], b[1], b[2], b[3], b[4], b[5])
	w.out = append(w.out, Field{
		Offset: 24 + w.pos - 6, Length: 6, Name: "数据采集时间", Type: "bcd",
		RawHex: utils.BytesToHex(b), RawValue: strings.Join(hexBytes(b), " "),
		OffsetVal: "-", Translate: trans,
	})
}

func hexBytes(b []byte) []string {
	out := make([]string, len(b))
	for i, x := range b {
		out[i] = fmt.Sprintf("%02X", x)
	}
	return out
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.4f", f)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// ---------------------------------------------------------------- TLV 组解析

func tlvName(flag byte) string {
	return map[byte]string{
		0x01: "整车数据", 0x02: "驱动电机数据", 0x03: "燃料电池数据", 0x04: "发动机数据",
		0x05: "车辆位置数据", 0x06: "极值数据", 0x07: "报警数据",
		0x08: "可充电储能装置电压数据", 0x09: "可充电储能装置温度数据",
	}[flag]
}

func parseTLVGroup(w *walker, flag byte) {
	switch flag {
	case 0x01:
		w.enumF("车辆状态", opStateLabels)
		w.enumF("充电状态", chargeStateLabels)
		w.enumF("运行模式", modeLabels)
		w.conv("车速", "km/h", 2, &codec.SpeedConverter)
		w.conv("累计里程", "km", 4, &codec.MileageConverter)
		w.conv("总电压", "V", 2, &codec.VoltageConverter)
		w.conv("总电流", "A", 2, &codec.CurrentConverter2016)
		w.u8("SOC", "%")
		w.enumF("DC/DC 状态", dcLabels)
		parseGear(w)
		w.u16("绝缘电阻", "kΩ")
		w.u8("加速踏板行程值", "%")
		w.u8("制动踏板状态", "%")
	case 0x02:
		cnt := w.u8("驱动电机个数", "")
		for i := 0; i < int(cnt.num); i++ {
			pfx := fmt.Sprintf("电机%d·", i+1)
			w.u8(pfx+"序号", "")
			w.enumF(pfx+"状态", motorStateLabels)
			w.conv(pfx+"控制器温度", "°C", 1, &codec.ControllerTempConverter)
			w.conv(pfx+"转速", "r/min", 2, &codec.MotorSpeedConverter2016)
			w.conv(pfx+"转矩", "N·m", 2, &codec.MotorTorqueConverter2016)
			w.conv(pfx+"电机温度", "°C", 1, &codec.MotorTempConverter)
			w.conv(pfx+"控制器输入电压", "V", 2, &codec.ControllerVoltageConverter)
			w.conv(pfx+"控制器母线电流", "A", 2, &codec.ControllerCurrentConverter)
		}
	case 0x03:
		w.conv("燃料电池电压", "V", 2, &codec.FuelCellVoltageConverter)
		w.conv("燃料电池电流", "A", 2, &codec.FuelCellCurrentConverter)
		w.conv("燃料消耗率", "kg/100km", 2, &codec.FuelConsumptionRateConverter)
		cnt := w.u16("温度探针总数", "")
		for i := 0; i < int(cnt.num); i++ {
			w.conv(fmt.Sprintf("探针温度[%d]", i+1), "°C", 1, &codec.ProbeTemperatureConverter)
		}
		w.conv("氢系统最高温度", "°C", 2, &codec.HighestTempHydrogenConverter)
		w.u8("氢系统最高温度探针代号", "")
		w.u16("氢气最高浓度", "ppm")
		w.u8("氢气最高浓度传感器代号", "")
		w.conv("氢气最高压力", "MPa", 2, &codec.HydrogenMaxPressureConverter)
		w.u8("氢气最高压力传感器代号", "")
		w.enumF("高压 DC/DC 状态", onOffLabels)
	case 0x04:
		w.enumF("发动机状态", engineStateLabels)
		w.u16("曲轴转速", "r/min")
		w.conv("燃料消耗率", "L/100km", 2, &codec.FuelConsumptionRateConverter)
	case 0x05:
		parseLocation(w)
	case 0x06:
		w.u8("最高电压电池子系统号", "")
		w.u8("最高电压电池单体代号", "")
		w.conv("单体电池电压最高值", "V", 2, &codec.ExtremumVoltageConverter)
		w.u8("最低电压电池子系统号", "")
		w.u8("最低电压电池单体代号", "")
		w.conv("单体电池电压最低值", "V", 2, &codec.ExtremumVoltageConverter)
		w.u8("最高温度子系统号", "")
		w.u8("最高温度探针序号", "")
		w.conv("最高温度值", "°C", 1, &codec.TemperatureConverter)
		w.u8("最低温度子系统号", "")
		w.u8("最低温度探针序号", "")
		w.conv("最低温度值", "°C", 1, &codec.TemperatureConverter)
	case 0x07:
		parseAlarm(w)
	case 0x08:
		cnt := w.u8("电压数据子系统个数", "")
		for i := 0; i < int(cnt.num); i++ {
			pfx := fmt.Sprintf("电压%d·", i+1)
			w.u8(pfx+"子系统号", "")
			w.conv(pfx+"总电压", "V", 2, &codec.VoltageConverter)
			w.conv(pfx+"总电流", "A", 2, &codec.CurrentConverterChargeElectric)
			w.u16(pfx+"单体电池总数", "")
			w.u16(pfx+"本帧起始电池序号", "")
			m := w.u8(pfx+"本帧单体电池总数", "")
			for j := 0; j < int(m.num); j++ {
				w.conv(fmt.Sprintf("%s电压[%d]", pfx, j+1), "V", 2, &codec.BatteryVoltageConverter)
			}
		}
	case 0x09:
		cnt := w.u8("温度数据子系统个数", "")
		for i := 0; i < int(cnt.num); i++ {
			pfx := fmt.Sprintf("温度%d·", i+1)
			w.u8(pfx+"子系统号", "")
			n := w.u16(pfx+"温度探针个数", "")
			for j := 0; j < int(n.num); j++ {
				w.conv(fmt.Sprintf("%s探针温度[%d]", pfx, j+1), "°C", 1, &codec.TemperatureConverter)
			}
		}
	default:
		w.warn(fmt.Sprintf("未知 TLV 类型 0x%02X,剩余数据按原始字节展示", flag))
		w.bytesF(w.remain(), "未知类型数据", "bytes")
	}
}

func parseGear(w *walker) {
	pendingName = "档位"
	b := w.take(1)
	if b == nil {
		return
	}
	g := b[0] & 0x0F
	parts := []string{}
	if b[0]&(1<<5) != 0 {
		parts = append(parts, "驱动力")
	}
	if b[0]&(1<<4) != 0 {
		parts = append(parts, "制动力")
	}
	parts = append(parts, gearLabels[g])
	w.emit("档位", "u8", b, fmt.Sprintf("0x%02X", b[0]), "-", strings.Join(parts, "+"), "")
}

func parseLocation(w *walker) {
	pendingName = "定位状态字节"
	b := w.take(1)
	if b == nil {
		return
	}
	valid := "有效"
	if b[0]&0x01 != 0 {
		valid = "无效"
	}
	ns := "北纬"
	if b[0]&0x02 != 0 {
		ns = "南纬"
	}
	ew := "东经"
	if b[0]&0x04 != 0 {
		ew = "西经"
	}
	w.emit("定位状态", "u8", b, fmt.Sprintf("0x%02X", b[0]), "-", valid+" / "+ns+" / "+ew, "")

	w.convLong("经度("+ew+")", 4)
	w.convLong("纬度("+ns+")", 4)
}

func (w *walker) convLong(name string, width int) {
	pendingName = name
	b := w.take(width)
	if b == nil {
		return
	}
	v := int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3])
	w.emit(name, "u32", b, fmt.Sprint(v), fmt.Sprintf("%.6f", float64(v)/1e6), "", "°")
}

func parseAlarm(w *walker) {
	w.u8("最高报警等级", "")
	pendingName = "通用报警标志"
	b := w.take(4)
	if b == nil {
		return
	}
	mask := int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3])
	var on []string
	for i, label := range schemaAlarmBitLabels() {
		if mask&(1<<i) != 0 {
			on = append(on, label)
		}
	}
	if mask>>19 != 0 {
		on = append(on, "保留位有置位")
	}
	trans := "无报警"
	if len(on) > 0 {
		trans = strings.Join(on, "、")
	}
	w.emit("通用报警标志", "u32", b, fmt.Sprintf("0x%08X", mask), "-", trans, "")

	for _, seg := range []struct{ name string }{
		{"可充电储能装置故障"}, {"驱动电机故障"}, {"发动机故障"}, {"其他故障"},
	} {
		cnt := w.u8(seg.name+"总数", "")
		for j := 0; j < int(cnt.num); j++ {
			pendingName = seg.name + "代码"
			fb := w.take(4)
			if fb == nil {
				return
			}
			code := int64(fb[0])<<24 | int64(fb[1])<<16 | int64(fb[2])<<8 | int64(fb[3])
			w.emit(fmt.Sprintf("%s代码[%d]", seg.name, j+1), "u32", fb, fmt.Sprintf("0x%08X", code), fmt.Sprint(code), "-", "")
		}
	}
}
