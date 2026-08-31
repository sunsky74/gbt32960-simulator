package bridge

import (
	"fmt"

	"gbt32960-simulator/internal/parser"
)

// ParserService 报文解析服务(协议调试平台的解析模块入口)。
type ParserService struct{}

// NewParserService 创建服务。
func NewParserService() *ParserService { return &ParserService{} }

// ParsePacket 解析一条 HEX 报文为逐字段结构。
func (s *ParserService) ParsePacket(hexInput string) (*parser.Result, error) {
	if hexInput == "" {
		return nil, fmt.Errorf("请输入 HEX 报文")
	}
	return parser.Parse(hexInput)
}
