package parser

import (
	"github.com/sunsky74/gb32960/types"
	"gbt32960-simulator/internal/schema"
)

// 枚举标签:值一律取自 gb32960-go types 常量(协议定义单一来源),此处只做值→中文文案。

var opStateLabels = map[byte]string{
	byte(types.OpStateOn): "启动", byte(types.OpStateOff): "关闭", byte(types.OpStateOther): "其他",
	byte(types.OpStateException): "异常", byte(types.OpStateInvalid): "无效",
}

var chargeStateLabels = map[byte]string{
	byte(types.ChargeStateNotCharging): "未充电", byte(types.ChargeStateCharging): "充电中",
	byte(types.ChargeStateComplete): "充电完成", byte(types.ChargeStateException): "异常",
	byte(types.ChargeStateInvalid): "无效",
}

var modeLabels = map[byte]string{
	byte(types.OpModeElectric): "纯电", byte(types.OpModeHybrid): "混动", byte(types.OpModeFuel): "燃油",
	byte(types.OpModeException): "异常", byte(types.OpModeInvalid): "无效",
}

var dcLabels = map[byte]string{
	byte(types.DCStateOn): "工作", byte(types.DCStateOff): "断开",
	byte(types.DCStateException): "异常", byte(types.DCStateInvalid): "无效",
}

var gearLabels = map[byte]string{
	byte(types.GearP): "P", byte(types.GearR): "R", byte(types.GearN): "N",
	byte(types.GearD): "D", byte(types.GearOther): "其他档位",
}

var motorStateLabels = map[byte]string{
	0x01: "驱动", 0x02: "发电", 0x03: "停机", 0x04: "准备",
	0xFE: "异常", 0xFF: "无效",
}

var engineStateLabels = map[byte]string{0x01: "启动", 0x02: "关闭"}

var onOffLabels = map[byte]string{0x01: "工作", 0x02: "断开"}

// schemaAlarmBitLabels 复用 schema 包的 2016 报警位标签(19 位),保证与配置表单一致。
func schemaAlarmBitLabels() []string { return schema.AlarmBitLabels2016 }
