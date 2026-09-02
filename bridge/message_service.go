package bridge

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	rt       *Runtime
	extMu    sync.Mutex
	extStops map[string]chan struct{}
}

// NewMessageService 创建服务。
func NewMessageService(rt *Runtime) *MessageService {
	ms := &MessageService{rt: rt, extStops: map[string]chan struct{}{}}
	if g := ms.loadGroups(); g != nil {
		rt.SetGroups(g)
	}
	return ms
}

// GetSchema 返回指定版本的组定义:标准组 + 激活扩展包的追加单元 + 扩展命令组。
// 版本门禁:包的 baseVersion 与请求版本一致才合并。
// 命令组 Source="command",前端据此分流到自定义数据 tab。
func (s *MessageService) GetSchema(version string) []schema.GroupSchema {
	groups := standardGroups(version)
	if p := s.rt.Pack(); p != nil && p.Meta.BaseVersion == version {
		for _, u := range p.Realtime.AppendUnits {
			groups = append(groups, ext.CompileUnit(u))
		}
		groups = append(groups, s.compileCommandGroups(p)...)
	}
	return groups
}

// compileCommandGroups 编译扩展命令组:fields 体 → 单组(键=命令 key);
// realtimeLike 体 → 每单元一组(键=命令key:单元key)。全部 Source="command"。
func (s *MessageService) compileCommandGroups(p *ext.Pack) []schema.GroupSchema {
	out := make([]schema.GroupSchema, 0, len(p.Commands))
	for _, c := range p.Commands {
		switch c.Body.Type {
		case "fields":
			g := ext.CompileUnit(ext.AppendUnit{Key: c.Key, Title: c.Label, Fields: c.Body.Fields, Enabled: true})
			g.Source = "command"
			out = append(out, g)
		case "realtimeLike":
			for _, u := range c.Body.Units {
				g := ext.CompileUnit(u)
				g.Key = c.Key + ":" + u.Key
				g.Title = c.Label + " · " + u.Title
				g.Source = "command"
				out = append(out, g)
			}
		}
	}
	return out
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
		for _, c := range p.Commands {
			switch c.Body.Type {
			case "fields":
				out[c.Key] = ext.DefaultsFor(ext.AppendUnit{Fields: c.Body.Fields})
			case "realtimeLike":
				for _, u := range c.Body.Units {
					out[c.Key+":"+u.Key] = ext.DefaultsFor(u)
				}
			}
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

// SaveGroups 校验(标准组装 + 扩展行干跑)、持久化并快照报文配置。
func (s *MessageService) SaveGroups(payload schema.GroupsPayload) error {
	groups := payload.ToMap()
	if _, err := schema.Assemble(s.version(), groups, time.Now()); err != nil {
		return err
	}
	if err := s.validateExtRows(groups); err != nil {
		return err
	}
	s.rt.SetGroups(groups)
	return store.Save(groupsFile, &groups)
}

// validateExtRows 对激活包的扩展组行值做字段级编码干跑:非法值在保存点拦截,不落盘。
// 跳过语义与组装一致(assembleBody/assembleCommandBody):未配置或未启用的组不校验。
func (s *MessageService) validateExtRows(groups map[string]schema.GroupConfig) error {
	p := s.rt.Pack()
	if p == nil {
		return nil
	}
	for _, u := range p.Realtime.AppendUnits {
		g, ok := groups[u.Key]
		if !ok || !g.Enabled {
			continue
		}
		for ri, row := range g.Rows {
			if _, err := ext.EncodeUnit(u, row); err != nil {
				return fmt.Errorf("扩展单元 %s 第 %d 行: %w", u.Key, ri+1, err)
			}
		}
	}
	for _, c := range p.Commands {
		switch c.Body.Type {
		case "fields":
			g, ok := groups[c.Key]
			if !ok || !g.Enabled || len(g.Rows) == 0 {
				continue
			}
			if _, err := ext.EncodeFields(c.Body.Fields, g.Rows[0]); err != nil {
				return fmt.Errorf("扩展命令 %s: %w", c.Key, err)
			}
		case "realtimeLike":
			for _, u := range c.Body.Units {
				g, ok := groups[c.Key+":"+u.Key]
				if !ok || !g.Enabled {
					continue
				}
				for ri, row := range g.Rows {
					if _, err := ext.EncodeUnit(u, row); err != nil {
						return fmt.Errorf("扩展命令 %s 单元 %s 第 %d 行: %w", c.Key, u.Key, ri+1, err)
					}
				}
			}
		}
	}
	return nil
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

// packCommand 按 key 查找激活包内的扩展命令;未绑定/不存在返回 nil。
func packCommand(p *ext.Pack, key string) *ext.Command {
	if p == nil {
		return nil
	}
	for i := range p.Commands {
		if p.Commands[i].Key == key {
			return &p.Commands[i]
		}
	}
	return nil
}

// assembleCommandBody 组装扩展命令报文体:
// fields=平铺字段单行;realtimeLike=6B 十进制时间 + 单元 TLV×N。
func (s *MessageService) assembleCommandBody(cmd ext.Command, at time.Time) ([]byte, error) {
	groups := s.rt.Groups()
	switch cmd.Body.Type {
	case "fields":
		grp, ok := groups[cmd.Key]
		if !ok || !grp.Enabled || len(grp.Rows) == 0 {
			return nil, fmt.Errorf("扩展命令 %s 未配置或未启用", cmd.Key)
		}
		return ext.EncodeFields(cmd.Body.Fields, grp.Rows[0])
	case "realtimeLike":
		t := ext.EncodeBeanTime(at)
		out := append([]byte{}, t[:]...)
		for _, u := range cmd.Body.Units {
			grp, ok := groups[cmd.Key+":"+u.Key]
			if !ok || !grp.Enabled || len(grp.Rows) == 0 {
				continue
			}
			for _, row := range grp.Rows {
				tlv, err := ext.EncodeUnit(u, row)
				if err != nil {
					return nil, fmt.Errorf("扩展命令 %s 单元 %s: %w", cmd.Key, u.Key, err)
				}
				out = append(out, tlv...)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("扩展命令 %s: 未知体类型 %q", cmd.Key, cmd.Body.Type)
	}
}

// SendExtension 手动发送一次扩展命令(须在线)。
func (s *MessageService) SendExtension(key string) error {
	p := s.rt.Pack()
	if p == nil {
		return fmt.Errorf("未绑定扩展包")
	}
	if p.Meta.BaseVersion != s.versionText() {
		return fmt.Errorf("扩展包基准版本 %s 与当前协议版本 %s 不匹配", p.Meta.BaseVersion, s.versionText())
	}
	cmd := packCommand(p, key)
	if cmd == nil {
		return fmt.Errorf("扩展命令不存在: %s", key)
	}
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	if s.version() != c.Version() {
		return fmt.Errorf("连接档案版本已变更,请重新连接后再发送扩展命令")
	}
	payload, err := s.assembleCommandBody(*cmd, time.Now())
	if err != nil {
		return err
	}
	return c.Send(context.Background(), byte(cmd.Code), engine.NewRawBody(s.version(), payload))
}

// SetExtAutoReport 开关扩展命令的周期发送(每命令独立 ticker)。
func (s *MessageService) SetExtAutoReport(key string, enabled bool, intervalSec int) error {
	if !enabled {
		s.stopExtReport(key)
		return nil
	}
	if intervalSec <= 0 {
		intervalSec = 10
	}
	p := s.rt.Pack()
	if p == nil {
		return fmt.Errorf("未绑定扩展包")
	}
	cmd := packCommand(p, key)
	if cmd == nil {
		return fmt.Errorf("扩展命令不存在: %s", key)
	}
	if p.Meta.BaseVersion != s.versionText() {
		return fmt.Errorf("扩展包基准版本 %s 与当前协议版本 %s 不匹配", p.Meta.BaseVersion, s.versionText())
	}
	if cmd.Trigger == "manual" {
		return fmt.Errorf("该命令不支持周期上报 (trigger=manual)") // 评审 P2-4:消费 trigger 语义
	}
	s.startExtReport(key, time.Duration(intervalSec)*time.Second)
	return nil
}

func (s *MessageService) stopExtReport(key string) {
	s.extMu.Lock()
	stop, ok := s.extStops[key]
	if ok {
		delete(s.extStops, key)
		close(stop)
	}
	s.extMu.Unlock()
}

// stopExtReportIfOwn 仅当注册表中 key 仍指向 own 时才删除并关闭它,
// 防止在途 ticker 的自停路径误杀解绑→重绑后新注册的 ticker(TOCTOU)。
func (s *MessageService) stopExtReportIfOwn(key string, own chan struct{}) {
	s.extMu.Lock()
	defer s.extMu.Unlock()
	if s.extStops[key] == own {
		delete(s.extStops, key)
		close(own)
	}
}

func (s *MessageService) startExtReport(key string, interval time.Duration) {
	s.stopExtReport(key)
	stop := make(chan struct{})
	s.extMu.Lock()
	s.extStops[key] = stop
	s.extMu.Unlock()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				// 包被解绑/命令消失/版本切换 → 自停清理
				p := s.rt.Pack()
				if p == nil || packCommand(p, key) == nil || p.Meta.BaseVersion != s.versionText() {
					s.stopExtReportIfOwn(key, stop)
					return
				}
				if err := s.SendExtension(key); err != nil {
					// 未连接属常态,静默跳过;其余错误进事件总线
					if !strings.Contains(err.Error(), "未连接") {
						s.rt.Bus().Emit(engine.Event{Kind: engine.EventError, Message: "扩展命令周期上报失败: " + err.Error()})
					}
				}
			}
		}
	}()
}
