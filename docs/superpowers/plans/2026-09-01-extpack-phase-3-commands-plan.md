# Phase 3 · 扩展命令字 Implementation Plan

> **For agentic workers:** Recommended execution: superpowers:ltdd(Quality-LTDD)或 superpowers:subagent-driven-development(P1/P2 已验证路径)。Steps 用 checkbox。

**Goal:** 实施人员导入含 `commands` 段的扩展包并绑定档案后,"自定义数据"tab 激活并出现每个私有命令的表单;手动发送产生**精确命令字节**的帧(上行预留区 0x09~0x7F 不被库折叠改写);周期开关按间隔发送。未绑包时现有行为逐字节不变(私有远控 0x8A 应答保留)。

**Architecture:** 三层增量——engine 层把 私有远控 硬编码命令名泛化为**精确码注册表**(规避库区间折叠坑,Oracle P1-2 契约);bridge 层复用 P1 的 DSL 编码器组装两种命令体(fields 平铺 / realtimeLike=6B 十进制时间+TLV);前端激活现成的"自定义数据"tab 占位。组配置继续走同一 GroupsConfig(命令组键命名空间化,持久化零迁移)。

**Tech Stack:** Go 1.25 / Wails v2 / Vue 3 + Ant Design Vue;`wails generate module` 再生成绑定。

**Master 索引:** `docs/superpowers/plans/2026-08-31-extpack-master-plan.md`
**设计契约:** `docs/superpowers/specs/2026-08-31-extpack-design.md` §4(§9 G-1/G-2 均已关闭)

## 阶段目标与业务里程碑

**阶段目标(一句话):** 让"发私有命令帧"成为零代码能力——放文件、绑档案、自定义数据 tab 填表、点发送,线上命令字节与协议文档一字不差。

**业务里程碑(验收顺序即达成顺序):**

| 里程碑 | 内容 | 覆盖任务 | 对应 Global AC |
|---|---|---|---|
| **M1 命令可注册** | 私有远控 硬编码迁移为精确码注册表;标准命令回退库枚举;0x8A 应答行为逐字节不变 | Task 1 | AC-3 |
| **M2 双体可组装** | fields 平铺体 + realtimeLike 体(6B 十进制时间 + TLV×N)黄金锁定 | Task 2-3 | AC-2 字节级 |
| **M3 表单可见** | "自定义数据"tab 激活,每个命令一组表单(与 0x02 同款交互) | Task 3-5 | AC-2 前半 |
| **M4 发送闭环** | 手动发送 wire command byte 锁定;周期开关按间隔发送 | Task 3-5 | AC-2 后半 |

## Global Constraints(继承 Master + 本阶段特有)

- 协议库 gb32960-go 零改动;P1/P2 交付 API 不改签名只消费(`ext.CompileUnit/DefaultsFor/EncodeUnit/EncodeFields`)
- **精确码副本契约(Oracle P1-2,硬)**:扩展命令必须经注册表构造 `Min=Max=code` 副本,禁止直调 `CommandVXXByCode`;P3 golden 断言"帧 hex 第 5-6 字符 == 命令码"
- **AC-3 硬约束**:未绑包时现有行为逐字节不变——私有远控 0x8A 应答路径保留(注册表 init 内置该条目),既有 downlink/私有远控 测试必须全绿
- 命令码合法性(P1 校验器已强制,不重复实现):标准占用拒绝;2016 上行预留区 0x09~0x7F / 2025 0x0C~0x7F;direction 仅 up;版本门禁 `p.Meta.BaseVersion == version`
- 命令组键命名空间:fields 体 → 组键=命令 key;realtimeLike 体 → 组键=`命令key:单元key`;均带 `GroupSchema.Source="command"`
- realtimeLike 时间 = **6B 十进制**(年-2000/月/日/时/分/秒,与库 BeanTime 一致,非 BCD——golden 已验证)
- 每命令独立周期 ticker(bridge 侧管理,不占引擎的单实例 SetAutoReport);ticker 每次触发检查包与命令仍存在,失效自停
- 前端零新增依赖;`wails generate module` 产物必须随代码提交;`go test ./...` + `npm run build` 全绿

## Final Acceptance Checklist (Refined from Spec) - MUST

- [PAC-1](源:AC-2) 绑定含 commands 的包后,自定义数据 tab 出现命令表单;手动发送帧的 wire command byte == 声明码
  Refinement: 单测 `TestSendFrameWireCommandByte`(fields 体 + code 0x0A → 全帧 hex `[4:6]=="0a"`,防折叠回归);手动 `wails dev`:tab 激活、填表、点发送、控制台 TX hex 第 5-6 字符==声明码
- [PAC-2](源:AC-2) 两种体形态组装黄金
  Refinement: `TestAssembleCommandBodyFieldsGolden`(seq u16=1 → `0001`)与 `TestAssembleCommandBodyRealtimeLikeGolden`(固定时刻 → `1a0102030405` + 单元 TLV)
- [PAC-3](源:AC-2) 周期开关按间隔发送
  Refinement: 单测 `TestExtReportTickerLifecycle`(ticker 注册/停止/解绑自停);手动验收观察连续 TX
- [PAC-4](源:AC-3) 未绑包行为不变 + 标准命令回退
  Refinement: `TestCommandForFallbackStandard`(0x02 走库枚举)、私有远控 既有测试全绿、GetSchema 无 command 组

---

## File Structure(本阶段触碰面)

| 文件 | 动作 | 职责 |
|---|---|---|
| `internal/engine/commands.go` | Create | 精确码注册表(含 私有远控 内置条目) |
| `internal/engine/downlink.go` | Modify | commandFor 优先查注册表;删除 remoteCommandNames |
| `internal/ext/time.go` | Create | EncodeBeanTime(6B 十进制) |
| `internal/schema/schema.go` | Modify(+1 字段) | GroupSchema.Source |
| `bridge/message_service.go` | Modify | GetSchema/unitDefaults 命令组合并、assembleCommandBody、SendExtension、SetExtAutoReport |
| `bridge/message_service_ext_test.go` | Modify(追加) | 命令组装/发送/wire byte/生命周期测试 |
| `bridge/testdata/extcmd.json` | Create | 含 commands 的夹具 |
| `frontend/src/composables/useFieldHelpers.ts` | Create | 字段取值/设值纯函数(从 RealTimePanel 提取) |
| `frontend/src/components/RealTimePanel.vue` | Modify | 引用 helpers;自定义数据 tab 接线 |
| `frontend/src/components/ExtensionCommands.vue` | Create | 扩展命令区(表单+发送+周期) |
| `frontend/src/state.ts` | Modify | store.extSchema + schema 分流 |
| `frontend/wailsjs/**` | Regenerate | SendExtension/SetExtAutoReport 绑定 + GroupSchema.source |

---

### Task 1: 引擎命令注册表(私有远控 迁移,AC-3 保行为)

**Level:** L3
**Level Rationale:** 协议层共享状态(并发 map)+ 既有发送路径行为改造,被 0x8A 应答与标准命令两条路径依赖。
**Linked Acceptance Items:** PAC-4
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=P1 私有远控 精确码 hack(本任务把写死变注册);下游=Task 3(bridge 激活包时注册命令)、所有发送路径。**私有远控 0x8A 的既有行为是 AC-3 的一部分,迁移必须逐字节保持。**
**重要性:** **P0(M1 核心)**——没有注册表,任何 0x0A~0x7F 私有命令都会在线上被折叠成 0x09(Oracle P1-2 实证),这是 P3 全部价值的前提。

**Files:**
- Create: `internal/engine/commands.go`
- Modify: `internal/engine/downlink.go`(commandFor 替换、删 remoteCommandNames)、`internal/engine/client.go`(handleFrame 的 私有远控 引用改注册表 + 两处 BCD 误导注释修正)
- Test: `internal/engine/commands_test.go`(Create)

**Interfaces:**
- Consumes: `types.CommandV2016/CommandV2025`(库)
- Produces: `RegisterCommand(v api.GBTVersion, code byte, name string)`、`ResetExtCommands()`(保留 私有远控 内置条目)、内部 `extCommandName(v, code) (string, bool)`;`commandFor` 语义升级(注册表优先、库枚举回退)

- [ ] **Step 1: 写失败测试**

`internal/engine/commands_test.go`:

```go
package engine

import (
	"testing"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

func TestCommandForRegisteredExactCode(t *testing.T) {
	RegisterCommand(api.V2016, 0x0A, "TEST_EXT")
	rt := commandFor(api.V2016, 0x0A)
	c, ok := rt.(*types.CommandV2016)
	if !ok || c.Code != 0x0A || c.Min != 0x0A || c.Max != 0x0A || c.Name != "TEST_EXT" {
		t.Fatalf("rt = %+v, want 精确副本(Code=Min=Max=0x0A)", rt)
	}
	ResetExtCommands()
	if _, ok := extCommandName(api.V2016, 0x0A); ok {
		t.Fatal("Reset 后自定义命令应清除")
	}
	if name, ok := extCommandName(api.V2016, 0x8A); !ok || name != "REMOTE_CONTROL" {
		t.Fatal("私有远控 内置条目应保留(AC-3)")
	}
}

func TestCommandForFallbackStandard(t *testing.T) {
	ResetExtCommands()
	rt := commandFor(api.V2016, 0x02)
	if _, ok := rt.(*types.CommandV2016); !ok {
		t.Fatalf("标准命令应回退库枚举,实际 %T", rt)
	}
}

func TestCommandForV2025Registered(t *testing.T) {
	RegisterCommand(api.V2025, 0x0C, "TEST_V2025")
	rt := commandFor(api.V2025, 0x0C)
	c, ok := rt.(*types.CommandV2025)
	if !ok || c.Code != 0x0C || c.Min != 0x0C || c.Max != 0x0C {
		t.Fatalf("rt = %+v", rt)
	}
	ResetExtCommands()
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/engine/ -run "TestCommandFor" -v`
Expected: FAIL,`undefined: RegisterCommand`

- [ ] **Step 3: 实现**

`internal/engine/commands.go`:

```go
package engine

import (
	"sync"

	"github.com/sunsky74/gb32960/api"
)

// extCommandNames 扩展命令注册表:code → 显示名。
// 注册的命令在 commandFor 中构造 Min=Max=code 的精确副本,
// 规避库枚举按区间折叠改写线上命令字节(如 0x0A 折叠为 0x09)。
var extCommands = struct {
	mu   sync.Mutex
	cmds map[api.GBTVersion]map[byte]string
}{cmds: map[api.GBTVersion]map[byte]string{}}

// remoteBuiltin 私有远控 0x8A 私有命令(迁移自原硬编码,AC-3:行为不变)。
const remoteCmdCode = 0x8A

const remoteCmdName = "REMOTE_CONTROL"

func init() {
	RegisterCommand(api.V2016, remoteCmdCode, remoteCmdName)
	RegisterCommand(api.V2025, remoteCmdCode, remoteCmdName)
}

// RegisterCommand 注册精确命令码(包激活时由 bridge 调用;幂等覆盖同名)。
func RegisterCommand(v api.GBTVersion, code byte, name string) {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	if extCommands.cmds[v] == nil {
		extCommands.cmds[v] = map[byte]string{}
	}
	extCommands.cmds[v][code] = name
}

// ResetExtCommands 清除全部扩展命令并恢复 私有远控 内置条目(换绑/解绑时调用)。
func ResetExtCommands() {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	extCommands.cmds = map[api.GBTVersion]map[byte]string{
		api.V2016: {remoteCmdCode: remoteCmdName},
		api.V2025: {remoteCmdCode: remoteCmdName},
	}
}

func extCommandName(v api.GBTVersion, code byte) (string, bool) {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	if m := extCommands.cmds[v]; m != nil {
		n, ok := m[code]
		return n, ok
	}
	return "", false
}
```

`internal/engine/downlink.go` 中:删除 `remoteCommandNames` 变量与 `commandFor` 原实现,替换为:

```go
// commandFor 取发送用命令对象。优先查扩展命令注册表(构造 Min=Max=code 精确副本),
// 未注册则回退库枚举区间查找(标准命令)。
func commandFor(v api.GBTVersion, cmd byte) any {
	if name, ok := extCommandName(v, cmd); ok {
		switch v {
		case api.V2025:
			return &types.CommandV2025{Code: cmd, Name: name, Min: cmd, Max: cmd}
		default:
			return &types.CommandV2016{Code: cmd, Name: name, Min: cmd, Max: cmd}
		}
	}
	switch v {
	case api.V2025:
		return types.CommandV2025ByCode(cmd)
	default:
		return types.CommandV2016ByCode(cmd)
	}
}
```

(import 区无需变化:`types`/`api` 既有。)

**同时(私有远控 引用迁移,评审 P1-1)**:`internal/engine/client.go` 的 `handleFrame` 中:

```go
		if alias, ok := remoteCommandNames[raw[2]]; ok {
```

改为:

```go
		if name, ok := extCommandName(pm.Version, raw[2]); ok {
```

(对应变量名按上下文同步;注册表 init 已内置 私有远控 双版本条目,行为不变。)

**同时(误导注释修正,评审 P2-7)**:`client.go` 第 541 行附近"解析 6 字节 BCD 时间"改为"解析 6 字节十进制时间";第 627 行附近"codec 负责 BCD"改为"codec 按十进制原字节编码(非 BCD)"。纯注释,零行为变化。

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./internal/engine/ -v && go test ./... -count=1`
Expected: PASS;既有 remote_test/downlink_test 全绿(0x8A 应答行为未变)

- [ ] **Step 5: 提交**

```bash
git add internal/engine/commands.go internal/engine/commands_test.go internal/engine/downlink.go internal/engine/client.go
git commit -m "feat(ext): 引擎精确码命令注册表,私有远控 硬编码迁移并保留行为"
```

---

### Task 2: ext.EncodeBeanTime(6B 十进制时间)

**Level:** L2
**Level Rationale:** 新包内纯函数,黄金字节单测聚焦覆盖;无跨模块影响。
**Linked Acceptance Items:** PAC-2
**Task Gate:** task reviewer + focused checks
**关联性:** 上游=P1 ext 包;下游=Task 3 realtimeLike 体组装。**线格式必须与库 BeanTime 一致(十进制非 BCD),golden 即契约。**
**重要性:** **P1(M2 前置)**——realtimeLike 命令体的时间头;格式错一处,网关解析整体错位。

**Files:**
- Create: `internal/ext/time.go`
- Test: `internal/ext/time_test.go`(Create)

**Interfaces:**
- Produces: `func EncodeBeanTime(at time.Time) [6]byte`(年-2000 截断 0~255,其余直接取分量)

- [ ] **Step 1: 写失败测试**

`internal/ext/time_test.go`:

```go
package ext

import (
	"testing"
	"time"
)

func TestEncodeBeanTimeGolden(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b := EncodeBeanTime(at)
	// 十进制(非 BCD):年-2000=26=0x1a,月1 日2 时3 分4 秒5
	const want = "1a0102030405"
	if got := hexBytes(b[:]); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestEncodeBeanTimeClamp(t *testing.T) {
	at := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	b := EncodeBeanTime(at)
	if b[0] != 0 {
		t.Fatalf("2000 年前应截断为 0,got %d", b[0])
	}
}

func hexBytes(b []byte) string {
	const d = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, d[x>>4], d[x&0x0F])
	}
	return string(out)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ext/ -run TestEncodeBeanTime -v`
Expected: FAIL,`undefined: EncodeBeanTime`

- [ ] **Step 3: 实现**

`internal/ext/time.go`:

```go
package ext

import "time"

// EncodeBeanTime 按协议库 BeanTime 线格式编码时间:
// 6 字节十进制(年-2000, 月, 日, 时, 分, 秒)——非 BCD。
func EncodeBeanTime(at time.Time) [6]byte {
	y := at.Year() - 2000
	if y < 0 {
		y = 0
	}
	if y > 255 {
		y = 255
	}
	return [6]byte{
		byte(y), byte(at.Month()), byte(at.Day()),
		byte(at.Hour()), byte(at.Minute()), byte(at.Second()),
	}
}
```

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./internal/ext/ -v && go test ./... -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/ext/time.go internal/ext/time_test.go
git commit -m "feat(ext): EncodeBeanTime 6 字节十进制时间编码器(非 BCD)"
```

---

### Task 3: bridge 命令后端(Schema.Source + 合并 + 组装 + 发送 + 周期)

**Level:** L3
**Level Rationale:** 跨 schema/bridge/engine 三层的业务流(命令发送与周期上报),AC-2 的字节级证明所在。
**Linked Acceptance Items:** PAC-1、PAC-2、PAC-3
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 1(注册表)、Task 2(时间)、P1 ext 编码器、P2 的 GetSchema/GetGroups/assembleBody 模式;下游=Task 4/5(前端消费 Source 分组与新绑定)。**命令组键命名空间与 GetGroups 的 order 过滤天然兼容——解绑后命令组配置自动被过滤,零额外代码。**
**重要性:** **P0(M2/M4 核心)**——P3 全部后端价值;wire command byte 断言是 P1-2 契约在本阶段的兑现。

**Files:**
- Modify: `internal/schema/schema.go`(GroupSchema 加 Source)
- Modify: `bridge/message_service.go`
- Modify: `bridge/runtime.go`(**P0-1:注册表接线**——新增 syncExtCommands,SetConnCfg/SetPacks 两处调用)
- Modify: `bridge/message_service_ext_test.go`(追加)
- Create: `bridge/testdata/extcmd.json`

**Interfaces:**
- Consumes: `engine.RegisterCommand/ResetExtCommands`(Task 1)、`ext.EncodeBeanTime`(Task 2)、`ext.CompileUnit/DefaultsFor/EncodeFields/EncodeUnit`(P1)
- Produces: `GroupSchema.Source`、`GetSchema` 合并命令组、`SendExtension(key string) error`、`SetExtAutoReport(key string, enabled bool, intervalSec int) error`、`assembleCommandBody(cmd ext.Command, at time.Time) ([]byte, error)`、**`Runtime.syncExtCommands`(P0-1 接线:包激活/换绑时注册/重置命令表)**

- [ ] **Step 1: 建夹具**

`bridge/testdata/extcmd.json`:

```json
{
  "meta": { "id": "extcmd", "label": "扩展命令包", "vendor": "Test", "baseVersion": "2016" },
  "realtime": { "appendUnits": [] },
  "commands": [
    {
      "key": "extData09", "label": "扩展数据上报", "code": 9,
      "direction": "up", "trigger": "manual+periodic",
      "body": { "type": "fields", "fields": [
        { "key": "seq", "label": "流水号", "type": "u16" },
        { "key": "volt", "label": "电压", "type": "u16", "scale": 0.1, "unit": "V" }
      ] }
    },
    {
      "key": "extReport0A", "label": "扩展报表", "code": 10,
      "direction": "up", "trigger": "manual",
      "body": { "type": "realtimeLike", "units": [{
        "key": "telemetry", "title": "私有遥测", "unitCode": 128, "enabled": true,
        "fields": [
          { "key": "soc2", "label": "SOC2", "type": "u8", "unit": "%" },
          { "key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V" }
        ]
      }] }
    }
  ]
}
```

- [ ] **Step 2: 写失败测试**

`bridge/message_service_ext_test.go` 追加:

```go
func newExtCmdRT(t *testing.T, bound bool) *Runtime {
	t.Helper()
	p, err := ext.LoadFile(filepath.Join("testdata", "extcmd.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{p})
	cfg := &ConnectionConfig{Version: "2016"}
	if bound {
		cfg.ExtensionPack = "extcmd"
	}
	rt.SetConnCfg(cfg)
	return rt
}

func TestGetSchemaCommandGroups(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	merged := ms.GetSchema("2016")
	var cmdGroups []schema.GroupSchema
	for _, g := range merged {
		if g.Source == "command" {
			cmdGroups = append(cmdGroups, g)
		}
	}
	// fields 体 1 组 + realtimeLike 1 单元 1 组
	if len(cmdGroups) != 2 {
		t.Fatalf("命令组数 = %d, want 2: %+v", len(cmdGroups), cmdGroups)
	}
	byKey := map[string]schema.GroupSchema{}
	for _, g := range cmdGroups {
		byKey[g.Key] = g
	}
	if g, ok := byKey["extData09"]; !ok || g.Title != "扩展数据上报" {
		t.Fatalf("extData09 = %+v", g)
	}
	if g, ok := byKey["extReport0A:telemetry"]; !ok || g.Title != "扩展报表 · 私有遥测" {
		t.Fatalf("extReport0A:telemetry = %+v", g)
	}
}

func TestGetSchemaNoPackNoCommandGroups(t *testing.T) {
	rt := newExtCmdRT(t, false)
	ms := NewMessageService(rt)
	for _, g := range ms.GetSchema("2016") {
		if g.Source == "command" {
			t.Fatalf("未绑包不应有命令组: %+v", g)
		}
	}
}

func TestAssembleCommandBodyFieldsGolden(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"extData09": {Enabled: true, Rows: []map[string]any{{"seq": 1, "volt": 3.3}}},
	})
	cmd := rt.Pack().Commands[0]
	b, err := ms.assembleCommandBody(cmd, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// seq u16=1 → 0001;volt 3.3/0.1=33 → 0021
	if got := hexBytes(b); got != "00010021" {
		t.Fatalf("fields 体 = %s, want 00010021", got)
	}
}

func TestAssembleCommandBodyRealtimeLikeGolden(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"extReport0A:telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	cmd := rt.Pack().Commands[1]
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b, err := ms.assembleCommandBody(cmd, at)
	if err != nil {
		t.Fatal(err)
	}
	// 6B 十进制时间 + TLV(80 0003 50 0021)
	if got := hexBytes(b); got != "1a0102030405800003500021" {
		t.Fatalf("realtimeLike 体 = %s", got)
	}
}

func TestSendFrameWireCommandByte(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"extData09": {Enabled: true, Rows: []map[string]any{{"seq": 1, "volt": 3.3}}},
	})
	cmd := rt.Pack().Commands[0]
	payload, err := ms.assembleCommandBody(cmd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := engine.BuildFrame(api.V2016, "LSV00000000000001", byte(cmd.Code), engine.NewRawBody(api.V2016, payload))
	if err != nil {
		t.Fatal(err)
	}
	hexStr := hexBytes(raw)
	// 帧布局:2323(2B)+ 命令码(1B)+ 响应标志(1B)...;命令码即 hex[4:6]
	if hexStr[4:6] != "09" {
		t.Fatalf("wire command byte = %s, want 09(折叠回归!)", hexStr[4:6])
	}
	// 对照组:0x0A 命令码不能折叠成 0x09
	cmdA := rt.Pack().Commands[1]
	payloadA, err := ms.assembleCommandBody(cmdA, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rawA, _, err := engine.BuildFrame(api.V2016, "LSV00000000000001", byte(cmdA.Code), engine.NewRawBody(api.V2016, payloadA))
	if err != nil {
		t.Fatal(err)
	}
	if h := hexBytes(rawA); h[4:6] != "0a" {
		t.Fatalf("wire command byte = %s, want 0a(0x0A 不得折叠为 0x09)", h[4:6])
	}
}

func TestDefaultGroupsCommandDefaults(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	if g, ok := m["extData09"]; !ok || !g.Enabled || len(g.Rows) != 1 {
		t.Fatalf("extData09 默认 = %+v", g)
	}
	if g, ok := m["extReport0A:telemetry"]; !ok || !g.Enabled {
		t.Fatalf("extReport0A:telemetry 默认 = %+v", g)
	}
}
```

测试文件 import 区追加 `"github.com/sunsky74/gb32960/api"` 与 `"gbt32960-simulator/internal/engine"`(若缺);并追加本地助手(评审 P1-2——bridge 包内无 hexBytes):

```go
func hexBytes(b []byte) string { return hex.EncodeToString(b) }
```

再追加周期生命周期测试(评审 P1-4,补 PAC-3 的自动化证据):

```go
func TestExtReportTickerLifecycle(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	// 执行期修订(控制器裁定 2026-09-01):intervalSec=50 超出下方 2s 自停 deadline,
	// 首 tick 永不触发导致测试必然失败;改 1(仅测试侧,实现语义不变)。
	if err := ms.SetExtAutoReport("extData09", true, 1); err != nil {
		t.Fatal(err)
	}
	ms.extMu.Lock()
	_, registered := ms.extStops["extData09"]
	ms.extMu.Unlock()
	if !registered {
		t.Fatal("ticker 未注册")
	}
	// 解绑 → 自停(每次 tick 检查包与命令存在性)
	rt.SetConnCfg(&ConnectionConfig{Version: "2016"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		ms.extMu.Lock()
		_, ok := ms.extStops["extData09"]
		ms.extMu.Unlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("解绑后 ticker 未自停")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 执行期修订(评审 Important#2,人类已裁决):原解绑态下 start 静默失败致重启路径空转,
	// 改为重绑包后重启并断言成功。
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	if err := ms.SetExtAutoReport("extData09", true, 50); err != nil {
		t.Fatalf("重绑后重启应成功: %v", err)
	}
	ms.stopExtReport("extData09")
	if err := ms.SetExtAutoReport("extData09", false, 0); err != nil {
		t.Fatalf("停止应成功: %v", err)
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestGetSchemaCommand|TestAssembleCommand|TestSendFrameWire|TestDefaultGroupsCommand" -v`
Expected: FAIL,`g.Source undefined` / `ms.assembleCommandBody undefined`

- [ ] **Step 4: 实现**

`internal/schema/schema.go` 的 GroupSchema 结构体,`Fields` 之后追加:

```go
	Source    string        `json:"source,omitempty"` // 空=标准实时; "command"=扩展命令组(前端分流到自定义数据 tab)
```

`bridge/message_service.go`:

GetSchema 替换为(命令组合并在同一门禁内):

```go
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
```

unitDefaults 替换为(扩展命令默认行并入同表):

```go
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
```

文件追加(**P0-1 接线先行**——`bridge/runtime.go` 新增并接线;`SetConnCfg` 与 `SetPacks` 的 `resolvePack` 之后各加一行 `rt.syncExtCommands(rt.pack)`):

```go
// syncExtCommands 按激活包同步引擎命令注册表:先重置(保留 私有远控 内置),再注册包内命令。
// 挂接在 SetConnCfg/SetPacks——包激活的唯一入口,查询接口(GetSchema)不携带副作用。
func (rt *Runtime) syncExtCommands(p *ext.Pack) {
	engine.ResetExtCommands()
	if p == nil {
		return
	}
	v := parseVersion(p.Meta.BaseVersion)
	for _, c := range p.Commands {
		engine.RegisterCommand(v, byte(c.Code), c.Label)
	}
}
```

MessageService 结构体与构造的精确改动如下,import 区新增 `"strings"` 与 `"sync"`:

```go
// MessageService 结构体定义改为(在 rt 字段后追加两个周期状态字段):
type MessageService struct {
	rt       *Runtime
	extMu    sync.Mutex
	extStops map[string]chan struct{}
}

// NewMessageService 追加初始化:
func NewMessageService(rt *Runtime) *MessageService {
	ms := &MessageService{rt: rt, extStops: map[string]chan struct{}{}}
	if g := ms.loadGroups(); g != nil {
		rt.SetGroups(g)
	}
	return ms
}
```

```go
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

// 执行期修订(评审 Important#1,人类已裁决 2026-09-01):compare-and-delete 变体——
// 仅当注册表中 key 仍指向 own 时才删除并关闭,防止在途 ticker 的自停路径
// 误杀解绑→重绑后新注册的 ticker(TOCTOU)。提交 6574f9f。
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
				// 包被解绑/命令消失 → 自停清理
				p := s.rt.Pack()
				if p == nil || packCommand(p, key) == nil {
					// 执行期修订(评审 Important#1):改用 compare-and-delete,防误杀新 ticker
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
```

(import 区确认:`context`、`fmt`、`strings`、`sync`、`time`、`engine`、`ext` 均已在或补齐;`strings` 若缺则加。)

- [ ] **Step 5: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -v && go test ./... -count=1`
Expected: PASS(P2 全部测试保持绿——GetGroups 过滤对命令键同样生效)

- [ ] **Step 6: 提交**

```bash
git add internal/schema/schema.go bridge/message_service.go bridge/runtime.go bridge/message_service_ext_test.go bridge/testdata/extcmd.json
git commit -m "feat(ext): 扩展命令后端——注册表接线/schema 合并/双体组装/发送/周期,wire 命令字节锁定"
```

---

### Task 4: 前端字段 helpers 抽取与 schema 分流

**Level:** L3
**Level Rationale:** 跨组件重构(纯函数提取,行为不变)+ 状态层分流,Task 5 的依赖根。
**Linked Acceptance Items:** PAC-1(M3 表单前置)
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 3 的 GroupSchema.Source;下游=Task 5(ExtensionCommands 复用 helpers)。**helpers 抽取是纯搬移,构建通过即证明行为不变。**
**重要性:** **P1**——消除 RealTimePanel 与 ExtensionCommands 的字段渲染代码重复(终审 DRY 原则),是 M3 的地基。

**Files:**
- Regenerate: `frontend/wailsjs/**`(**评审 P1-3:`wails generate module` 前移到本任务 Step 1**,否则 applySchema 消费的 `g.source` 类型在本任务提交点不存在)
- Create: `frontend/src/composables/useFieldHelpers.ts`
- Modify: `frontend/src/components/RealTimePanel.vue`(删除内联 helpers 改为 import)
- Modify: `frontend/src/state.ts`(store.extSchema + applySchema 分流)

**Interfaces:**
- Produces: `defaultFor(f)`、`numOf/setNum/boolOf/setBool/setEnum`、`arrayValues/setArrayValue/addArrayItem`、`hexOf/setHex`、**`bitsObjOf/bitsArrayOf/setBitsArray/bitOptions`(评审 P1-5,共 15 个纯函数)**;`store.extSchema: GroupSchema[]`、`applySchema(list: GroupSchema[])`

- [ ] **Step 1: 再生成前端绑定(评审 P1-3,从 Task 5 前移)**

Run: `wails generate module`
Expected: models.ts 的 GroupSchema 增加 `source?: string`;MessageService 绑定含 SendExtension/SetExtAutoReport(Task 3 已落地的 Go 方法一并生成)

- [ ] **Step 2: 建 helpers 文件**

`frontend/src/composables/useFieldHelpers.ts`(内容 = RealTimePanel.vue 现有 defaultFor/numOf/setNum/boolOf/setBool/setEnum/arrayValues/setArrayValue/addArrayItem/hexOf/setHex/bitsObjOf/bitsArrayOf/setBitsArray/bitOptions **15 个函数原样搬移**(评审 P1-5:补全位函数),加 `export` 与 `import type { FieldSchema } from '../api/backend'`):

```ts
import type { FieldSchema } from '../api/backend'

export function defaultFor(f: FieldSchema): unknown {
  switch (f.kind) {
    case 'enum':
      return f.enum?.[0]?.value ?? 1
    case 'bool':
      return false
    case 'bitgroup': {
      const bits: Record<string, boolean> = {}
      for (const b of f.bits ?? []) bits[`bit${b.index}`] = false
      return bits
    }
    case 'array_float':
      return []
    case 'bytes':
      return f.length ? '00'.repeat(f.length) : ''
    case 'int':
      return f.min ?? 0
    default:
      return f.min ?? 0
  }
}

export function numOf(row: Record<string, unknown>, key: string): number {
  const v = row[key]
  return typeof v === 'number' ? v : Number(v) || 0
}

export function setNum(row: Record<string, unknown>, key: string, v: number | string | null | undefined) {
  row[key] = typeof v === 'number' ? v : Number(v) || 0
}

export function boolOf(row: Record<string, unknown>, key: string): boolean {
  return row[key] === true
}

export function setBool(row: Record<string, unknown>, key: string, v: unknown) {
  row[key] = v === true
}

export function setEnum(row: Record<string, unknown>, key: string, v: unknown) {
  row[key] = typeof v === 'number' ? v : Number(v) || 0
}

export function arrayValues(row: Record<string, unknown>, key: string): number[] {
  const v = row[key]
  return Array.isArray(v) ? v.map(Number) : []
}

export function setArrayValue(row: Record<string, unknown>, key: string, idx: number, v: number | string | null | undefined) {
  const arr = arrayValues(row, key).slice()
  arr[idx] = typeof v === 'number' ? v : Number(v) || 0
  row[key] = arr
}

export function addArrayItem(row: Record<string, unknown>, key: string) {
  const arr = arrayValues(row, key)
  arr.push(0)
  row[key] = arr
}

export function hexOf(row: Record<string, unknown>, key: string): string {
  const v = row[key]
  return typeof v === 'string' ? v : ''
}

export function setHex(row: Record<string, unknown>, key: string, f: FieldSchema, v: string) {
  const s = v.trim().toLowerCase().replace(/\s+/g, '')
  if (s === '') {
    row[key] = ''
    return
  }
  if (!/^[0-9a-f]*$/.test(s)) return
  if (f.length && s.length > f.length * 2) return
  row[key] = s
}

export function bitsObjOf(row: Record<string, unknown>, field: FieldSchema): Record<string, boolean> {
  const v = row[field.key]
  return (v && typeof v === 'object' ? v : {}) as Record<string, boolean>
}

export function bitsArrayOf(row: Record<string, unknown>, field: FieldSchema): string[] {
  return Object.entries(bitsObjOf(row, field))
    .filter(([, on]) => on)
    .map(([k]) => k)
}

export function setBitsArray(row: Record<string, unknown>, field: FieldSchema, vals: Array<string | number | boolean>) {
  const picked = new Set(vals.map(String))
  const bits: Record<string, boolean> = {}
  for (const b of field.bits ?? []) bits[`bit${b.index}`] = picked.has(`bit${b.index}`)
  row[field.key] = bits
}

export function bitOptions(field: FieldSchema) {
  return (field.bits ?? []).map((b) => ({ label: b.label, value: `bit${b.index}` }))
}
```

- [ ] **Step 3: RealTimePanel 改引用**

script 区:删除上述 15 个函数定义,改为:

```ts
import {
  addArrayItem, arrayValues, bitOptions, bitsArrayOf, bitsObjOf, boolOf, defaultFor,
  hexOf, numOf, setArrayValue, setBitsArray, setBool, setEnum, setHex, setNum,
} from '../composables/useFieldHelpers'
```

(其余 import 不动;`FieldSchema` 类型若模板不再直接引用则从 import 中移除,以 vue-tsc 为准。)

- [ ] **Step 4: state.ts 分流**

```ts
export const store = reactive({
  connState: 'idle',
  connBusy: false,
  config: null as ConnConfig | null,
  profiles: [] as bridge.ProfileSummary[],
  schema: [] as GroupSchema[],
  extSchema: [] as GroupSchema[],
  groups: {} as GroupsState,
  consoleEvents: [] as ConsoleEvent[],
  consolePaused: false,
  consoleFilter: 'all' as 'all' | 'tx' | 'rx' | 'conn' | 'error',
})
```

新增并在 loadInitialData/reloadSchemaForVersion/reloadSchemaPreservingGroups 三处替换 `store.schema = ...` 赋值:

```ts
export function applySchema(list: GroupSchema[]) {
  store.schema = list.filter((g) => g.source !== 'command')
  store.extSchema = list.filter((g) => g.source === 'command')
}
```

即:三处 `store.schema = await MessageService.GetSchema(...)` 改为 `applySchema(await MessageService.GetSchema(...))`。

- [ ] **Step 5: 构建验证**

Run: `cd frontend && npm run build`
Expected: vue-tsc 零错误(helpers 搬移是行为不变的纯重构;source 类型已由 Step 1 再生成就绪)

- [ ] **Step 6: 提交**

```bash
git add frontend/wailsjs frontend/src/composables/useFieldHelpers.ts frontend/src/components/RealTimePanel.vue frontend/src/state.ts
git commit -m "feat(ext): 字段 helpers 抽取(含位函数)、绑定再生成与 schema 按 source 分流"
```

---

### Task 5: 自定义数据 tab 与扩展命令区(前端交付面)

**Level:** L3
**Level Rationale:** 用户可见 UI(新 tab + 新组件 + 两个新绑定)跨前后端契约。
**Linked Acceptance Items:** PAC-1、PAC-3
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 3 绑定(SendExtension/SetExtAutoReport,经 wailsjs 再生成)、Task 4(helpers/extSchema);下游=用户与 P4。**自定义数据 tab 是 P2 时代就存在的禁用占位,本任务将其激活,不做 tab 结构改动。**
**重要性:** **P1(M3/M4 的前端半场)**——没有本任务,后端能力对实施人员不可见。

**Files:**
- Create: `frontend/src/components/ExtensionCommands.vue`
- Modify: `frontend/src/components/RealTimePanel.vue`(tab 激活 + 引用)
- (wailsjs 再生成已前移至 Task 4 Step 1,本任务无再生成步骤)

**Interfaces:**
- Consumes: `store.extSchema`/`store.groups`(Task 4)、`MessageService.SendExtension/SetExtAutoReport`(Task 4 再生成绑定)、helpers(Task 4)
- Produces: 自定义数据 tab 内容(每命令组:表单[含 bitgroup 分支] + [发送] + 周期开关/间隔)

- [ ] **Step 1: 建 ExtensionCommands.vue**

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import type { GroupSchema } from '../api/backend'
import { stateToPayload } from '../api/backend'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import { store } from '../state'
import {
  bitOptions, bitsArrayOf, defaultFor, hexOf, numOf, setBitsArray, setEnum, setHex, setNum,
} from '../composables/useFieldHelpers'

const reports = ref<Record<string, { on: boolean; interval: number }>>({})

function ensureGroup(g: GroupSchema) {
  if (!store.groups[g.key]) {
    const row: Record<string, unknown> = {}
    for (const f of g.fields) row[f.key] = defaultFor(f)
    store.groups[g.key] = { enabled: true, rows: [row] }
  }
  return store.groups[g.key]
}

function cmdKeyOf(g: GroupSchema): string {
  return g.key.split(':')[0]
}

function reportOf(key: string) {
  if (!reports.value[key]) reports.value[key] = { on: false, interval: 10 }
  return reports.value[key]
}

async function saveAll() {
  // 执行期修订(评审 Important,人类已裁决 2026-09-01):移除内部 try/catch,错误透传——
  // send/toggleReport 的 catch 统一中止并提示,与 0x02 abort 语义对齐。提交 cd61254。
  const order = [...store.schema.map((g) => g.key), ...store.extSchema.map((g) => g.key)]
  await MessageService.SaveGroups(stateToPayload(store.groups, order))
}

async function send(g: GroupSchema) {
  try {
    await saveAll()
    await MessageService.SendExtension(cmdKeyOf(g))
    message.success(`「${g.title}」已发送`)
  } catch (e) {
    message.error('发送失败: ' + String(e))
  }
}

async function toggleReport(g: GroupSchema, checked: boolean) {
  const r = reportOf(cmdKeyOf(g))
  try {
    await saveAll()
    await MessageService.SetExtAutoReport(cmdKeyOf(g), checked, r.interval)
    r.on = checked
    message.success(checked ? `周期上报已开启 (每 ${r.interval}s)` : '周期上报已停止')
  } catch (e) {
    r.on = false // 评审 P2-1:失败回滚,与 RealTimePanel 的 toggleAutoReport 行为一致
    message.error(String(e))
  }
}

async function onReportIntervalChange(g: GroupSchema) {
  const r = reportOf(cmdKeyOf(g))
  if (!r.on) return
  try {
    await MessageService.SetExtAutoReport(cmdKeyOf(g), true, r.interval)
  } catch (e) {
    message.error(String(e))
  }
}
</script>

<template>
  <div class="zone-body">
    <a-empty v-if="store.extSchema.length === 0" description="绑定含 commands 的扩展包后,在此配置与发送私有命令" />
    <a-collapse v-else ghost expand-icon-position="end" class="group-collapse">
      <a-collapse-panel v-for="g in store.extSchema" :key="g.key">
        <template #header>
          <span class="group-title">{{ g.title }}</span>
        </template>
        <div class="group-body">
          <div class="extcmd-actions">
            <a-button size="small" type="primary" @click="send(g)">发送</a-button>
          </div>
          <div class="group-row">
            <div class="fields-grid">
              <template v-for="f in g.fields" :key="f.key">
                <div v-if="f.kind === 'enum'" class="field">
                  <span class="field-label">{{ f.label }}</span>
                  <a-select
                    :value="numOf(ensureGroup(g).rows[0], f.key)"
                    size="small"
                    :options="f.enum?.map((e) => ({ value: e.value, label: e.label })) ?? []"
                    @change="(v: unknown) => setEnum(ensureGroup(g).rows[0], f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'int' || f.kind === 'float'" class="field">
                  <span class="field-label">{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span>
                  <a-input-number
                    :value="numOf(ensureGroup(g).rows[0], f.key)"
                    size="small"
                    :step="f.kind === 'int' ? 1 : 0.1"
                    :min="f.min"
                    :max="f.max"
                    style="width: 100%"
                    @change="(v: number | string | null | undefined) => setNum(ensureGroup(g).rows[0], f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'bytes'" class="field">
                  <span class="field-label">{{ f.label }}<em v-if="f.length"> ({{ f.length }}B hex)</em></span>
                  <a-input
                    :value="hexOf(ensureGroup(g).rows[0], f.key)"
                    class="hex-input"
                    size="small"
                    :placeholder="f.length ? `${f.length * 2} 个 hex 字符` : 'hex'"
                    @update:value="(v: string) => setHex(ensureGroup(g).rows[0], f.key, f, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'bitgroup'" class="field field-bits">
                  <span class="field-label">{{ f.label }}</span>
                  <a-checkbox-group
                    :value="bitsArrayOf(ensureGroup(g).rows[0], f)"
                    :options="bitOptions(f)"
                    class="bits-group"
                    @change="(vals: Array<string | number | boolean>) => setBitsArray(ensureGroup(g).rows[0], f, vals)"
                  />
                </div>
              </template>
            </div>
          </div>
          <div class="extcmd-actions">
            <a-switch v-model:checked="reportOf(cmdKeyOf(g)).on" size="small" @change="(v: unknown) => toggleReport(g, v === true)" />
            <span class="report-label">周期上报</span>
            <a-input-number
              v-model:value="reportOf(cmdKeyOf(g)).interval"
              :min="1"
              :max="3600"
              size="small"
              addon-after="秒"
              style="width: 130px"
              @change="() => onReportIntervalChange(g)"
            />
          </div>
        </div>
      </a-collapse-panel>
    </a-collapse>
  </div>
</template>

<style scoped>
.extcmd-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.hex-input :deep(input) {
  font-family: var(--font-mono);
}
</style>
```

- [ ] **Step 2: RealTimePanel tab 激活**

模板第 406 行改为:

```html
      <a-tab-pane key="custom" tab="自定义数据" :disabled="store.extSchema.length === 0">
        <ExtensionCommands />
      </a-tab-pane>
```

script 区 import `ExtensionCommands from './ExtensionCommands.vue'` 与 `store`(若未 import)。

- [ ] **Step 3: 构建验证**

Run: `cd frontend && npm run build`
Expected: vue-tsc 零错误 + vite 构建成功

- [ ] **Step 4: 手动验收(wails dev,控制器执行)**

1. 把 `bridge/testdata/extcmd.json` 复制到 packs 目录,启动 `wails dev`
2. 连接卡选择"扩展命令包 (2016)"并保存 → 自定义数据 tab 变为可用,含"扩展数据上报"与"扩展报表 · 私有遥测"两组
3. extData09 填 seq=1、volt=3.3,点"发送" → 控制台 TX 帧 hex `[4:6]=="09"`、体尾 `00010021`
4. extData09 开周期开关(间隔 10s)→ 观察连续 TX;关开关停止。**extReport0A 的周期开关会报"不支持周期上报"(trigger=manual 门禁),属预期行为**
5. 解绑包保存 → 自定义数据 tab 恢复禁用,标准面板不受影响
6. 重启 wails dev → 绑定保持,tab 直接可用(激活链)

**已知限制(评审 P2-3/P2-5,预期行为非 bug)**:①重启后周期开关需重新开启(ticker 与前端开关均为会话级状态,与 0x02 周期上报一致);②解绑后在标准 tab 保存配置会把命令组配置从 message.json 抹掉(命令配置随绑定生命周期),重绑后命令组恢复默认值。

```bash
mkdir -p ~/Library/Application\ Support/gbt32960-simulator/packs
cp bridge/testdata/extcmd.json ~/Library/Application\ Support/gbt32960-simulator/packs/
```

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/ExtensionCommands.vue frontend/src/components/RealTimePanel.vue
git commit -m "feat(ext): 自定义数据 tab 与扩展命令区,发送/周期交互落地"
```

---

## 任务依赖图

```
Task 1(注册表)─┐
Task 2(时间)  ─┼─▶ Task 3(命令后端)─▶ Task 4(helpers+分流)─▶ Task 5(前端 tab)
```

关键耦合点(评审重点):Task 3 的版本门禁沿用 P2 同款表达式;命令组键命名空间(fields=`key`、realtimeLike=`key:unitkey`)在 Task 3 编译、Task 3 测试、Task 5 前端三处必须一致;wire byte 断言 hex[4:6] 依赖帧布局(2323+cmd+RS)。

## Self-Review 记录

1. **Spec 覆盖**:设计 §4.5 接入点#1(注册表+精确码副本)= Task 1/3 测试;realtimeLike 体(§4.3 十进制时间)= Task 2/3;命令组合并与 Source(§4.5#3 延伸)= Task 3/4;前端扩展命令区 = Task 5。AC-2/AC-3 → PAC-1~4 全覆盖。
2. **占位符扫描**:无 TBD;所有代码步骤含完整代码。命令码不进入 UI 文案(按钮显示"发送"、成功文案带命令名)——wire 命令字节由控制台 TX hex 与单测断言验证,避免把 `cmdKeyOf`(键)误当命令码展示。
3. **类型一致性**:`extCommandName` Task 1 定义、Task 1 测试消费;`EncodeBeanTime` Task 2 定义、Task 3 消费;`assembleCommandBody(cmd ext.Command, at time.Time) ([]byte, error)` Task 3 定义与测试一致;helpers 15 函数 Task 4 定义、Task 5 消费与 RealTimePanel 共用;`applySchema` Task 4 定义、三处替换;`syncExtCommands` Task 3 定义、SetConnCfg/SetPacks 两处接线。
4. **拆分决策**:P3 独立可验收(M1~M4),符合 Master 分解。
5. **Level/Gate 完整**:4 个 L3 + 1 个 L2,均带 rationale 与 gate。
6. **L3 AC 绑定**:每个 L3 任务 Linked AC 非空且存在于 PAC 清单。
7. **AC 覆盖**:PAC-1←Task 3/5;PAC-2←Task 2/3;PAC-3←Task 3/5;PAC-4←Task 1/3。
8. **AC 可执行**:每条 PAC 带测试名/黄金 hex/手动步骤。
9. **已知执行期笔误风险**:Golang 任务(Task 1-3)代码经逐字推演可编译(MessageService 结构体改动与 import 清单已在 Task 3 写死);Vue 模板经 vue-tsc 语义核对(helpers 签名与 Task 4 一致)。
10. **Oracle 执行前评审修订(2026-09-01)**:已落实 P0-1(runtime.syncExtCommands 接线——激活/换绑时注册与重置命令表,挂在 SetConnCfg/SetPacks)、P1-1(client.go:504 私有远控 引用迁移 + 两处 BCD 误导注释修正)、P1-2(测试本地 hexBytes 助手)、P1-3(wails generate module 前移至 Task 4 Step 1)、P1-4(TestExtReportTickerLifecycle 补周期自动化证据)、P1-5(位函数加入 helpers + bitgroup 模板分支)、P2-1(toggleReport 失败回滚)、P2-2(间隔修改重启 ticker)、P2-3/P2-5(验收清单已知限制注明)、P2-4(SetExtAutoReport trigger=manual 门禁)、P2-6(函数计数与验收文案订正)。
