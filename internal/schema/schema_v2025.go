package schema

// V2025Groups 返回 2025 版实时数据的全部组定义。
// 与 2016 的结构差异(依据 gb32960-go model/gbt2025 与 docs/standard/2025.md):
//   - 无极值组、无储能电压组、无储能温度组(2025 用最小并联电压/电池包温度替代)
//   - 整车电流偏移 +3000(2016 为 +1000);无加速踏板/制动踏板字段,绝缘电阻保留
//   - 挡位字节补全表A.1位定义:bit7 挡位无效、bit6 预留(恒 0)、bit5/4 驱动力/制动力
//   - 电机组无控制器电压/电流;转速偏移 +32000、转矩偏移 +20000
//   - 位置组新增坐标系(表21);报警位扩展到 28 位(2016 为 19 位),新增 N5 等级列表
//   - 新增:燃料电池电堆 / 超级电容 / 超级电容极值
//
// 各字段 Min/Max 严格按 2025.md 表10~表26 的有效值范围标注;
// 0xFF..(0xFFFF/0xFFFFFFFF) 为异常/无效哨兵,不得作为量程上限。
// 自定义数据(0x80~)与签名(0xFF)暂不暴露:自定义无固定结构,签名属加密链路(M4)。
func V2025Groups() []GroupSchema {
	return []GroupSchema{
		{
			Key: GroupVehicle, Title: "整车数据", Enabled: true,
			Fields: []FieldSchema{
				{Key: "operatingState", Label: "车辆状态", Kind: "enum", Enum: opStateEnum},
				{Key: "chargingState", Label: "充电状态", Kind: "enum", Enum: chargeEnum},
				{Key: "operationMode", Label: "运行模式", Kind: "enum", Enum: modeEnum},
				// 表10:车速有效值 0~5000(0~500 km/h),0.1 km/h
				{Key: "speed", Label: "车速", Kind: "float", Unit: "km/h", Min: ptr(0), Max: ptr(500), ScaleNote: "×0.1"},
				// 表10:累计里程有效值 0~9999999(0~999999.9 km),0.1 km
				{Key: "mileage", Label: "累计里程", Kind: "float", Unit: "km", Min: ptr(0), Max: ptr(999999.9), ScaleNote: "×0.1"},
				// 表10:总电压有效值 0~60000(0~6000 V),0.1 V
				{Key: "voltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(6000), ScaleNote: "×0.1"},
				// 表10:总电流有效值 0~60000(偏移 3000A,-3000~+3000A),0.1 A
				{Key: "current", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-3000), Max: ptr(3000), ScaleNote: "×0.1, 偏移+3000 (2025)"},
				{Key: "soc", Label: "SOC", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "dc", Label: "DC/DC 状态", Kind: "enum", Enum: dcEnum},
				// 附录 A.1:挡位字节 bit3~0 挡位码(空挡/1~6挡/倒挡/自动D/停车P),
				// bit7 挡位无效(1=无效)、bit6 预留(恒 0)、bit5/4 驱动力/制动力
				{Key: "gear", Label: "档位", Kind: "enum", Enum: gearEnum},
				{Key: "gearInvalid", Label: "挡位无效", Kind: "bool"},
				{Key: "drivingForce", Label: "有驱动力", Kind: "bool"},
				{Key: "brakingForce", Label: "有制动力", Kind: "bool"},
				// 表10:高压对地绝缘电阻有效值 0~60000(0~60000 kΩ),1 kΩ
				{Key: "insulance", Label: "绝缘电阻", Kind: "int", Unit: "kΩ", Min: ptr(0), Max: ptr(60000)},
			},
		},
		{
			// 表15:驱动电机个数有效值 1~253
			Key: GroupMotor, Title: "驱动电机数据", Enabled: true, Multiple: true, MaxRows: 253,
			Fields: []FieldSchema{
				{Key: "seq", Label: "电机序号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "state", Label: "电机状态", Kind: "enum", Enum: motorStateEnum},
				{Key: "controllerTemp", Label: "控制器温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				// 表16:转速有效值 0~65531(偏移 32000,-32000~+33531 r/min),1 r/min
				{Key: "speed", Label: "转速", Kind: "float", Unit: "r/min", Min: ptr(-32000), Max: ptr(33531), ScaleNote: "偏移+32000 (2025)"},
				// 表16:转矩有效值 0~400000(偏移 20000,-20000~+20000 N·m),0.1 N·m
				{Key: "torque", Label: "转矩", Kind: "float", Unit: "N·m", Min: ptr(-20000), Max: ptr(20000), ScaleNote: "×0.1, 偏移+20000 (2025)"},
				{Key: "motorTemp", Label: "电机温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupFuelCell, Title: "燃料电池发动机及车载氢系统数据", Enabled: false,
			Fields: []FieldSchema{
				// 表17:氢系统最高温度 0~250(偏移 40,-40~+210 ℃),1 ℃
				{Key: "hydrogenMaxTemp", Label: "车载氢系统最高温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "hydrogenMaxTempProbe", Label: "最高温度探针代号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				// 表17:氢气最高浓度 0~60000(体积分数 0~6%),最小计量单元 0.0001%
				{Key: "hydrogenMaxCon", Label: "氢气最高浓度", Kind: "float", Unit: "%", Min: ptr(0), Max: ptr(6), ScaleNote: "×0.0001% (体积分数)"},
				{Key: "hydrogenMaxConSensor", Label: "最高浓度传感器代号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				// 表17:氢气最高压力 0~1000(0~100 MPa),0.1 MPa
				{Key: "hydrogenMaxPressure", Label: "氢气最高压力", Kind: "float", Unit: "MPa", Min: ptr(0), Max: ptr(100), ScaleNote: "×0.1"},
				{Key: "hydrogenMaxPressureSensor", Label: "最高压力传感器代号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "highVoltageDC", Label: "高压 DC/DC 状态", Kind: "enum", Enum: onOffEnum},
				// 表17:剩余氢量百分比 0~100,1%
				{Key: "fuelPercentage", Label: "剩余氢量百分比", Kind: "int", Unit: "%", Min: ptr(0), Max: ptr(100)},
				{Key: "dcControllerTemp", Label: "高压 DC/DC 控制器温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupEngine, Title: "发动机数据", Enabled: false,
			Fields: []FieldSchema{
				// 表20:曲轴转速 0~60000,1 r/min
				{Key: "crankshaftSpeed", Label: "曲轴转速", Kind: "int", Unit: "r/min", Min: ptr(0), Max: ptr(60000)},
			},
		},
		{
			Key: GroupLocation, Title: "车辆位置数据", Enabled: true,
			Fields: []FieldSchema{
				// 表22:定位状态 bit0(0=有效定位;1=无效定位),装配时按 valid 取反写入
				{Key: "valid", Label: "定位有效", Kind: "bool"},
				// 表21:坐标系 0x01 WGS84;0x02 GCJ02;0x03 其他
				{Key: "coordinateSystem", Label: "坐标系", Kind: "enum", Enum: coordSysEnum},
				// 表22:bit1 南/北纬、bit2 东/西经由经纬度正负号推导,bit3~7 保留
				{Key: "longitude", Label: "经度", Kind: "float", Unit: "°", Min: ptr(-180), Max: ptr(180), ScaleNote: "×10^6, 东/西经按正负号自动判定"},
				{Key: "latitude", Label: "纬度", Kind: "float", Unit: "°", Min: ptr(-90), Max: ptr(90), ScaleNote: "×10^6, 南/北纬按正负号自动判定"},
			},
		},
		{
			Key: GroupAlarm, Title: "报警数据", Enabled: true,
			Fields: []FieldSchema{
				// 表23:最高报警等级 0~4(4=热事件故障,最高级)
				{Key: "maxAlarmLevel", Label: "最高报警等级", Kind: "int", Min: ptr(0), Max: ptr(4)},
				// 表23:通用报警标志为 4 字节 DWORD;表24 定义 bit0~27,bit28~31 预留(编码恒 0)
				alarmBitsFieldV2025(),
				{Key: "batteryFaults", Label: "储能装置故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "motorFaults", Label: "驱动电机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "engineFaults", Label: "发动机故障码", Kind: "array_float", ItemLabel: "故障码"},
				{Key: "otherFaults", Label: "其他故障码", Kind: "array_float", ItemLabel: "故障码"},
				// 表23(续):通用报警故障等级列表 2×N5 = (标志位序号, 等级) 对;
				// 以下两数组按下标一一配对,装配时合成 N5 条目;
				// 等级取值文档未明示范围,随最高报警等级按 1~4 约束
				{Key: "commonAlertSeqs", Label: "通用报警位序号", Kind: "array_float", ItemLabel: "位序号", Min: ptr(0), Max: ptr(27), ScaleNote: "与等级列表按序配对 (表24 位序号)"},
				{Key: "commonAlertLevels", Label: "通用报警故障等级", Kind: "array_float", ItemLabel: "等级", Min: ptr(1), Max: ptr(4), ScaleNote: "与位序号列表按序配对"},
			},
		},
		{
			// 表11:动力蓄电池包个数 0~50
			Key: GroupMinParallel, Title: "动力蓄电池最小并联单元电压数据", Enabled: true, Multiple: true, MaxRows: 50,
			Fields: []FieldSchema{
				// 表12:电池包号 1~50
				{Key: "batteryPackSeq", Label: "电池包号", Kind: "int", Min: ptr(1), Max: ptr(50)},
				// 表12:电池包电压 0~60000(0~6000 V),0.1 V
				{Key: "voltage", Label: "电池包电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(6000), ScaleNote: "×0.1"},
				// 表12:电池包电流 0~60000(偏移 3000A,-3000~+3000A),0.1 A
				{Key: "current", Label: "电池包电流", Kind: "float", Unit: "A", Min: ptr(-3000), Max: ptr(3000), ScaleNote: "×0.1, 偏移+3000"},
				// 表12:本帧最小并联单元电压 0~60000(0~60.000 V),0.001 V;总数由数组长度生成
				{Key: "batteryVoltages", Label: "并联单元电压", Kind: "array_float", ItemLabel: "电压", Unit: "V", Min: ptr(0), Max: ptr(60), MinItems: 1, ScaleNote: "×0.001"},
			},
		},
		{
			// 表13:动力蓄电池包个数 0~50
			Key: GroupBatteryTemp, Title: "动力蓄电池温度数据", Enabled: true, Multiple: true, MaxRows: 50,
			Fields: []FieldSchema{
				// 表14:电池包号 1~50
				{Key: "batteryPackSeq", Label: "电池包号", Kind: "int", Min: ptr(1), Max: ptr(50)},
				// 表14:探针温度 0~250(偏移 40,-40~+210 ℃),1 ℃;个数由数组长度生成
				{Key: "probeTemps", Label: "探针温度值", Kind: "array_float", ItemLabel: "探针", Unit: "°C", Min: ptr(-40), Max: ptr(210), MinItems: 1, ScaleNote: "偏移+40"},
			},
		},
		{
			// 表18:燃料电池电堆个数 1~253
			Key: GroupFCStack, Title: "燃料电池电堆数据", Enabled: false, Multiple: true, MaxRows: 253,
			Fields: []FieldSchema{
				// 表19:电堆序号 1~253
				{Key: "stackSeq", Label: "电堆序号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				// 表19:电堆电压 0~20000(0~2000 V),0.1 V
				{Key: "voltage", Label: "电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(2000), ScaleNote: "×0.1"},
				// 表19:电堆电流 0~20000(0~2000 A),0.1 A
				{Key: "current", Label: "电流", Kind: "float", Unit: "A", Min: ptr(0), Max: ptr(2000), ScaleNote: "×0.1"},
				// 表19:氢气入口压力 0~5000(偏移 100kPa,-100~+400 kPa),0.1 kPa
				{Key: "gasPressure", Label: "氢气入口压力", Kind: "float", Unit: "kPa", Min: ptr(-100), Max: ptr(400), ScaleNote: "×0.1, 偏移+100"},
				{Key: "airPressure", Label: "空气入口压力", Kind: "float", Unit: "kPa", Min: ptr(-100), Max: ptr(400), ScaleNote: "×0.1, 偏移+100"},
				{Key: "airInletTemp", Label: "空气入口温度", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				// 表19:冷却水出水口温度 0~250(偏移 40);探针总数由数组长度生成
				{Key: "coolingWaterTemps", Label: "冷却水出水口温度", Kind: "array_float", ItemLabel: "出水口", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupSuperCap, Title: "超级电容器数据", Enabled: false,
			Fields: []FieldSchema{
				// 表25:管理系统号 1~253
				{Key: "managementSystemNumber", Label: "管理系统号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				// 表25:总电压 0~10000(0~1000 V),0.1 V
				{Key: "totalVoltage", Label: "总电压", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(1000), ScaleNote: "×0.1"},
				// 表25:总电流 0~60000(偏移 3000A,-3000~+3000A),0.1 A
				{Key: "totalCurrent", Label: "总电流", Kind: "float", Unit: "A", Min: ptr(-3000), Max: ptr(3000), ScaleNote: "×0.1, 偏移+3000"},
				// 表25:单体电压 0~60000(0~60.000 V),0.001 V;单体总数由数组长度生成
				{Key: "capacitorVoltages", Label: "单体电压", Kind: "array_float", ItemLabel: "电压", Unit: "V", Min: ptr(0), Max: ptr(60), MinItems: 1, ScaleNote: "×0.001"},
				// 表25:探针温度 0~250(偏移 40);探针总数由数组长度生成
				{Key: "probeTemperatures", Label: "探针温度", Kind: "array_float", ItemLabel: "探针", Unit: "°C", Min: ptr(-40), Max: ptr(210), MinItems: 1, ScaleNote: "偏移+40"},
			},
		},
		{
			Key: GroupSuperCapExtremum, Title: "超级电容器极值数据", Enabled: false,
			Fields: []FieldSchema{
				// 表26:管理系统号 BYTE 1~253;单体/探针代号 WORD 1~65531
				{Key: "voltageMaxSubsystem", Label: "最高电压管理系统号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "voltageMaxBattery", Label: "最高电压单体代号", Kind: "int", Min: ptr(1), Max: ptr(65531)},
				// 表26:单体电压最高值 0~60000(0~60.000 V),0.001 V
				{Key: "maxVoltage", Label: "单体电压最高值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(60), ScaleNote: "×0.001"},
				{Key: "voltageMinSubsystem", Label: "最低电压管理系统号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "voltageMinBattery", Label: "最低电压单体代号", Kind: "int", Min: ptr(1), Max: ptr(65531)},
				{Key: "minVoltage", Label: "单体电压最低值", Kind: "float", Unit: "V", Min: ptr(0), Max: ptr(60), ScaleNote: "×0.001"},
				{Key: "tempMaxSubsystem", Label: "最高温度管理系统号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "tempMaxProbe", Label: "最高温度探针代号", Kind: "int", Min: ptr(1), Max: ptr(65531)},
				{Key: "maxTemp", Label: "最高温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
				{Key: "tempMinSubsystem", Label: "最低温度管理系统号", Kind: "int", Min: ptr(1), Max: ptr(253)},
				{Key: "tempMinProbe", Label: "最低温度探针代号", Kind: "int", Min: ptr(1), Max: ptr(65531)},
				{Key: "minTemp", Label: "最低温度值", Kind: "float", Unit: "°C", Min: ptr(-40), Max: ptr(210), ScaleNote: "偏移+40"},
			},
		},
	}
}

// coordSysEnum 表21:坐标系 0x01 WGS84;0x02 GCJ02;0x03 其他。
var coordSysEnum = enum(
	EnumDef{0x01, "WGS84"}, EnumDef{0x02, "GCJ02"}, EnumDef{0x03, "其他"},
)

// v2025AlarmBitLabels 28 个报警位(bit0..27),GB/T 32960.3-2025 表24。
// 注意与 2016 文案不同:bit5/6/10 由"单体"改称"最小并联单元",
// bit11 改称"绝缘电阻失效",bit19~27 为 2025 新增。
var v2025AlarmBitLabels = []string{
	"温度差异报警", "电池高温报警", "车载储能装置类型过压报警", "车载储能装置类型欠压报警",
	"SOC 低报警", "最小并联单元过压报警", "最小并联单元欠压报警", "SOC 过高报警",
	"SOC 跳变报警", "可充电储能系统不匹配报警", "最小并联单元一致性差报警", "绝缘电阻失效报警",
	"DC-DC 温度报警", "制动系统报警", "DC-DC 状态报警", "驱动电机控制器温度报警",
	"高压互锁状态报警", "驱动电机温度报警", "车载储能装置类型过充报警", "驱动电机超速报警",
	"驱动电机过流报警", "超级电容过温报警", "超级电容过压报警", "可充电储能装置热事件报警",
	"氢气泄漏异常报警", "车载氢系统压力异常报警", "车载氢系统温度异常报警", "燃料电池电堆超温报警",
}

func alarmBitsFieldV2025() FieldSchema {
	defs := make([]BitDef, len(v2025AlarmBitLabels))
	for i, label := range v2025AlarmBitLabels {
		defs[i] = BitDef{Index: i, Label: label}
	}
	return FieldSchema{Key: "bits", Label: "通用报警标志位 (28 位)", Kind: "bitgroup", Bits: defs}
}
