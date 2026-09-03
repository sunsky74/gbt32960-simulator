# Phase 2 · 实时数据扩展端到端 Implementation Plan

> **For agentic workers:** Recommended execution: use superpowers:ltdd for Quality-LTDD. Alternatives: superpowers:subagent-driven-development(Phase 1 已用此方式验证顺畅), superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 实施人员在 packs 目录放入扩展包 JSON 并在连接档案中选择后,模拟器的实时面板自动出现私有字段表单,0x02 实时 / 0x03 补发 / hex 预览 / 周期上报四条路径统一在帧尾追加私有 TLV;未绑定扩展包时现有行为逐字节不变。

**Architecture:** P1 的 `internal/ext`(加载/校验/编码/编译)为地基;本阶段纯增量接线:Runtime 增加扩展包状态 → ConnectionConfig 增加绑定字段 → MessageService 的 schema 与组装管线感知扩展 → 前端渲染 bytes kind 与扩展包选择。标准体继续走 typed struct,扩展 TLV 在字节层拼接(设计 §4.3/§4.5)。

**Tech Stack:** Go 1.25 / Wails v2 / Vue 3 + Ant Design Vue;`wails generate module` 再生成前端绑定。

**Master 索引:** `docs/superpowers/plans/2026-08-31-extpack-master-plan.md`
**设计契约:** `docs/superpowers/specs/2026-08-31-extpack-design.md` §4(§9 G-1 已落地、G-2 已裁决)

## 阶段目标与业务里程碑

**阶段目标(一句话):** 让"0x02 报文加私有字段"从开发者能力变成实施人员能力——放文件、选档案、填表、发送,全程零代码。

**业务里程碑(验收顺序即达成顺序):**

| 里程碑 | 内容 | 覆盖任务 | 对应 Global AC |
|---|---|---|---|
| **M1 包可发现、可绑定** | packs 目录扫描出包列表;连接档案记住扩展包选择;Runtime 持有激活包 | Task 1-2 | AC-5 前半 |
| **M2 表单可见** | 绑定后实时面板渲染私有组(int/float/bitgroup/bytes 全 kind),bytes 为 hex 输入 | Task 3 + Task 6 | AC-1 前半 |
| **M3 发送闭环** | 预览=发送=周期=补发,四路统一在标准体后追加私有 TLV;hex 预览即所得 | Task 4-5 | AC-1 后半 |
| **M4 回归无损** | 未绑包逐字节不变;换绑/解绑后 schema/配置正确刷新,残留键被过滤 | Task 3-5 测试 | AC-3、AC-5 |

## Global Constraints(继承 Master + 本阶段特有)

- 协议库 gb32960-go 零改动;P1 交付的 `internal/ext` 导出 API **不改签名,只消费**
- **未绑定扩展包(或绑定的包无启用单元)时,Preview/SendRealtime/SetAutoReport/SendReissue 与现状逐字节一致**(AC-3 硬约束,单测锁定)
- 扩展合并与追加的门禁:`rt.Pack() != nil && pack.Meta.BaseVersion == 当前版本`(版本不匹配 → 不合并不追加,视为无包)
- 扩展 TLV 仅追加在报文体尾部;`multiple` 单元每行一个独立 TLV
- packs 目录 = `store.Dir()/packs`(即 `os.UserConfigDir()/gbt32960-simulator/packs`),由 ExtService 创建与扫描;坏包文件不阻塞好包
- 前端零新增依赖;bytes 输入用 a-input + hex 校验(等宽字体)
- 前端绑定(frontend/wailsjs)经 `wails generate module` 再生成后**必须随代码提交**
- 现有 golden/单测与 `npm run build` 必须保持全绿;收尾跑 `go test ./...` + `cd frontend && npm run build`

## Final Acceptance Checklist (Refined from Spec) - MUST

- [PAC-1](源:AC-1) 绑定含 appendUnits 的包并绑定档案后,实时面板出现私有组表单;0x02 载荷尾含正确 TLV(其后仅 1 字节 BCC)
  Refinement: 后端单测 `TestAssembleBodyAppendsTLV`(固定时刻 → 体 hex 逐字符 `1a0102030405` + `800003500021`)与 `TestPreviewTailGolden`(Preview 全帧 `2323` 开头、**剥离末位 BCC 后**载荷以 TLV 结尾);手动 `wails dev`:面板出现"私有遥测"组、bytes 输入可用、发送后控制台 hex 载荷尾同 TLV
- [PAC-2](源:AC-3) 未绑包时行为逐字节不变
  Refinement: 主证据为既有 golden 全绿(`go test ./...`);`TestAssembleBodyNoPackIdentical` 为管线烟雾检查(无包时 assembleBody 原样透传标准体,断言与 `schema.Assemble` 字节一致)
- [PAC-3](源:AC-5) 换绑/解绑经档案切换生效;持久化配置不丢标准组、残留扩展键被过滤
  Refinement: `TestGetGroupsFiltersStaleKeys`(groups 含已解绑包的旧键 → 返回 payload 不含该键且标准键保留);前端 onSwitchProfile/saveOnly 在 extensionPack 变化时刷新 schema(`reloadSchemaPreservingGroups`)
- [PAC-4](源:AC-1 补发一致 + 修复既有 bug) SendReissue 统一走扩展管线且 2025 连接不再发 2016 形体
  Refinement: `TestAssembleBodyV2025VersionDispatch` 锁定 assembleBody 的版本分发(V2025 配置下与 `AssembleRealtimeV2025` 字节一致);SendReissue 本身需 online client 无法离线测,统一由 grep 判据保证(bridge 层不再出现 `schema.AssembleRealtime` 直调)

---

## File Structure(本阶段触碰面)

| 文件 | 动作 | 职责 |
|---|---|---|
| `bridge/connection_service.go` | Modify(+1 字段) | ConnectionConfig.ExtensionPack |
| `bridge/runtime.go` | Modify(+pack 状态) | packs/pack 持有与按配置解析 |
| `bridge/ext_service.go` | Create | 包目录扫描/列表/重载(前端绑定) |
| `app.go` / `main.go` | Modify(装配) | ExtService 创建与 Bind |
| `bridge/message_service.go` | Modify | GetSchema 合并、DefaultGroups/GetGroups 感知、assembleBody 四路拼接 |
| `bridge/runtime_ext_test.go`、`bridge/ext_service_test.go`、`bridge/message_service_ext_test.go`、`bridge/testdata/extpack.json` | Create | 本阶段测试与夹具 |
| `frontend/src/components/RealTimePanel.vue` | Modify | bytes kind 渲染 + hex 辅助函数 |
| `frontend/src/components/cards/ConnectionCard.vue` | Modify | 扩展包下拉选择 |
| `frontend/src/state.ts` | Modify | reloadSchemaPreservingGroups |
| `frontend/src/composables/useConnActions.ts`、`frontend/src/pages/ClientSimulatorPage.vue` | Modify | 包变化联动刷新 |
| `frontend/wailsjs/**`(再生成) | Regenerate | ExtService 绑定 + 类型更新 |

---

### Task 1: ConnectionConfig.ExtensionPack 字段与 Runtime 扩展包状态

**Level:** L3
**Level Rationale:** 跨模块状态层改动(配置结构 + Runtime 并发状态),是后续所有任务的依赖根;含持久化契约变化。
**Linked Acceptance Items:** PAC-3(绑定状态的载体)
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=P1 `ext.Pack` 类型;下游=Task 2(ExtService 写入 packs)、Task 3/4(读 Pack())。本任务建立"配置→激活包"的唯一解析入口,后续任务只读写 `rt.Pack()`。
**重要性:** **P0(地基)**——没有激活包状态,M2/M3 全部无从谈起;解析逻辑收敛在 Runtime 一处,避免各服务各自查找造成漂移。

**Files:**
- Modify: `bridge/connection_service.go`(ConnectionConfig 结构体,约 18-32 行)
- Modify: `bridge/runtime.go`
- Test: `bridge/runtime_ext_test.go`(Create)

**Interfaces:**
- Consumes: `ext.Pack`(P1)
- Produces: `ConnectionConfig.ExtensionPack string`(json `extensionPack,omitempty`)、`Runtime.SetPacks(packs []*ext.Pack)`、`Runtime.Packs() []*ext.Pack`、`Runtime.Pack() *ext.Pack`——`SetConnCfg` 内部自动按 `cfg.ExtensionPack` 解析激活包(所有既有调用点自动获得该行为,无需逐个改)

- [ ] **Step 1: 写失败测试**

`bridge/runtime_ext_test.go`:

```go
package bridge

import (
	"testing"

	"gbt32960-simulator/internal/ext"
)

func extTestPack(id string) *ext.Pack {
	return &ext.Pack{
		Meta: ext.Meta{ID: id, Label: id, BaseVersion: "2016"},
		Realtime: ext.Realtime{AppendUnits: []ext.AppendUnit{{
			Key: "telemetry", Title: "私有遥测", UnitCode: 128, Fields: []ext.FieldSpec{
				{Key: "soc2", Label: "SOC2", Type: "u8"},
			},
		}}},
	}
}

func TestRuntimePackResolution(t *testing.T) {
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{extTestPack("demo"), extTestPack("other")})

	cases := []struct {
		name string
		cfg  *ConnectionConfig
		want string // 期望激活包 id,"" 表示 nil
	}{
		{"按 id 激活", &ConnectionConfig{ExtensionPack: "demo"}, "demo"},
		{"未绑定", &ConnectionConfig{ExtensionPack: ""}, ""},
		{"id 不存在", &ConnectionConfig{ExtensionPack: "nope"}, ""},
		{"配置为 nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt.SetConnCfg(tc.cfg)
			got := rt.Pack()
			if tc.want == "" {
				if got != nil {
					t.Fatalf("期望 nil,实际 %s", got.Meta.ID)
				}
			} else if got == nil || got.Meta.ID != tc.want {
				t.Fatalf("期望 %s,实际 %v", tc.want, got)
			}
		})
	}

	t.Run("后设包集合同样按当前配置解析", func(t *testing.T) {
		rt.SetConnCfg(&ConnectionConfig{ExtensionPack: "other"})
		rt.SetPacks([]*ext.Pack{extTestPack("demo"), extTestPack("other")})
		if p := rt.Pack(); p == nil || p.Meta.ID != "other" {
			t.Fatalf("期望 other,实际 %v", p)
		}
	})
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run TestRuntimePackResolution -v`
Expected: FAIL,编译错误 `cfg.ExtensionPack undefined`

- [ ] **Step 3: 实现**

`bridge/connection_service.go` 的 ConnectionConfig 结构体,在 `ReissueOffsetSec` 之后、`TLS` 之前追加:

```go
	ExtensionPack   string        `json:"extensionPack,omitempty"` // 绑定的扩展包 id(空=不使用)
```

`bridge/runtime.go` 全量改为:

```go
// Package bridge 是 wails 胶水层:持有引擎客户端、暴露给前端的服务与事件转发。
// 本包是唯一允许 import wails 的地方。
package bridge

import (
	"sync"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
)

// Runtime 持有共享事件总线、当前引擎客户端与最新配置快照。
type Runtime struct {
	bus     *engine.Bus
	mu      sync.Mutex
	client  *engine.Client
	connCfg *ConnectionConfig
	groups  map[string]schema.GroupConfig
	packs   []*ext.Pack
	pack    *ext.Pack
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
func (rt *Runtime) SetConnCfg(cfg *ConnectionConfig) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.connCfg = cfg
	rt.pack = resolvePack(rt.packs, cfg)
}

// ConnCfg 返回连接配置快照(可能为 nil)。
func (rt *Runtime) ConnCfg() *ConnectionConfig {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.connCfg
}

// resolvePack 按连接配置的扩展包 id 在已加载集合中查找;未绑定或找不到返回 nil。
func resolvePack(packs []*ext.Pack, cfg *ConnectionConfig) *ext.Pack {
	if cfg == nil || cfg.ExtensionPack == "" {
		return nil
	}
	for _, p := range packs {
		if p.Meta.ID == cfg.ExtensionPack {
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
	rt.pack = resolvePack(packs, rt.connCfg)
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
```

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -run TestRuntimePackResolution -v && go test ./... -count=1`
Expected: PASS;全仓 ok(现有 golden 不受影响——ExtensionPack 为 omitempty 新字段,旧 profiles.json 反序列化得零值 "")

- [ ] **Step 5: 提交**

```bash
git add bridge/connection_service.go bridge/runtime.go bridge/runtime_ext_test.go
git commit -m "feat(ext): ConnectionConfig 扩展包绑定字段与 Runtime 激活状态"
```

---

### Task 2: ExtService(包目录扫描/列表)与应用装配

**Level:** L3
**Level Rationale:** 新增前端绑定服务(跨 bridge/app/main 三处装配),是前端与包目录的唯一通道;含启动时预加载行为。
**Linked Acceptance Items:** PAC-1/PAC-3 的 M1 里程碑
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 1 的 `Runtime.SetPacks`;下游=Task 6(ConnectionCard 调 ListPacks)、P4(导入 UI 调 ReloadPacks)。`reloadDir(dir)` 带目录参数正是为了可测与 P4 复用。
**重要性:** **P0(入口)**——M1"包可发现"的全部后端;没有它,放进去的 JSON 永远不会被看见。ListPacks 每次重新扫描(支持热放置文件)是实施人员体验的关键决策。

**Files:**
- Create: `bridge/ext_service.go`
- Modify: `app.go`、`main.go`、`bridge/connection_service.go`(GetConfig 回填 Runtime)
- Test: `bridge/ext_service_test.go`、`bridge/testdata/extpack.json`(Create)

**Interfaces:**
- Consumes: `ext.LoadDir`(P1)、`store.Dir`、`Runtime.SetPacks/Packs`
- Produces: `type PackInfo{ID,Label,Vendor,BaseVersion string; UnitCount int}`(json 驼峰)、`NewExtService(rt)`(创建即预载包集合)、`ListPacks() ([]PackInfo, error)`(坏文件不阻塞列表)、`ReloadPacks() error`、内部 `reloadDir(dir) error` / `listPacksDir(dir) []PackInfo`(测试与 P4 复用);**`ConnectionService.GetConfig` 行为升级:命中即回填 Runtime(启动激活链闭合)**

- [ ] **Step 1: 建夹具**

`bridge/testdata/extpack.json`(本阶段黄金包,unitCode 0x80、两字段,enable=true):

```json
{
  "meta": { "id": "p2golden", "label": "P2 黄金包", "vendor": "Test", "baseVersion": "2016" },
  "realtime": { "appendUnits": [{
    "key": "telemetry", "title": "私有遥测", "unitCode": 128, "enabled": true,
    "fields": [
      { "key": "soc2", "label": "SOC2", "type": "u8", "unit": "%" },
      { "key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V" }
    ]
  }] }
}
```

- [ ] **Step 2: 写失败测试**

`bridge/ext_service_test.go`:

```go
package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyExtFixture(t *testing.T, dir, name string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "extpack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtServiceReloadDir(t *testing.T) {
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)

	dir := t.TempDir()
	copyExtFixture(t, dir, "good.json")
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("skip"), 0o644)

	err := s.reloadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("err = %v, want 提及 broken.json", err)
	}
	packs := rt.Packs()
	if len(packs) != 1 || packs[0].Meta.ID != "p2golden" {
		t.Fatalf("packs = %+v, want 1 个 p2golden", packs)
	}

	t.Run("绑定后可激活", func(t *testing.T) {
		rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "p2golden"})
		if p := rt.Pack(); p == nil || p.Meta.ID != "p2golden" {
			t.Fatalf("激活失败: %v", p)
		}
	})
}

func TestExtServiceListPacksBadFileIgnored(t *testing.T) {
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)
	dir := t.TempDir()
	copyExtFixture(t, dir, "good.json")
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	infos := s.listPacksDir(dir)
	if len(infos) != 1 || infos[0].ID != "p2golden" {
		t.Fatalf("坏文件不应阻塞列表: %+v", infos)
	}
}

func TestExtServiceListPacksShape(t *testing.T) {
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)
	dir := t.TempDir()
	copyExtFixture(t, dir, "a.json")
	if err := s.reloadDir(dir); err != nil {
		t.Fatal(err)
	}
	infos := packInfosOf(rt.Packs())
	if len(infos) != 1 {
		t.Fatalf("infos = %+v", infos)
	}
	i := infos[0]
	if i.ID != "p2golden" || i.BaseVersion != "2016" || i.UnitCount != 1 || i.Label != "P2 黄金包" {
		t.Fatalf("info = %+v", i)
	}
}
```

注:`NewExtServiceForTest` 与 `packInfosOf` 见 Step 3 实现(测试助手与列表转换函数);`NewExtService` 因构造即扫真实用户目录,测试统一走 `reloadDir`。

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestExtService" -v`
Expected: FAIL,`undefined: NewExtServiceForTest`

- [ ] **Step 4: 实现**

`bridge/ext_service.go`:

```go
package bridge

import (
	"fmt"
	"path/filepath"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/store"
)

// PackInfo 扩展包摘要(前端下拉与包管理用)。
type PackInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Vendor      string `json:"vendor,omitempty"`
	BaseVersion string `json:"baseVersion"`
	UnitCount   int    `json:"unitCount"`
}

// ExtService 扩展包管理:扫描 packs 目录并提供列表/重载。
type ExtService struct {
	rt *Runtime
}

// NewExtService 创建服务并预加载一次包集合。
// 绑定激活发生在 GetConfig 回填 connCfg 时(见本任务 GetConfig 修改):
// 启动链 loadInitialData → GetConfig → SetConnCfg → resolvePack。
func NewExtService(rt *Runtime) *ExtService {
	s := &ExtService{rt: rt}
	_ = s.ReloadPacks()
	return s
}

// NewExtServiceForTest 测试用构造:不触碰用户目录。
func NewExtServiceForTest(rt *Runtime) *ExtService { return &ExtService{rt: rt} }

func packsDir() (string, error) {
	dir, err := store.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "packs"), nil
}

// reloadDir 扫描指定目录:合法包推入 Runtime,坏文件聚合上报且不阻塞好包。
func (s *ExtService) reloadDir(dir string) error {
	packs, err := ext.LoadDir(dir)
	s.rt.SetPacks(packs)
	return err
}

// ReloadPacks 扫描默认 packs 目录。
func (s *ExtService) ReloadPacks() error {
	dir, err := packsDir()
	if err != nil {
		return fmt.Errorf("定位扩展包目录失败: %w", err)
	}
	if err := s.reloadDir(dir); err != nil {
		return fmt.Errorf("扩展包目录存在非法文件: %w", err)
	}
	return nil
}

// listPacksDir 扫描目录并返回列表。坏文件不阻塞列表:reloadDir 已把好包推入
// Runtime,扫描错误忽略(P4 导入 UI 再做逐文件提示)。
func (s *ExtService) listPacksDir(dir string) []PackInfo {
	_ = s.reloadDir(dir)
	return packInfosOf(s.rt.Packs())
}

// ListPacks 返回全部可绑定扩展包摘要。每次调用重新扫描,支持热放置文件;
// 存在坏文件时列表照常返回(全局约束:坏包不阻塞好包)。
func (s *ExtService) ListPacks() ([]PackInfo, error) {
	dir, err := packsDir()
	if err != nil {
		return nil, fmt.Errorf("定位扩展包目录失败: %w", err)
	}
	return s.listPacksDir(dir), nil
}

func packInfosOf(packs []*ext.Pack) []PackInfo {
	out := make([]PackInfo, 0, len(packs))
	for _, p := range packs {
		out = append(out, PackInfo{
			ID: p.Meta.ID, Label: p.Meta.Label, Vendor: p.Meta.Vendor,
			BaseVersion: p.Meta.BaseVersion, UnitCount: len(p.Realtime.AppendUnits),
		})
	}
	return out
}
```

`app.go`:App 结构体增加 `extsvc *bridge.ExtService` 字段(排序放在 parser 之后);`NewApp` 返回值增加 `extsvc: bridge.NewExtService(rt),`。

`main.go` 的 Bind 列表追加一项:

```go
		Bind: []interface{}{
			app.conn,
			app.msg,
			app.console,
			app.parser,
			app.extsvc,
		},
```

`bridge/connection_service.go` 的 `GetConfig` 改为(命中与默认分支都回填 Runtime——启动链由此完成扩展包激活,并顺带修复"启动初期 Preview 实际使用默认配置而非激活档案"的既有缺陷):

```go
// GetConfig 读取当前激活档案;无档案时返回默认值。命中即回填 Runtime
// (启动链 loadInitialData → GetConfig 由此完成连接配置与扩展包激活)。
func (s *ConnectionService) GetConfig() (*ConnectionConfig, error) {
	pd, err := s.getProfiles()
	if err != nil {
		return nil, err
	}
	for i := range pd.Items {
		if pd.Items[i].Name == pd.Active {
			s.rt.SetConnCfg(&pd.Items[i])
			return &pd.Items[i], nil
		}
	}
	def := DefaultConnectionConfig()
	s.rt.SetConnCfg(def)
	return def, nil
}
```

- [ ] **Step 5: 跑测试与构建**

Run: `go test ./bridge/ -run "TestExtService|TestRuntimePackResolution" -v && go build ./...`
Expected: PASS + 构建通过

- [ ] **Step 6: 提交**

```bash
git add bridge/ext_service.go bridge/ext_service_test.go bridge/testdata/extpack.json bridge/connection_service.go app.go main.go
git commit -m "feat(ext): ExtService 包扫描与列表服务;GetConfig 回填 Runtime 完成启动激活"
```

---

### Task 3: GetSchema 合并与 DefaultGroups/GetGroups 扩展感知(含版本硬编码修复)

**Level:** L3
**Level Rationale:** 跨层行为变化(schema 输出直接影响前端表单),同时修复 GetGroups 硬编码 "2016" 的既有缺陷;AC-5 的过滤语义在此落地。
**Linked Acceptance Items:** PAC-1(表单)、PAC-3(过滤)
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 1 的 `rt.Pack()`;下游=Task 4(DefaultGroups 被 assembleBody 兜底复用)、Task 6(前端 store.schema 消费合并结果)。`unitDefaults` 复用 P1 的 `ext.DefaultsFor`(bytes/offset 默认值的唯一来源)。
**重要性:** **P0(M2 里程碑核心)**——schema 合并是"表单可见"的全部后端;版本门禁(baseVersion 匹配才合并)在这里定死,Task 4 的追加管线必须用同一门禁,防止两处判断漂移。

**Files:**
- Modify: `bridge/message_service.go`(GetSchema/DefaultGroups/GetGroups)
- Test: `bridge/message_service_ext_test.go`(Create)

**Interfaces:**
- Consumes: `ext.CompileUnit/DefaultsFor`(P1)、`rt.Pack()`
- Produces: `GetSchema(version)` 语义升级(标准 + 激活包同版本单元)、`unitDefaults(version) map[string]schema.RowValue`、`versionText() string`、`standardGroups(version)`(内部助手)

- [ ] **Step 1: 写失败测试**

`bridge/message_service_ext_test.go`:

```go
package bridge

import (
	"path/filepath"
	"testing"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
)

// newExtRT 构造绑定了 p2golden 包与 2016 配置的 Runtime(测试统一入口)。
func newExtRT(t *testing.T, packBound bool) *Runtime {
	t.Helper()
	p, err := ext.LoadFile(filepath.Join("testdata", "extpack.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{p})
	cfg := &ConnectionConfig{Version: "2016"}
	if packBound {
		cfg.ExtensionPack = "p2golden"
	}
	rt.SetConnCfg(cfg)
	return rt
}

func TestGetSchemaMerged(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)

	merged := ms.GetSchema("2016")
	if want := len(schema.V2016Groups()) + 1; len(merged) != want {
		t.Fatalf("合并后组数 = %d, want %d", len(merged), want)
	}
	last := merged[len(merged)-1]
	if last.Key != "telemetry" || last.Title != "私有遥测" || !last.Enabled {
		t.Fatalf("扩展组 = %+v", last)
	}
	if len(last.Fields) != 2 || last.Fields[0].Kind != "int" || last.Fields[1].Kind != "float" {
		t.Fatalf("字段 kind = %+v", last.Fields)
	}
}

func TestGetSchemaVersionGate(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	// 包 baseVersion=2016,请求 2025 → 不合并
	if got := len(ms.GetSchema("2025")); got != len(schema.V2025Groups()) {
		t.Fatalf("2025 组数 = %d, want %d", got, len(schema.V2025Groups()))
	}
}

func TestGetSchemaNoPack(t *testing.T) {
	rt := newExtRT(t, false)
	ms := NewMessageService(rt)
	if got := len(ms.GetSchema("2016")); got != len(schema.V2016Groups()) {
		t.Fatalf("未绑包组数 = %d, want %d", got, len(schema.V2016Groups()))
	}
}

func TestDefaultGroupsExtDefaults(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	g, ok := m["telemetry"]
	if !ok || !g.Enabled || len(g.Rows) != 1 {
		t.Fatalf("telemetry 默认 = %+v", g)
	}
	if g.Rows[0]["soc2"] != 0.0 || g.Rows[0]["packVolt"] != 0.0 {
		t.Fatalf("默认行值 = %+v", g.Rows[0])
	}
}

func TestGetGroupsFiltersStaleKeys(t *testing.T) {
	rt := newExtRT(t, false) // 已解绑
	// 注意顺序:NewMessageService 构造会读用户目录 message.json 并 SetGroups,
	// 测试注入必须在其后覆盖(全计划测试统一遵守"先构造、后注入")。
	ms := NewMessageService(rt)
	rows := []map[string]any{{"soc2": 80}}
	rt.SetGroups(map[string]schema.GroupConfig{
		"vehicle":   {Enabled: true, Rows: []map[string]any{{}}},
		"telemetry": {Enabled: true, Rows: rows}, // 旧包残留键
	})
	payload, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	m := payload.ToMap()
	if _, exists := m["telemetry"]; exists {
		t.Fatal("残留扩展键应被过滤")
	}
	if _, exists := m["vehicle"]; !exists {
		t.Fatal("标准组不应丢失")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestGetSchema|TestDefaultGroupsExt|TestGetGroupsFilters" -v`
Expected: FAIL(合并/过滤用例失败,未绑包用例可能通过)

- [ ] **Step 3: 实现**

`bridge/message_service.go` 中,替换 `GetSchema`、`DefaultGroups`、`GetGroups` 三个方法,并新增助手(import 区增加 `"gbt32960-simulator/internal/ext"`):

```go
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
```

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -v && go test ./... -count=1`
Expected: PASS(含既有 console_service 测试)

- [ ] **Step 5: 提交**

```bash
git add bridge/message_service.go bridge/message_service_ext_test.go
git commit -m "feat(ext): GetSchema 合并扩展组;默认值/版本感知与残留键过滤"
```

---

### Task 4: 组装管线扩展拼接(assembleBody)与 Preview/SendRealtime/SetAutoReport 三路接入

**Level:** L3
**Level Rationale:** 核心业务流(发送路径)跨 schema/ext/engine 三层;AC-1 与 AC-3 的字节级证明都在本任务。
**Linked Acceptance Items:** PAC-1、PAC-2
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 1(rt.Pack)、Task 3(DefaultGroups 兜底、版本门禁同款);下游=Task 5(SendReissue 复用 assembleBody)、前端预览(消费 Preview 输出)。**版本门禁与 Task 3 必须同一表达式** `p.Meta.BaseVersion == version`,两处漂移即幽灵 bug。
**重要性:** **P0(M3 里程碑核心,整个扩展机制的价值兑现点)**——四路统一从这里开始;`未启用单元→原样返回标准体` 的分支是 AC-3 逐字节不变的实现保障。

**Files:**
- Modify: `bridge/message_service.go`(新增 assembleBody;改 Preview/SendRealtime/SetAutoReport)
- Test: `bridge/message_service_ext_test.go`(追加)

**Interfaces:**
- Consumes: `schema.Assemble`(标准体)、`ext.EncodeUnit`(TLV)、`engine.NewRawBody`(downlink.go 既有)
- Produces: `func (s *MessageService) assembleBody(at time.Time) (model.MessageBody, error)`——Task 5 与周期上报闭包的统一组装入口;行为契约:无包/版本不符/无启用行 → 返回标准体(逐字节同现状);否则返回 rawBody(标准体字节 + 追加 TLV)

- [ ] **Step 1: 追加失败测试**

`bridge/message_service_ext_test.go` 追加:

```go
func TestAssembleBodyAppendsTLV(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	b, err := body.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// 标准体(空组=仅 6B 十进制时间 1a0102030405)+ TLV(80 0003 50 0021)
	const want = "1a0102030405800003500021"
	if got := hex.EncodeToString(b); got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestAssembleBodyNoPackIdentical(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	groups := map[string]schema.GroupConfig{
		"telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	}
	rt.SetConnCfg(&ConnectionConfig{Version: "2016"}) // 解绑
	rt.SetGroups(groups)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	got, err := body.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	wantBody, err := schema.Assemble(api.V2016, groups, at)
	if err != nil {
		t.Fatal(err)
	}
	want, err := wantBody.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("未绑包须逐字节一致: got %x want %x", got, want)
	}
}

func TestAssembleBodyDisabledUnitSkipped(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"telemetry": {Enabled: false, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := body.Bytes()
	if got := hex.EncodeToString(b); got != "1a0102030405" {
		t.Fatalf("禁用单元不应追加: %s", got)
	}
}

func TestPreviewTailGolden(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", VIN: "LSV00000000000001", ExtensionPack: "p2golden"})
	rt.SetGroups(map[string]schema.GroupConfig{
		"telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	r, err := ms.Preview()
	if err != nil {
		t.Fatal(err)
	}
	payload := r.Hex[:len(r.Hex)-2] // 完整帧末字节是 BCC,断言须剥离
	if !strings.HasPrefix(payload, "2323") {
		t.Fatalf("帧头 = %s", payload[:8])
	}
	if !strings.HasSuffix(payload, "800003500021") {
		t.Fatalf("载荷尾 = ...%s", payload[len(payload)-24:])
	}
}
```

测试文件 import 区改为:

```go
import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
)
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestAssembleBody|TestPreviewTail" -v`
Expected: FAIL,`ms.assembleBody undefined`

- [ ] **Step 3: 实现**

`bridge/message_service.go` 新增(import 区:`"time"`、`"gbt32960-simulator/internal/engine"` 既有;**新增 `"github.com/sunsky74/gb32960/model"`**——assembleBody 返回 model.MessageBody):

```go
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
```

`Preview()` 中,替换:

```go
	body, err := schema.Assemble(parseVersion(cfg.Version), groups, time.Now())
	if err != nil {
		return nil, err
	}
```

为:

```go
	body, err := s.assembleBody(time.Now())
	if err != nil {
		return nil, err
	}
```

(其上的 `groups := s.rt.Groups()` 与默认回填两行随之删除——回填逻辑已并入 assembleBody;`raw, name, err := engine.BuildFrame(parseVersion(cfg.Version), cfg.VIN, 0x02, body)` 保持不变。)

`SendRealtime()` 中,替换:

```go
	groups := s.rt.Groups()
	if groups == nil {
		return fmt.Errorf("报文配置为空,请先保存报文配置")
	}
	body, err := schema.Assemble(s.version(), groups, time.Now())
	if err != nil {
		return err
	}
```

为:

```go
	if s.rt.Groups() == nil {
		return fmt.Errorf("报文配置为空,请先保存报文配置")
	}
	body, err := s.assembleBody(time.Now())
	if err != nil {
		return err
	}
```

`SetAutoReport` 的闭包内,替换:

```go
		groups := s.rt.Groups()
		if groups == nil {
			return fmt.Errorf("报文配置为空")
		}
		body, err := schema.Assemble(s.version(), groups, time.Now())
		if err != nil {
			return err
		}
```

为:

```go
		if s.rt.Groups() == nil {
			return fmt.Errorf("报文配置为空")
		}
		body, err := s.assembleBody(time.Now())
		if err != nil {
			return err
		}
```

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -v && go test ./... -count=1`
Expected: PASS(golden 全绿——标准路径在无包时逐字节不变已由 TestAssembleBodyNoPackIdentical 与既有 golden 共同锁定)

- [ ] **Step 5: 提交**

```bash
git add bridge/message_service.go bridge/message_service_ext_test.go
git commit -m "feat(ext): assembleBody 组装管线,Preview/Send/AutoReport 三路接入扩展 TLV"
```

---

### Task 5: SendReissue 统一到扩展管线并修复 2025 既有 bug

**Level:** L3
**Level Rationale:** 用户可见业务流(补发)行为变化:统一管线 + 修复 2025 连接发 2016 形体的既有缺陷(设计 §4.5#2 与 Oracle 终审 P1-3 记录的 backlog)。
**Linked Acceptance Items:** PAC-4
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 4 的 assembleBody(复用,不新增逻辑);本任务是四路管线的最后一路,完成后 M3 闭环。**若不做本任务,补发报文与实时报文体不一致,平台侧解析口径分裂。**
**重要性:** **P1(一致性 + 顺手修复既有 bug)**——扩展部分是增量,2025 修复是存量缺陷清偿,两者在同一行代码上完成。

**Files:**
- Modify: `bridge/message_service.go`(SendReissue)
- Test: `bridge/message_service_ext_test.go`(追加)

**Interfaces:**
- Consumes: `assembleBody`(Task 4)
- Produces: SendReissue 的 0x03 体 = assembleBody(at)(版本正确 + 扩展追加);`schema.AssembleRealtime` 在 bridge 层不再有调用方

- [ ] **Step 1: 追加失败测试**

`bridge/message_service_ext_test.go` 追加:

```go
func TestAssembleBodyV2025VersionDispatch(t *testing.T) {
	// 修复既有 bug:V2025 连接的补发/实时体必须走 2025 组装(此前硬编码 2016 形体)
	rt := newExtRT(t, false)
	rt.SetConnCfg(&ConnectionConfig{Version: "2025"})
	ms := NewMessageService(rt)
	groups := ms.DefaultGroups("2025").ToMap()
	rt.SetGroups(groups)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	got, err := body.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	wantBody, err := schema.Assemble(api.V2025, groups, at)
	if err != nil {
		t.Fatal(err)
	}
	want, err := wantBody.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("V2025 分发失败: got %x want %x", got, want)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run TestAssembleBodyV2025 -v`
Expected: PASS——该测试锁定的是 assembleBody 的版本分发(Task 4 已实现),此处不会出现 RED;SendReissue 本身需 online client 无法离线测试,其统一的完成判据在 Step 4 的 grep(bridge 层不再出现 `AssembleRealtime` 直调)。本测试在此的价值:防止 Task 5 改动破坏分发。

- [ ] **Step 3: 实现**

`SendReissue` 循环体内,替换:

```go
		body, err := schema.AssembleRealtime(groups, at)
		if err != nil {
			return err
		}
```

为:

```go
		body, err := s.assembleBody(at)
		if err != nil {
			return err
		}
```

(方法开头 `groups := s.rt.Groups()` 与空值报错保留不动——`assembleBody` 的 nil 回填不会绕过该用户提示。)

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -v && go test ./... -count=1`
Run: `grep -c "AssembleRealtime" bridge/message_service.go`
Expected: 测试全绿;grep 输出 0(bridge 层无直调;两条命令独立执行——grep 无匹配时 exit 1 属预期,以输出数字为准)

- [ ] **Step 5: 提交**

```bash
git add bridge/message_service.go bridge/message_service_ext_test.go
git commit -m "feat(ext): SendReissue 统一扩展管线;修复 2025 连接补发 2016 形体既有 bug"
```

---

### Task 6: 前端交付面——bytes 渲染 / 扩展包选择 / schema 联动刷新 / 绑定再生成

**Level:** L3
**Level Rationale:** 用户可见 UI 变化(表单新 kind、连接卡新控件)+ 前后端契约(再生成绑定)跨层。
**Linked Acceptance Items:** PAC-1(bytes 表单)、PAC-3(联动刷新)
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 2(ListPacks 绑定)、Task 3(合并 schema 与 FieldSchema.Length 已在 P1/Task3 生效);下游=用户与 P4(设置页包管理复用 ListPacks/ReloadPacks)。`reloadSchemaPreservingGroups` 与既有 `reloadSchemaForVersion`(重置语义)并存:**版本变化→重置,包变化→保留标准组配置**,两者不可混用。
**重要性:** **P1(M2 的前端半场)**——后端一切就绪后,没有本任务实施人员看不到任何变化;hex 输入校验(非法字符不写入、长度上限)是 bytes kind 的唯一防错层。

**Files:**
- Regenerate: `frontend/wailsjs/**`(`wails generate module`)
- Modify: `frontend/src/components/RealTimePanel.vue`
- Modify: `frontend/src/components/cards/ConnectionCard.vue`
- Modify: `frontend/src/state.ts`
- Modify: `frontend/src/composables/useConnActions.ts`
- Modify: `frontend/src/composables/useConnConfig.ts`(resetToNewProfile 清绑定)
- Modify: `frontend/src/pages/ClientSimulatorPage.vue`

**Interfaces:**
- Consumes: `ExtService.ListPacks`(Task 2 绑定)、`FieldSchema.length`、`ConnectionConfig.extensionPack`(再生成后类型自动可用)
- Produces: bytes kind 的渲染与输入约定(hex 小写字符串,长度 = length×2)、`reloadSchemaPreservingGroups(version)`(state.ts 导出)

- [ ] **Step 1: 再生成前端绑定**

Run: `wails generate module`
Expected: 生成 `frontend/wailsjs/go/bridge/ExtService.js/.d.ts`;`models.ts` 的 `ConnectionConfig` 增加 `extensionPack`、`FieldSchema` 增加 `length`

- [ ] **Step 2: RealTimePanel.vue 增加 bytes kind**

`defaultFor` 函数的 switch 中、`case 'int':` 之前插入:

```ts
    case 'bytes':
      return f.length ? '00'.repeat(f.length) : ''
```

同时把 `case 'int':` 分支与 `default:` 分支的 `return 0` 均改为 `return f.min ?? 0`——数值字段默认值=物理下限(CompileField 的 min = lo×scale+offset,u 型 lo=0 时恰为 offset,即线值 0),与后端 `ext.DefaultsFor` 语义对齐;f32 未设 min 时回落 0。

脚本区(numOf 等助手旁)新增:

```ts
function hexOf(row: Record<string, unknown>, key: string): string {
  const v = row[key]
  return typeof v === 'string' ? v : ''
}

function setHex(row: Record<string, unknown>, key: string, f: FieldSchema, v: string) {
  const s = v.trim().toLowerCase().replace(/\s+/g, '')
  if (s === '') {
    row[key] = ''
    return
  }
  if (!/^[0-9a-f]*$/.test(s)) return // 非法字符不写入
  if (f.length && s.length > f.length * 2) return // 超长不写入
  row[key] = s
}
```

模板中 `v-else-if="f.kind === 'int' || f.kind === 'float'"` 分支之后插入:

```html
                      <div v-else-if="f.kind === 'bytes'" class="field">
                        <span class="field-label">{{ f.label }}<em v-if="f.length"> ({{ f.length }}B hex)</em></span>
                        <a-input
                          :value="hexOf(row, f.key)"
                          class="hex-input"
                          size="small"
                          :placeholder="f.length ? `${f.length * 2} 个 hex 字符` : 'hex'"
                          @update:value="(v: string) => setHex(row, f.key, f, v)"
                        />
                      </div>
```

RealTimePanel.vue 现无 style 块——在文件末尾(`</template>` 之后)**新增**:

```vue
<style scoped>
.hex-input :deep(input) {
  font-family: var(--font-mono);
}
</style>
```

- [ ] **Step 3: ConnectionCard.vue 增加扩展包选择**

script 区 import 改为并新增:

```ts
import { onMounted, ref } from 'vue'
import * as ExtService from '../../wailsjs/go/bridge/ExtService'
```

```ts
const packOptions = ref<{ value: string; label: string }[]>([])
onMounted(async () => {
  try {
    const packs = await ExtService.ListPacks()
    packOptions.value = packs.map((p) => ({ value: p.id, label: `${p.label} (${p.baseVersion})` }))
  } catch {
    packOptions.value = []
  }
})
```

模板"报文版本"表单项之后插入:

```html
      <a-form-item label="扩展包" name="extensionPack">
        <a-select v-model:value="cfg.extensionPack" :options="packOptions" allow-clear placeholder="不使用扩展包" />
      </a-form-item>
```

- [ ] **Step 4: state.ts 增加 reloadSchemaPreservingGroups**

```ts
export async function reloadSchemaPreservingGroups(version: string) {
  store.schema = await MessageService.GetSchema(version)
  const payload = await MessageService.GetGroups()
  store.groups = payloadToState(payload)
}
```

(与 `reloadSchemaForVersion` 的区别:后者重置为默认值,用于版本切换;前者保留已持久化/已填的标准组配置,用于包切换时只增删扩展组。)

- [ ] **Step 5: 包变化联动刷新**

`useConnActions.ts` 新增共用刷新助手,`saveOnly` 与 `connect()` 都接入(connect 直接走 `Connect→SaveConfig`,若不刷新会出现"帧带 TLV 而面板无私有组"的所见非所得):

```ts
async function reloadSchemaOnBindingChange(prevVersion: string | undefined, prevPack: string | undefined) {
  if (cfg.version !== prevVersion) {
    await reloadSchemaForVersion(cfg.version)
  } else if (cfg.extensionPack !== prevPack) {
    await reloadSchemaPreservingGroups(cfg.version)
  }
}

export async function saveOnly() {
  if (!(await validateForm())) return
  try {
    const prevVersion = store.config?.version
    const prevPack = store.config?.extensionPack
    await ConnectionService.SaveConfig(cfg)
    store.config = cfg
    await reloadSchemaOnBindingChange(prevVersion, prevPack)
    message.success(`配置「${cfg.name}」已保存`)
    await loadProfiles()
  } catch (e) {
    message.error(`保存失败: ${String(e)}`)
  }
}
```

`connect()` 的 try 块改为(在 Connect 与 `store.config = cfg` 之后插入刷新):

```ts
    const prevVersion = store.config?.version
    const prevPack = store.config?.extensionPack
    await ConnectionService.Connect(cfg)
    store.config = cfg
    await reloadSchemaOnBindingChange(prevVersion, prevPack)
    await refreshState()
    await loadProfiles()
```

`useConnConfig.ts` 的 `resetToNewProfile` 重置对象中、`reissueOffsetSec: 60,` 之后补一行(新档案不继承旧绑定):

```ts
    extensionPack: '',
```

(import 区补 `reloadSchemaPreservingGroups`。)

`ClientSimulatorPage.vue` 的 `onSwitchProfile` 改为:

```ts
async function onSwitchProfile(name: string) {
  try {
    const prevVersion = store.config?.version
    const prevPack = store.config?.extensionPack
    store.config = await ConnectionService.SwitchProfile(name)
    await loadProfiles()
    if (store.config.version !== prevVersion) {
      await reloadSchemaForVersion(store.config.version)
    } else if (store.config.extensionPack !== prevPack) {
      await reloadSchemaPreservingGroups(store.config.version)
    }
  } catch (e) {
    console.error('切换档案失败', e)
  }
}
```

(顶部 import 的 `reloadSchemaForVersion` 所在行补上 `reloadSchemaPreservingGroups`。)

- [ ] **Step 6: 构建验证**

Run: `cd frontend && npm run build`
Expected: vue-tsc 零错误 + vite 构建成功

- [ ] **Step 7: 手动验收(wails dev,PAC-1 前端面)**

1. 把 `bridge/testdata/extpack.json` 复制到 packs 目录(见下方命令),启动 `wails dev`
2. 连接配置卡出现"扩展包"下拉(含"P2 黄金包 (2016)"),选中并保存 → 实时面板出现"私有遥测"组(SOC2 整数、包电压 float 显示 ×0.1)
3. 填 soc2=80、packVolt=3.3,点"预览 HEX" → 弹窗 hex 载荷以 `800003500021` 结尾(其后仅剩 1 字节 BCC)
4. 清空扩展包保存 → 私有组消失,标准组已填值保留
5. 恢复绑定,连接平台发送 0x02 → 控制台 TX hex 载荷尾同为 `800003500021`(BCC 之前)
6. 重启 `wails dev` 不做任何操作 → 面板直接出现"私有遥测"组(验证 GetConfig 启动激活链)

```bash
mkdir -p ~/Library/Application\ Support/gbt32960-simulator/packs
cp bridge/testdata/extpack.json ~/Library/Application\ Support/gbt32960-simulator/packs/
```

- [ ] **Step 8: 提交**

```bash
git add frontend/wailsjs frontend/src/components/RealTimePanel.vue frontend/src/components/cards/ConnectionCard.vue frontend/src/state.ts frontend/src/composables/useConnActions.ts frontend/src/composables/useConnConfig.ts frontend/src/pages/ClientSimulatorPage.vue
git commit -m "feat(ext): 前端 bytes 渲染与扩展包选择,包切换/保存/连接三路联动刷新"
```

---

## 任务依赖图

```
Task 1(状态层)──▶ Task 2(ExtService)──▶ Task 6(前端)
   │                                        ▲
   └────────▶ Task 3(schema 合并)──▶ Task 4(组装管线)──▶ Task 5(补发统一)
```

关键耦合点(评审重点):Task 3 与 Task 4 的版本门禁表达式必须一致(`p.Meta.BaseVersion == version`);Task 4 与 Task 5 共用 assembleBody;Task 6 的 reload 语义区分(版本=重置 / 包=保留)。

## Self-Review 记录

1. **Spec 覆盖**:设计 §4.5 接入点 2(四路管线)= Task 4+5;接入点 3(GetSchema 合并 + Length + DefaultsFor + GetGroups 硬编码/残留键)= Task 3;接入点 4(Profile 绑定)= Task 1+2;前端 bytes/选择/联动 = Task 6。AC-1/3/5 → PAC-1~4 全覆盖。AC-2/AC-6 属 P3/P4,不在本计划。
2. **占位符扫描**:无 TBD/TODO;所有代码步骤含完整代码;前端 Step 7 的 shell 为直接可执行命令。
3. **类型一致性**:`assembleBody(at time.Time) (model.MessageBody, error)` 在 Task 4 定义、Task 5 消费一致;`PackInfo` Task 2 定义、Task 6 消费(经再生成绑定);`reloadSchemaPreservingGroups` Task 6 Step 4 定义、Step 5 两处消费。
4. **拆分决策**:Phase 2 独立可验收(M1~M4 里程碑),符合 Master 分解。
5. **Level/Gate 完整**:6 任务全部 L3(跨模块/业务流)+ rationale + gate。
6. **L3 AC 绑定**:每任务 Linked AC 非空且存在于 PAC 清单。
7. **AC 覆盖**:PAC-1←Task 3/4/6;PAC-2←Task 4;PAC-3←Task 1/3/6;PAC-4←Task 5。无 L1 任务。
8. **AC 可执行**:每条 PAC 带测试名/黄金 hex/手动步骤。
9. **风险预案**:Task 5 Step 2 对"测试可能意外通过"给出 grep 判定标准,消除歧义。
10. **Oracle 评审修订(2026-09-01,执行前评审)**:已落实 P0-1(Preview 断言剥离末位 BCC + PAC-1/手动步骤文案)、P0-2(GetGroups 先按 order 过滤再 FromMap——FromMap 会追加未知键,过滤必须在调用前)、P0-3(测试注入统一"先构造后注入",规避 NewMessageService 读用户目录)、P1-1(补 model import)、P1-2(GetConfig 命中即回填 Runtime,闭合启动激活链 + 修复启动 Preview 用默认配置的既有缺陷)、P1-3(listPacksDir:坏文件不阻塞列表 + 新测试)、P1-4(style 块为"新增"而非"追加")、P1-5(mkdir/cp 命令修正)、P1-6(connect() 接入刷新助手)、P2-1(Task 5 判据归因修正 + 命令拆分)、P2-2(defaultFor 数值默认 = f.min ?? 0,对齐 DefaultsFor)、P2-3(resetToNewProfile 清 extensionPack)、P2-4(PAC-2 主证据表述为既有 golden)。
