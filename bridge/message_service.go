package bridge

import (
	"context"
	"fmt"
	"time"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/store"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

const groupsFile = "message.json"

// PreviewResult 发送前的报文预览。
type PreviewResult struct {
	Cmd string `json:"cmd"`
	Hex string `json:"hex"`
	Len int    `json:"len"`
}

// MessageService 报文配置与发送服务。
type MessageService struct {
	rt *Runtime
}

// NewMessageService 创建服务。
func NewMessageService(rt *Runtime) *MessageService {
	ms := &MessageService{rt: rt}
	if g := ms.loadGroups(); g != nil {
		rt.SetGroups(g)
	}
	return ms
}

// GetSchema 返回指定版本的组定义:标准组 + 激活扩展包的追加单元。
// 版本门禁:包的 baseVersion 与请求版本一致才合并。
func (s *MessageService) GetSchema(version string) []schema.GroupSchema {
	groups := standardGroups(version)
	if p := s.rt.Pack(); p != nil && p.Meta.BaseVersion == version {
		for _, u := range p.Realtime.AppendUnits {
			groups = append(groups, ext.CompileUnit(u))
		}
	}
	return groups
}

func standardGroups(version string) []schema.GroupSchema {
	if version == "2025" {
		return schema.V2025Groups()
	}
	return schema.V2016Groups()
}

// DefaultGroups 基于组定义生成带初始值的配置(每组一行)。
// 扩展组的默认行值来自 ext.DefaultsFor(数值=offset、位=false、bytes=00 填充)。
func (s *MessageService) DefaultGroups(version string) *schema.GroupsPayload {
	groups := s.GetSchema(version)
	unitDefaults := s.unitDefaults(version)
	out := map[string]schema.GroupConfig{}
	for _, g := range groups {
		if d, ok := unitDefaults[g.Key]; ok {
			out[g.Key] = schema.GroupConfig{Enabled: g.Enabled, Rows: []map[string]any{d}}
			continue
		}
		row := schema.RowValue{}
		for _, f := range g.Fields {
			row[f.Key] = defaultFieldValue(f)
		}
		out[g.Key] = schema.GroupConfig{Enabled: g.Enabled, Rows: []map[string]any{row}}
	}
	return schema.FromMap(out, groupOrder(groups))
}

func (s *MessageService) unitDefaults(version string) map[string]schema.RowValue {
	out := map[string]schema.RowValue{}
	if p := s.rt.Pack(); p != nil && p.Meta.BaseVersion == version {
		for _, u := range p.Realtime.AppendUnits {
			out[u.Key] = ext.DefaultsFor(u)
		}
	}
	return out
}

// GetGroups 读取持久化的报文配置;无历史返回默认值。
// 必须先按 order 过滤再 FromMap:schema.FromMap 的第二段循环会把 order 之外
// 的键追加进 payload(不丢弃未知键),残留扩展键的过滤只能在调用前完成。
func (s *MessageService) GetGroups() (*schema.GroupsPayload, error) {
	order := groupOrder(s.GetSchema(s.versionText()))
	src := s.rt.Groups()
	if src == nil {
		src = s.loadGroups()
		if src == nil {
			return s.DefaultGroups(s.versionText()), nil
		}
	}
	filtered := make(map[string]schema.GroupConfig, len(order))
	for _, k := range order {
		if g, ok := src[k]; ok {
			filtered[k] = g
		}
	}
	return schema.FromMap(filtered, order), nil
}

// versionText 当前连接配置的版本字符串(修复既有硬编码 "2016")。
func (s *MessageService) versionText() string {
	if cfg := s.rt.ConnCfg(); cfg != nil {
		return cfg.Version
	}
	return "2016"
}

func groupOrder(groups []schema.GroupSchema) []string {
	order := make([]string, len(groups))
	for i, g := range groups {
		order[i] = g.Key
	}
	return order
}

func defaultFieldValue(f schema.FieldSchema) any {
	switch f.Kind {
	case "enum":
		if len(f.Enum) > 0 {
			return f.Enum[0].Value
		}
		return 1
	case "bool":
		return false
	case "bitgroup":
		bits := map[string]any{}
		for _, b := range f.Bits {
			bits[fmt.Sprintf("bit%d", b.Index)] = false
		}
		return bits
	case "array_float":
		return []any{}
	case "int":
		return 0
	default:
		return float64(0)
	}
}

// SaveGroups 校验(组装一遍)、持久化并快照报文配置。
func (s *MessageService) SaveGroups(payload schema.GroupsPayload) error {
	groups := payload.ToMap()
	if _, err := schema.Assemble(s.version(), groups, time.Now()); err != nil {
		return err
	}
	s.rt.SetGroups(groups)
	return store.Save(groupsFile, &groups)
}

func (s *MessageService) loadGroups() map[string]schema.GroupConfig {
	var g map[string]schema.GroupConfig
	if err := store.Load(groupsFile, &g); err != nil || g == nil {
		return nil
	}
	return g
}

// assembleBody 组装 0x02/0x03 报文体:标准体(typed)+ 激活扩展包的追加 TLV。
// 未绑包/版本不符/无启用行 → 原样返回标准体(与既有行为逐字节一致)。
func (s *MessageService) assembleBody(at time.Time) (model.MessageBody, error) {
	groups := s.rt.Groups()
	if groups == nil {
		groups = s.DefaultGroups(s.versionText()).ToMap()
	}
	base, err := schema.Assemble(s.version(), groups, at)
	if err != nil {
		return nil, err
	}
	p := s.rt.Pack()
	if p == nil || p.Meta.BaseVersion != s.versionText() {
		return base, nil
	}
	tail := make([]byte, 0, 64)
	for _, u := range p.Realtime.AppendUnits {
		g, ok := groups[u.Key]
		if !ok || !g.Enabled || len(g.Rows) == 0 {
			continue
		}
		for _, row := range g.Rows { // multiple 语义:每行独立 TLV
			tlv, err := ext.EncodeUnit(u, row)
			if err != nil {
				return nil, fmt.Errorf("扩展单元 %s: %w", u.Key, err)
			}
			tail = append(tail, tlv...)
		}
	}
	if len(tail) == 0 {
		return base, nil
	}
	baseBytes, err := base.Bytes()
	if err != nil {
		return nil, err
	}
	return engine.NewRawBody(s.version(), append(baseBytes, tail...)), nil
}

// Preview 用当前配置生成 0x02 报文 hex(不发送)。
func (s *MessageService) Preview() (*PreviewResult, error) {
	cfg := s.rt.ConnCfg()
	if cfg == nil {
		cfg = DefaultConnectionConfig()
	}
	body, err := s.assembleBody(time.Now())
	if err != nil {
		return nil, err
	}
	raw, name, err := engine.BuildFrame(parseVersion(cfg.Version), cfg.VIN, 0x02, body)
	if err != nil {
		return nil, err
	}
	return &PreviewResult{Cmd: name, Hex: utils.BytesToHex(raw), Len: len(raw)}, nil
}

// SendRealtime 立即发送一次 0x02 实时信息上报。
func (s *MessageService) SendRealtime() error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录 (state=%s)", s.stateText())
	}
	if s.rt.Groups() == nil {
		return fmt.Errorf("报文配置为空,请先保存报文配置")
	}
	body, err := s.assembleBody(time.Now())
	if err != nil {
		return err
	}
	return c.Send(context.Background(), 0x02, body)
}

// SendReissue 发送 count 条 0x03 补发。时间戳从 now-offsetSec 起,
// 每条按 intervalSec 前移 —— 模拟离线时段内每 intervalSec 采集一条的补报。
func (s *MessageService) SendReissue(count int, offsetSec int, intervalSec int) error {
	if count <= 0 || count > 100 {
		return fmt.Errorf("补发条数须在 1~100 之间")
	}
	if offsetSec < 0 {
		return fmt.Errorf("起始时间偏移不能为负")
	}
	if intervalSec <= 0 || intervalSec > 86400 {
		return fmt.Errorf("补发间隔须在 1~86400 秒之间")
	}
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录 (state=%s)", s.stateText())
	}
	groups := s.rt.Groups()
	if groups == nil {
		return fmt.Errorf("报文配置为空,请先保存报文配置")
	}
	base := time.Now().Add(-time.Duration(offsetSec) * time.Second)
	for i := 0; i < count; i++ {
		at := base.Add(-time.Duration(i) * time.Duration(intervalSec) * time.Second)
		body, err := s.assembleBody(at)
		if err != nil {
			return err
		}
		if err := c.Send(context.Background(), 0x03, body); err != nil {
			return err
		}
	}
	return nil
}

// SetAutoReport 开关周期上报(默认 10s,可改)。enabled=false 时停止。
func (s *MessageService) SetAutoReport(enabled bool, intervalSec int) error {
	c := s.rt.CurrentClient()
	if c == nil {
		return fmt.Errorf("未连接")
	}
	if !enabled {
		c.SetAutoReport(0, nil)
		return nil
	}
	if intervalSec <= 0 {
		intervalSec = 10
	}
	c.SetAutoReport(time.Duration(intervalSec)*time.Second, func() error {
		if s.rt.Groups() == nil {
			return fmt.Errorf("报文配置为空")
		}
		body, err := s.assembleBody(time.Now())
		if err != nil {
			return err
		}
		return c.Send(context.Background(), 0x02, body)
	})
	return nil
}

// version 取当前连接配置的协议版本。
func (s *MessageService) version() api.GBTVersion {
	if cfg := s.rt.ConnCfg(); cfg != nil {
		return parseVersion(cfg.Version)
	}
	return api.V2016
}

// ParamRespondRow 0x80 参数查询应答行(前端提交)。
type ParamRespondRow = engine.ParamResponseRow

// RespondAck 对平台下行命令发送应答码(空载荷)。适用 0x81/0x82 标准应答
// 与 私有远控 0x8A 第一层 ACK。respCode: 0x01 成功 / 0x02 错误 / ...
func (s *MessageService) RespondAck(cmd byte, respCode byte) error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return c.RespondAck(cmd, types.ResponseType(respCode))
}

// RespondParamQuery 对 0x80 参数查询发送参数值应答。
func (s *MessageService) RespondParamQuery(rows []ParamRespondRow, respCode byte) error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return c.RespondParamQuery(rows, types.ResponseType(respCode))
}

// RespondRemoteSecondLayer 发送 私有远控 0x8A 第二层业务应答(回显表21头+信息体)。
func (s *MessageService) RespondRemoteSecondLayer(headerHex string, bodyHex string) error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return c.RespondRemoteSecondLayer(headerHex, bodyHex)
}

func (s *MessageService) stateText() string {
	if c := s.rt.CurrentClient(); c != nil {
		return string(c.State())
	}
	return string(engine.StateIdle)
}
