package bridge

import (
	"fmt"
	"sync"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"github.com/sunsky74/gb32960/api"
)

// MessageService 报文配置与发送服务。
type MessageService struct {
	rt       *Runtime
	extMu    sync.Mutex
	extStops map[string]chan struct{}

	trackMu        sync.Mutex
	track          trackHook
	reportMu       sync.Mutex
	reportOn       bool
	reportInterval int // 秒;0 = 未设置(取连接配置或默认 10)
}

// trackHook 周期上报与轨迹回放的耦合点(由 TrackService 实现,测试可替换):
// 每次 0x02 tick 组装前推进一个轨迹点;发送失败终止回放;停报即停回放。
// 方法不导出:仅包内协作,不进入前端 RPC 绑定面。
type trackHook interface {
	advanceForReport()
	failOnSend(err error)
	StopReplay()
}

// setTrackReplay 注入轨迹回放钩子(app 装配经 wiring.go 调用)。
func (s *MessageService) setTrackReplay(h trackHook) {
	s.trackMu.Lock()
	defer s.trackMu.Unlock()
	s.track = h
}

func (s *MessageService) replayHook() trackHook {
	s.trackMu.Lock()
	defer s.trackMu.Unlock()
	return s.track
}

// NewMessageService 创建服务。
func NewMessageService(rt *Runtime) *MessageService {
	ms := &MessageService{rt: rt, extStops: map[string]chan struct{}{}}
	if g := ms.loadAllGroups(); g != nil {
		rt.SetGroups(g)
	}
	return ms
}

// GetSchema 返回指定版本的组定义:标准组 + 激活扩展包的追加单元 + 扩展命令组。
// 版本门禁:包的 baseVersion 与请求版本一致才合并;scope 门禁:包须声明 client 应用范围。
// 命令组 Source="command",前端据此分流到自定义数据 tab。
func (s *MessageService) GetSchema(version string) []schema.GroupSchema {
	groups := standardGroups(version)
	if p := s.rt.Pack(); p != nil && ext.ScopeHas(p, ext.ScopeClient) && p.Meta.BaseVersion == version {
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
		if c.Direction != "up" {
			continue // 平台下发模板不进客户端命令组
		}
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
	if p := s.rt.Pack(); p != nil && ext.ScopeHas(p, ext.ScopeClient) && p.Meta.BaseVersion == version {
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
		src = s.loadAllGroups()
		if src == nil {
			return s.DefaultGroups(s.versionText()), nil
		}
	} else if ext := s.loadExtGroups(); len(ext) > 0 {
		// 实时面板保存后内存快照缺命令组:叠加磁盘扩展组配置(内存优先)。
		merged := make(map[string]schema.GroupConfig, len(src)+len(ext))
		for k, v := range src {
			merged[k] = v
		}
		for k, v := range ext {
			if _, ok := merged[k]; !ok {
				merged[k] = v
			}
		}
		src = merged
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

// version 取当前连接配置的协议版本。
func (s *MessageService) version() api.GBTVersion {
	if cfg := s.rt.ConnCfg(); cfg != nil {
		return parseVersion(cfg.Version)
	}
	return api.V2016
}
