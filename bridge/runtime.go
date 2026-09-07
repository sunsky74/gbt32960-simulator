// Package bridge 是 wails 胶水层:持有引擎客户端、暴露给前端的服务与事件转发。
// 本包是唯一允许 import wails 的地方。
package bridge

import (
	"sync"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/servermode"
	"github.com/sunsky74/gb32960/types"
)

// Runtime 持有共享事件总线、当前引擎客户端与最新配置快照。
type Runtime struct {
	bus      *engine.Bus
	mu       sync.Mutex
	client   *engine.Client
	connCfg  *ConnectionConfig
	groups   map[string]schema.GroupConfig
	packs    []*ext.Pack
	pack     *ext.Pack
	disabled map[string]bool
}

// NewRuntime 创建运行时。Bus 全局共享:客户端重建不影响前端订阅。
func NewRuntime() *Runtime { return &Runtime{bus: engine.NewBus()} }

// Bus 返回共享事件总线。
func (rt *Runtime) Bus() *engine.Bus { return rt.bus }

// CurrentClient 返回当前客户端(可能为 nil)。
func (rt *Runtime) CurrentClient() *engine.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.client
}

// replaceClient 用新的 options 重建客户端(旧客户端由调用方负责先断开)。
func (rt *Runtime) replaceClient(c *engine.Client) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.client = c
}

// SetConnCfg 保存连接配置快照,并按其 ExtensionPack 解析激活扩展包。
// 换绑/换版本时内存组快照失效:后续 GetGroups 回读磁盘并按包合并扩展组配置。
func (rt *Runtime) SetConnCfg(cfg *ConnectionConfig) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	prev := rt.connCfg
	rt.connCfg = cfg
	rt.pack = rt.resolvePackLocked(rt.packs, cfg)
	rt.syncExtCommands(rt.pack)
	if prev == nil || cfg == nil || prev.Version != cfg.Version || prev.ExtensionPack != cfg.ExtensionPack {
		rt.groups = nil
	}
}

// ConnCfg 返回连接配置快照(可能为 nil)。
func (rt *Runtime) ConnCfg() *ConnectionConfig {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.connCfg
}

// resolvePack 按连接配置的扩展包 id 在已加载集合中查找;未绑定、找不到或已停用返回 nil。
func (rt *Runtime) resolvePackLocked(packs []*ext.Pack, cfg *ConnectionConfig) *ext.Pack {
	if cfg == nil || cfg.ExtensionPack == "" {
		return nil
	}
	for _, p := range packs {
		if p.Meta.ID == cfg.ExtensionPack && !rt.disabled[p.Meta.ID] {
			return p
		}
	}
	return nil
}

// SetPacks 保存已加载的扩展包集合,并按当前连接配置重新解析激活包。
func (rt *Runtime) SetPacks(packs []*ext.Pack) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.packs = packs
	rt.pack = rt.resolvePackLocked(packs, rt.connCfg)
	rt.syncExtCommands(rt.pack)
	rt.syncServerExtCmds(packs)
}

// SetPackStates 更新包级启用/停用状态并重新解析激活包。
// 停用当前绑定包 = 运行时立即失效(命令注销、扩展组不合并),绑定关系保留,重新启用即恢复。
func (rt *Runtime) SetPackStates(disabled map[string]bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.disabled = disabled
	rt.pack = rt.resolvePackLocked(rt.packs, rt.connCfg)
	rt.syncExtCommands(rt.pack)
}

// syncExtCommands 按激活包同步引擎命令注册表:先重置(保留 私有远控 内置),再注册包内命令。
// 挂接在 SetConnCfg/SetPacks——包激活的唯一入口,查询接口(GetSchema)不携带副作用。
// scope 未声明 client 的包不进入客户端链路(仅用于报文解析等场景)。
// 同码多命令先到先得(首个 label 作为该码的显示名);down 模板是服务端下发用,不注册。
func (rt *Runtime) syncExtCommands(p *ext.Pack) {
	engine.ResetExtCommands()
	if p == nil || !ext.ScopeHas(p, ext.ScopeClient) {
		return
	}
	v := parseVersion(p.Meta.BaseVersion)
	for _, c := range p.Commands {
		if c.Direction != "up" {
			continue
		}
		engine.RegisterCommandIfAbsent(v, byte(c.Code), c.Label)
	}
}

// Packs 返回已加载扩展包集合。
func (rt *Runtime) Packs() []*ext.Pack {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.packs
}

// Pack 返回当前激活的扩展包(可能为 nil)。
func (rt *Runtime) Pack() *ext.Pack {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.pack
}

// SetGroups 保存报文配置快照并返回副本。
func (rt *Runtime) SetGroups(g map[string]schema.GroupConfig) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.groups = g
}

// Groups 返回报文配置副本(周期上报 tick 时读取,避免并发修改)。
func (rt *Runtime) Groups() map[string]schema.GroupConfig {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.groups == nil {
		return nil
	}
	out := make(map[string]schema.GroupConfig, len(rt.groups))
	for k, v := range rt.groups {
		out[k] = v
	}
	return out
}

// responseTypeOf 扩展包 respType 语义名 → 协议库应答标志(缺省 command/0xFE)。
func responseTypeOf(name string) types.ResponseType {
	if name == ext.RespTypeSuccess {
		return types.ResponseSuccess
	}
	return types.ResponseCommand
}

// syncServerExtCmds 按已导入包集合同步服务端模式扩展命令规则:
// scope 含 server 的包参与;up 命令注册显示名,声明 serverReply 的同时注册自动应答;
// 同码先到先得。挂接 SetPacks——包增删/启停的唯一汇聚点。
func (rt *Runtime) syncServerExtCmds(packs []*ext.Pack) {
	servermode.ResetExtCmds()
	for _, p := range packs {
		if !ext.ScopeHas(p, ext.ScopeServer) {
			continue
		}
		for _, c := range p.Commands {
			if c.Direction != "up" {
				continue
			}
			if _, exists := servermode.ExtCmd(byte(c.Code)); exists {
				continue
			}
			rule := servermode.ExtCmdRule{Label: c.Label}
			if c.ServerReply != nil {
				rule.Reply = true
				rule.Echo = c.ServerReply.Echo
				rule.RespType = responseTypeOf(c.ServerReply.RespType)
			}
			servermode.RegisterExtCmd(byte(c.Code), rule)
		}
	}
}
