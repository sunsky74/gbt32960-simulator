package schema

// V2025Groups 返回 2025 版实时数据的全部组定义。
// 与 2016 的结构差异(依据 gb32960-go model/gbt2025):
//   - 无极值组、无储能电压组、无储能温度组(2025 用最小并联电压/电池包温度替代)
//   - 整车电流偏移 +3000(2016 为 +1000);无加速踏板/制动踏板字段
//   - 电机组无控制器电压/电流;转速偏移 +32000、转矩偏移 +20000
//   - 报警位扩展到 28 位(2016 为 19 位)
//   - 新增:燃料电池电堆 / 超级电容 / 超级电容极值
//
// 自定义数据(0x80~)与签名(0xFF)暂不暴露:自定义无固定结构,签名属加密链路(M4)。
func V2025Groups() []GroupSchema {
	sharedMotorStateEnum := enum(
		EnumDef{1, "驱动"}, EnumDef{2, "发电"}, EnumDef{3, "停机"}, EnumDef{4, "准备"},
		EnumDef{0xFE, "异常"}, EnumDef{0xFF, "无效"},
	)
	return []GroupSchema{
		{
			Key: GroupVehicle, Title: "整车数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "operatingState", Label: "车辆状态", Kind: "enum", Enum: opStateEnum},
				{Key: "chargingState", Label: "充电状态", Kind: "enum", Enum: chargeEnum},
				{Key: "operationMode", Label: "运行模式", Kind: "enum", Enum: modeEnum},
				{Key: "speed", Label: "车速", Kind: "float", Unit: "km/h", Min: ptr(0), Max: ptr(1020), ScaleNote: "×0.1"},
				{Key: "mileage", Label: "累计里程", Kind: "float", Unit: "km", Min: ptr(0), Max: ptr(9999999.9), ScaleNote: "×0.1"},
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(10000), ScaleNote: "×0.1"},
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-3000), Max: ptr(3553.5), ScaleNote: "×0.1, 偏移+3000 (2025)"},
				{Key: "soc", Label: "SOC", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "dc", Label: "DC/DC 状态", Kind: "enum", Enum: dcEnum},
				{Key: "gear", Label: "档位", Kind: "enum", Enum: gearEnum},
				{Key: "drivingForce", Label: "有驱动力", Kind: "bool"},
				{Key: "brakingForce", Label: "有制动力", Kind: "bool"},
			},
		},
		{
			Key: GroupMotor, Title: "驱动电机数据", Enabled: true, Multiple: true, MaxRows: 30,
			Fields: []FieldSchema{
				{Key: "seq", Label: "电机序号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "state", Label: "电机状态", Kind: "enum", Enum: sharedMotorStateEnum},
				{Key: "controllerTemp", Label: "控制器温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "speed", Label: "转速", Kind: "float", Unit: "r/min", Min: ptr(-32000), Max: ptr(33535), ScaleNote: "偏移+32000 (2025)"},
				{Key: "torque", Label: "转矩", Kind: "float", Unit: "N·m", Min: ptr(-20000), Max: ptr(45530), ScaleNote: "×0.1, 偏移+20000 (2025)"},
				{Key: "motorTemp", Label: "电机温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupFuelCell, Title: "燃料电池发动机数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "hydrogenMaxTemp", Label: "氢系统最高温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "hydrogenMaxTempProbe", Label: "最高温度探针代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "hydrogenMaxCon", Label: "氢气最高浓度", Kind: "float", Unit: "%", Min: ptr(0), Max: ptr(100), ScaleNote: "×0.1"},
				{Key: "hydrogenMaxConSensor", Label: "最高浓度传感器代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "hydrogenMaxPressure", Label: "氢气最高压力", Kind: "float", Unit: "MPa", Min: ptr(0), Max: ptr(6553.5), ScaleNote: "×0.1"},
				{Key: "hydrogenMaxPressureSensor", Label: "最高压力传感器代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "highVoltageDC", Label: "高压 DC/DC 状态", Kind: "enum", Enum: onOffEnum},
				{Key: "fuelPercentage", Label: "燃料余量", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "dcControllerTemp", Label: "DC 控制器温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupEngine, Title: "发动机数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "crankshaftSpeed", Label: "曲轴转速", Kind: "int", Unit: "r/min", Min: ptr(0), Max: ptr(60000)},
			},
		},
		{
			Key: GroupLocation, Title: "位置数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "valid", Label: "定位有效", Kind: "bool"},
				{Key: "longitude", Label: "经度", Kind: "float", Unit: "°", Min: ptr(-180), Max: ptr(180), ScaleNote: "×10^6, 东/西经按正负号自动判定"},
				{Key: "latitude", Label: "纬度", Kind: "float", Unit: "°", Min: ptr(-90), Max: ptr(90), ScaleNote: "×10^6, 南/北纬按正负号自动判定"},
			},
		},
		{
			Key: GroupAlarm, Title: "报警数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "maxAlarmLevel", Label: "最高报警等级", Kind: "int", Min: ptr(0), Max: ptr(3)},
				alarmBitsFieldV2025(),
				{Key: "batteryFaults", Label: "储能装置故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "motorFaults", Label: "驱动电机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "engineFaults", Label: "发动机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "otherFaults", Label: "其他故障码", Kind: "array_float", ItemLabel: "故障码"},
			},
		},
		{
			Key: GroupMinParallel, Title: "最小并联单元电压数据", Enabled: true, Multiple: true, MaxRows: 10,
			Fields: []FieldSchema{
				{Key: "batteryPackSeq", Label: "电池包号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(10000), ScaleNote: "×0.1"},
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-3000), Max: ptr(3553.5), ScaleNote: "×0.1, 偏移+3000"},
				{Key: "batteryVoltages", Label: "并联单元电压", Kind: "array_float", ItemLabel: "电压", Unit: "V", ScaleNote: "×0.001"},
			},
		},
		{
			Key: GroupBatteryTemp, Title: "电池包温度数据", Enabled: true, Multiple: true, MaxRows: 10,
			Fields: []FieldSchema{
				{Key: "batteryPackSeq", Label: "电池包号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "probeTemps", Label: "探针温度值", Kind: "array_float", ItemLabel: "探针", Unit: "°C", ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupFCStack, Title: "燃料电池电堆数据", Enabled: false, Multiple: true, MaxRows: 10,
			Fields: []FieldSchema{
				{Key: "stackSeq", Label: "电堆序号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "voltage", Label: "电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(10000), ScaleNote: "×0.1"},
				{Key: "current", Label: "电流", Kind: "float", Unit: "A", Min: ptr(0), Max: ptr(6553.5), ScaleNote: "×0.1"},
				{Key: "gasPressure", Label: "氢气压力", Kind: "float", Unit: "kPa", Min: ptr(-100), Max: ptr(6443.5), ScaleNote: "×0.1, 偏移+100"},
				{Key: "airPressure", Label: "空气压力", Kind: "float", Unit: "kPa", Min: ptr(-100), Max: ptr(6443.5), ScaleNote: "×0.1, 偏移+100"},
				{Key: "airInletTemp", Label: "进气温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "coolingWaterTemps", Label: "冷却水温度", Kind: "array_float", ItemLabel: "出水口", Unit: "°C", ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupSuperCap, Title: "超级电容数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "managementSystemNumber", Label: "管理系统号", Kind: "int", Min: ptr(1), Max: ptr(250)},
				{Key: "totalVoltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(10000), ScaleNote: "×0.1"},
				{Key: "totalCurrent", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-3000), Max: ptr(3553.5), ScaleNote: "×0.1, 偏移+3000"},
				{Key: "capacitorVoltages", Label: "单体电压", Kind: "array_float", ItemLabel: "电压", Unit: "V", ScaleNote: "×0.001"},
				{Key: "probeTemperatures", Label: "探针温度", Kind: "array_float", ItemLabel: "探针", Unit: "°C", ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupSuperCapExtremum, Title: "超级电容极值数据", Enabled: false,
			Fields: []FieldSchema{
				{Key: "voltageMaxSubsystem", Label: "最高电压管理系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "voltageMaxBattery", Label: "最高电压单体代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "maxVoltage", Label: "单体电压最高值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(65.535), ScaleNote: "×0.001"},
				{Key: "voltageMinSubsystem", Label: "最低电压管理系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "voltageMinBattery", Label: "最低电压单体代号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "minVoltage", Label: "单体电压最低值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(65.535), ScaleNote: "×0.001"},
				{Key: "tempMaxSubsystem", Label: "最高温度管理系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "tempMaxProbe", Label: "最高温度探针号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "maxTemp", Label: "最高温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "tempMinSubsystem", Label: "最低温度管理系统号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "tempMinProbe", Label: "最低温度探针号", Kind: "int", Min: ptr(0), Max: ptr(255)},
				{Key: "minTemp", Label: "最低温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
	}
}

// v2025AlarmBitLabels 28 个报警位(bit0..27),前 19 位与 2016 相同,后 9 位为 2025 新增。
var v2025AlarmBitLabels = append(
	append([]string{}, AlarmBitLabels2016...),
	"驱动电机超速报警", "驱动电机过流报警", "超级电容过温报警", "超级电容过压报警",
	"可充电储能装置热事件", "氢气泄漏报警", "氢气压力异常报警", "氢气温度异常报警", "燃料电池堆过温报警",
)

func alarmBitsFieldV2025() FieldSchema {
	defs := make([]BitDef, len(v2025AlarmBitLabels))
	for i, label := range v2025AlarmBitLabels {
		defs[i] = BitDef{Index: i, Label: label}
	}
	return FieldSchema{Key: "bits", Label: "通用报警标志位 (28 位)", Kind: "bitgroup", Bits: defs}
}
