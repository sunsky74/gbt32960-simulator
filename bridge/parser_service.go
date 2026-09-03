package bridge

import (
	"fmt"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/parser"
)

// ParserService 报文解析服务(协议调试平台的解析模块入口)。
type ParserService struct {
	rt *Runtime
}

// NewParserService 创建服务。持有 Runtime 以读取扩展包集合(解析页可选包)。
func NewParserService(rt *Runtime) *ParserService { return &ParserService{rt: rt} }

// ParserPacks 返回解析页可用的扩展包(scope 含 parser 且未停用)。
func (s *ParserService) ParserPacks() []PackInfo {
	disabled := loadPackStates()
	return packInfosOf(s.rt.Packs(), disabled, func(p *ext.Pack) bool {
		return ext.ScopeHas(p, ext.ScopeParser)
	})
}

// ParsePacket 解析一条 HEX 报文为逐字段结构。packID 为空表示不使用扩展包;
// 指定包时,包内自定义单元(unitCode 0x80~0xFE)按字段 DSL 解码。
func (s *ParserService) ParsePacket(hexInput string, packID string) (*parser.Result, error) {
	if hexInput == "" {
		return nil, fmt.Errorf("请输入 HEX 报文")
	}
	if packID == "" {
		return parser.Parse(hexInput)
	}
	var pack *ext.Pack
	for _, p := range s.rt.Packs() {
		if p.Meta.ID == packID {
			pack = p
			break
		}
	}
	if pack == nil {
		return nil, fmt.Errorf("扩展包不存在: %s(可能已被删除,请重新选择)", packID)
	}
	if !ext.ScopeHas(pack, ext.ScopeParser) {
		return nil, fmt.Errorf("扩展包「%s」未声明报文解析应用范围(meta.scope 须含 parser)", pack.Meta.Label)
	}
	return parser.ParseWithPack(hexInput, pack)
}
