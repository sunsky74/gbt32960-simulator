package schema

// deep_roundtrip_test.go —— 库升级后的深度往返/值域防回归网。
//
// 覆盖三个面:
//  1. 双版本「全组同时启用」装配 → 库编码 → 库解码 后逐字段断言;
//     各字段取 docs/standard/2016.md 表 9~表 18/表 B.4~B.8 与
//     docs/standard/2025.md 表 10~表 26 的有效值边界。
//  2. 文档定义了异常/无效哨兵的数值字段:显式锁定当前库「哨兵按原始值直通」的
//     行为(decoded == raw float)。标注「库行为锁」——若将来库改为按类型折算
//     (如 0xFFFF → NaN/特殊值),这些测试应随之更新而非静默通过。
//  3. 2016 表 B.6 单体电压 200/帧拆帧的边界完整性。
//
// 注释里的 Lxxxx 一律指对应 docs/standard 文件的行号;两版字段与边界均逐行核对过。
// 注:2016 版加速踏板/制动踏板仅定义在表 B.4(L722/L723),表 9 无此两字段;
// max/min 成对字段(极值组)在同一变体内拆向取值,两个变体合起来覆盖两端边界。
//
// 温度类字段(1℃ 分辨率:B.8 探针温度、表16 极值温度、2025 表14/表17/表19/表25/表26)
// 一律使用整数值,避免最小计量单元以下的舍入噪声。

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	mdl25 "github.com/sunsky74/gb32960/model/gbt2025"
)

var (
	deepAt2016 = time.Date(2026, 8, 30, 8, 15, 30, 0, time.Local)
	deepAt2025 = time.Date(2026, 8, 31, 9, 16, 31, 0, time.Local)
)

// ---------------------------------------------------------------- 通用断言助手

// deepBoolField 反射读取报警结构的布尔位字段,字段缺失或类型不符即失败。
func deepBoolField(t *testing.T, target any, name string) bool {
	t.Helper()
	fv := reflect.ValueOf(target).Elem().FieldByName(name)
	if !fv.IsValid() || fv.Kind() != reflect.Bool {
		t.Fatalf("布尔字段 %s 不存在或类型非 bool", name)
	}
	return fv.Bool()
}

func deepSameFloats(t *testing.T, label string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 长度: got %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %v, want %v", label, i, got[i], want[i])
		}
	}
}

func deepSameInt64s(t *testing.T, label string, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 长度: got %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %d, want %d", label, i, got[i], want[i])
		}
	}
}

func deepFaultCodes(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = float64(i)
	}
	return out
}

// deepAllBits 生成 bit0..n-1 全开(high)或全关的 bitgroup 值。
func deepAllBits(labels []string, high bool) map[string]any {
	bits := make(map[string]any, len(labels))
	for i := range labels {
		bits[fmt.Sprintf("bit%d", i)] = high
	}
	return bits
}

// ---------------------------------------------------------------- V2016 全组边界

// deepV2016BoundaryGroups 返回 2016 版 9 组全启用、各字段取文档边界的配置。
// high=true 取「上界」变体,false 取「下界」变体;max/min 成对字段拆向取值,
// 使每个字段在两个变体间都被两端边界覆盖。
func deepV2016BoundaryGroups(high bool) GroupsConfig {
	v := func(lo, hi float64) float64 {
		if high {
			return hi
		}
		return lo
	}
	// flip 用于 max/min 成对字段的另一侧:两个变体各覆盖另一端。
	flip := func(lo, hi float64) float64 {
		if high {
			return lo
		}
		return hi
	}
	gear := 0x00
	if high {
		gear = 0x0F // 附录 A.1:停车 P 挡(上界变体同时带驱动力/制动力)
	}

	vehicle := RowValue{
		"operatingState": v(1, 3),        // 表9(L162):0x01启动/0x02熄火/0x03其他
		"chargingState":  v(1, 4),        // 表9(L163):0x04 充电完成
		"operationMode":  v(1, 3),        // 表9(L164):0x01纯电/0x02混动/0x03燃油
		"speed":          v(0, 220),      // 表9(L165)/B.4(L709):0~2200 → 0~220 km/h,0.1 km/h
		"mileage":        v(0, 999999.9), // 表9(L166)/B.4(L710):0~9999999 → 0~999999.9 km,0.1 km
		"voltage":        v(0, 1000),     // 表9(L167)/B.4(L716):0~10000 → 0~1000 V,0.1 V
		"current":        v(-1000, 1000), // 表9(L168)/B.4(L717):偏移 +1000,0.1 A
		"soc":            v(0, 100),      // 表9(L169)/B.4(L718):0~100,1%
		"dc":             v(1, 2),        // 表9(L170)/B.4(L719):0x01工作/0x02断开
		"gear":           gear,           // 表9(L171)/B.4(L720):位定义见附录 A.1(L402)
		"drivingForce":   high,           // 附录 A.1(L402):bit5 1=有驱动力
		"brakingForce":   high,           // 附录 A.1(L402):bit4 1=有制动力
		"insulance":      v(0, 60000),    // 表9(L172):0~60000 kΩ,1 kΩ(无哨兵)
		// 以下两字段仅 表B.4 定义(表9 无):加速踏板 L722、制动踏板 L723
		"accelerationValue": v(0, 100), // 表B.4(L722):0~100,1%
		"brakePedal":        v(0, 101), // 表B.4(L723):0~100;0x65(101)=制动有效
	}

	motor := RowValue{
		"seq":               v(1, 253),        // 表11(L190):1~253
		"state":             v(1, 4),          // 表11(L191):0x01耗电/0x02发电/0x03关闭/0x04准备
		"controllerTemp":    v(-40, 210),      // 表11(L192):0~250,1℃,偏移 +40
		"speed":             v(-20000, 45531), // 表11(L193):0~65531,偏移 +20000
		"torque":            v(-2000, 4553.1), // 表11(L194):0~65531,0.1 N·m,偏移 +2000
		"motorTemp":         v(-40, 210),      // 表11(L195):同控制器温度
		"controllerVoltage": v(0, 6000),       // 表11(L196):0~60000 → 0~6000 V,0.1 V
		"controllerCurrent": v(-1000, 1000),   // 表11(L197):0~20000,0.1 A,偏移 +1000
	}

	fuelCell := RowValue{
		"voltage":                   v(0, 2000),                             // 表12(L207):0~20000 → 0~2000 V,0.1 V
		"current":                   v(0, 2000),                             // 表12(L208):0~20000 → 0~2000 A,0.1 A
		"consumption":               v(0, 600),                              // 表12(L209):0~60000 → 0~600 kg/100km,0.01
		"probeTemps":                []any{v(-40, 200), 25, flip(-40, 200)}, // 表12(L211):0~240,偏移 +40(无哨兵)
		"hydrogenMaxTemp":           v(-40, 200),                            // 表12(L219):0~2400,0.1℃,偏移 +40
		"hydrogenMaxTempProbe":      v(1, 252),                              // 表12(L220):1~252
		"hydrogenMaxCon":            v(0, 60000),                            // 表12(L221):0~60000
		"hydrogenMaxConSensor":      v(1, 252),                              // 表12(L222):1~252
		"hydrogenMaxPressure":       v(0, 100),                              // 表12(L223):0~1000 → 0~100 MPa,0.1(无哨兵)
		"hydrogenMaxPressureSensor": v(1, 252),                              // 表12(L224):1~252
		"highVoltageDC":             v(1, 2),                                // 表12(L225):0x01工作/0x02断开
	}

	engine := RowValue{
		"state":           v(1, 2),     // 表13(L235):0x01启动/0x02关闭
		"crankshaftSpeed": v(0, 60000), // 表13(L236):0~60000,1 r/min
		"consumption":     v(0, 600),   // 表13(L237):0~60000 → 0~600 L/100km,0.01
	}

	location := RowValue{
		// 表14(L247)+表15(L260):定位状态 bit0,0=有效定位
		"valid": high,
		// 表14(L248/L254):经度/纬度 ×10^6;表15(L261/L262):bit1 南纬、bit2 西经
		"longitude": v(-180, 180),
		"latitude":  v(-90, 90),
	}

	extremum := RowValue{
		"voltageMaxSubsystem": v(1, 250), // 表16(L273):1~250
		"voltageMaxBattery":   v(1, 250), // 表16(L274):1~250
		"maxVoltage":          v(0, 15),  // 表16(L275):0~15000 → 0~15 V,0.001 V
		"voltageMinSubsystem": flip(1, 250),
		"voltageMinBattery":   flip(1, 250),
		"minVoltage":          flip(0, 15),    // 表16(L278)
		"tempMaxSubsystem":    v(1, 250),      // 表16(L279)
		"tempMaxProbe":        v(1, 250),      // 表16(L280)
		"maxTemp":             v(-40, 210),    // 表16(L281):0~250,1℃,偏移 +40
		"tempMinSubsystem":    flip(1, 250),   // 表16(L282)
		"tempMinProbe":        flip(1, 250),   // 表16(L283)
		"minTemp":             flip(-40, 210), // 表16(L284)
	}

	alarm := RowValue{
		"maxAlarmLevel": v(0, 3),                               // 表17(L294):0无故障/1~3 级
		"bits":          deepAllBits(AlarmBitLabels2016, high), // 表18(L309~L335):bit0~18,bit19~31 预留
		"batteryFaults": deepFaultList(high, 252),              // 表17(L296):N1 0~252
		"motorFaults":   deepFaultList(high, 2),                // 表17(L298):N2 0~252
		"engineFaults":  deepFaultList(high, 1),                // 表17(L300):N3 0~252
		"otherFaults":   deepFaultList(high, 2),                // 表17(L302):N4 0~252
	}

	voltage := RowValue{
		"subsystem":     v(1, 250),
		"voltage":       v(0, 1000),     // 表B.6(L767):0~10000 → 0~1000 V,0.1 V
		"current":       v(-1000, 1000), // 表B.6(L768):0~20000,0.1 A,偏移 +1000
		"batteryTotal":  v(1, 65531),    // 表B.6(L769):1~65531
		"frameStartSeq": v(1, 65531),    // 表B.6(L770):1~65531,超 200 拆帧
		// 表B.6(L772):单体电池电压 0~60000 → 0~60.000 V,0.001 V(无哨兵定义)
		"batteryVoltages": []any{v(0.001, 60), 3.65, flip(0.001, 60)},
	}

	temperature := RowValue{
		"subsystem": v(1, 250),
		// 表B.8(L793):温度探针 0~250 → -40~+210 ℃,1 ℃(BYTE"0xFE"异常/"0xFF"无效)
		"probeTemps": []any{v(-40, 210), 25, flip(-40, 210)},
	}

	return GroupsConfig{
		GroupVehicle:     {Enabled: true, Rows: []RowValue{vehicle}},
		GroupMotor:       {Enabled: true, Rows: []RowValue{motor}},
		GroupFuelCell:    {Enabled: true, Rows: []RowValue{fuelCell}},
		GroupEngine:      {Enabled: true, Rows: []RowValue{engine}},
		GroupLocation:    {Enabled: true, Rows: []RowValue{location}},
		GroupExtremum:    {Enabled: true, Rows: []RowValue{extremum}},
		GroupAlarm:       {Enabled: true, Rows: []RowValue{alarm}},
		GroupVoltage:     {Enabled: true, Rows: []RowValue{voltage}},
		GroupTemperature: {Enabled: true, Rows: []RowValue{temperature}},
	}
}

// deepFaultList high=true 时返回 n 条故障码,false 时返回空表(下界 0 条)。
func deepFaultList(high bool, n int) []any {
	if !high {
		return nil
	}
	return deepFaultCodes(n)
}

// deepAlertList 2025 通用报警位序号/等级列表:上界变体给全表,下界变体给空表。
func deepAlertList(high bool, full []any) []any {
	if !high {
		return nil
	}
	return full
}

func TestDeepV2016AllGroupsBoundaryRoundtrip(t *testing.T) {
	for _, variant := range []struct {
		name string
		high bool
	}{{"下界", false}, {"上界", true}} {
		t.Run(variant.name, func(t *testing.T) {
			cfg := deepV2016BoundaryGroups(variant.high)
			body, err := AssembleRealtime(cfg, deepAt2016)
			if err != nil {
				t.Fatalf("装配失败: %v", err)
			}
			if body.VehicleData == nil || body.MotorDataList == nil || body.FuelCellData == nil ||
				body.EngineData == nil || body.LocationData == nil || body.ExtremumData == nil ||
				body.AlarmData == nil || body.ChargeableSubsystemElectricList == nil ||
				body.ChargeableSubsystemTemperatureList == nil {
				t.Fatal("九组同时启用时装配结果不完整")
			}
			got := decodeRealtime(t, body)
			assertDeepV2016(t, got, cfg)
		})
	}
}

func assertDeepV2016(t *testing.T, got *mdl.RealTimeData, cfg GroupsConfig) {
	t.Helper()

	if want := deepAt2016.Format("2006-01-02 15:04:05"); got.BeanTime.String() != want {
		t.Errorf("BeanTime = %s, want %s", got.BeanTime.String(), want)
	}

	t.Run("整车", func(t *testing.T) {
		r := cfg[GroupVehicle].Rows[0]
		g := got.VehicleData
		if int(g.OperatingState) != getInt(r, "operatingState", 0) ||
			int(g.ChargingState) != getInt(r, "chargingState", 0) ||
			int(g.OperationMode) != getInt(r, "operationMode", 0) {
			t.Errorf("状态枚举 = %d/%d/%d", g.OperatingState, g.ChargingState, g.OperationMode)
		}
		if g.Speed != getFloat(r, "speed", 0) || g.Mileage != getFloat(r, "mileage", 0) {
			t.Errorf("车速/里程 = %v/%v", g.Speed, g.Mileage)
		}
		if g.Voltage != getFloat(r, "voltage", 0) || g.Current != getFloat(r, "current", 0) {
			t.Errorf("总电压/总电流 = %v/%v", g.Voltage, g.Current)
		}
		if g.SOC != getInt(r, "soc", 0) || int(g.DC) != getInt(r, "dc", 0) {
			t.Errorf("SOC/DC = %d/%d", g.SOC, g.DC)
		}
		wantOrigin := byte(getInt(r, "gear", 0))
		if getBool(r, "drivingForce", false) {
			wantOrigin |= 1 << 5
		}
		if getBool(r, "brakingForce", false) {
			wantOrigin |= 1 << 4
		}
		if g.GearPosition.Origin != wantOrigin ||
			int(g.GearPosition.GP) != getInt(r, "gear", 0) ||
			g.GearPosition.DrivingForceActive != getBool(r, "drivingForce", false) ||
			g.GearPosition.BrakingTorqueApplied != getBool(r, "brakingForce", false) {
			t.Errorf("挡位 = %+v, want origin 0x%02X", g.GearPosition, wantOrigin)
		}
		if g.Insulance != getInt(r, "insulance", 0) ||
			g.AccelerationValue != getInt(r, "accelerationValue", 0) ||
			g.BrakePedalCondition != getInt(r, "brakePedal", 0) {
			t.Errorf("绝缘/加速/制动 = %d/%d/%d", g.Insulance, g.AccelerationValue, g.BrakePedalCondition)
		}
	})

	t.Run("电机", func(t *testing.T) {
		r := cfg[GroupMotor].Rows[0]
		l := got.MotorDataList
		if l.Count != 1 || len(l.Items) != 1 {
			t.Fatalf("电机条目 = %+v", l)
		}
		m := l.Items[0]
		if m.MotorSeq != getInt(r, "seq", 0) || int(m.MotorState) != getInt(r, "state", 0) {
			t.Errorf("序号/状态 = %d/%d", m.MotorSeq, m.MotorState)
		}
		if m.ControllerTemperature != getFloat(r, "controllerTemp", 0) ||
			m.MotorTemperature != getFloat(r, "motorTemp", 0) {
			t.Errorf("控制器/电机温度 = %v/%v", m.ControllerTemperature, m.MotorTemperature)
		}
		if m.MotorSpeed != getFloat(r, "speed", 0) || m.MotorTorque != getFloat(r, "torque", 0) {
			t.Errorf("转速/转矩 = %v/%v", m.MotorSpeed, m.MotorTorque)
		}
		if m.ControllerVoltage != getFloat(r, "controllerVoltage", 0) ||
			m.ControllerCurrent != getFloat(r, "controllerCurrent", 0) {
			t.Errorf("控制器电压/电流 = %v/%v", m.ControllerVoltage, m.ControllerCurrent)
		}
	})

	t.Run("燃料电池", func(t *testing.T) {
		r := cfg[GroupFuelCell].Rows[0]
		g := got.FuelCellData
		if g.FuelCellVoltage != getFloat(r, "voltage", 0) || g.FuelCellCurrent != getFloat(r, "current", 0) {
			t.Errorf("电压/电流 = %v/%v", g.FuelCellVoltage, g.FuelCellCurrent)
		}
		if g.FuelConsumptionRate != getFloat(r, "consumption", 0) {
			t.Errorf("消耗率 = %v", g.FuelConsumptionRate)
		}
		wantProbes := getFloatArray(r, "probeTemps")
		if g.TotalNumberOfFcTp != len(wantProbes) {
			t.Errorf("探针总数 = %d, want %d", g.TotalNumberOfFcTp, len(wantProbes))
		}
		deepSameFloats(t, "探针温度", g.ProbeTemperatureValues, wantProbes)
		if g.HighestTempOfHydrogenSystem != getFloat(r, "hydrogenMaxTemp", 0) ||
			g.HighestTempProbeCodeOfHydrogenSystem != getInt(r, "hydrogenMaxTempProbe", 0) {
			t.Errorf("氢系统最高温度/探针 = %v/%d", g.HighestTempOfHydrogenSystem, g.HighestTempProbeCodeOfHydrogenSystem)
		}
		if g.HighestConOfHydrogen != getInt(r, "hydrogenMaxCon", 0) ||
			g.HighestHyConSensorCode != getInt(r, "hydrogenMaxConSensor", 0) {
			t.Errorf("氢浓度/传感器 = %d/%d", g.HighestConOfHydrogen, g.HighestHyConSensorCode)
		}
		if g.HydrogenMaxPressure != getFloat(r, "hydrogenMaxPressure", 0) ||
			g.HydrogenMaxPressureSensorCode != getInt(r, "hydrogenMaxPressureSensor", 0) {
			t.Errorf("氢压力/传感器 = %v/%d", g.HydrogenMaxPressure, g.HydrogenMaxPressureSensorCode)
		}
		if int(g.HighVoltageDCState) != getInt(r, "highVoltageDC", 0) {
			t.Errorf("高压 DC/DC = %d", g.HighVoltageDCState)
		}
	})

	t.Run("发动机", func(t *testing.T) {
		r := cfg[GroupEngine].Rows[0]
		g := got.EngineData
		if int(g.EngineState) != getInt(r, "state", 0) ||
			g.CrankshaftSpeed != getInt(r, "crankshaftSpeed", 0) ||
			g.FuelConsumptionRate != getFloat(r, "consumption", 0) {
			t.Errorf("发动机 = %+v", g)
		}
	})

	t.Run("位置", func(t *testing.T) {
		r := cfg[GroupLocation].Rows[0]
		g := got.LocationData
		if g.Valid != getBool(r, "valid", true) ||
			g.Longitude != getFloat(r, "longitude", 0) ||
			g.Latitude != getFloat(r, "latitude", 0) {
			t.Errorf("位置 = %+v", g)
		}
	})

	t.Run("极值", func(t *testing.T) {
		r := cfg[GroupExtremum].Rows[0]
		g := got.ExtremumData
		if g.VoltageMaxSubsystem != getInt(r, "voltageMaxSubsystem", 0) ||
			g.VoltageMaxBattery != getInt(r, "voltageMaxBattery", 0) ||
			g.MaxVoltage != getFloat(r, "maxVoltage", 0) {
			t.Errorf("电压极值 = %+v", g)
		}
		if g.VoltageMinSubsystem != getInt(r, "voltageMinSubsystem", 0) ||
			g.VoltageMinBattery != getInt(r, "voltageMinBattery", 0) ||
			g.MinVoltage != getFloat(r, "minVoltage", 0) {
			t.Errorf("电压极小 = %+v", g)
		}
		if g.TemperatureMaxSubsystem != getInt(r, "tempMaxSubsystem", 0) ||
			g.TemperatureMaxProbe != getInt(r, "tempMaxProbe", 0) ||
			g.MaxTemperature != getFloat(r, "maxTemp", 0) {
			t.Errorf("温度极值 = %+v", g)
		}
		if g.TemperatureMinSubsystem != getInt(r, "tempMinSubsystem", 0) ||
			g.TemperatureMinProbe != getInt(r, "tempMinProbe", 0) ||
			g.MinTemperature != getFloat(r, "minTemp", 0) {
			t.Errorf("温度极小 = %+v", g)
		}
	})

	t.Run("报警", func(t *testing.T) {
		r := cfg[GroupAlarm].Rows[0]
		a := got.AlarmData
		if a.MaxAlarmLevel != getInt(r, "maxAlarmLevel", 0) {
			t.Errorf("最高报警等级 = %d", a.MaxAlarmLevel)
		}
		bitsVal, _ := r["bits"].(map[string]any)
		var wantMask int64
		for i, name := range alarmBoolFields {
			want := getBool(bitsVal, fmt.Sprintf("bit%d", i), false)
			if got := deepBoolField(t, a, name); got != want {
				t.Errorf("报警位 %s = %v, want %v", name, got, want)
			}
			if want {
				wantMask |= 1 << i
			}
		}
		if a.AlarmBitIdentify != wantMask {
			t.Errorf("报警标志位 = 0x%X, want 0x%X", a.AlarmBitIdentify, wantMask)
		}
		deepSameInt64s(t, "储能故障码", a.BatteryFaultDatas, toInt64s(getFloatArray(r, "batteryFaults")))
		deepSameInt64s(t, "电机故障码", a.MotorFaultDatas, toInt64s(getFloatArray(r, "motorFaults")))
		deepSameInt64s(t, "发动机故障码", a.EngineFaultDatas, toInt64s(getFloatArray(r, "engineFaults")))
		deepSameInt64s(t, "其他故障码", a.OtherFaultDatas, toInt64s(getFloatArray(r, "otherFaults")))
		if a.BatteryFaultNum != len(a.BatteryFaultDatas) || a.MotorFaultNum != len(a.MotorFaultDatas) ||
			a.EngineFaultNum != len(a.EngineFaultDatas) || a.OtherFaultNum != len(a.OtherFaultDatas) {
			t.Errorf("故障计数 = %d/%d/%d/%d", a.BatteryFaultNum, a.MotorFaultNum, a.EngineFaultNum, a.OtherFaultNum)
		}
	})

	t.Run("储能电压", func(t *testing.T) {
		r := cfg[GroupVoltage].Rows[0]
		l := got.ChargeableSubsystemElectricList
		if l.ElectricCount != 1 || len(l.Items) != 1 {
			t.Fatalf("储能电压条目 = %+v", l)
		}
		e := l.Items[0]
		if e.ChargeableSubSystemNumber != getInt(r, "subsystem", 0) ||
			e.Voltage != getFloat(r, "voltage", 0) ||
			e.Current != getFloat(r, "current", 0) {
			t.Errorf("子系统/电压/电流 = %+v", e)
		}
		if e.BatteryTotalCount != getInt(r, "batteryTotal", 0) ||
			e.FrameStartBatterySeq != getInt(r, "frameStartSeq", 0) {
			t.Errorf("单体总数/起始序号 = %d/%d", e.BatteryTotalCount, e.FrameStartBatterySeq)
		}
		wantVolts := getFloatArray(r, "batteryVoltages")
		if e.BatteryCount != len(wantVolts) {
			t.Errorf("本帧单体数 = %d, want %d", e.BatteryCount, len(wantVolts))
		}
		deepSameFloats(t, "单体电压", e.BatteryVoltages, wantVolts)
	})

	t.Run("储能温度", func(t *testing.T) {
		r := cfg[GroupTemperature].Rows[0]
		l := got.ChargeableSubsystemTemperatureList
		if l.TemperatureCount != 1 || len(l.Items) != 1 {
			t.Fatalf("储能温度条目 = %+v", l)
		}
		e := l.Items[0]
		if e.SubSystemNumber != getInt(r, "subsystem", 0) {
			t.Errorf("子系统号 = %d", e.SubSystemNumber)
		}
		wantTemps := getFloatArray(r, "probeTemps")
		if e.TemperatureProbeCount != len(wantTemps) {
			t.Errorf("探针数 = %d, want %d", e.TemperatureProbeCount, len(wantTemps))
		}
		deepSameFloats(t, "探针温度", e.ProbeTemperatures, wantTemps)
	})
}

// ---------------------------------------------------------------- V2025 全组边界

// deepV2025BoundaryGroups 返回 2025 版 11 组全启用、各字段取文档边界的配置。
func deepV2025BoundaryGroups(high bool) GroupsConfig {
	v := func(lo, hi float64) float64 {
		if high {
			return hi
		}
		return lo
	}
	flip := func(lo, hi float64) float64 {
		if high {
			return lo
		}
		return hi
	}
	gear := 0x00
	if high {
		gear = 0x0F // 附录 A.1:停车 P 挡
	}

	vehicle := RowValue{
		"operatingState": v(1, 3),        // 2025.md 表10(L181):0x01启动/0x02熄火/0x03其他
		"chargingState":  v(1, 4),        // 表10(L182):0x01停车充电/0x02行驶充电/0x03未充电/0x04充电完成
		"operationMode":  v(1, 3),        // 表10(L183):0x01纯电/0x02混动/0x03燃油
		"speed":          v(0, 500),      // 表10(L184):0~5000 → 0~500 km/h,0.1 km/h
		"mileage":        v(0, 999999.9), // 表10(L185):0~9999999 → 0~999999.9 km,0.1 km
		"voltage":        v(0, 6000),     // 表10(L186):0~60000 → 0~6000 V,0.1 V
		"current":        v(-3000, 3000), // 表10(L187):0~60000,偏移 3000 A,0.1 A
		"soc":            v(0, 100),      // 表10(L188):0~100,1%
		"dc":             v(1, 2),        // 表10(L189):0x01工作/0x02断开
		"gear":           gear,           // 表10(L190):位定义见附录 A.1(L505)
		"gearInvalid":    high,           // 附录 A.1(L505):bit7=1 挡位无效
		"drivingForce":   high,           // 附录 A.1(L505):bit5 1=有驱动力
		"brakingForce":   high,           // 附录 A.1(L505):bit4 1=有制动力
		"insulance":      v(0, 60000),    // 表10(L191):0~60000 kΩ,1 kΩ
	}

	motor := RowValue{
		"seq":            v(1, 253),        // 表16(L245):1~253
		"state":          v(1, 4),          // 表16(L246):0x01耗电/0x02发电/0x03关闭/0x04准备
		"controllerTemp": v(-40, 210),      // 表16(L247):0~250,1℃,偏移 +40
		"speed":          v(-32000, 33531), // 表16(L248):0~65531,偏移 +32000
		"torque":         v(-20000, 20000), // 表16(L249):0~400000,0.1 N·m,偏移 +20000
		"motorTemp":      v(-40, 210),      // 表16(L250):同控制器温度
	}

	fuelCell := RowValue{
		"hydrogenMaxTemp":           v(-40, 210), // 表17(L259):0~250,1℃,偏移 +40
		"hydrogenMaxTempProbe":      v(1, 253),   // 表17(L260):1~253
		"hydrogenMaxCon":            v(0, 6),     // 表17(L266):0~60000 → 0%~6%,0.0001%
		"hydrogenMaxConSensor":      v(1, 253),   // 表17(L267):1~253
		"hydrogenMaxPressure":       v(0, 100),   // 表17(L268):0~1000 → 0~100 MPa,0.1 MPa
		"hydrogenMaxPressureSensor": v(1, 253),   // 表17(L269):1~253
		"highVoltageDC":             v(1, 2),     // 表17(L270):0x01工作/0x02断开
		"fuelPercentage":            v(0, 100),   // 表17(L271):0~100,1%
		"dcControllerTemp":          v(-40, 210), // 表17(L272):0~250,1℃,偏移 +40
	}

	engine := RowValue{"crankshaftSpeed": v(0, 60000)} // 表20(L309):0~60000,1 r/min(2025 仅此一个字段)

	location := RowValue{
		// 表21(L318)+表22(L328):定位状态 bit0,0=有效定位
		"valid": high,
		// 表21(L319):坐标系 0x01 WGS84 / 0x02 GCJ02 / 0x03 其他
		"coordinateSystem": v(1, 3),
		// 表21(L320/L321):经度/纬度 DWORD ×10^6;表22(L329/L330):bit1 南纬、bit2 西经
		"longitude": v(-180, 180),
		"latitude":  v(-90, 90),
	}

	alarm := RowValue{
		"maxAlarmLevel":     v(0, 4),                                // 表23(L340):0无故障/1~4 级(4=热事件)
		"bits":              deepAllBits(v2025AlarmBitLabels, high), // 表24(L362~L395):bit0~27,bit28~31 预留
		"batteryFaults":     deepFaultList(high, 253),               // 表23(L342):N1 0~253
		"motorFaults":       deepFaultList(high, 1),                 // 表23(L344):N2 0~253
		"engineFaults":      deepFaultList(high, 1),                 // 表23(L346):N3 0~253
		"otherFaults":       deepFaultList(high, 1),                 // 表23(L348):N4 0~253
		"commonAlertSeqs":   deepAlertList(high, []any{0, 27}),      // 表23(L356):N5 列表,1 字节位序号
		"commonAlertLevels": deepAlertList(high, []any{1, 4}),       // 表23(L356):1 字节故障等级
	}

	minParallelRow1 := RowValue{
		"batteryPackSeq":  v(1, 50),                                    // 表12(L207):1~50
		"voltage":         v(0, 6000),                                  // 表12(L208):0~60000 → 0~6000 V,0.1 V
		"current":         v(-3000, 3000),                              // 表12(L209):0~60000,偏移 3000 A,0.1 A
		"batteryVoltages": []any{v(0.001, 60), 3.331, flip(0.001, 60)}, // 表12(L211):0~60000 → 0~60.000 V,0.001 V
	}
	minParallelRow2 := RowValue{
		"batteryPackSeq":  50,
		"voltage":         123.4,
		"current":         -55.5,
		"batteryVoltages": []any{1.234, 5.678},
	}

	batteryTempRow1 := RowValue{
		"batteryPackSeq": v(1, 50), // 表14(L227):1~50
		// 表14(L229):0~250,1℃,偏移 +40(探针数 L228:1~65531)
		"probeTemps": []any{v(-40, 210), 25, flip(-40, 210)},
	}
	batteryTempRow2 := RowValue{
		"batteryPackSeq": 25,
		"probeTemps":     []any{0, 1, 2},
	}

	fcStackRow1 := RowValue{
		"stackSeq":          v(1, 253),                             // 表19(L288):1~253
		"voltage":           v(0, 2000),                            // 表19(L289):0~20000 → 0~2000 V,0.1 V
		"current":           v(0, 2000),                            // 表19(L290):0~20000 → 0~2000 A,0.1 A
		"gasPressure":       v(-100, 400),                          // 表19(L291):0~5000,0.1 kPa,偏移 +100
		"airPressure":       v(-100, 400),                          // 表19(L297):同氢气入口压力
		"airInletTemp":      v(-40, 210),                           // 表19(L298):0~250,1℃,偏移 +40
		"coolingWaterTemps": []any{v(-40, 210), 0, flip(-40, 210)}, // 表19(L300):0~250,1℃,偏移 +40
	}
	fcStackRow2 := RowValue{
		"stackSeq":          253,
		"voltage":           12.3,
		"current":           45.6,
		"gasPressure":       -100,
		"airPressure":       400,
		"airInletTemp":      -40,
		"coolingWaterTemps": []any{0, 210},
	}

	superCap := RowValue{
		"managementSystemNumber": v(1, 253),                                  // 表25(L404):1~253
		"totalVoltage":           v(0, 1000),                                 // 表25(L405):0~10000 → 0~1000 V,0.1 V
		"totalCurrent":           v(-3000, 3000),                             // 表25(L406):0~60000,偏移 3000 A,0.1 A
		"capacitorVoltages":      []any{v(0.001, 60), 2.71, flip(0.001, 60)}, // 表25(L408):0~60000 → 0~60.000 V,0.001 V
		"probeTemperatures":      []any{v(-40, 210), 25, flip(-40, 210)},     // 表25(L410):0~250,1℃,偏移 +40
	}

	superCapExtremum := RowValue{
		"voltageMaxSubsystem": v(1, 253),      // 表26(L419):1~253
		"voltageMaxBattery":   v(1, 65531),    // 表26(L420):1~65531(文档未定义哨兵)
		"maxVoltage":          v(0, 60),       // 表26(L421):0~60000 → 0~60.000 V,0.001 V
		"voltageMinSubsystem": flip(1, 253),   // 表26(L422)
		"voltageMinBattery":   flip(1, 65531), // 表26(L423)
		"minVoltage":          flip(0, 60),    // 表26(L424)
		"tempMaxSubsystem":    v(1, 253),      // 表26(L425)
		"tempMaxProbe":        v(1, 65531),    // 表26(L426)
		"maxTemp":             v(-40, 210),    // 表26(L427):0~250,1℃,偏移 +40
		"tempMinSubsystem":    flip(1, 253),   // 表26(L428)
		"tempMinProbe":        flip(1, 65531), // 表26(L429)
		"minTemp":             flip(-40, 210), // 表26(L430)
	}

	return GroupsConfig{
		GroupVehicle:          {Enabled: true, Rows: []RowValue{vehicle}},
		GroupMotor:            {Enabled: true, Rows: []RowValue{motor}},
		GroupFuelCell:         {Enabled: true, Rows: []RowValue{fuelCell}},
		GroupEngine:           {Enabled: true, Rows: []RowValue{engine}},
		GroupLocation:         {Enabled: true, Rows: []RowValue{location}},
		GroupAlarm:            {Enabled: true, Rows: []RowValue{alarm}},
		GroupMinParallel:      {Enabled: true, Rows: []RowValue{minParallelRow1, minParallelRow2}},
		GroupBatteryTemp:      {Enabled: true, Rows: []RowValue{batteryTempRow1, batteryTempRow2}},
		GroupFCStack:          {Enabled: true, Rows: []RowValue{fcStackRow1, fcStackRow2}},
		GroupSuperCap:         {Enabled: true, Rows: []RowValue{superCap}},
		GroupSuperCapExtremum: {Enabled: true, Rows: []RowValue{superCapExtremum}},
	}
}

func TestDeepV2025AllGroupsBoundaryRoundtrip(t *testing.T) {
	for _, variant := range []struct {
		name string
		high bool
	}{{"下界", false}, {"上界", true}} {
		t.Run(variant.name, func(t *testing.T) {
			cfg := deepV2025BoundaryGroups(variant.high)
			body, err := AssembleRealtimeV2025(cfg, deepAt2025)
			if err != nil {
				t.Fatalf("装配失败: %v", err)
			}
			if body.VehicleData == nil || body.MotorDataList == nil || body.FuelCellData == nil ||
				body.EngineData == nil || body.LocationData == nil || body.AlarmData == nil ||
				body.MinParallelCellVoltages == nil || body.BatteryPackTemperatures == nil ||
				body.FuelCellStackDataList == nil || body.SuperCapacitorData == nil ||
				body.SuperCapacitorExtremumData == nil {
				t.Fatal("十一组同时启用时装配结果不完整")
			}
			got := decodeV2025Realtime(t, body)
			assertDeepV2025(t, got, cfg)
		})
	}
}

func assertDeepV2025(t *testing.T, got *mdl25.RealTimeV2025Data, cfg GroupsConfig) {
	t.Helper()

	if want := deepAt2025.Format("2006-01-02 15:04:05"); got.BeanTime.String() != want {
		t.Errorf("BeanTime = %s, want %s", got.BeanTime.String(), want)
	}

	t.Run("整车", func(t *testing.T) {
		r := cfg[GroupVehicle].Rows[0]
		g := got.VehicleData
		if int(g.OperatingState) != getInt(r, "operatingState", 0) ||
			int(g.ChargingState) != getInt(r, "chargingState", 0) ||
			int(g.OperationMode) != getInt(r, "operationMode", 0) {
			t.Errorf("状态枚举 = %d/%d/%d", g.OperatingState, g.ChargingState, g.OperationMode)
		}
		if g.Speed != getFloat(r, "speed", 0) || g.Mileage != getFloat(r, "mileage", 0) {
			t.Errorf("车速/里程 = %v/%v", g.Speed, g.Mileage)
		}
		if g.Voltage != getFloat(r, "voltage", 0) || g.Current != getFloat(r, "current", 0) {
			t.Errorf("总电压/总电流 = %v/%v", g.Voltage, g.Current)
		}
		if g.SOC != getInt(r, "soc", 0) || int(g.DC) != getInt(r, "dc", 0) ||
			g.Insulance != getInt(r, "insulance", 0) {
			t.Errorf("SOC/DC/绝缘 = %d/%d/%d", g.SOC, g.DC, g.Insulance)
		}
		wantOrigin := byte(getInt(r, "gear", 0))
		if getBool(r, "gearInvalid", false) {
			wantOrigin |= 1 << 7 // 附录 A.1:bit7=1 挡位无效
		}
		if getBool(r, "drivingForce", false) {
			wantOrigin |= 1 << 5
		}
		if getBool(r, "brakingForce", false) {
			wantOrigin |= 1 << 4
		}
		if g.GearPosition.Origin != wantOrigin ||
			int(g.GearPosition.GP) != getInt(r, "gear", 0) {
			t.Errorf("挡位 = %+v, want origin 0x%02X", g.GearPosition, wantOrigin)
		}
	})

	t.Run("电机", func(t *testing.T) {
		r := cfg[GroupMotor].Rows[0]
		l := got.MotorDataList
		if l.MotorCount != 1 || len(l.Items) != 1 {
			t.Fatalf("电机条目 = %+v", l)
		}
		m := l.Items[0]
		if m.MotorSeq != getInt(r, "seq", 0) || int(m.MotorState) != getInt(r, "state", 0) {
			t.Errorf("序号/状态 = %d/%d", m.MotorSeq, m.MotorState)
		}
		if m.ControllerTemperature != getFloat(r, "controllerTemp", 0) ||
			m.MotorTemperature != getFloat(r, "motorTemp", 0) {
			t.Errorf("控制器/电机温度 = %v/%v", m.ControllerTemperature, m.MotorTemperature)
		}
		if m.MotorSpeed != getFloat(r, "speed", 0) || m.MotorTorque != getFloat(r, "torque", 0) {
			t.Errorf("转速/转矩 = %v/%v", m.MotorSpeed, m.MotorTorque)
		}
	})

	t.Run("燃料电池发动机", func(t *testing.T) {
		r := cfg[GroupFuelCell].Rows[0]
		g := got.FuelCellData
		if g.HighestTempOfHydrogenSystem != getFloat(r, "hydrogenMaxTemp", 0) ||
			g.HighestTempProbeCodeOfHydrogenSystem != getInt(r, "hydrogenMaxTempProbe", 0) {
			t.Errorf("氢系统最高温度/探针 = %v/%d", g.HighestTempOfHydrogenSystem, g.HighestTempProbeCodeOfHydrogenSystem)
		}
		if g.HighestConOfHydrogen != getFloat(r, "hydrogenMaxCon", 0) ||
			g.HighestHyConSensorCode != getInt(r, "hydrogenMaxConSensor", 0) {
			t.Errorf("氢浓度/传感器 = %v/%d", g.HighestConOfHydrogen, g.HighestHyConSensorCode)
		}
		if g.HydrogenMaxPressure != getFloat(r, "hydrogenMaxPressure", 0) ||
			g.HydrogenMaxPressureSensorCode != getInt(r, "hydrogenMaxPressureSensor", 0) {
			t.Errorf("氢压力/传感器 = %v/%d", g.HydrogenMaxPressure, g.HydrogenMaxPressureSensorCode)
		}
		if int(g.HighVoltageDCState) != getInt(r, "highVoltageDC", 0) ||
			g.FuelPercentage != getInt(r, "fuelPercentage", 0) ||
			g.DCControllerTemperature != getFloat(r, "dcControllerTemp", 0) {
			t.Errorf("DC/DC、剩余氢量、控制器温度 = %+v", g)
		}
	})

	t.Run("发动机", func(t *testing.T) {
		r := cfg[GroupEngine].Rows[0]
		if got.EngineData.CrankshaftSpeed != getInt(r, "crankshaftSpeed", 0) {
			t.Errorf("曲轴转速 = %d", got.EngineData.CrankshaftSpeed)
		}
	})

	t.Run("位置", func(t *testing.T) {
		r := cfg[GroupLocation].Rows[0]
		g := got.LocationData
		lon := getFloat(r, "longitude", 0)
		lat := getFloat(r, "latitude", 0)
		if g.Valid != getBool(r, "valid", true) {
			t.Errorf("定位有效 = %v", g.Valid)
		}
		if int(g.CoordinateType) != getInt(r, "coordinateSystem", 0) {
			t.Errorf("坐标系 = %d", g.CoordinateType)
		}
		if g.OriginLongitude != lon || g.OriginLatitude != lat {
			t.Errorf("经纬度 = %v/%v, want %v/%v", g.OriginLongitude, g.OriginLatitude, lon, lat)
		}
		if g.EastFlag != (lon >= 0) || g.NorthernFlag != (lat >= 0) {
			t.Errorf("半球标志 = east:%v north:%v, want east:%v north:%v", g.EastFlag, g.NorthernFlag, lon >= 0, lat >= 0)
		}
	})

	t.Run("报警", func(t *testing.T) {
		r := cfg[GroupAlarm].Rows[0]
		a := got.AlarmData
		if a.MaxAlarmLevel != getInt(r, "maxAlarmLevel", 0) {
			t.Errorf("最高报警等级 = %d", a.MaxAlarmLevel)
		}
		bitsVal, _ := r["bits"].(map[string]any)
		var wantMask int64
		for i, name := range v2025AlarmBoolFields {
			want := getBool(bitsVal, fmt.Sprintf("bit%d", i), false)
			if got := deepBoolField(t, a, name); got != want {
				t.Errorf("报警位 %s = %v, want %v", name, got, want)
			}
			if want {
				wantMask |= 1 << i
			}
		}
		// 表24:bit0~27 定义,bit28~31 预留(编码恒 0)。
		if a.AlarmBitIdentify != wantMask {
			t.Errorf("报警标志位 = 0x%X, want 0x%X", a.AlarmBitIdentify, wantMask)
		}
		if a.AlarmBitIdentify>>28 != 0 {
			t.Errorf("报警标志位 bit28~31 应为 0: 0x%X", a.AlarmBitIdentify)
		}
		deepSameInt64s(t, "储能故障码", a.BatteryFaultDatas, toInt64s(getFloatArray(r, "batteryFaults")))
		deepSameInt64s(t, "电机故障码", a.MotorFaultDatas, toInt64s(getFloatArray(r, "motorFaults")))
		deepSameInt64s(t, "发动机故障码", a.EngineFaultDatas, toInt64s(getFloatArray(r, "engineFaults")))
		deepSameInt64s(t, "其他故障码", a.OtherFaultDatas, toInt64s(getFloatArray(r, "otherFaults")))
		if a.BatteryFaultNum != len(a.BatteryFaultDatas) || a.MotorFaultNum != len(a.MotorFaultDatas) ||
			a.EngineFaultNum != len(a.EngineFaultDatas) || a.OtherFaultNum != len(a.OtherFaultDatas) {
			t.Errorf("故障计数 = %d/%d/%d/%d", a.BatteryFaultNum, a.MotorFaultNum, a.EngineFaultNum, a.OtherFaultNum)
		}
		wantSeqs := getFloatArray(r, "commonAlertSeqs")
		wantLevels := getFloatArray(r, "commonAlertLevels")
		if a.CommonAlertNum != len(wantSeqs) || len(a.CommonAlertDatas) != len(wantSeqs) {
			t.Fatalf("通用报警条目数 = %d, want %d", a.CommonAlertNum, len(wantSeqs))
		}
		for i := range wantSeqs {
			if a.CommonAlertDatas[i].Seq != int(wantSeqs[i]) || a.CommonAlertDatas[i].Level != int(wantLevels[i]) {
				t.Errorf("通用报警[%d] = %+v, want seq=%d level=%d",
					i, a.CommonAlertDatas[i], int(wantSeqs[i]), int(wantLevels[i]))
			}
		}
	})

	t.Run("最小并联单元电压", func(t *testing.T) {
		rows := cfg[GroupMinParallel].Rows
		l := got.MinParallelCellVoltages
		if l.BatteryPackCount != len(rows) || len(l.Items) != len(rows) {
			t.Fatalf("电池包条目 = %+v, want %d 行", l, len(rows))
		}
		for i, r := range rows {
			e := l.Items[i]
			if e.BatteryPackSeq != getInt(r, "batteryPackSeq", 0) ||
				e.Voltage != getFloat(r, "voltage", 0) ||
				e.Current != getFloat(r, "current", 0) {
				t.Errorf("电池包[%d] = %+v", i, e)
			}
			wantVolts := getFloatArray(r, "batteryVoltages")
			if e.MinParallelUnits != len(wantVolts) {
				t.Errorf("电池包[%d] 最小并联单元数 = %d, want %d", i, e.MinParallelUnits, len(wantVolts))
			}
			deepSameFloats(t, fmt.Sprintf("电池包[%d] 并联单元电压", i), e.BatteryVoltages, wantVolts)
		}
	})

	t.Run("电池包温度", func(t *testing.T) {
		rows := cfg[GroupBatteryTemp].Rows
		l := got.BatteryPackTemperatures
		if l.BatteryPackCount != len(rows) || len(l.Items) != len(rows) {
			t.Fatalf("电池包条目 = %+v, want %d 行", l, len(rows))
		}
		for i, r := range rows {
			e := l.Items[i]
			wantTemps := getFloatArray(r, "probeTemps")
			if e.BatteryPackSeq != getInt(r, "batteryPackSeq", 0) ||
				e.TemperatureProbeCount != len(wantTemps) {
				t.Errorf("电池包[%d] = %+v, want 探针 %d", i, e, len(wantTemps))
			}
			deepSameFloats(t, fmt.Sprintf("电池包[%d] 探针温度", i), e.ProbeTemperatures, wantTemps)
		}
	})

	t.Run("燃料电池电堆", func(t *testing.T) {
		rows := cfg[GroupFCStack].Rows
		l := got.FuelCellStackDataList
		if l.StackCount != len(rows) || len(l.Items) != len(rows) {
			t.Fatalf("电堆条目 = %+v, want %d 行", l, len(rows))
		}
		for i, r := range rows {
			e := l.Items[i]
			if e.StackSeq != getInt(r, "stackSeq", 0) ||
				e.Voltage != getFloat(r, "voltage", 0) ||
				e.Current != getFloat(r, "current", 0) ||
				e.GasPressure != getFloat(r, "gasPressure", 0) ||
				e.AirPressure != getFloat(r, "airPressure", 0) ||
				e.AirInletTemp != getFloat(r, "airInletTemp", 0) {
				t.Errorf("电堆[%d] = %+v", i, e)
			}
			wantTemps := getFloatArray(r, "coolingWaterTemps")
			if e.CoolingWaterProbeCount != len(wantTemps) {
				t.Errorf("电堆[%d] 冷却水探针数 = %d, want %d", i, e.CoolingWaterProbeCount, len(wantTemps))
			}
			deepSameFloats(t, fmt.Sprintf("电堆[%d] 冷却水温度", i), e.CoolingWaterTemps, wantTemps)
		}
	})

	t.Run("超级电容", func(t *testing.T) {
		r := cfg[GroupSuperCap].Rows[0]
		g := got.SuperCapacitorData
		if g.ManagementSystemNumber != getInt(r, "managementSystemNumber", 0) ||
			g.TotalVoltage != getFloat(r, "totalVoltage", 0) ||
			g.TotalCurrent != getFloat(r, "totalCurrent", 0) {
			t.Errorf("超级电容 = %+v", g)
		}
		wantVolts := getFloatArray(r, "capacitorVoltages")
		wantTemps := getFloatArray(r, "probeTemperatures")
		if g.CapacitorCount != len(wantVolts) || g.TemperatureProbeCount != len(wantTemps) {
			t.Errorf("单体数/探针数 = %d/%d, want %d/%d", g.CapacitorCount, g.TemperatureProbeCount, len(wantVolts), len(wantTemps))
		}
		deepSameFloats(t, "超级电容单体电压", g.CapacitorVoltages, wantVolts)
		deepSameFloats(t, "超级电容探针温度", g.ProbeTemperatures, wantTemps)
	})

	t.Run("超级电容极值", func(t *testing.T) {
		r := cfg[GroupSuperCapExtremum].Rows[0]
		g := got.SuperCapacitorExtremumData
		if g.VoltageMaxSubsystem != getInt(r, "voltageMaxSubsystem", 0) ||
			g.VoltageMaxBattery != getInt(r, "voltageMaxBattery", 0) ||
			g.MaxVoltage != getFloat(r, "maxVoltage", 0) {
			t.Errorf("电压极值 = %+v", g)
		}
		if g.VoltageMinSubsystem != getInt(r, "voltageMinSubsystem", 0) ||
			g.VoltageMinBattery != getInt(r, "voltageMinBattery", 0) ||
			g.MinVoltage != getFloat(r, "minVoltage", 0) {
			t.Errorf("电压极小 = %+v", g)
		}
		if g.TemperatureMaxSubsystem != getInt(r, "tempMaxSubsystem", 0) ||
			g.TemperatureMaxProbe != getInt(r, "tempMaxProbe", 0) ||
			g.MaxTemperature != getFloat(r, "maxTemp", 0) {
			t.Errorf("温度极值 = %+v", g)
		}
		if g.TemperatureMinSubsystem != getInt(r, "tempMinSubsystem", 0) ||
			g.TemperatureMinProbe != getInt(r, "tempMinProbe", 0) ||
			g.MinTemperature != getFloat(r, "minTemp", 0) {
			t.Errorf("温度极小 = %+v", g)
		}
	})
}

// ---------------------------------------------------------------- 哨兵直通(库行为锁)

// TestDeepV2016SentinelPassthroughLibLock 锁定 2016 版数值字段哨兵的当前库行为。
// 文档在每张表的「描述及要求」列逐字段定义哨兵:BYTE 为“0xFE”表示异常、“0xFF”表示无效;
// WORD 为“0xFF,0xFE”表示异常、“0xFF,0xFF”表示无效;累计里程(DWORD)为
// 0xFFFFFFFE/0xFFFFFFFF(2016.md L165/L166/L192 等)。
// 注意文档并非所有数值字段都定义了哨兵(如绝缘电阻 L172、氢气最高压力 L223、
// 燃料电池探针温度 L211、B.6 单体电池电压 L772),此处只覆盖文档定义了哨兵的字段。
//
// 当前库 ValueConverter 的策略是「哨兵原样直通」:解码值 == 原始哨兵数值,
// 而非按 scale/offset 折算。本测试显式固定该口径(库行为锁)——若将来库把哨兵
// 改判为 NaN/特殊语义,这里会失败,提醒同步更新消费侧,而不是静默变化。
func TestDeepV2016SentinelPassthroughLibLock(t *testing.T) {
	veh := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupVehicle: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	motor := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupMotor: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	fuel := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupFuelCell: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	extr := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupExtremum: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	volt := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupVoltage: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	temp := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupTemperature: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	alarm := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupAlarm: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}

	cases := []struct {
		name    string
		cfg     GroupsConfig
		want    float64
		extract func(*mdl.RealTimeData) float64
	}{
		{"整车·车速 WORD 0xFFFF(无效, L165)", veh(RowValue{"speed": 65535.0}), 65535,
			func(d *mdl.RealTimeData) float64 { return d.VehicleData.Speed }},
		{"整车·车速 WORD 0xFFFE(异常, L165)", veh(RowValue{"speed": 65534.0}), 65534,
			func(d *mdl.RealTimeData) float64 { return d.VehicleData.Speed }},
		{"整车·累计里程 DWORD 0xFFFFFFFF(无效, L166)", veh(RowValue{"mileage": 4294967295.0}), 4294967295,
			func(d *mdl.RealTimeData) float64 { return d.VehicleData.Mileage }},
		{"整车·总电流 WORD 0xFFFF(无效, L168)", veh(RowValue{"current": 65535.0}), 65535,
			func(d *mdl.RealTimeData) float64 { return d.VehicleData.Current }},
		{"电机·控制器温度 BYTE 0xFF(无效, L192)", motor(RowValue{"controllerTemp": 255.0}), 255,
			func(d *mdl.RealTimeData) float64 { return d.MotorDataList.Items[0].ControllerTemperature }},
		{"电机·控制器温度 BYTE 0xFE(异常, L192)", motor(RowValue{"controllerTemp": 254.0}), 254,
			func(d *mdl.RealTimeData) float64 { return d.MotorDataList.Items[0].ControllerTemperature }},
		{"电机·电机温度 BYTE 0xFF(无效, L195)", motor(RowValue{"motorTemp": 255.0}), 255,
			func(d *mdl.RealTimeData) float64 { return d.MotorDataList.Items[0].MotorTemperature }},
		{"电机·转速 WORD 0xFFFF(无效, L193)", motor(RowValue{"speed": 65535.0}), 65535,
			func(d *mdl.RealTimeData) float64 { return d.MotorDataList.Items[0].MotorSpeed }},
		{"电机·转矩 WORD 0xFFFF(无效, L194)", motor(RowValue{"torque": 65535.0}), 65535,
			func(d *mdl.RealTimeData) float64 { return d.MotorDataList.Items[0].MotorTorque }},
		{"燃料电池·氢系统最高温度 WORD 0xFFFF(无效, L219)", fuel(RowValue{"hydrogenMaxTemp": 65535.0}), 65535,
			func(d *mdl.RealTimeData) float64 { return d.FuelCellData.HighestTempOfHydrogenSystem }},
		{"极值·单体电压最高值 WORD 0xFFFF(无效, L275)", extr(RowValue{"maxVoltage": 65535.0}), 65535,
			func(d *mdl.RealTimeData) float64 { return d.ExtremumData.MaxVoltage }},
		{"极值·最高温度值 BYTE 0xFF(无效, L281)", extr(RowValue{"maxTemp": 255.0}), 255,
			func(d *mdl.RealTimeData) float64 { return d.ExtremumData.MaxTemperature }},
		{"储能电压·总电流 WORD 0xFFFF(无效, L768)", volt(RowValue{"current": 65535.0, "batteryVoltages": []any{1.0}}), 65535,
			func(d *mdl.RealTimeData) float64 {
				return d.ChargeableSubsystemElectricList.Items[0].Current
			}},
		{"储能温度·探针温度 BYTE 0xFF(无效, L793)", temp(RowValue{"probeTemps": []any{255.0}}), 255,
			func(d *mdl.RealTimeData) float64 {
				return d.ChargeableSubsystemTemperatureList.Items[0].ProbeTemperatures[0]
			}},
		{"报警·最高报警等级 BYTE 0xFF(无效, L294)", alarm(RowValue{"maxAlarmLevel": 255.0}), 255,
			func(d *mdl.RealTimeData) float64 { return float64(d.AlarmData.MaxAlarmLevel) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := AssembleRealtime(tc.cfg, deepAt2016)
			if err != nil {
				t.Fatalf("装配失败: %v", err)
			}
			if got := tc.extract(decodeRealtime(t, body)); got != tc.want {
				t.Errorf("哨兵直通行为已变: got %v, want %v(原始哨兵值)", got, tc.want)
			}
		})
	}
}

// TestDeepV2025SentinelPassthroughLibLock 同 2016 的库行为锁,覆盖 2025 版:
// WORD “0xFF,0xFE”异常/“0xFF,0xFF”无效、DWORD “0xFF,0xFF,0xFF,0xFE”异常/
// “0xFF,0xFF,0xFF,0xFF”无效、BYTE “0xFE”异常/“0xFF”无效,哨兵均按原始值直通
// (docs/standard/2025.md 表10 L184~L191、表12 L208~L211、表14 L229、表16 L247~L250、
// 表17 L259~L272、表19 L289~L300、表23 L340、表25 L405~L410 等)。
// 文档未定义哨兵的数值字段(如经度/纬度 L320/L321、挡位 L190、表26 单体/探针代号 L420/L423/L426/L429)
// 不在本测试覆盖范围。
func TestDeepV2025SentinelPassthroughLibLock(t *testing.T) {
	veh := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupVehicle: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	motor := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupMotor: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	fuel := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupFuelCell: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	mp := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupMinParallel: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	bt := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupBatteryTemp: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	stack := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupFCStack: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	sc := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupSuperCap: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}
	alarm := func(r RowValue) GroupsConfig {
		return GroupsConfig{GroupAlarm: GroupConfig{Enabled: true, Rows: []RowValue{r}}}
	}

	cases := []struct {
		name    string
		cfg     GroupsConfig
		want    float64
		extract func(*mdl25.RealTimeV2025Data) float64
	}{
		{"整车·车速 WORD 0xFFFF(无效, L184)", veh(RowValue{"speed": 65535.0}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.VehicleData.Speed }},
		{"整车·总电流 WORD 0xFFFF(无效, L187)", veh(RowValue{"current": 65535.0}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.VehicleData.Current }},
		{"整车·绝缘电阻 WORD 0xFFFF(无效, L191)", veh(RowValue{"insulance": 65535.0}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return float64(d.VehicleData.Insulance) }},
		{"电机·转速 WORD 0xFFFF(无效, L248)", motor(RowValue{"speed": 65535.0}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.MotorDataList.Items[0].MotorSpeed }},
		{"电机·转矩 DWORD 0xFFFFFFFF(无效, L249)", motor(RowValue{"torque": 4294967295.0}), 4294967295,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.MotorDataList.Items[0].MotorTorque }},
		{"燃料电池·氢系统最高温度 WORD 0xFFFF(无效, L259)", fuel(RowValue{"hydrogenMaxTemp": 65535.0}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.FuelCellData.HighestTempOfHydrogenSystem }},
		{"报警·最高报警等级 BYTE 0xFF(无效, L340)", alarm(RowValue{"maxAlarmLevel": 255.0}), 255,
			func(d *mdl25.RealTimeV2025Data) float64 { return float64(d.AlarmData.MaxAlarmLevel) }},
		{"最小并联·电池包电压 WORD 0xFFFF(无效, L208)", mp(RowValue{"voltage": 65535.0, "batteryVoltages": []any{1.0}}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.MinParallelCellVoltages.Items[0].Voltage }},
		{"最小并联·单体电压 WORD 0xFFFF(无效, L211)", mp(RowValue{"batteryVoltages": []any{65535.0}}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.MinParallelCellVoltages.Items[0].BatteryVoltages[0] }},
		{"电池包温度·探针 BYTE 0xFF(无效, L229)", bt(RowValue{"probeTemps": []any{255.0}}), 255,
			func(d *mdl25.RealTimeV2025Data) float64 {
				return d.BatteryPackTemperatures.Items[0].ProbeTemperatures[0]
			}},
		{"电堆·空气入口温度 BYTE 0xFF(无效, L298)", stack(RowValue{"airInletTemp": 255.0, "coolingWaterTemps": []any{1.0}}), 255,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.FuelCellStackDataList.Items[0].AirInletTemp }},
		{"超级电容·总电压 WORD 0xFFFF(无效, L405)", sc(RowValue{
			"totalVoltage":      65535.0,
			"capacitorVoltages": []any{1.0},
			"probeTemperatures": []any{1.0},
		}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.SuperCapacitorData.TotalVoltage }},
		{"超级电容·单体电压 WORD 0xFFFF(无效, L408)", sc(RowValue{
			"capacitorVoltages": []any{65535.0},
			"probeTemperatures": []any{1.0},
		}), 65535,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.SuperCapacitorData.CapacitorVoltages[0] }},
		{"超级电容·探针温度 BYTE 0xFF(无效, L410)", sc(RowValue{
			"capacitorVoltages": []any{1.0},
			"probeTemperatures": []any{255.0},
		}), 255,
			func(d *mdl25.RealTimeV2025Data) float64 { return d.SuperCapacitorData.ProbeTemperatures[0] }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := AssembleRealtimeV2025(tc.cfg, deepAt2025)
			if err != nil {
				t.Fatalf("装配失败: %v", err)
			}
			if got := tc.extract(decodeV2025Realtime(t, body)); got != tc.want {
				t.Errorf("哨兵直通行为已变: got %v, want %v(原始哨兵值)", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------- 2016 拆帧完整性

// TestDeepV2016VoltageFrameSplitIntegrity 覆盖表 B.6「本帧单体电池总数 m 有效值
// 1~200(L771),当本帧单体个数超过 200 时,应拆分成多帧数据进行传输;本帧起始电池序号
// 1~65531(L770)、单体电池总数 1~65531(L769)、单体电池电压 0~60000(L772)」的拆帧边界:
// 200(整帧不拆)/201/400/401(且起始序号非 1),以及多子系统各行独立拆帧;
// 并对每帧做 装配 → 编码 → 解码 的逐值校验。
func TestDeepV2016VoltageFrameSplitIntegrity(t *testing.T) {
	// cellV 生成 0.001 V 分辨率的整毫伏电压,便于逐值精确比较。
	cellV := func(i int) float64 { return float64(3000+i) / 1000 }
	cells := func(n, base int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = cellV(base + i)
		}
		return out
	}

	cases := []struct {
		name       string
		startSeq   int
		total      int
		wantCounts []int
		wantStarts []int
	}{
		{"200 单体整帧不拆", 1, 200, []int{200}, []int{1}},
		{"201 单体拆 200+1", 1, 201, []int{200, 1}, []int{1, 201}},
		{"400 单体拆 200+200", 1, 400, []int{200, 200}, []int{1, 201}},
		{"401 单体拆 200+200+1 且起始序号非 1", 101, 401, []int{200, 200, 1}, []int{101, 301, 501}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GroupsConfig{GroupVoltage: GroupConfig{Enabled: true, Rows: []RowValue{{
				"subsystem": 7, "voltage": 512.3, "current": -123.4,
				"batteryTotal": tc.total, "frameStartSeq": tc.startSeq,
				"batteryVoltages": cells(tc.total, 0),
			}}}}
			body, err := AssembleRealtime(cfg, deepAt2016)
			if err != nil {
				t.Fatalf("装配失败: %v", err)
			}

			type frame struct {
				count, start int
				vals         []float64
			}
			assertFrames := func(label string, items []frame) {
				t.Helper()
				if len(items) != len(tc.wantCounts) {
					t.Fatalf("%s 帧数 = %d, want %d", label, len(items), len(tc.wantCounts))
				}
				offset := 0
				for i, it := range items {
					if it.count != tc.wantCounts[i] || it.start != tc.wantStarts[i] {
						t.Errorf("%s 帧%d = count:%d start:%d, want count:%d start:%d",
							label, i, it.count, it.start, tc.wantCounts[i], tc.wantStarts[i])
					}
					want := make([]float64, 0, tc.wantCounts[i])
					for j := offset; j < offset+tc.wantCounts[i]; j++ {
						want = append(want, cellV(j))
					}
					deepSameFloats(t, fmt.Sprintf("%s 帧%d 单体电压", label, i), it.vals, want)
					offset += it.count
				}
			}

			l := body.ChargeableSubsystemElectricList
			if l.ElectricCount != len(tc.wantCounts) {
				t.Errorf("ElectricCount = %d, want %d", l.ElectricCount, len(tc.wantCounts))
			}
			frames := make([]frame, len(l.Items))
			for i, it := range l.Items {
				if it.ChargeableSubSystemNumber != 7 || it.BatteryTotalCount != tc.total {
					t.Errorf("帧%d 子系统号/单体总数 = %d/%d, want 7/%d",
						i, it.ChargeableSubSystemNumber, it.BatteryTotalCount, tc.total)
				}
				if it.Voltage != 512.3 || it.Current != -123.4 {
					t.Errorf("帧%d 电压/电流 = %v/%v", i, it.Voltage, it.Current)
				}
				frames[i] = frame{it.BatteryCount, it.FrameStartBatterySeq, it.BatteryVoltages}
			}
			assertFrames("装配", frames)

			// 整帧编解码闭环:线上按个数逐帧读取,拆帧后每帧的单体电压必须完整保持
			d := decodeRealtime(t, body)
			dl := d.ChargeableSubsystemElectricList
			if dl.ElectricCount != len(tc.wantCounts) || len(dl.Items) != len(tc.wantCounts) {
				t.Fatalf("解码帧数 = count:%d items:%d, want %d", dl.ElectricCount, len(dl.Items), len(tc.wantCounts))
			}
			dframes := make([]frame, len(dl.Items))
			for i, it := range dl.Items {
				dframes[i] = frame{it.BatteryCount, it.FrameStartBatterySeq, it.BatteryVoltages}
			}
			assertFrames("解码", dframes)
		})
	}

	t.Run("多子系统各行独立拆帧", func(t *testing.T) {
		cfg := GroupsConfig{GroupVoltage: GroupConfig{Enabled: true, Rows: []RowValue{
			{"subsystem": 1, "voltage": 500, "current": 20, "batteryTotal": 250, "frameStartSeq": 1,
				"batteryVoltages": cells(250, 0)},
			{"subsystem": 2, "voltage": 100, "current": -10, "batteryTotal": 3, "frameStartSeq": 7,
				"batteryVoltages": cells(3, 1000)},
		}}}
		body, err := AssembleRealtime(cfg, deepAt2016)
		if err != nil {
			t.Fatalf("装配失败: %v", err)
		}
		l := body.ChargeableSubsystemElectricList
		if l.ElectricCount != 3 || len(l.Items) != 3 {
			t.Fatalf("拆帧后条目 = count:%d items:%d, want 3", l.ElectricCount, len(l.Items))
		}
		want := []struct {
			sub, count, start, total int
		}{
			{1, 200, 1, 250},
			{1, 50, 201, 250},
			{2, 3, 7, 3},
		}
		for i, w := range want {
			it := l.Items[i]
			if it.ChargeableSubSystemNumber != w.sub || it.BatteryCount != w.count ||
				it.FrameStartBatterySeq != w.start || it.BatteryTotalCount != w.total {
				t.Errorf("条目%d = sub:%d count:%d start:%d total:%d, want %+v",
					i, it.ChargeableSubSystemNumber, it.BatteryCount, it.FrameStartBatterySeq, it.BatteryTotalCount, w)
			}
		}
		// 子系统 2 起始序号独立(7),不受子系统 1 拆帧计数影响
		d := decodeRealtime(t, body)
		dl := d.ChargeableSubsystemElectricList
		if dl.ElectricCount != 3 || len(dl.Items) != 3 {
			t.Fatalf("解码条目 = count:%d items:%d, want 3", dl.ElectricCount, len(dl.Items))
		}
		last := dl.Items[2]
		if last.ChargeableSubSystemNumber != 2 || last.FrameStartBatterySeq != 7 ||
			len(last.BatteryVoltages) != 3 || last.BatteryVoltages[0] != cellV(1000) {
			t.Errorf("解码子系统 2 = %+v", last)
		}
	})
}

// ---------------------------------------------------------------- V2025 经纬度舍入(回归)

// TestDeepV2025LocationRounding 锁定 V2025 经纬度「四舍五入到百万分之一度」的编码行为(H4 修复)。
// docs/standard/2025.md:320/321(表21):经度/纬度 DWORD,度值 ×10^6,精确到百万分之一度。
// 取值使其 ×10^6 后的原始整数为 128000003/16000002/249——旧截断实现(int64(v × 10^6))
// 会各丢 1 LSB(128000002/16000001/248);装配→库编码→库解码后必须与输入逐位相等,
// 若退回截断(128.000003 → 128.000002 等),本测试即失败。
func TestDeepV2025LocationRounding(t *testing.T) {
	cases := []struct {
		name     string
		lon, lat float64
	}{
		{"大值(原始整数 128000003/16000002)", 128.000003, 16.000002},
		{"小值(原始整数 249)", 0.000249, 0.000249},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GroupsConfig{GroupLocation: GroupConfig{Enabled: true, Rows: []RowValue{{
				"valid": true, "coordinateSystem": 1,
				"longitude": tc.lon, "latitude": tc.lat,
			}}}}
			body, err := AssembleRealtimeV2025(cfg, deepAt2025)
			if err != nil {
				t.Fatalf("装配失败: %v", err)
			}
			got := decodeV2025Realtime(t, body).LocationData
			if got == nil {
				t.Fatal("位置组已启用但解码结果为空")
			}
			if got.OriginLongitude != tc.lon || got.OriginLatitude != tc.lat {
				t.Errorf("经纬度 = %v/%v, want %v/%v(截断编码回归)",
					got.OriginLongitude, got.OriginLatitude, tc.lon, tc.lat)
			}
		})
	}
}
