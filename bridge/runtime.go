// Package bridge 是 wails 胶水层:持有引擎客户端、暴露给前端的服务与事件转发。
// 本包是唯一允许 import wails 的地方。
package bridge

import (
	"sync"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/schema"
)

// Runtime 持有共享事件总线、当前引擎客户端与最新配置快照。
type Runtime struct {
	bus     *engine.Bus
	mu      sync.Mutex
	client  *engine.Client
	connCfg *ConnectionConfig
	groups  map[string]schema.GroupConfig
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

// SetConnCfg 保存连接配置快照。
func (rt *Runtime) SetConnCfg(cfg *ConnectionConfig) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.connCfg = cfg
}

// ConnCfg 返回连接配置快照(可能为 nil)。
func (rt *Runtime) ConnCfg() *ConnectionConfig {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.connCfg
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
