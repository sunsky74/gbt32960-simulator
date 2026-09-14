package schema

import (
	"fmt"
	"reflect"
	"time"

	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	"github.com/sunsky74/gb32960/model/gbt2016/realtime"
	"github.com/sunsky74/gb32960/types"
)

// GroupConfig 一个数据组配置。单行组一行;多行组(电机/储能)多行。
// Rows 直接用 []map[string]any 而非 RowValue 别名:wails 绑定生成器
// 不为 map 别名生成 TS 类型,别名会令前端 .d.ts 引用悬空。
type GroupConfig struct {
	Enabled bool             `json:"enabled"`
	Rows    []map[string]any `json:"rows"`
}

// RowValue 行字段值的别名(内部代码用,与 map[string]any 同型)。
type RowValue = map[string]any

// NamedGroup 带组标识的配置项(wails 跨语言载荷用切片而非 map:
// 绑定生成器对 map 值类型不生成 TS 类,切片会正常下钻)。
type NamedGroup struct {
	Key     string           `json:"key"`
	Enabled bool             `json:"enabled"`
	Rows    []map[string]any `json:"rows"`
}

// GroupsPayload 报文配置的跨语言载荷形态。
type GroupsPayload struct {
	Groups []NamedGroup `json:"groups"`
}

// ToMap 载荷转内部 map 形态。
func (p *GroupsPayload) ToMap() map[string]GroupConfig {
	if p == nil {
		return nil
	}
	out := make(map[string]GroupConfig, len(p.Groups))
	for _, g := range p.Groups {
		out[g.Key] = GroupConfig{Enabled: g.Enabled, Rows: g.Rows}
	}
	return out
}

// FromMap 内部 map 转载荷。
func FromMap(m map[string]GroupConfig, order []string) *GroupsPayload {
	p := &GroupsPayload{Groups: make([]NamedGroup, 0, len(m))}
	for _, key := range order {
		if g, ok := m[key]; ok {
			p.Groups = append(p.Groups, NamedGroup{Key: key, Enabled: g.Enabled, Rows: g.Rows})
		}
	}
	for key, g := range m {
		known := false
		for _, k := range order {
			if k == key {
				known = true
				break
			}
		}
		if !known {
			p.Groups = append(p.Groups, NamedGroup{Key: key, Enabled: g.Enabled, Rows: g.Rows})
		}
	}
	return p
}

// GroupsConfig 整个 0x02/0x03 数据单元的配置(key = GroupSchema.Key)。
type GroupsConfig map[string]GroupConfig

// AssembleRealtime 把配置值组装为 RealTimeData 报文体。Enabled=false 或缺 key 的组不编入。
func AssembleRealtime(cfg GroupsConfig, at time.Time) (*mdl.RealTimeData, error) {
	m := &mdl.RealTimeData{
		BeanTime: model.BeanTime{
			Year: at.Year() - 2000, Month: int(at.Month()), Day: at.Day(),
			Hour: at.Hour(), Minute: at.Minute(), Second: at.Second(),
		},
	}

	if g, ok := cfg[GroupVehicle]; ok && g.Enabled && len(g.Rows) > 0 {
		v, err := assembleVehicle(g.Rows[0])
		if err != nil {
			return nil, fmt.Errorf("整车数据: %w", err)
		}
		m.VehicleData = v
	}
	if g, ok := cfg[GroupMotor]; ok && g.Enabled {
		l, err := assembleMotors(g.Rows)
		if err != nil {
			return nil, fmt.Errorf("驱动电机: %w", err)
		}
		m.MotorDataList = l
	}
	if g, ok := cfg[GroupFuelCell]; ok && g.Enabled && len(g.Rows) > 0 {
		v, err := assembleFuelCell(g.Rows[0])
		if err != nil {
			return nil, fmt.Errorf("燃料电池: %w", err)
		}
		m.FuelCellData = v
	}
	if g, ok := cfg[GroupEngine]; ok && g.Enabled && len(g.Rows) > 0 {
		m.EngineData = &realtime.EngineData{
			EngineState:         byte(getInt(g.Rows[0], "state", 1)),
			CrankshaftSpeed:     getInt(g.Rows[0], "crankshaftSpeed", 0),
			FuelConsumptionRate: getFloat(g.Rows[0], "consumption", 0),
		}
	}
	if g, ok := cfg[GroupLocation]; ok && g.Enabled && len(g.Rows) > 0 {
		m.LocationData = &realtime.LocationData{
			Valid:     getBool(g.Rows[0], "valid", true),
			Longitude: getFloat(g.Rows[0], "longitude", 0),
			Latitude:  getFloat(g.Rows[0], "latitude", 0),
		}
	}
	if g, ok := cfg[GroupExtremum]; ok && g.Enabled && len(g.Rows) > 0 {
		m.ExtremumData = assembleExtremum(g.Rows[0])
	}
	if g, ok := cfg[GroupAlarm]; ok && g.Enabled && len(g.Rows) > 0 {
		a, err := assembleAlarm(g.Rows[0])
		if err != nil {
			return nil, fmt.Errorf("报警数据: %w", err)
		}
		m.AlarmData = a
	}
	if g, ok := cfg[GroupVoltage]; ok && g.Enabled {
		l, err := assembleVoltageList(g.Rows)
		if err != nil {
			return nil, fmt.Errorf("储能电压: %w", err)
		}
		m.ChargeableSubsystemElectricList = l
	}
	if g, ok := cfg[GroupTemperature]; ok && g.Enabled {
		l, err := assembleTemperatureList(g.Rows)
		if err != nil {
			return nil, fmt.Errorf("储能温度: %w", err)
		}
		m.ChargeableSubsystemTemperatureList = l
	}
	return m, nil
}

func assembleVehicle(r RowValue) (*realtime.VehicleData, error) {
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

	return &realtime.VehicleData{
		OperatingState:      types.OperatingState(getInt(r, "operatingState", 1)),
		ChargingState:       types.ChargingState(getInt(r, "chargingState", 1)),
		OperationMode:       types.OperationMode(getInt(r, "operationMode", 1)),
		Speed:               getFloat(r, "speed", 0),
		Mileage:             getFloat(r, "mileage", 0),
		Voltage:             getFloat(r, "voltage", 0),
		Current:             getFloat(r, "current", 0),
		SOC:                 soc,
		DC:                  types.DCState(getInt(r, "dc", 1)),
		GearPosition:        realtime.GearPosition{Origin: origin, GP: types.GearPositionEnum(gearCode)},
		Insulance:           getInt(r, "insulance", 0),
		AccelerationValue:   getInt(r, "accelerationValue", 0),
		BrakePedalCondition: getInt(r, "brakePedal", 0),
	}, nil
}

func assembleMotors(rows []RowValue) (*realtime.MotorDataList, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行电机数据")
	}
	list := &realtime.MotorDataList{Count: len(rows), Items: make([]realtime.MotorData, 0, len(rows))}
	for i, r := range rows {
		list.Items = append(list.Items, realtime.MotorData{
			MotorSeq:              getIntDefault(r, "seq", i+1),
			MotorState:            byte(getInt(r, "state", 1)),
			ControllerTemperature: getFloat(r, "controllerTemp", 0),
			MotorSpeed:            getFloat(r, "speed", 0),
			MotorTorque:           getFloat(r, "torque", 0),
			MotorTemperature:      getFloat(r, "motorTemp", 0),
			ControllerVoltage:     getFloat(r, "controllerVoltage", 0),
			ControllerCurrent:     getFloat(r, "controllerCurrent", 0),
		})
	}
	return list, nil
}

func assembleFuelCell(r RowValue) (*realtime.FuelCellData, error) {
	probes := getFloatArray(r, "probeTemps")
	return &realtime.FuelCellData{
		FuelCellVoltage:                      getFloat(r, "voltage", 0),
		FuelCellCurrent:                      getFloat(r, "current", 0),
		FuelConsumptionRate:                  getFloat(r, "consumption", 0),
		TotalNumberOfFcTp:                    len(probes),
		ProbeTemperatureValues:               probes,
		HighestTempOfHydrogenSystem:          getFloat(r, "hydrogenMaxTemp", 0),
		HighestTempProbeCodeOfHydrogenSystem: getInt(r, "hydrogenMaxTempProbe", 0),
		HighestConOfHydrogen:                 getInt(r, "hydrogenMaxCon", 0),
		HighestHyConSensorCode:               getInt(r, "hydrogenMaxConSensor", 0),
		HydrogenMaxPressure:                  getFloat(r, "hydrogenMaxPressure", 0),
		HydrogenMaxPressureSensorCode:        getInt(r, "hydrogenMaxPressureSensor", 0),
		HighVoltageDCState:                   byte(getInt(r, "highVoltageDC", 1)),
	}, nil
}

func assembleExtremum(r RowValue) *realtime.ExtremumData {
	return &realtime.ExtremumData{
		VoltageMaxSubsystem:     getInt(r, "voltageMaxSubsystem", 0),
		VoltageMaxBattery:       getInt(r, "voltageMaxBattery", 0),
		MaxVoltage:              getFloat(r, "maxVoltage", 0),
		VoltageMinSubsystem:     getInt(r, "voltageMinSubsystem", 0),
		VoltageMinBattery:       getInt(r, "voltageMinBattery", 0),
		MinVoltage:              getFloat(r, "minVoltage", 0),
		TemperatureMaxSubsystem: getInt(r, "tempMaxSubsystem", 0),
		TemperatureMaxProbe:     getInt(r, "tempMaxProbe", 0),
		MaxTemperature:          getFloat(r, "maxTemp", 0),
		TemperatureMinSubsystem: getInt(r, "tempMinSubsystem", 0),
		TemperatureMinProbe:     getInt(r, "tempMinProbe", 0),
		MinTemperature:          getFloat(r, "minTemp", 0),
	}
}

// alarmBoolFields bit 序 → AlarmData 布尔字段名(与 alarmBitLabels 同序)。
var alarmBoolFields = []string{
	"TemperatureDifferential", "BatteryHighTemperature", "DeviceTypeOverVoltage",
	"DeviceTypeUnderVoltage", "SocLow", "MonomerBatteryOverVoltage", "MonomerBatteryUnderVoltage",
	"SocHigh", "SocJump", "DeviceTypeDontMatch", "BatteryConsistencyPoor", "Insulation",
	"DcTemperature", "BrakingSystem", "DcStatus", "DriveMotorControllerTemperature",
	"HighPressureInterlock", "DriveMotorTemperature", "DeviceTypeOverFilling",
}

func assembleAlarm(r RowValue) (*realtime.AlarmData, error) {
	a := &realtime.AlarmData{
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

	var mask int64
	bitsVal, _ := r["bits"].(map[string]any)
	for i, name := range alarmBoolFields {
		on := false
		if bitsVal != nil {
			on = getBool(bitsVal, fmt.Sprintf("bit%d", i), false)
		}
		if on {
			mask |= 1 << i
		}
		if err := setAlarmBool(a, name, on); err != nil {
			return nil, err
		}
	}
	a.AlarmBitIdentify = mask
	return a, nil
}

// setAlarmBool 反射写入 2016 报警结构的布尔位;未知字段或非布尔字段返回错误。
func setAlarmBool(a *realtime.AlarmData, field string, on bool) error {
	return setBoolFieldByName(a, field, on)
}

// setBoolFieldByName 反射按字段名写入 bool:字段不存在或类型非 bool 返回错误。
// 两版报警结构字段名不同(2025 为 SOCLow/DCStatus 等),此处统一写入口径。
func setBoolFieldByName(target any, field string, on bool) error {
	fv := reflect.ValueOf(target).Elem().FieldByName(field)
	if !fv.IsValid() {
		return fmt.Errorf("未知报警位字段: %s", field)
	}
	if fv.Kind() != reflect.Bool {
		return fmt.Errorf("报警位字段类型非布尔: %s", field)
	}
	fv.SetBool(on)
	return nil
}

func assembleVoltageList(rows []RowValue) (*realtime.ChargeableSubsystemElectricList, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行储能电压数据")
	}
	list := &realtime.ChargeableSubsystemElectricList{ElectricCount: len(rows)}
	for _, r := range rows {
		list.Items = append(list.Items, realtime.ChargeableSubsystemElectric{
			ChargeableSubSystemNumber: getInt(r, "subsystem", 1),
			Voltage:                   getFloat(r, "voltage", 0),
			Current:                   getFloat(r, "current", 0),
			BatteryTotalCount:         getIntDefault(r, "batteryTotal", 0),
			FrameStartBatterySeq:      getIntDefault(r, "frameStartSeq", 1),
			BatteryCount:              0,
			BatteryVoltages:           getFloatArray(r, "batteryVoltages"),
		})
	}
	for i := range list.Items {
		list.Items[i].BatteryCount = len(list.Items[i].BatteryVoltages)
	}
	return list, nil
}

func assembleTemperatureList(rows []RowValue) (*realtime.ChargeableSubsystemTemperatureList, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("至少需要一行储能温度数据")
	}
	list := &realtime.ChargeableSubsystemTemperatureList{TemperatureCount: len(rows)}
	for _, r := range rows {
		list.Items = append(list.Items, realtime.ChargeableSubsystemTemperature{
			SubSystemNumber:       getInt(r, "subsystem", 1),
			TemperatureProbeCount: 0,
			ProbeTemperatures:     getFloatArray(r, "probeTemps"),
		})
	}
	for i := range list.Items {
		list.Items[i].TemperatureProbeCount = len(list.Items[i].ProbeTemperatures)
	}
	return list, nil
}

// ---------------------------------------------------------------- 取值工具(JSON 宽松解码)

func getFloat(r map[string]any, key string, def float64) float64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case string:
			var f float64
			if _, err := fmt.Sscanf(n, "%g", &f); err == nil {
				return f
			}
		case bool:
			if n {
				return 1
			}
		}
	}
	return def
}

func getInt(r map[string]any, key string, def int) int {
	return int(getFloat(r, key, float64(def)))
}

func getIntDefault(r map[string]any, key string, def int) int {
	if _, ok := r[key]; !ok {
		return def
	}
	return getInt(r, key, def)
}

func getBool(r map[string]any, key string, def bool) bool {
	if v, ok := r[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

func getFloatArray(r map[string]any, key string) []float64 {
	arr, ok := r[key].([]any)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		out = append(out, getFloat(map[string]any{"v": v}, "v", 0))
	}
	return out
}

func toInt64s(fs []float64) []int64 {
	if len(fs) == 0 {
		return nil
	}
	out := make([]int64, len(fs))
	for i, f := range fs {
		out[i] = int64(f)
	}
	return out
}
