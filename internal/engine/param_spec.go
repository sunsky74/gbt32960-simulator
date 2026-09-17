package engine

import "github.com/sunsky74/gb32960/api"

// ParamOption 枚举型参数的取值。
type ParamOption struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

// ParamSpec 平台参数定义(2016 表B.12 / 2025 表B.8)。
// 值域为「线上原始值(RAW)」口径——0x80 参数查询应答直接填原始字节,
// 物理含义(秒/毫秒/单位)见 Note;Length=0 表示可变长,规则见 Note。
type ParamSpec struct {
	ID      int           `json:"id"`
	Name    string        `json:"name"`
	Type    string        `json:"type"`   // u8 | u16 | bytes | string
	Length  int           `json:"length"` // 定长字节数;0=可变长
	RawMin  *int          `json:"rawMin,omitempty"`
	RawMax  *int          `json:"rawMax,omitempty"`
	Note    string        `json:"note,omitempty"`
	Options []ParamOption `json:"options,omitempty"`
}

func intp(n int) *int { return &n }

// ParamSpecs 返回指定协议版本的参数定义(以 2025 表B.8 为基准,2016 表B.12 覆盖差异项:
// 0x01 计量单元 1ms、0x02 WORD 1~600s、0x03 WORD 0~60000ms)。
func ParamSpecs(v api.GBTVersion) []ParamSpec {
	specs := []ParamSpec{
		{ID: 0x01, Name: "车载终端本地存储时间周期", Type: "u16", Length: 2, RawMin: intp(0), RawMax: intp(60000),
			Note: "0~60000(0~60s),最小计量单元 0.001s;0xFFFE 异常/0xFFFF 无效"},
		{ID: 0x02, Name: "正常时信息上报时间周期", Type: "u8", Length: 1, RawMin: intp(1), RawMax: intp(30),
			Note: "1~30(1~30s),最小计量单元 1s;0xFE 异常/0xFF 无效"},
		{ID: 0x03, Name: "出现报警时信息上报时间周期", Type: "u8", Length: 1, RawMin: intp(0), RawMax: intp(1),
			Note: "0~1(0~1s),最小计量单元 1s;0xFE 异常/0xFF 无效"},
		{ID: 0x04, Name: "远程服务与管理平台域名长度 M", Type: "u8", Length: 1},
		{ID: 0x05, Name: "远程服务与管理平台域名", Type: "bytes", Length: 0,
			Note: "长度 M 由同应答中 0x04 的值决定(1×M)"},
		{ID: 0x06, Name: "远程服务与管理平台端口", Type: "u16", Length: 2, RawMin: intp(0), RawMax: intp(65531),
			Note: "0~65531;0xFFFE 异常/0xFFFF 无效"},
		{ID: 0x07, Name: "硬件版本", Type: "string", Length: 5, Note: "5 字节,厂商自定义;设置命令不含该项"},
		{ID: 0x08, Name: "固件版本", Type: "string", Length: 5, Note: "5 字节,厂商自定义;设置命令不含该项"},
		{ID: 0x09, Name: "车载终端心跳发送周期", Type: "u8", Length: 1, RawMin: intp(1), RawMax: intp(240),
			Note: "1~240(1~240s),最小计量单元 1s;0xFE 异常/0xFF 无效"},
		{ID: 0x0A, Name: "终端应答超时时间", Type: "u16", Length: 2, RawMin: intp(1), RawMax: intp(600),
			Note: "1~600(1~600s),最小计量单元 1s;0xFFFE 异常/0xFFFF 无效"},
		{ID: 0x0B, Name: "平台应答超时时间", Type: "u16", Length: 2, RawMin: intp(1), RawMax: intp(600),
			Note: "1~600(1~600s),最小计量单元 1s;0xFFFE 异常/0xFFFF 无效"},
		{ID: 0x0C, Name: "连续三次登入失败后的重登间隔", Type: "u8", Length: 1, RawMin: intp(1), RawMax: intp(240),
			Note: "1~240(1~240min),最小计量单元 1min;0xFE 异常/0xFF 无效"},
		{ID: 0x0D, Name: "公共平台域名长度 N", Type: "u8", Length: 1},
		{ID: 0x0E, Name: "公共平台域名", Type: "bytes", Length: 0,
			Note: "长度 N 由同应答中 0x0D 的值决定(1×N)"},
		{ID: 0x0F, Name: "公共平台端口", Type: "u16", Length: 2, RawMin: intp(0), RawMax: intp(65531),
			Note: "0~65531;0xFFFE 异常/0xFFFF 无效"},
		{ID: 0x10, Name: "是否处于抽样监测中", Type: "u8", Length: 1,
			Options: []ParamOption{{0x01, "是"}, {0x02, "否"}, {0xFE, "异常"}, {0xFF, "无效"}}},
	}
	if v != api.V2025 {
		// 2016 表B.12:0x01 最小计量单元 1ms;0x02/0x03 为 WORD(1~600s / 0~60000ms)
		specs[0].Note = "0~60000(0~60000ms),最小计量单元 1ms;0xFFFE 异常/0xFFFF 无效"
		specs[1] = ParamSpec{ID: 0x02, Name: "正常时信息上报时间周期", Type: "u16", Length: 2, RawMin: intp(1), RawMax: intp(600),
			Note: "1~600(1~600s),最小计量单元 1s;0xFFFE 异常/0xFFFF 无效"}
		specs[2] = ParamSpec{ID: 0x03, Name: "出现报警时信息上报时间周期", Type: "u16", Length: 2, RawMin: intp(0), RawMax: intp(60000),
			Note: "0~60000(0~60000ms),最小计量单元 1ms;0xFFFE 异常/0xFFFF 无效"}
	}
	return specs
}
