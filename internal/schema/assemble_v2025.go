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
			Valid:            getBool(g.Rows[0], "valid", true),
			NorthernFlag:     lat >= 0,
			EastFlag:         lon >= 0,
			CoordinateType:   0,
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
	gearCode := getInt(r, "gear", 1)
	if gearCode < 1 || gearCode > 5 {
		return nil, fmt.Errorf("档位取值非法: %d", gearCode)
	}
	var origin byte
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
	}, nil
}

func assembleMotorsV2025(rows []RowValue) (*mdlrt.MotorDataV2025List, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行电机数据")
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
	list := &mdlrt.MinParallelCellVoltageList{BatteryPackCount: len(rows)}
	for _, r := range rows {
		volts := getFloatArray(r, "batteryVoltages")
		list.Items = append(list.Items, mdlrt.MinParallelCellVoltage{
			BatteryPackSeq:   getInt(r, "batteryPackSeq", 1),
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
	list := &mdlrt.BatteryTempList{BatteryPackCount: len(rows)}
	for _, r := range rows {
		probes := getFloatArray(r, "probeTemps")
		list.Items = append(list.Items, mdlrt.BatteryTemp{
			BatteryPackSeq:        getInt(r, "batteryPackSeq", 1),
			TemperatureProbeCount: len(probes),
			ProbeTemperatures:     probes,
		})
	}
	return list, nil
}
