// Package schema 定义报文配置的「组-字段」元数据,并提供从配置值组装
// 协议报文体的装配器。schema 由 Go 输出、前端动态渲染,加字段只改 Go。
package schema

// GroupSchema 一个实时数据组(对应 0x02/0x03 数据单元的一个 TLV 子记录)。
type GroupSchema struct {
	Key      string        `json:"key"`      // 组标识,如 "vehicle"
	Title    string        `json:"title"`    // 中文名,如 "整车数据"
	Enabled  bool          `json:"enabled"`  // 默认是否勾选编入报文
	Multiple bool          `json:"multiple"` // true=多行可增删(电机/储能电压/储能温度)
	MaxRows  int           `json:"maxRows,omitempty"`
	Fields   []FieldSchema `json:"fields"`
}

// FieldSchema 单个字段元数据。
type FieldSchema struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Kind      string     `json:"kind"` // int|float|bool|enum|array_float
	Unit      string     `json:"unit,omitempty"`
	Min       *float64   `json:"min,omitempty"`
	Max       *float64   `json:"max,omitempty"`
	Enum      []EnumDef  `json:"enum,omitempty"`  // kind=enum
	Bits      []BitDef   `json:"bits,omitempty"`  // 预留:位域展示
	ItemLabel string     `json:"itemLabel,omitempty"` // kind=array_float 的元素名
	ScaleNote string     `json:"scaleNote,omitempty"` // 换算说明,如 "×0.1 km/h"
}

// EnumDef 枚举选项。
type EnumDef struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

// BitDef 位定义(报警标志等)。
type BitDef struct {
	Index int    `json:"index"`
	Label string `json:"label"`
}

func ptr(v float64) *float64 { return &v }

func enum(defs ...EnumDef) []EnumDef { return defs }

// 车辆/充电/运行模式/DC 的通用枚举(GB/T 32960.3-2016 表 8~11)
var (
	opStateEnum = enum(
		EnumDef{1, "启动"}, EnumDef{2, "关闭"}, EnumDef{3, "其他"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	chargeEnum = enum(
		EnumDef{1, "未充电"}, EnumDef{2, "充电中"}, EnumDef{3, "充电完成"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	modeEnum = enum(
		EnumDef{1, "纯电"}, EnumDef{2, "混动"}, EnumDef{3, "燃油"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	dcEnum = enum(
		EnumDef{1, "工作"}, EnumDef{2, "断开"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	gearEnum = enum(
		EnumDef{0x01, "P"}, EnumDef{0x02, "R"}, EnumDef{0x03, "N"},
		EnumDef{0x04, "D"}, EnumDef{0x05, "其他"},
	)
	motorStateEnum = enum(
		EnumDef{1, "驱动"}, EnumDef{2, "发电"}, EnumDef{3, "停机"}, EnumDef{4, "准备"},
	)
	onOffEnum = enum(EnumDef{1, "启动/工作"}, EnumDef{2, "关闭/断开"})
)

// V2016Groups 返回 2016 版实时数据的全部组定义。
func V2016Groups() []GroupSchema {
	return []GroupSchema{
		{
			Key: "vehicle", Title: "整车数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "operatingState", Label: "车辆状态", Kind: "enum", Enum: opStateEnum},
				{Key: "chargingState", Label: "充电状态", Kind: "enum", Enum: chargeEnum},
				{Key: "operationMode", Label: "运行模式", Kind: "enum", Enum: modeEnum},
				{Key: "speed", Label: "车速", Kind: "float", Unit: "km/h", Min: ptr(0), Max: ptr(1020), ScaleNote: "×0.1"},
				{Key: "mileage", Label: "累计里程", Kind: "float", Unit: "km", Min: ptr(0), Max: ptr(9999999.9), ScaleNote: "×0.1"},
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(10000), ScaleNote: "×0.1"},
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-1000), Max: ptr(1000), ScaleNote: "×0.1, 偏移+1000"},
				{Key: "soc", Label: "SOC", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "dc", Label: "DC/DC 状态", Kind: "enum", Enum: dcEnum},
				{Key: "gear", Label: "档位", Kind: "enum", Enum: gearEnum},
				{Key: "drivingForce", Label: "有驱动力", Kind: "bool"},
				{Key: "brakingForce", Label: "有制动力", Kind: "bool"},
				{Key: "insulance", Label: "绝缘电阻", Kind: "int", Unit: "kΩ", Min: ptr(0), Max: ptr(65535)},
				{Key: "accelerationValue", Label: "加速踏板行程", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "brakePedal", Label: "制动踏板状态", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
			},
		},
		{
			Key: "motor", Title: "驱动电机数据", Enabled: true, Multiple: true, MaxRows: 30,
			Fields: []FieldSchema{
				{Key: "seq", Label: "电机序号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "state", Label: "电机状态", Kind: "enum", Enum: motorStateEnum},
				{Key: "controllerTemp", Label: "控制器温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "speed", Label: "转速", Kind: "float", Unit: "r/min", Min: ptr(-20000), Max: ptr(45535), ScaleNote: "偏移+20000"},
				{Key: "torque", Label: "转矩", Kind: "float", Unit: "N·m", Min: ptr(-2000), Max: ptr(4553.5), ScaleNote: "×0.1, 偏移+2000"},
				{Key: "motorTemp", Label: "电机温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "controllerVoltage", Label: "控制器输入电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(6000), ScaleNote: "×0.1"},
				{Key: "controllerCurrent", Label: "控制器母线电流", Kind: "float", Unit: "A", Min: ptr(-1000), Max: ptr(1000), ScaleNote: "×0.1, 偏移+1000"},
			},
		},
		{
			Key: "fuelcell", Title: "燃料电池数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "voltage", Label: "燃料电池电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(6000), ScaleNote: "×0.1"},
				{Key: "current", Label: "燃料电池电流", Kind: "float", Unit: "A", Min: ptr(0), Max: ptr(2000), ScaleNote: "×0.1"},
				{Key: "consumption", Label: "燃料消耗率", Kind: "float", Unit: "kg/100km", Min: ptr(0), Max: ptr(655.35), ScaleNote: "×0.01"},
				{Key: "probeTemps", Label: "探针温度值", Kind: "array_float", ItemLabel: "探针", Unit: "°C", ScaleNote: "无偏移, 0~250 整数"},
				{Key: "hydrogenMaxTemp", Label: "氢系统最高温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(6000), ScaleNote: "×0.1, 偏移+40"},
				{Key: "hydrogenMaxTempProbe", Label: "最高温度探针代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "hydrogenMaxCon", Label: "氢气最高浓度", Kind: "int", Unit: "ppm", Min: ptr(0), Max: ptr(60000)},
				{Key: "hydrogenMaxConSensor", Label: "最高浓度传感器代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "hydrogenMaxPressure", Label: "氢气最高压力", Kind: "float", Unit: "MPa", Min: ptr(0), Max: ptr(3000), ScaleNote: "×0.1"},
				{Key: "hydrogenMaxPressureSensor", Label: "最高压力传感器代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "highVoltageDC", Label: "高压 DC/DC 状态", Kind: "enum", Enum: onOffEnum},
			},
		},
		{
			Key: "engine", Title: "发动机数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "state", Label: "发动机状态", Kind: "enum", Enum: onOffEnum},
				{Key: "crankshaftSpeed", Label: "曲轴转速", Kind: "int", Unit: "r/min", Min: ptr(0), Max: ptr(65535)},
				{Key: "consumption", Label: "燃料消耗率", Kind: "float", Unit: "L/100km", Min: ptr(0), Max: ptr(655.35), ScaleNote: "×0.01"},
			},
		},
		{
			Key: "location", Title: "位置数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "valid", Label: "定位有效", Kind: "bool"},
				{Key: "longitude", Label: "经度", Kind: "float", Unit: "°", Min: ptr(-180), Max: ptr(180), ScaleNote: "×10^6"},
				{Key: "latitude", Label: "纬度", Kind: "float", Unit: "°", Min: ptr(-90), Max: ptr(90), ScaleNote: "×10^6"},
			},
		},
		{
			Key: "extremum", Title: "极值数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "voltageMaxSubsystem", Label: "最高电压子系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "voltageMaxBattery", Label: "最高电压单体代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "maxVoltage", Label: "单体电压最高值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(65.535), ScaleNote: "×0.001"},
				{Key: "voltageMinSubsystem", Label: "最低电压子系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "voltageMinBattery", Label: "最低电压单体代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "minVoltage", Label: "单体电压最低值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(65.535), ScaleNote: "×0.001"},
				{Key: "tempMaxSubsystem", Label: "最高温度子系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "tempMaxProbe", Label: "最高温度探针号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "maxTemp", Label: "最高温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "tempMinSubsystem", Label: "最低温度子系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "tempMinProbe", Label: "最低温度探针号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "minTemp", Label: "最低温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: "alarm", Title: "报警数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "maxAlarmLevel", Label: "最高报警等级", Kind: "int", Min: ptr(0), Max: ptr(3)},
				// 19 个报警位按 bit 顺序生成 bool 字段
				alarmBitsField(),
				{Key: "batteryFaults", Label: "储能装置故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "motorFaults", Label: "驱动电机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "engineFaults", Label: "发动机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "otherFaults", Label: "其他故障码", Kind: "array_float", ItemLabel: "故障码"},
			},
		},
		{
			Key: "voltage", Title: "储能装置电压数据", Enabled: true, Multiple: true, MaxRows: 10,
			Fields: []FieldSchema{
				{Key: "subsystem", Label: "子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(10000), ScaleNote: "×0.1"},
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-1000), Max: ptr(1000), ScaleNote: "×0.1, 偏移+1000"},
				{Key: "batteryTotal", Label: "单体电池总数", Kind: "int", Min: ptr(0), Max: ptr(65535)},
				{Key: "frameStartSeq", Label: "本帧起始电池序号", Kind: "int", Min: ptr(1), Max: ptr(65535)},
				{Key: "batteryVoltages", Label: "单体电池电压", Kind: "array_float", ItemLabel: "电压", Unit: "V", ScaleNote: "×0.001"},
			},
		},
		{
			Key: "temperature", Title: "储能装置温度数据", Enabled: true, Multiple: true, MaxRows: 10,
			Fields: []FieldSchema{
				{Key: "subsystem", Label: "子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "probeTemps", Label: "探针温度值", Kind: "array_float", ItemLabel: "探针", Unit: "°C", ScaleNote: "偏移+40"},
			},
		},
	}
}

// AlarmBitLabels2016 19 个通用报警位标签(bit0..18)。
// 导出供 parser 包复用(解析翻译与配置表单同源,避免双份漂移)。
var AlarmBitLabels2016 = []string{
	"温度差异报警", "电池高温报警", "储能装置过压", "储能装置欠压", "SOC 过低",
	"单体过压", "单体欠压", "SOC 过高", "SOC 跳变", "储能装置类型不匹配",
	"电池一致性差", "绝缘报警", "DC 温度报警", "制动系统报警", "DC 状态报警",
	"电机控制器温度报警", "高压互锁报警", "驱动电机温度报警", "储能装置过充",
}

// alarmBitsField 报警位组字段:Kind=bitgroup,前端按 Bits 渲染开关列表。
func alarmBitsField() FieldSchema {
	defs := make([]BitDef, len(AlarmBitLabels2016))
	for i, label := range AlarmBitLabels2016 {
		defs[i] = BitDef{Index: i, Label: label}
	}
	return FieldSchema{Key: "bits", Label: "通用报警标志位", Kind: "bitgroup", Bits: defs}
}
