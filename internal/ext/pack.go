// Package ext 加载与编译 GB/T 32960 扩展包(单 JSON 文件):
// 0x02 实时报文尾部追加私有数据单元 + 私有命令帧声明。
package ext

// Pack 一个扩展包的完整声明。
type Pack struct {
	Meta     Meta      `json:"meta"`
	Realtime Realtime  `json:"realtime"`
	Commands []Command `json:"commands,omitempty"`
}

// Meta 包元信息。BaseVersion: "2016" | "2025"。
type Meta struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Vendor      string   `json:"vendor,omitempty"`
	BaseVersion string   `json:"baseVersion"`
	Scope       []string `json:"scope,omitempty"` // 应用范围: ScopeClient / ScopeParser;缺省视为仅 client
}

// 扩展包应用范围取值。
const (
	// ScopeClient 客户端模拟:实时追加单元 + 私有命令 + 私有远控 应答模板。
	ScopeClient = "client"
	// ScopeParser 报文解析:解析页可选用该包解码自定义数据单元。
	ScopeParser = "parser"
	// ScopeServer 服务端模式:私有命令的下行应答规则(serverReply)与平台下发模板(direction=down)。
	ScopeServer = "server"
)

// ScopeHas 报告包是否声明了指定应用范围。scope 缺省(空)视为仅 client(向后兼容旧包)。
func ScopeHas(p *Pack, scope string) bool {
	if p == nil {
		return false
	}
	if len(p.Meta.Scope) == 0 {
		return scope == ScopeClient
	}
	for _, s := range p.Meta.Scope {
		if s == scope {
			return true
		}
	}
	return false
}

// Realtime 0x02 实时报文的扩展段。
type Realtime struct {
	AppendUnits []AppendUnit `json:"appendUnits,omitempty"`
}

// AppendUnit 追加到标准体之后的私有数据单元(TLV)。
type AppendUnit struct {
	Key      string      `json:"key"`
	Title    string      `json:"title"`
	UnitCode int         `json:"unitCode"`
	Enabled  bool        `json:"enabled"`
	Multiple bool        `json:"multiple,omitempty"`
	MaxRows  int         `json:"maxRows,omitempty"`
	Fields   []FieldSpec `json:"fields"`
}

// Command 私有命令帧声明(Phase 3 实现发送,Phase 1 仅定义与校验)。
// RemoteSub > 0 时表示 私有远控 0x8A 远程控制应答模板(命令码固定 0x8A,
// 子指令码 = RemoteSub,发送帧走应答标志 0x01,载荷 = 表21头 + 应答体)。
// RespType 声明发送帧的应答标志(见 ResponseType 取值);ServerReply 声明
// 服务端模式(scope 含 server)对该命令请求帧的自动应答规则。
// Direction=down 表示平台下发模板:仅供服务端模式下发,不进客户端命令组。
type Command struct {
	Key         string       `json:"key"`
	Label       string       `json:"label"`
	Code        int          `json:"code"`
	Direction   string       `json:"direction"` // up | down
	Trigger     string       `json:"trigger"`   // manual | periodic | manual+periodic
	RemoteSub   int          `json:"remoteSub,omitempty"`
	RespType    string       `json:"respType,omitempty"`    // command | success;缺省 command
	ServerReply *ServerReply `json:"serverReply,omitempty"` // 服务端收到该命令请求帧时的应答规则
	Body        CommandBody  `json:"body"`
}

// ServerReply 服务端模式的下行应答规则:收到命令码命中的请求帧(标志 0xFE)时自动回应答。
type ServerReply struct {
	RespType string `json:"respType"` // command | success;缺省 success
	Echo     bool   `json:"echo"`     // true=回显请求体;false=空体应答
}

// 命令应答标志语义名(与协议库 ResponseType 的映射由 bridge 层完成)。
const (
	RespTypeCommand = "command" // 请求/命令标志 0xFE(上行数据帧缺省)
	RespTypeSuccess = "success" // 成功应答标志 0x01
)

// CommandBody 命令体布局:平铺字段或实时报文同构(6B 十进制时间 + TLV 单元)。
type CommandBody struct {
	Type   string       `json:"type"` // fields | realtimeLike
	Fields []FieldSpec  `json:"fields,omitempty"`
	Units  []AppendUnit `json:"units,omitempty"`
}

// FieldSpec 字段 DSL 声明。物理值 = 线值×scale + offset。
// tail:变长 hex 原样追加(用于故障列表等按数量变化的尾部),无 scale/offset/length 约束。
type FieldSpec struct {
	Key    string    `json:"key"`
	Label  string    `json:"label"`
	Type   string    `json:"type"` // u8/u16/u32/i8/i16/i32/f32/bits/bytes/tail
	Unit   string    `json:"unit,omitempty"`
	Scale  *float64  `json:"scale,omitempty"`
	Offset *float64  `json:"offset,omitempty"`
	Bits   []BitSpec `json:"bits,omitempty"`   // type=bits
	Length int       `json:"length,omitempty"` // type=bytes
}

// BitSpec 位段定义,index 0~31。
type BitSpec struct {
	Index int    `json:"index"`
	Label string `json:"label"`
}
