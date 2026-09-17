package parser

import (
	"gbt32960-simulator/internal/schema"
	"github.com/sunsky74/gb32960/types"
)

// 枚举标签:parser 仅解析 V2016 帧,标签一律按 GB/T 32960.3-2016 表 9/表 11/附录 A.1。
// 值优先取自 gb32960-go types 常量;两版定义不同处(充电状态/挡位)按 2016 文档用字面量。

var opStateLabels = map[byte]string{
	byte(types.OpStateOn): "启动", byte(types.OpStateOff): "熄火", byte(types.OpStateOther): "其他",
	byte(types.OpStateException): "异常", byte(types.OpStateInvalid): "无效",
}

// 2016 表 9:0x01 停车充电;0x02 行驶充电;0x03 未充电;0x04 充电完成。
// types.ChargeState* 常量是 2025 版语义命名(0x01 未充电…),不适用于 2016,故用字面量。
var chargeStateLabels = map[byte]string{
	0x01: "停车充电", 0x02: "行驶充电", 0x03: "未充电", 0x04: "充电完成",
	0xFE: "异常", 0xFF: "无效",
}

var modeLabels = map[byte]string{
	byte(types.OpModeElectric): "纯电", byte(types.OpModeHybrid): "混动", byte(types.OpModeFuel): "燃油",
	byte(types.OpModeException): "异常", byte(types.OpModeInvalid): "无效",
}

var dcLabels = map[byte]string{
	byte(types.DCStateOn): "工作", byte(types.DCStateOff): "断开",
	byte(types.DCStateException): "异常", byte(types.DCStateInvalid): "无效",
}

// 2016 附录 A.1 挡位状态位 bit3~0:0x0 空挡;0x1~0x6 = 1~6 挡;0xD 倒挡;0xE 自动D;0xF 停车P。
// types.GearPositionEnum(P=1/R=2/N=3/D=4)是 2025 版定义,不适用于 2016,故用字面量。
var gearLabels = map[byte]string{
	0x00: "空挡", 0x01: "1挡", 0x02: "2挡", 0x03: "3挡",
	0x04: "4挡", 0x05: "5挡", 0x06: "6挡",
	0x0D: "倒挡", 0x0E: "自动D挡", 0x0F: "停车P挡",
}

// 2016 表 11:0x01 耗电;0x02 发电;0x03 关闭状态;0x04 准备状态
var motorStateLabels = map[byte]string{
	0x01: "耗电", 0x02: "发电", 0x03: "关闭", 0x04: "准备",
	0xFE: "异常", 0xFF: "无效",
}

var engineStateLabels = map[byte]string{0x01: "启动", 0x02: "关闭", 0xFE: "异常", 0xFF: "无效"}

var onOffLabels = map[byte]string{0x01: "工作", 0x02: "断开", 0xFE: "异常", 0xFF: "无效"}

// schemaAlarmBitLabels 复用 schema 包的 2016 报警位标签(19 位),保证与配置表单一致。
func schemaAlarmBitLabels() []string { return schema.AlarmBitLabels2016 }
