package bridge

import (
	"fmt"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/servermode"
	"github.com/sunsky74/gb32960/api"
)

// ExtCommandInfo 服务端可下发的扩展命令模板(来自已导入包 scope=server 的 down 命令)。
type ExtCommandInfo struct {
	PackID    string               `json:"packId"`
	PackLabel string               `json:"packLabel"`
	Key       string               `json:"key"`
	Label     string               `json:"label"`
	Code      int                  `json:"code"`
	RespType  string               `json:"respType"`
	Fields    []schema.FieldSchema `json:"fields"`
	Defaults  map[string]any       `json:"defaults"`
}

// ServerExtCommands 枚举所有已导入包(scope 含 server)的平台下发模板(direction=down)。
func (s *ServerService) ServerExtCommands() []ExtCommandInfo {
	var out []ExtCommandInfo
	if s.rt == nil {
		return out
	}
	for _, p := range s.rt.Packs() {
		if !ext.ScopeHas(p, ext.ScopeServer) {
			continue
		}
		for _, c := range p.Commands {
			if c.Direction != "down" || c.Body.Type != "fields" {
				continue
			}
			fields := make([]schema.FieldSchema, len(c.Body.Fields))
			for i, f := range c.Body.Fields {
				fields[i] = ext.CompileField(f)
			}
			out = append(out, ExtCommandInfo{
				PackID:    p.Meta.ID,
				PackLabel: p.Meta.Label,
				Key:       c.Key,
				Label:     c.Label,
				Code:      c.Code,
				RespType:  c.RespType,
				Fields:    fields,
				Defaults:  ext.DefaultsFor(ext.AppendUnit{Fields: c.Body.Fields}),
			})
		}
	}
	return out
}

// SendExtCommand 按 包ID/命令key 组帧并下发到指定 VIN 会话(服务须已启动)。
// row 为字段值(前端表单);组帧 respType 按命令声明(缺省 command/0xFE)。
func (s *ServerService) SendExtCommand(packID, key, vin string, row map[string]any) error {
	if s.rt == nil {
		return fmt.Errorf("运行时未注入")
	}
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil {
		return fmt.Errorf("服务未启动")
	}
	var cmd *ext.Command
	for _, p := range s.rt.Packs() {
		if p.Meta.ID != packID || !ext.ScopeHas(p, ext.ScopeServer) {
			continue
		}
		for i := range p.Commands {
			if p.Commands[i].Key == key && p.Commands[i].Direction == "down" {
				cmd = &p.Commands[i]
				break
			}
		}
	}
	if cmd == nil {
		return fmt.Errorf("下发模板不存在: %s/%s", packID, key)
	}
	if cmd.Body.Type != "fields" {
		return fmt.Errorf("仅支持平铺 fields 体模板")
	}
	body, err := ext.EncodeFields(cmd.Body.Fields, schema.RowValue(row))
	if err != nil {
		return fmt.Errorf("字段编码失败: %w", err)
	}
	raw, err := servermode.BuildFrame(api.V2016, vin, byte(cmd.Code), responseTypeOf(cmd.RespType), body)
	if err != nil {
		return fmt.Errorf("组帧失败: %w", err)
	}
	return srv.WriteFrameVIN(vin, byte(cmd.Code), raw, "平台下发 "+cmd.Label)
}
