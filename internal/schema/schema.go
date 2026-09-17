// Package schema 定义报文配置的「组-字段」元数据,并提供从配置值组装
// 协议报文体的装配器。schema 由 Go 输出、前端动态渲染,加字段只改 Go。
package schema

// 标准报文组键常量:V2016 与 V2025 组定义的并集(共 14 个)。
// 组定义、装配查表与标准键集合统一引用常量,避免字面量散落漂移。
const (
	GroupVehicle          = "vehicle"          // 整车数据
	GroupMotor            = "motor"            // 驱动电机数据
	GroupFuelCell         = "fuelcell"         // 燃料电池数据
	GroupEngine           = "engine"           // 发动机数据
	GroupLocation         = "location"         // 车辆位置数据
	GroupExtremum         = "extremum"         // 极值数据 (2016)
	GroupAlarm            = "alarm"            // 报警数据
	GroupVoltage          = "voltage"          // 可充电储能装置电压数据 (2016)
	GroupTemperature      = "temperature"      // 可充电储能装置温度数据 (2016)
	GroupMinParallel      = "minparallel"      // 动力蓄电池最小并联单元电压数据 (2025)
	GroupBatteryTemp      = "batterytemp"      // 动力蓄电池温度数据 (2025)
	GroupFCStack          = "fcstack"          // 燃料电池电堆数据 (2025)
	GroupSuperCap         = "supercap"         // 超级电容数据 (2025)
	GroupSuperCapExtremum = "supercapextremum" // 超级电容极值数据 (2025)
)

// GroupSchema 一个实时数据组(对应 0x02/0x03 数据单元的一个 TLV 子记录)。
type GroupSchema struct {
	Key      string        `json:"key"`      // 组标识,如 "vehicle"
	Title    string        `json:"title"`    // 中文名,如 "整车数据"
	Enabled  bool          `json:"enabled"`  // 默认是否勾选编入报文
	Multiple bool          `json:"multiple"` // true=多行可增删(电机/储能电压/储能温度)
	MaxRows  int           `json:"maxRows,omitempty"`
	Fields   []FieldSchema `json:"fields"`
	Source   string        `json:"source,omitempty"` // 空=标准实时; "command"=扩展命令组(前端分流到自定义数据 tab)
}

// FieldSchema 单个字段元数据。
type FieldSchema struct {
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	Kind      string    `json:"kind"` // int|float|bool|enum|array_float|bitgroup
	Unit      string    `json:"unit,omitempty"`
	Min       *float64  `json:"min,omitempty"`
	Max       *float64  `json:"max,omitempty"`
	Enum      []EnumDef `json:"enum,omitempty"`      // kind=enum
	Bits      []BitDef  `json:"bits,omitempty"`      // kind=bitgroup:位定义列表
	ItemLabel string    `json:"itemLabel,omitempty"` // kind=array_float 的元素名
	MinItems  int       `json:"minItems,omitempty"`  // kind=array_float 最少元素数(0=不限;表12/14/25 "总数"下限 1)
	ScaleNote string    `json:"scaleNote,omitempty"` // 换算说明,如 "×0.1 km/h"
	Length    int       `json:"length,omitempty"`    // bytes 字段字节长度(扩展包编译器使用)
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

// 整车/电机枚举(2016 表 9/表 11/附录 A.1 与 2025 表 10/表 16/附录 A.1 对应字段
// 定义一致,双版本共用同一组枚举,以 docs/standard 标准文本为准)。
var (
	opStateEnum = enum(
		EnumDef{1, "启动"}, EnumDef{2, "熄火"}, EnumDef{3, "其他"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	// 充电状态:0x01 停车充电;0x02 行驶充电;0x03 未充电状态;0x04 充电完成
	chargeEnum = enum(
		EnumDef{0x01, "停车充电"}, EnumDef{0x02, "行驶充电"}, EnumDef{0x03, "未充电"},
		EnumDef{0x04, "充电完成"},
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
	// 挡位(附录 A.1 挡位状态位 bit3~0):0x0 空挡;0x1~0x6 = 1~6 挡;0xD 倒挡;0xE 自动D;0xF 停车P
	gearEnum = enum(
		EnumDef{0x00, "空挡"}, EnumDef{0x01, "1挡"}, EnumDef{0x02, "2挡"}, EnumDef{0x03, "3挡"},
		EnumDef{0x04, "4挡"}, EnumDef{0x05, "5挡"}, EnumDef{0x06, "6挡"},
		EnumDef{0x0D, "倒挡"}, EnumDef{0x0E, "自动D挡"}, EnumDef{0x0F, "停车P挡"},
	)
	// 电机状态:0x01 耗电;0x02 发电;0x03 关闭状态;0x04 准备状态
	motorStateEnum = enum(
		EnumDef{1, "耗电"}, EnumDef{2, "发电"}, EnumDef{3, "关闭"}, EnumDef{4, "准备"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	onOffEnum = enum(
		EnumDef{1, "启动/工作"}, EnumDef{2, "关闭/断开"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	// 表17(2016):最高报警等级 0 无故障;1~3 级故障,等级越高越严重
	alarmLevelEnum = enum(
		EnumDef{0, "无故障"}, EnumDef{1, "1级故障"}, EnumDef{2, "2级故障"}, EnumDef{3, "3级故障"},
	)
)

// V2016Groups 返回 2016 版实时数据的全部组定义。
func V2016Groups() []GroupSchema {
	return []GroupSchema{
		{
			Key: GroupVehicle, Title: "整车数据", Enabled: true,
			Fields: []FieldSchema{
			{Key: "operatingState", Label: "车辆状态", Kind: "enum", Enum: opStateEnum},
			{Key: "chargingState", Label: "充电状态", Kind: "enum", Enum: chargeEnum},
				{Key: "operationMode", Label: "运行模式", Kind: "enum", Enum: modeEnum},
				// 表9/B.4:车速有效值 0~2200(0~220 km/h),0.1 km/h
				{Key: "speed", Label: "车速", Kind: "float", Unit: "km/h", Min: ptr(0), Max: ptr(220), ScaleNote: "×0.1"},
				// 表9/B.4:累计里程有效值 0~9999999(0~999999.9 km),0.1 km
				{Key: "mileage", Label: "累计里程", Kind: "float", Unit: "km", Min: ptr(0), Max: ptr(999999.9), ScaleNote: "×0.1"},
				// 表9/B.4:总电压有效值 0~10000(0~1000 V),0.1 V(×10 后不得超 WORD)
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(1000), ScaleNote: "×0.1"},
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-1000), Max: ptr(1000), ScaleNote: "×0.1, 偏移+1000"},
				{Key: "soc", Label: "SOC", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "dc", Label: "DC/DC 状态", Kind: "enum", Enum: dcEnum},
			// 附录 A.1:枚举值即挡位字节 bit3~0(装配时与驱动力 bit5/制动力 bit4 组合)
			{Key: "gear", Label: "档位", Kind: "enum", Enum: gearEnum},
				{Key: "drivingForce", Label: "有驱动力", Kind: "bool"},
				{Key: "brakingForce", Label: "有制动力", Kind: "bool"},
				// 表9/B.4:绝缘电阻有效值 0~60000,1 kΩ
				{Key: "insulance", Label: "绝缘电阻", Kind: "int", Unit: "kΩ", Min: ptr(0), Max: ptr(60000)},
				{Key: "accelerationValue", Label: "加速踏板行程", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				// 表B.4:"0"表示制动关;无行程值时用 0x65(101)表示制动有效
				{Key: "brakePedal", Label: "制动踏板状态", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(101), ScaleNote: "0=制动关; 101(0x65)=制动有效"},
			},
		},
		{
			// 表10:驱动电机个数有效值 1~253
			Key: GroupMotor, Title: "驱动电机数据", Enabled: true, Multiple: true, MaxRows: 253,
			Fields: []FieldSchema{
				{Key: "seq", Label: "电机序号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "state", Label: "电机状态", Kind: "enum", Enum: motorStateEnum},
				{Key: "controllerTemp", Label: "控制器温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				// 表11:转速有效值 0~65531(偏移 20000 → -20000~45531 r/min),0xFFFF 为无效
				{Key: "speed", Label: "转速", Kind: "float", Unit: "r/min", Min: ptr(-20000), Max: ptr(45531), ScaleNote: "偏移+20000"},
				// 表11:转矩有效值 0~65531(偏移 20000 → -2000~4553.1 N·m),0.1 N·m
				{Key: "torque", Label: "转矩", Kind: "float", Unit: "N·m", Min: ptr(-2000), Max: ptr(4553.1), ScaleNote: "×0.1, 偏移+2000"},
				{Key: "motorTemp", Label: "电机温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "controllerVoltage", Label: "控制器输入电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(6000), ScaleNote: "×0.1"},
				{Key: "controllerCurrent", Label: "控制器母线电流", Kind: "float", Unit: "A", Min: ptr(-1000), Max: ptr(1000), ScaleNote: "×0.1, 偏移+1000"},
			},
		},
		{
			Key: GroupFuelCell, Title: "燃料电池数据", Enabled: false,
			Fields: []FieldSchema{
				// 表12:燃料电池电压有效值 0~20000(0~2000 V),0.1 V
				{Key: "voltage", Label: "燃料电池电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(2000), ScaleNote: "×0.1"},
				{Key: "current", Label: "燃料电池电流", Kind: "float", Unit: "A", Min: ptr(0), Max: ptr(2000), ScaleNote: "×0.1"},
				// 表12:燃料消耗率有效值 0~60000(0~600 kg/100km),0.01 kg/100km
				{Key: "consumption", Label: "燃料消耗率", Kind: "float", Unit: "kg/100km", Min: ptr(0), Max: ptr(600), ScaleNote: "×0.01"},
				// 表12:探针温度有效值 0~240(偏移 40 → -40~+200 ℃),1 ℃;总数由数组长度生成
				{Key: "probeTemps", Label: "探针温度值", Kind: "array_float", ItemLabel: "探针", Unit: "°C", Min: ptr(-40), Max: ptr(200), ScaleNote: "偏移+40, 0~240"},
				// 表12:氢系统最高温度有效值 0~2400(偏移 40 → -40~+200 ℃),0.1 ℃
				{Key: "hydrogenMaxTemp", Label: "氢系统最高温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(200), ScaleNote: "×0.1, 偏移+40"},
				// 表12:探针/传感器代号有效值 1~252(0xFE/0xFF 为异常/无效)
				{Key: "hydrogenMaxTempProbe", Label: "最高温度探针代号", Kind: "int", Min: ptr(1), Max: ptr(252)},
				// 表12:氢气最高浓度有效值 0~60000(原文注"表示 0~50000 mg/kg"与本量程矛盾,暂按原始值上限)
				{Key: "hydrogenMaxCon", Label: "氢气最高浓度", Kind: "int", Unit: "mg/kg", Min: ptr(0), Max: ptr(60000)},
				{Key: "hydrogenMaxConSensor", Label: "最高浓度传感器代号", Kind: "int", Min: ptr(1), Max: ptr(252)},
				// 表12:氢气最高压力有效值 0~1000(0~100 MPa),0.1 MPa
				{Key: "hydrogenMaxPressure", Label: "氢气最高压力", Kind: "float", Unit: "MPa", Min: ptr(0), Max: ptr(100), ScaleNote: "×0.1"},
				{Key: "hydrogenMaxPressureSensor", Label: "最高压力传感器代号", Kind: "int", Min: ptr(1), Max: ptr(252)},
				{Key: "highVoltageDC", Label: "高压 DC/DC 状态", Kind: "enum", Enum: onOffEnum},
			},
		},
		{
			Key: GroupEngine, Title: "发动机数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "state", Label: "发动机状态", Kind: "enum", Enum: onOffEnum},
				// 表13:曲轴转速有效值 0~60000,1 r/min
				{Key: "crankshaftSpeed", Label: "曲轴转速", Kind: "int", Unit: "r/min", Min: ptr(0), Max: ptr(60000)},
				// 表13:燃料消耗率有效值 0~60000(0~600 L/100km),0.01 L/100km
				{Key: "consumption", Label: "燃料消耗率", Kind: "float", Unit: "L/100km", Min: ptr(0), Max: ptr(600), ScaleNote: "×0.01"},
			},
		},
		{
			Key: GroupLocation, Title: "位置数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "valid", Label: "定位有效", Kind: "bool"},
				{Key: "longitude", Label: "经度", Kind: "float", Unit: "°", Min: ptr(-180), Max: ptr(180), ScaleNote: "×10^6"},
				{Key: "latitude", Label: "纬度", Kind: "float", Unit: "°", Min: ptr(-90), Max: ptr(90), ScaleNote: "×10^6"},
			},
		},
		{
			Key: GroupExtremum, Title: "极值数据", Enabled: true,
			Fields: []FieldSchema{
				// 表16:子系统号/单体代号/探针序号有效值 1~250(0xFE/0xFF 为异常/无效)
				{Key: "voltageMaxSubsystem", Label: "最高电压子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "voltageMaxBattery", Label: "最高电压单体代号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				// 表16:单体电压有效值 0~15000(0~15 V),0.001 V
				{Key: "maxVoltage", Label: "单体电压最高值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(15), ScaleNote: "×0.001"},
				{Key: "voltageMinSubsystem", Label: "最低电压子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "voltageMinBattery", Label: "最低电压单体代号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "minVoltage", Label: "单体电压最低值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(15), ScaleNote: "×0.001"},
				{Key: "tempMaxSubsystem", Label: "最高温度子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "tempMaxProbe", Label: "最高温度探针号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "maxTemp", Label: "最高温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "tempMinSubsystem", Label: "最低温度子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "tempMinProbe", Label: "最低温度探针号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "minTemp", Label: "最低温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupAlarm, Title: "报警数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "maxAlarmLevel", Label: "最高报警等级", Kind: "enum", Enum: alarmLevelEnum},
				// 19 个报警位按 bit 顺序生成 bool 字段
				alarmBitsField(),
				{Key: "batteryFaults", Label: "储能装置故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "motorFaults", Label: "驱动电机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "engineFaults", Label: "发动机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "otherFaults", Label: "其他故障码", Kind: "array_float", ItemLabel: "故障码"},
			},
		},
		{
			// 表B.5:可充电储能子系统个数有效值 1~250
			Key: GroupVoltage, Title: "可充电储能装置电压数据", Enabled: true, Multiple: true, MaxRows: 250,
			Fields: []FieldSchema{
				{Key: "subsystem", Label: "子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				// 表B.6:可充电储能装置电压有效值 0~10000(0~1000 V),0.1 V
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(1000), ScaleNote: "×0.1"},
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-1000), Max: ptr(1000), ScaleNote: "×0.1, 偏移+1000"},
				// 表B.6:单体电池总数有效值 1~65531
				{Key: "batteryTotal", Label: "单体电池总数", Kind: "int", Min: ptr(1), Max: ptr(65531)},
				{Key: "frameStartSeq", Label: "本帧起始电池序号", Kind: "int", Min: ptr(1), Max: ptr(65531)},
				// 表B.6:本帧单体总数 m 有效值 1~200,超出自动拆帧(装配器按 200/帧拆分)
				{Key: "batteryVoltages", Label: "单体电池电压", Kind: "array_float", ItemLabel: "电压", Unit: "V", Min: ptr(0), Max: ptr(60), MinItems: 1, ScaleNote: "×0.001, >200 自动拆帧"},
			},
		},
		{
			// 表B.7:可充电储能子系统个数有效值 1~250
			Key: GroupTemperature, Title: "可充电储能装置温度数据", Enabled: true, Multiple: true, MaxRows: 250,
			Fields: []FieldSchema{
				{Key: "subsystem", Label: "子系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				// 表B.8:探针温度有效值 0~250(偏移 40 → -40~+210 ℃),1 ℃
				{Key: "probeTemps", Label: "探针温度值", Kind: "array_float", ItemLabel: "探针", Unit: "°C", Min: ptr(-40), Max: ptr(210), MinItems: 1, ScaleNote: "偏移+40"},
			},
		},
	}
}

// AlarmBitLabels2016 19 个通用报警位标签(bit0..18),按 2016.md 表18 原文;bit19~31 预留。
// 导出供 parser 包复用(解析翻译与配置表单同源,避免双份漂移)。
var AlarmBitLabels2016 = []string{
	"温度差异报警", "电池高温报警", "车载储能装置类型过压报警", "车载储能装置类型欠压报警", "SOC 低报警",
	"单体电池过压报警", "单体电池欠压报警", "SOC 过高报警", "SOC 跳变报警", "可充电储能系统不匹配报警",
	"电池单体一致性差报警", "绝缘报警", "DC-DC 温度报警", "制动系统报警", "DC-DC 状态报警",
	"驱动电机控制器温度报警", "高压互锁状态报警", "驱动电机温度报警", "车载储能装置类型过充",
}

// alarmBitsField 报警位组字段:Kind=bitgroup,前端按 Bits 渲染开关列表。
func alarmBitsField() FieldSchema {
	defs := make([]BitDef, len(AlarmBitLabels2016))
	for i, label := range AlarmBitLabels2016 {
		defs[i] = BitDef{Index: i, Label: label}
	}
	return FieldSchema{Key: "bits", Label: "通用报警标志位", Kind: "bitgroup", Bits: defs}
}

// standardGroupKeys 标准报文组键集合(V2016 与 V2025 组定义并集,共 14 个),
// 由 Group* 常量构建;扩展组键与标准键同名会被 ext.Validate 拒绝。
var standardGroupKeys = map[string]bool{
	GroupVehicle: true, GroupMotor: true, GroupFuelCell: true, GroupEngine: true,
	GroupLocation: true, GroupExtremum: true, GroupAlarm: true, GroupVoltage: true,
	GroupTemperature: true, GroupMinParallel: true, GroupBatteryTemp: true,
	GroupFCStack: true, GroupSuperCap: true, GroupSuperCapExtremum: true,
}

// IsStandardGroupKey 报告 key 是否属于任一协议版本的标准报文组键。
// 持久化层据此把扩展组键挡在 message.json 之外(扩展组值按包独立存储)。
func IsStandardGroupKey(key string) bool { return standardGroupKeys[key] }
