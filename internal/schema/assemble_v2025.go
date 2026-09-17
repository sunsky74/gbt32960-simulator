package schema

import (
	"fmt"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	mdlrt16 "github.com/sunsky74/gb32960/model/gbt2016/realtime"
	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	mdlrt "github.com/sunsky74/gb32960/model/gbt2025/realtime"
	"github.com/sunsky74/gb32960/types"
)

// Assemble 按协议版本分发组装实时报文体。
func Assemble(version api.GBTVersion, cfg GroupsConfig, at time.Time) (model.MessageBody, error) {
	if version == api.V2025 {
		return AssembleRealtimeV2025(cfg, at)
	}
	return AssembleRealtime(cfg, at)
}

// AssembleRealtimeV2025 把配置值组装为 V2025 实时报文体。
func AssembleRealtimeV2025(cfg GroupsConfig, at time.Time) (*mdl.RealTimeV2025Data, error) {
	m := &mdl.RealTimeV2025Data{
		BeanTime: model.BeanTime{
			Year: at.Year() - 2000, Month: int(at.Month()), Day: at.Day(),
			Hour: at.Hour(), Minute: at.Minute(), Second: at.Second(),
		},
	}

	if g, ok := cfg[GroupVehicle]; ok && g.Enabled && len(g.Rows) > 0 {
		v, err := assembleVehicleV2025(g.Rows[0])
		if err != nil {
			return nil, fmt.Errorf("整车数据: %w", err)
		}
		m.VehicleData = v
	}
	if g, ok := cfg[GroupMotor]; ok && g.Enabled {
		l, err := assembleMotorsV2025(g.Rows)
		if err != nil {
			return nil, fmt.Errorf("驱动电机: %w", err)
		}
		m.MotorDataList = l
	}
	if g, ok := cfg[GroupFuelCell]; ok && g.Enabled && len(g.Rows) > 0 {
		m.FuelCellData = &mdlrt.FuelCellEngineV2025Data{
			HighestTempOfHydrogenSystem:          getFloat(g.Rows[0], "hydrogenMaxTemp", 0),
			HighestTempProbeCodeOfHydrogenSystem: getInt(g.Rows[0], "hydrogenMaxTempProbe", 0),
			HighestConOfHydrogen:                 getFloat(g.Rows[0], "hydrogenMaxCon", 0),
			HighestHyConSensorCode:               getInt(g.Rows[0], "hydrogenMaxConSensor", 0),
			HydrogenMaxPressure:                  getFloat(g.Rows[0], "hydrogenMaxPressure", 0),
			HydrogenMaxPressureSensorCode:        getInt(g.Rows[0], "hydrogenMaxPressureSensor", 0),
			HighVoltageDCState:                   byte(getInt(g.Rows[0], "highVoltageDC", 1)),
			FuelPercentage:                       getInt(g.Rows[0], "fuelPercentage", 0),
			DCControllerTemperature:              getFloat(g.Rows[0], "dcControllerTemp", 0),
		}
	}
	if g, ok := cfg[GroupEngine]; ok && g.Enabled && len(g.Rows) > 0 {
		m.EngineData = &mdlrt.EngineV2025Data{
			CrankshaftSpeed: getInt(g.Rows[0], "crankshaftSpeed", 0),
		}
	}
	if g, ok := cfg[GroupLocation]; ok && g.Enabled && len(g.Rows) > 0 {
		lon := getFloat(g.Rows[0], "longitude", 0)
		lat := getFloat(g.Rows[0], "latitude", 0)
		m.LocationData = &mdlrt.LocationV2025Data{
			Valid:        getBool(g.Rows[0], "valid", true),
			NorthernFlag: lat >= 0,
			EastFlag:     lon >= 0,
			// 表21:坐标系 0x01 WGS84 / 0x02 GCJ02 / 0x03 其他;未配置默认 WGS84
			CoordinateType:   byte(getInt(g.Rows[0], "coordinateSystem", 1)),
			OriginLongitude:  lon,
			OriginLatitude:   lat,
			ConvertLongitude: lon,
			ConvertLatitude:  lat,
		}
	}
	if g, ok := cfg[GroupAlarm]; ok && g.Enabled && len(g.Rows) > 0 {
		a, err := assembleAlarmV2025(g.Rows[0])
		if err != nil {
			return nil, fmt.Errorf("报警数据: %w", err)
		}
		m.AlarmData = a
	}
	if g, ok := cfg[GroupMinParallel]; ok && g.Enabled {
		l, err := assembleMinParallel(g.Rows)
		if err != nil {
			return nil, fmt.Errorf("最小并联电压: %w", err)
		}
		m.MinParallelCellVoltages = l
	}
	if g, ok := cfg[GroupBatteryTemp]; ok && g.Enabled {
		l, err := assembleBatteryTemp(g.Rows)
		if err != nil {
			return nil, fmt.Errorf("电池包温度: %w", err)
		}
		m.BatteryPackTemperatures = l
	}
	if g, ok := cfg[GroupFCStack]; ok && g.Enabled {
		// 表18:燃料电池电堆个数 1~253
		if len(g.Rows) == 0 {
			return nil, fmt.Errorf("燃料电池电堆: 至少需要一行数据")
		}
		if len(g.Rows) > 253 {
			return nil, fmt.Errorf("燃料电池电堆个数超限: %d (1~253)", len(g.Rows))
		}
		l := &mdlrt.FuelCellStackDataList{StackCount: len(g.Rows)}
		for _, r := range g.Rows {
			l.Items = append(l.Items, mdlrt.FuelCellStackData{
				StackSeq:               getInt(r, "stackSeq", 1),
				Voltage:                getFloat(r, "voltage", 0),
				Current:                getFloat(r, "current", 0),
				GasPressure:            getFloat(r, "gasPressure", 0),
				AirPressure:            getFloat(r, "airPressure", 0),
				AirInletTemp:           getFloat(r, "airInletTemp", 0),
				CoolingWaterProbeCount: 0,
				CoolingWaterTemps:      getFloatArray(r, "coolingWaterTemps"),
			})
		}
		for i := range l.Items {
			l.Items[i].CoolingWaterProbeCount = len(l.Items[i].CoolingWaterTemps)
		}
		m.FuelCellStackDataList = l
	}
	if g, ok := cfg[GroupSuperCap]; ok && g.Enabled && len(g.Rows) > 0 {
		m.SuperCapacitorData = &mdlrt.SuperCapacitorData{
			ManagementSystemNumber: getInt(g.Rows[0], "managementSystemNumber", 1),
			TotalVoltage:           getFloat(g.Rows[0], "totalVoltage", 0),
			TotalCurrent:           getFloat(g.Rows[0], "totalCurrent", 0),
			CapacitorCount:         0,
			CapacitorVoltages:      getFloatArray(g.Rows[0], "capacitorVoltages"),
			ProbeTemperatures:      getFloatArray(g.Rows[0], "probeTemperatures"),
		}
		m.SuperCapacitorData.CapacitorCount = len(m.SuperCapacitorData.CapacitorVoltages)
		m.SuperCapacitorData.TemperatureProbeCount = len(m.SuperCapacitorData.ProbeTemperatures)
		// 表25:单体总数与温度探针总数 1~65531,总数即数组长度
		if m.SuperCapacitorData.CapacitorCount == 0 {
			return nil, fmt.Errorf("超级电容单体电压为空 (单体总数需 1~65531)")
		}
		if m.SuperCapacitorData.TemperatureProbeCount == 0 {
			return nil, fmt.Errorf("超级电容温度探针为空 (探针总数需 1~65531)")
		}
	}
	if g, ok := cfg[GroupSuperCapExtremum]; ok && g.Enabled && len(g.Rows) > 0 {
		m.SuperCapacitorExtremumData = &mdlrt.SuperCapacitorExtremumData{
			VoltageMaxSubsystem:     getInt(g.Rows[0], "voltageMaxSubsystem", 0),
			VoltageMaxBattery:       getInt(g.Rows[0], "voltageMaxBattery", 0),
			MaxVoltage:              getFloat(g.Rows[0], "maxVoltage", 0),
			VoltageMinSubsystem:     getInt(g.Rows[0], "voltageMinSubsystem", 0),
			VoltageMinBattery:       getInt(g.Rows[0], "voltageMinBattery", 0),
			MinVoltage:              getFloat(g.Rows[0], "minVoltage", 0),
			TemperatureMaxSubsystem: getInt(g.Rows[0], "tempMaxSubsystem", 0),
			TemperatureMaxProbe:     getInt(g.Rows[0], "tempMaxProbe", 0),
			MaxTemperature:          getFloat(g.Rows[0], "maxTemp", 0),
			TemperatureMinSubsystem: getInt(g.Rows[0], "tempMinSubsystem", 0),
			TemperatureMinProbe:     getInt(g.Rows[0], "tempMinProbe", 0),
			MinTemperature:          getFloat(g.Rows[0], "minTemp", 0),
		}
	}
	return m, nil
}

func assembleVehicleV2025(r RowValue) (*mdlrt16.VehicleData, error) {
	// 附录 A.1:挡位字节 bit3~0 为挡位码
	// (0x0 空挡,0x1~0x6 = 1~6 挡,0xD 倒挡,0xE 自动D,0xF 停车P),
	// bit4 制动力,bit5 驱动力,bit6 预留(恒 0),bit7 挡位无效(1=无效);
	// gearEnum 的枚举值即挡位码,此处校验合法集合。
	gearCode := getInt(r, "gear", 1)
	switch gearCode {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x0D, 0x0E, 0x0F:
	default:
		return nil, fmt.Errorf("档位取值非法: 0x%02X", gearCode)
	}
	var origin byte
	if getBool(r, "gearInvalid", false) {
		origin |= 1 << 7
	}
	if getBool(r, "drivingForce", false) {
		origin |= 1 << 5
	}
	if getBool(r, "brakingForce", false) {
		origin |= 1 << 4
	}
	origin |= byte(gearCode) & 0x0F

	soc := getInt(r, "soc", 0)
	if soc < 0 || soc > 100 {
		return nil, fmt.Errorf("SOC 超范围: %d (0~100)", soc)
	}

	return &mdlrt16.VehicleData{
		OperatingState: types.OperatingState(getInt(r, "operatingState", 1)),
		ChargingState:  types.ChargingState(getInt(r, "chargingState", 1)),
		OperationMode:  types.OperationMode(getInt(r, "operationMode", 1)),
		Speed:          getFloat(r, "speed", 0),
		Mileage:        getFloat(r, "mileage", 0),
		Voltage:        getFloat(r, "voltage", 0),
		Current:        getFloat(r, "current", 0),
		SOC:            soc,
		DC:             types.DCState(getInt(r, "dc", 1)),
		GearPosition:   mdlrt16.GearPosition{Origin: origin, GP: types.GearPositionEnum(gearCode)},
		// 表10:高压对地绝缘电阻(WORD,0~60000 kΩ)
		Insulance: getInt(r, "insulance", 0),
	}, nil
}

func assembleMotorsV2025(rows []RowValue) (*mdlrt.MotorDataV2025List, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行电机数据")
	}
	// 表15:驱动电机个数 1~253
	if len(rows) > 253 {
		return nil, fmt.Errorf("驱动电机个数超限: %d (1~253)", len(rows))
	}
	list := &mdlrt.MotorDataV2025List{MotorCount: len(rows)}
	for i, r := range rows {
		list.Items = append(list.Items, mdlrt.MotorDataV2025{
			MotorSeq:              getIntDefault(r, "seq", i+1),
			MotorState:            byte(getInt(r, "state", 1)),
			ControllerTemperature: getFloat(r, "controllerTemp", 0),
			MotorSpeed:            getFloat(r, "speed", 0),
			MotorTorque:           getFloat(r, "torque", 0),
			MotorTemperature:      getFloat(r, "motorTemp", 0),
		})
	}
	return list, nil
}

// v2025AlarmBoolFields bit 序 → AlarmV2025Data 布尔字段名。
// 注意与 2016 的命名差异:SOCLow/SOCHigh/DCTemperature/DCStatus(全大写)。
var v2025AlarmBoolFields = []string{
	"TemperatureDifferential", "BatteryHighTemperature", "DeviceTypeOverVoltage",
	"DeviceTypeUnderVoltage", "SOCLow", "MonomerBatteryOverVoltage", "MonomerBatteryUnderVoltage",
	"SOCHigh", "SOCJump", "DeviceTypeDontMatch", "BatteryConsistencyPoor", "Insulation",
	"DCTemperature", "BrakingSystem", "DCStatus", "DriveMotorControllerTemperature",
	"HighPressureInterlock", "DriveMotorTemperature", "DeviceTypeOverFilling",
	"DriveMotorOverSpeed", "DriveMotorOverCurrent", "SuperCapacitorOverTemp",
	"SuperCapacitorOverVoltage", "DeviceThermalEvent", "HydrogenLeakage",
	"HydrogenPressureAbnormal", "HydrogenTemperatureAbnormal", "FuelCellStackOverTemperature",
}

func assembleAlarmV2025(r RowValue) (*mdlrt.AlarmV2025Data, error) {
	a := &mdlrt.AlarmV2025Data{
		MaxAlarmLevel:     getInt(r, "maxAlarmLevel", 0),
		BatteryFaultDatas: toInt64s(getFloatArray(r, "batteryFaults")),
		MotorFaultDatas:   toInt64s(getFloatArray(r, "motorFaults")),
		EngineFaultDatas:  toInt64s(getFloatArray(r, "engineFaults")),
		OtherFaultDatas:   toInt64s(getFloatArray(r, "otherFaults")),
	}
	a.BatteryFaultNum = len(a.BatteryFaultDatas)
	a.MotorFaultNum = len(a.MotorFaultDatas)
	a.EngineFaultNum = len(a.EngineFaultDatas)
	a.OtherFaultNum = len(a.OtherFaultDatas)
	// 表23:N1~N4 有效值 0~253(0xFE/0xFF 为异常/无效哨兵,不可作为个数上线)
	for _, c := range []struct {
		name string
		n    int
	}{
		{"可充电储能装置故障", a.BatteryFaultNum},
		{"驱动电机故障", a.MotorFaultNum},
		{"发动机故障", a.EngineFaultNum},
		{"其他故障", a.OtherFaultNum},
	} {
		if c.n > 253 {
			return nil, fmt.Errorf("%s总数超限: %d (0~253)", c.name, c.n)
		}
	}

	// 表23(续):通用报警故障等级列表 2×N5 = (标志位序号, 等级) 对;
	// 位序号与等级两数组按下标一一配对合成 N5 条目。
	seqs := getFloatArray(r, "commonAlertSeqs")
	levels := getFloatArray(r, "commonAlertLevels")
	if len(seqs) != len(levels) {
		return nil, fmt.Errorf("通用报警位序号(%d)与等级(%d)数量应一致", len(seqs), len(levels))
	}
	if len(seqs) > 253 {
		return nil, fmt.Errorf("通用报警故障总数超限: %d (0~253)", len(seqs))
	}
	for i := range seqs {
		a.CommonAlertDatas = append(a.CommonAlertDatas, mdlrt.CommonAlertData{
			Seq: int(seqs[i]), Level: int(levels[i]),
		})
	}
	a.CommonAlertNum = len(a.CommonAlertDatas)

	bitsVal, _ := r["bits"].(map[string]any)
	for i, name := range v2025AlarmBoolFields {
		on := false
		if bitsVal != nil {
			on = getBool(bitsVal, fmt.Sprintf("bit%d", i), false)
		}
		if err := setAlarmV2025Bool(a, name, on); err != nil {
			return nil, err
		}
	}
	// AlarmBitIdentify 留 0:codec 检测到 0 时从 28 个布尔字段重建掩码
	return a, nil
}

// setAlarmV2025Bool 反射写入 2025 报警结构的布尔位;未知字段或非布尔字段返回错误。
func setAlarmV2025Bool(a *mdlrt.AlarmV2025Data, field string, on bool) error {
	return setBoolFieldByName(a, field, on)
}

func assembleMinParallel(rows []RowValue) (*mdlrt.MinParallelCellVoltageList, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行电池包数据")
	}
	// 表11:动力蓄电池包个数 0~50
	if len(rows) > 50 {
		return nil, fmt.Errorf("动力蓄电池包个数超限: %d (0~50)", len(rows))
	}
	list := &mdlrt.MinParallelCellVoltageList{BatteryPackCount: len(rows)}
	for _, r := range rows {
		seq := getInt(r, "batteryPackSeq", 1)
		volts := getFloatArray(r, "batteryVoltages")
		// 表12:最小并联单元总数 1~65531,总数即数组长度
		if len(volts) == 0 {
			return nil, fmt.Errorf("电池包 %d 的最小并联单元电压为空 (总数需 1~65531)", seq)
		}
		list.Items = append(list.Items, mdlrt.MinParallelCellVoltage{
			BatteryPackSeq:   seq,
			Voltage:          getFloat(r, "voltage", 0),
			Current:          getFloat(r, "current", 0),
			MinParallelUnits: len(volts),
			BatteryVoltages:  volts,
		})
	}
	return list, nil
}

func assembleBatteryTemp(rows []RowValue) (*mdlrt.BatteryTempList, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行电池包数据")
	}
	// 表13:动力蓄电池包个数 0~50
	if len(rows) > 50 {
		return nil, fmt.Errorf("动力蓄电池包个数超限: %d (0~50)", len(rows))
	}
	list := &mdlrt.BatteryTempList{BatteryPackCount: len(rows)}
	for _, r := range rows {
		seq := getInt(r, "batteryPackSeq", 1)
		probes := getFloatArray(r, "probeTemps")
		// 表14:温度探针个数 1~65531,个数即数组长度
		if len(probes) == 0 {
			return nil, fmt.Errorf("电池包 %d 的温度探针为空 (个数需 1~65531)", seq)
		}
		list.Items = append(list.Items, mdlrt.BatteryTemp{
			BatteryPackSeq:        seq,
			TemperatureProbeCount: len(probes),
			ProbeTemperatures:     probes,
		})
	}
	return list, nil
}
