# Phase 4 · 管理与交付面 Implementation Plan

> **For agentic workers:** Recommended execution: superpowers:ltdd(Quality-LTDD)或 superpowers:subagent-driven-development(P1/P2/P3 已验证路径)。Steps 用 checkbox。

**Goal:** 实施人员在设置页完成扩展包的导入/删除/换绑闭环,保存的扩展配置在换绑/解绑后不丢、非法行值保存被拦截且报 JSON 路径级错误;并交付《插件包编写指南》与内置示例包。

**Architecture:** 后端在既有 `ExtService` 上增量(目录自建/导入/删除,复用 P1 的 `ext.LoadFile` 预校验);`MessageService.SaveGroups` 增扩展行干跑(复用 P1 编码器)与按包独立存储(extgroups.json);前端设置页新建包管理组件,扩展命令区补齐版本可见性/多行表单。

**Tech Stack:** Go 1.25 / Wails v2 / Vue 3 + Ant Design Vue;`wails generate module` 再生成绑定。

**Master 索引:** `docs/superpowers/plans/2026-08-31-extpack-master-plan.md`
**设计契约:** `docs/superpowers/specs/2026-08-31-extpack-design.md` §4(§9 G-1/G-2 均已关闭)

## 阶段目标与业务里程碑

**阶段目标(一句话):** 让扩展包具备产品级交付面——设置页管包、配置不丢、错误可见、有指南有示例,实施人员按指南从拿到包到发出第一帧私有数据全程无代码。

**业务里程碑(验收顺序即达成顺序):**

| 里程碑 | 内容 | 覆盖任务 | 对应 Global AC |
|---|---|---|---|
| **M1 包管理闭环** | packs 目录自建;设置页导入/删除/列表/重扫;删除绑定包自动解绑 | Task 1-2 | AC-4 的 UI 面 |
| **M2 配置可靠** | 保存扩展行干跑(非法值不落盘);扩展组配置按包独立存储(解绑不抹) | Task 3-4 | AC-5 强化 |
| **M3 命令区无静默** | 版本不匹配可见文案;multiple 单元多行表单;ensureGroup 对齐 g.enabled | Task 5 | oracle 审核 D/F 消化 |
| **M4 交付面** | 《插件包编写指南》+ 内置示例包;设计契约回写(命令码唯一/key 约束/多行支持) | Task 6 | AC-6 |

## Global Constraints(继承 Master + 本阶段特有)

- 协议库 gb32960-go 零改动;P1/P2/P3 交付 API 不改签名只消费(ext.LoadFile/LoadDir/Validate/DryRun/EncodeUnit/EncodeFields/DefaultsFor/CompileUnit)
- 标准路径现有 golden/私有远控/downlink/PAC 测试必须保持全绿(逐字节)
- **坏包不阻塞好包**(继承 LoadDir 模式):扫描聚合错误;导入单个坏包报错且**不落盘**(AC-4)
- 错误信息一律带 JSON 路径定位(如 `realtime.appendUnits[0].fields[3].scale`);新错误文案用中文、沿用 `verrf`/`fmt.Errorf` 风格
- 版本门禁沿用 `p.Meta.BaseVersion == version`(字符串比较);仅 2016/2025 两版本
- 前端零新增依赖;`wails generate module` 产物必须随代码提交。**执行期修订(2026-09-01,控制器裁定)**:wails v2.15 前端注入层无 dialog 导出,文件选择改走 Go 侧 `ExtService.PickPackFile()`(runtime.OpenFileDialog + SetContext,与 ConsoleService 同款先例),Task 2 提交需含 bridge/ext_service.go 与 app.go
- 命令组键命名空间(fields=`key`、realtimeLike=`key:unitkey`)不变;扩展组键由 P3 修复批禁冒号 + 保留表兜底,本阶段不重复校验
- **绝不 `git add -A`**(工作区有历史脏文件);每任务 SDD 双门(实现者→评审者);计划笔误最小修复 + 申报 + 回写

## Final Acceptance Checklist (Refined from Spec) - MUST

- [PAC-1](源:AC-4 的 UI 面) 设置页导入非法包(语法/结构/语义/干跑任一失败)报带路径的错误且不落盘;导入合法包后下拉与列表立即可见
  Refinement: 单测 `TestImportPackInvalidNotPersisted`(非法 JSON → err 非 nil + packs 目录空)、`TestImportPackValid`(合法包落盘 + ListPacks 可见);手动 `wails dev`:设置页导入 → 连接卡下拉出现
- [PAC-2](源:AC-4 的 UI 面) 删除包后列表消失;删除当前绑定包 → 激活自动解绑(自定义数据 tab 禁用、注册表重置)
  Refinement: 单测 `TestDeletePackAutoUnbind`(删除绑定包后 `rt.Pack()==nil`);手动验收:删除后 tab 恢复禁用
- [PAC-3](源:AC-5 强化) 扩展组配置(命令组 + 实时追加单元)在解绑保存 → 重绑后恢复;标准组配置始终不丢
  Refinement: 单测 `TestExtGroupsPersistAcrossUnbind`(绑包存 extData09{seq:42} → 解绑存标准 → 重绑 GetGroups 含 seq=42)
- [PAC-4](源:值校验) 保存含非法行值(如 u16 超范围)报错且不落盘/不写内存快照;合法值保存成功
  Refinement: 单测 `TestSaveGroupsExtDryRunRejects`(err 含"扩展命令 extData09" + `rt.Groups()==nil`)、`TestSaveGroupsExtDryRunPasses`
- [PAC-5](源:oracle D) 绑定版本不匹配的包后,自定义数据 tab 可用并显示"版本不匹配"文案(不再静默禁用);未绑包时 tab 保持禁用
  Refinement: 手动验收:绑 2016 包 + 档案 2025 → tab 可点开,空态显示基准版本与档案版本
- [PAC-6](源:oracle F) realtimeLike multiple 单元可加行/删行,每行一个 TLV 发送;fields 体维持单行
  Refinement: 手动验收:示例包 multiple 单元加 2 行 → 发送 → 控制台 TX 体尾含两个 TLV
- [PAC-7](源:AC-6) 按《指南》导入内置示例包并完成第一个自定义字段扩展(计时,目标 < 10 分钟)
  Refinement: 单测 `TestDocsSamplePackValid`(示例包经 ext.LoadFile 全过);手动:按指南实操计时

---

## File Structure(本阶段触碰面)

| 文件 | 动作 | 职责 |
|---|---|---|
| `bridge/ext_service.go` | Modify | packsDir 自建;PackInfo+CommandCount;ImportPack/DeletePack |
| `bridge/ext_service_test.go` | Modify(追加) | 包管理后端测试(执行期修订:P2 已建该文件,原标 Create 有误) |
| `bridge/runtime.go` | Modify | SetConnCfg 换绑/换版本时清内存组快照(按包独立存储的前置) |
| `bridge/message_service.go` | Modify | validateExtRows(干跑);saveExtGroups/loadAllGroups/extKeys(按包存储) |
| `bridge/message_service_ext_test.go` | Modify(追加) | 干跑拦截 + 跨解绑持久测试 |
| `frontend/wailsjs/**` | Regenerate | ImportPack/DeletePack 绑定 + PackInfo.commandCount |
| `frontend/src/state.ts` | Modify | store.packs |
| `bridge/ext_service.go` | Modify(执行期修订) | PickPackFile/SetContext(前端无 dialog 导出的替代) |
| `app.go` | Modify(执行期修订) | startup 注入 ExtService ctx |
| `frontend/src/components/PackManager.vue` | Create | 设置页包管理(列表/导入/删除/重扫) |
| `frontend/src/pages/SettingsPage.vue` | Modify | 挂载 PackManager |
| `frontend/src/components/cards/ConnectionCard.vue` | Modify | refreshPacks 抽取 + 下拉重扫 + store.packs 同步 |
| `frontend/src/components/ExtensionCommands.vue` | Modify | 三态空态/ensureGroup 对齐/多行表单 |
| `frontend/src/components/RealTimePanel.vue` | Modify | tab 禁用条件改为"未绑包才禁用" |
| `docs/extpack-guide.md` | Create | 《插件包编写指南》(AC-6) |
| `docs/extpack/sample-pack.json` | Create | 内置示例包 |
| `internal/ext/sample_test.go` | Create | 示例包合法性回归锁 |
| `docs/superpowers/specs/2026-08-31-extpack-design.md` | Modify | §4.3/§4.4 契约回写 |

---

### Task 1: 后端包管理(目录自建 + 导入 + 删除)

**Level:** L2
**Level Rationale:** 局部行为变更(ExtService 新方法 + packsDir 自建);跨模块效果(换绑/解绑)经既有 `SetPacks → resolvePack → syncExtCommands` 已测通路,不新增跨模块逻辑。
**Linked Acceptance Items:** PAC-1、PAC-2
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=P1 `ext.LoadFile`(预校验复用)、P2 ExtService/Runtime;下游=Task 2(前端消费 ImportPack/DeletePack 绑定与 PackInfo.commandCount)。**删除绑定包后自动解绑是既有 resolvePack 链路的既有行为,本任务只是给它一个入口。**
**重要性:** **P0(M1 核心)**——没有导入/删除,P4 的"设置页管包"无从谈起;AC-4 的"非法包不落盘"在本任务兑现后端半场。

**Files:**
- Modify: `bridge/ext_service.go`
- Test: `bridge/ext_service_test.go`(Modify 追加——执行期修订:P2 已建该文件,既有 3 测试保留)

**Interfaces:**
- Consumes: `ext.LoadFile(path) (*Pack, error)`(load.go:14)、`ext.LoadDir(dir)`、`store.Dir()`、`rt.SetPacks/SetConnCfg/Pack/Packs`
- Produces: `packsDir()` 自建目录、`PackInfo.CommandCount int`、`func (s *ExtService) ImportPack(path string) (PackInfo, error)`、`func (s *ExtService) DeletePack(id string) error`、内部 `packInfoOf(p *ext.Pack) PackInfo`

- [ ] **Step 1: 写失败测试**

`bridge/ext_service_test.go`(追加到既有文件末尾,既有 P2 测试零改动):

```go
package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

// tempHome 让 store.Dir() 落到临时目录(macOS UserConfigDir 跟随 $HOME)。
func tempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const extcmdJSON = `{
  "meta": {"id": "extcmd", "label": "扩展命令包", "vendor": "Test", "baseVersion": "2016"},
  "realtime": {"appendUnits": []},
  "commands": [
    {"key": "extData09", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual+periodic",
     "body": {"type": "fields", "fields": [
       {"key": "seq", "label": "流水号", "type": "u16"},
       {"key": "volt", "label": "电压", "type": "u16", "scale": 0.1, "unit": "V"}
     ]}}
  ]
}`

func TestImportPackValid(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	src := writeFile(t, t.TempDir(), "vendor-pack.json", extcmdJSON)
	info, err := svc.ImportPack(src)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "extcmd" || info.CommandCount != 1 || info.BaseVersion != "2016" {
		t.Fatalf("info = %+v", info)
	}
	dir, err := packsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "extcmd.json")); err != nil {
		t.Fatalf("应落盘 packs 目录: %v", err)
	}
	packs, err := svc.ListPacks()
	if err != nil || len(packs) != 1 {
		t.Fatalf("packs = %+v err = %v", packs, err)
	}
}

func TestImportPackInvalidNotPersisted(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	src := writeFile(t, t.TempDir(), "bad.json", `{"meta": {}}`)
	if _, err := svc.ImportPack(src); err == nil {
		t.Fatal("非法包应拒绝")
	}
	dir, _ := packsDir()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("非法包不得落盘: %v", entries)
	}
}

func TestDeletePackAutoUnbind(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	src := writeFile(t, t.TempDir(), "vendor-pack.json", extcmdJSON)
	if _, err := svc.ImportPack(src); err != nil {
		t.Fatal(err)
	}
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	if rt.Pack() == nil {
		t.Fatal("导入后应可绑定")
	}
	if err := svc.DeletePack("extcmd"); err != nil {
		t.Fatal(err)
	}
	if rt.Pack() != nil {
		t.Fatal("删除绑定包后应自动解绑")
	}
	packs, _ := svc.ListPacks()
	if len(packs) != 0 {
		t.Fatalf("删除后列表应空: %+v", packs)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestImportPack|TestDeletePack" -v`
Expected: FAIL,`undefined: svc.ImportPack`(DeletePack 同理)

- [ ] **Step 3: 实现**

`bridge/ext_service.go` 的修改(import 区新增 `"os"` 与 `"strings"`):

```go
// PackInfo 扩展包摘要(前端下拉与包管理用)。
type PackInfo struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Vendor       string `json:"vendor,omitempty"`
	BaseVersion  string `json:"baseVersion"`
	UnitCount    int    `json:"unitCount"`
	CommandCount int    `json:"commandCount"`
}

// extGroupsFile 扩展组配置按包独立存储:map[packID]map[groupKey]GroupConfig。
// 本常量在 Task 1 定义(ImportPack 覆盖导入清配置需要),Task 4 直接复用。
const extGroupsFile = "extgroups.json"
```

`packsDir` 替换为(自建目录,导入与扫描共用):

```go
func packsDir() (string, error) {
	dir, err := store.Dir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "packs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建扩展包目录失败: %w", err)
	}
	return dir, nil
}
```

`packInfosOf` 替换为(提取单包 helper,供 ImportPack 复用):

```go
func packInfoOf(p *ext.Pack) PackInfo {
	return PackInfo{
		ID: p.Meta.ID, Label: p.Meta.Label, Vendor: p.Meta.Vendor,
		BaseVersion: p.Meta.BaseVersion, UnitCount: len(p.Realtime.AppendUnits),
		CommandCount: len(p.Commands),
	}
}

func packInfosOf(packs []*ext.Pack) []PackInfo {
	out := make([]PackInfo, 0, len(packs))
	for _, p := range packs {
		out = append(out, packInfoOf(p))
	}
	return out
}
```

文件末尾追加(import 区需补 `"gbt32960-simulator/internal/schema"`):

```go
// clearExtGroupsFor 清除指定包的扩展组配置(覆盖导入同 id 新布局时调用,
// 防止旧布局行值残留导致新布局保存被"缺少值"拦截——评审 P0-2)。
func clearExtGroupsFor(packID string) error {
	var all map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &all); err != nil || all == nil {
		return nil // 无扩展组配置,无需清理
	}
	if _, ok := all[packID]; !ok {
		return nil
	}
	delete(all, packID)
	return store.Save(extGroupsFile, &all)
}

// ImportPack 校验并导入一个扩展包文件到 packs 目录(同 id 覆盖,绑定自动跟随新内容)。
// 复用 ext.LoadFile 做 反序列化→静态校验→干跑,任一步失败不落盘(AC-4)。
// 同 id 覆盖导入时清除该包的扩展组配置(新旧布局行值不兼容,保留只会让保存失败)。
func (s *ExtService) ImportPack(path string) (PackInfo, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return PackInfo{}, fmt.Errorf("导入失败: 仅支持 .json 文件: %s", filepath.Base(path))
	}
	p, err := ext.LoadFile(path)
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: %w", err)
	}
	dir, err := packsDir()
	if err != nil {
		return PackInfo{}, err
	}
	dest := filepath.Join(dir, p.Meta.ID+".json")
	if _, err := os.Stat(dest); err == nil {
		if err := clearExtGroupsFor(p.Meta.ID); err != nil {
			return PackInfo{}, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: 读取 %s: %w", path, err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: 写入 %s: %w", dest, err)
	}
	if err := s.ReloadPacks(); err != nil {
		return PackInfo{}, err
	}
	for _, pk := range s.rt.Packs() {
		if pk.Meta.ID == p.Meta.ID {
			return packInfoOf(pk), nil
		}
	}
	return packInfoOf(p), nil
}

// DeletePack 删除指定扩展包文件并重扫。若删除的是当前绑定包,重扫后
// resolvePack 落空,激活自动解绑(SetPacks → syncExtCommands 重置注册表)。
func (s *ExtService) DeletePack(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("删除失败: 包 id 不能为空")
	}
	dir, err := packsDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, id+".json")
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return fmt.Errorf("删除失败: 扩展包不存在: %s", id)
	}
	if err := os.Remove(dest); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return s.ReloadPacks()
}
```

- [ ] **Step 1b: 补覆盖导入清配置测试**(评审 P0-2)

`bridge/ext_service_test.go` 追加(import 区补 `"gbt32960-simulator/internal/schema"` 与 `"gbt32960-simulator/internal/store"`):

```go
func TestImportPackOverwriteClearsExtGroups(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	if _, err := svc.ImportPack(writeFile(t, t.TempDir(), "vendor-pack.json", extcmdJSON)); err != nil {
		t.Fatal(err)
	}
	// 预置旧布局下已保存的扩展组配置
	all := map[string]map[string]schema.GroupConfig{
		"extcmd": {"extData09": {Enabled: true, Rows: []map[string]any{{"seq": float64(42), "volt": 3.3}}}},
	}
	if err := store.Save(extGroupsFile, &all); err != nil {
		t.Fatal(err)
	}
	// 覆盖导入同 id 新布局(仅 seq,无 volt)
	newJSON := `{
  "meta": {"id": "extcmd", "label": "扩展命令包v2", "vendor": "Test", "baseVersion": "2016"},
  "realtime": {"appendUnits": []},
  "commands": [
    {"key": "extData09", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual",
     "body": {"type": "fields", "fields": [{"key": "seq", "label": "流水号", "type": "u16"}]}}
  ]
}`
	if _, err := svc.ImportPack(writeFile(t, t.TempDir(), "vendor-pack-v2.json", newJSON)); err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["extcmd"]; ok {
		t.Fatal("覆盖导入应清除该包扩展组配置")
	}
}
```

Run: `go test ./bridge/ -run TestImportPackOverwriteClearsExtGroups -v`
Expected: PASS(覆盖导入后 extgroups.json 不再含 extcmd 键)

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -run "TestImportPack|TestDeletePack" -v && go test ./... -count=1`
Expected: PASS;既有 bridge/engine/ext/schema 测试全绿

- [ ] **Step 5: 提交**

```bash
git add bridge/ext_service.go bridge/ext_service_test.go
git commit -m "feat(ext): 扩展包后端管理——packs 目录自建、ImportPack 预校验导入、DeletePack 删除自动解绑"
```

---

### Task 2: 设置页包管理 UI 与下拉重扫

**Level:** L3
**Level Rationale:** 用户可见 UI + 两个新绑定的前后端契约 + store.packs 共享状态改造(ConnectionCard 与 PackManager 双消费)。
**Linked Acceptance Items:** PAC-1、PAC-2
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 1(ImportPack/DeletePack/PackInfo.commandCount);下游=Task 5(三态空态消费 store.packs)。**SettingsPage 现为 17 行空占位,本任务全新建造;ConnectionCard 的 ListPacks 现只在 onMounted 调一次,重扫即本任务。**
**重要性:** **P0(M1 的前端半场)**——后端能力对实施人员不可见,等于没有。

**Files:**
- Regenerate: `frontend/wailsjs/**`(**Step 1 先行**,否则 PackManager 消费的 ImportPack/DeletePack 绑定不存在)
- Modify: `frontend/src/state.ts`、`frontend/src/components/cards/ConnectionCard.vue`、`frontend/src/pages/SettingsPage.vue`
- Create: `frontend/src/components/PackManager.vue`

**Interfaces:**
- Consumes: `ExtService.ImportPack/DeletePack/ListPacks`(Task 1 再生成)、`ExtService.PickPackFile()`(执行期修订:Go 侧文件选择)、`store.config.extensionPack`、`reloadSchemaPreservingGroups`(state.ts)
- Produces: `store.packs: bridge.PackInfo[]`;`PackManager.vue`(列表/导入/删除/重扫)

- [ ] **Step 1: 再生成前端绑定**

Run: `wails generate module`
Expected: ExtService 绑定含 ImportPack/DeletePack;models.ts PackInfo 含 `commandCount?: number`

- [ ] **Step 2: state.ts 增 store.packs 并初始化**

`frontend/src/state.ts` 的 store 定义,`profiles` 之后追加:

```ts
  packs: [] as bridge.PackInfo[],
```

import 区追加(评审 P1-2:loadInitialData 需在启动链就位 packs,否则 Task 5 extHint 在 ConnectionCard 异步加载完成前误显"未找到"):

```ts
import * as ExtService from '../wailsjs/go/bridge/ExtService'
```

`loadInitialData`(约 L64 `store.config = await ConnectionService.GetConfig()` 之后)追加:

```ts
    try {
      store.packs = await ExtService.ListPacks()
    } catch {
      store.packs = []
    }
```

- [ ] **Step 3: 建 PackManager.vue**

`frontend/src/components/PackManager.vue`:

```vue
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import * as ExtService from '../../wailsjs/go/bridge/ExtService'
import { reloadSchemaPreservingGroups, store } from '../state'

const loading = ref(false)

async function refresh() {
  loading.value = true
  try {
    store.packs = await ExtService.ListPacks()
  } catch (e) {
    message.error('读取扩展包列表失败: ' + String(e))
  } finally {
    loading.value = false
  }
}

// 执行期修订(控制器裁定):前端 wailsjs runtime 无 dialog 导出,改调 Go 侧 PickPackFile。
async function importPack() {
  try {
    const path = await ExtService.PickPackFile()
    if (!path) return
    const info = await ExtService.ImportPack(path)
    message.success(`已导入「${info.label}」`)
    await refresh()
  } catch (e) {
    message.error(String(e))
  }
}

function deletePack(id: string, label: string) {
  Modal.confirm({
    title: `删除扩展包「${label}」?`,
    content: '删除后无法恢复;若当前档案绑定该包,绑定将自动解除。',
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    async onOk() {
      try {
        await ExtService.DeletePack(id)
        message.success('已删除')
        if (store.config?.extensionPack === id) {
          await reloadSchemaPreservingGroups(store.config?.version ?? '2016')
        }
        await refresh()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}

const boundId = computed(() => store.config?.extensionPack)

onMounted(refresh)
</script>

<template>
  <div class="pack-manager">
    <div class="pack-actions">
      <a-button size="small" type="primary" @click="importPack">导入扩展包</a-button>
      <a-button size="small" @click="refresh" :loading="loading">重新扫描</a-button>
    </div>
    <a-empty v-if="store.packs.length === 0" description="暂无扩展包。导入一个 JSON 扩展包即可在连接档案中绑定。" />
    <a-table
      v-else
      :data-source="store.packs"
      :pagination="false"
      size="small"
      row-key="id"
    >
      <a-table-column title="名称" data-index="label" />
      <a-table-column title="ID" data-index="id" />
      <a-table-column title="厂商" data-index="vendor" />
      <a-table-column title="版本" data-index="baseVersion" width="70" />
      <a-table-column title="实时单元" data-index="unitCount" width="80" />
      <a-table-column title="命令" data-index="commandCount" width="70" />
      <a-table-column title="状态" width="90">
        <template #default="{ record }">
          <a-tag v-if="record.id === boundId" color="green">已绑定</a-tag>
          <span v-else class="dim">未绑定</span>
        </template>
      </a-table-column>
      <a-table-column title="操作" width="80">
        <template #default="{ record }">
          <a-button size="small" type="text" danger @click="deletePack(record.id, record.label)">删除</a-button>
        </template>
      </a-table-column>
    </a-table>
  </div>
</template>

<style scoped>
.pack-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.dim {
  color: var(--text-secondary, #999);
}
</style>
```

- [ ] **Step 4: SettingsPage 挂载**

`frontend/src/pages/SettingsPage.vue` 全文替换为:

```vue
<script setup lang="ts">
import PackManager from '../components/PackManager.vue'
</script>

<template>
  <div class="page">
    <div class="section-title">扩展包管理</div>
    <p class="section-hint">导入 JSON 扩展包(实时私有数据单元 / 私有命令字),在连接配置中绑定后即可使用。</p>
    <PackManager />
  </div>
</template>

<style scoped>
.page {
  padding: 16px;
}
.section-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 4px;
}
.section-hint {
  color: #888;
  font-size: 12px;
  margin-bottom: 12px;
}
</style>
```

- [ ] **Step 5: ConnectionCard 抽取 refreshPacks 并挂下拉重扫**

`frontend/src/components/cards/ConnectionCard.vue` script 区替换为:

```ts
import { computed, onMounted } from 'vue'
import CollapsibleCard from '../layout/CollapsibleCard.vue'
import { cfg } from '../../composables/useConnConfig'
import { formRef, rules, saveOnly, onVersionChange } from '../../composables/useConnActions'
import * as ExtService from '../../../wailsjs/go/bridge/ExtService'
import { store } from '../../state'

async function refreshPacks() {
  try {
    store.packs = await ExtService.ListPacks()
  } catch {
    store.packs = []
  }
}

const packOptions = computed(() =>
  store.packs.map((p) => ({ value: p.id, label: `${p.label} (${p.baseVersion})` })),
)

onMounted(refreshPacks)
```

模板第 44-46 行的扩展包下拉改为:

```html
      <a-form-item label="扩展包" name="extensionPack">
        <a-select
          v-model:value="cfg.extensionPack"
          :options="packOptions"
          allow-clear
          placeholder="不使用扩展包"
          @dropdown-visible-change="(open: boolean) => open && refreshPacks()"
        />
      </a-form-item>
```

- [ ] **Step 6: 构建验证**

Run: `cd frontend && npm run build`
Expected: vue-tsc 零错误 + vite 构建成功

- [ ] **Step 7: 提交**

```bash
git add frontend/wailsjs frontend/src/state.ts frontend/src/components/PackManager.vue frontend/src/pages/SettingsPage.vue frontend/src/components/cards/ConnectionCard.vue bridge/ext_service.go app.go
git commit -m "feat(ext): 设置页包管理 UI(导入/删除/重扫)与连接卡下拉实时刷新"
```

---

### Task 3: SaveGroups 扩展行干跑校验

**Level:** L2
**Level Rationale:** 局部行为变更(SaveGroups 增一道校验),失败语义=不落盘,不改变成功路径的字节产出。
**Linked Acceptance Items:** PAC-4
**Task Gate:** task reviewer + focused checks
**关联性:** 上游=P1 `ext.EncodeUnit/EncodeFields`、P3 assembleCommandBody(跳过语义对齐);下游=Task 4(与按包存储同文件,顺序在前)。**跳过语义与组装一致:未配置/未启用组不校验,保存只拦截"将被发送的东西"。**
**重要性:** **P1(M2 半场)**——非法值目前到发送时才会炸,且错误在事件总线里而非保存弹窗;干跑校验把 AC-4 的"值级错误"提前到保存点。

**Files:**
- Modify: `bridge/message_service.go`(SaveGroups 插入 + validateExtRows)
- Test: `bridge/message_service_ext_test.go`(追加)

**Interfaces:**
- Consumes: `ext.EncodeUnit(u AppendUnit, row schema.RowValue) ([]byte, error)`、`ext.EncodeFields(fields []FieldSpec, row schema.RowValue) ([]byte, error)`、`s.rt.Pack()`
- Produces: `func (s *MessageService) validateExtRows(groups map[string]schema.GroupConfig) error`(SaveGroups 在 Assemble 之后、SetGroups 之前调用)

- [ ] **Step 1: 写失败测试**

`bridge/message_service_ext_test.go` 追加(import 区已有 `strings`/`schema`/`time`;若缺 `strings` 则补):

```go
func TestSaveGroupsExtDryRunRejects(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 隔离真实用户 message.json
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 70000, "volt": 3.3}}}
	err := ms.SaveGroups(*schema.FromMap(m, nil))
	if err == nil || !strings.Contains(err.Error(), "扩展命令 extData09") {
		t.Fatalf("超范围值应报扩展命令错误, got %v", err)
	}
	if rt.Groups() != nil {
		t.Fatal("失败不得写入内存快照")
	}
}

func TestSaveGroupsExtDryRunPasses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 1, "volt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestSaveGroupsExtDryRun" -v`
Expected: FAIL(`SaveGroups` 未拦截 70000,err 为 nil)

- [ ] **Step 3: 实现**

`bridge/message_service.go` 的 SaveGroups(现 L190-198)替换为:

```go
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
```

同文件追加:

```go
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
```

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -run "TestSaveGroupsExtDryRun" -v && go test ./... -count=1`
Expected: PASS;P2/P3 既有测试全绿(validateExtRows 对合法值零拦截)

- [ ] **Step 5: 提交**

```bash
git add bridge/message_service.go bridge/message_service_ext_test.go
git commit -m "feat(ext): SaveGroups 扩展行字段级干跑校验,非法值保存不落盘"
```

---

### Task 4: 扩展组配置按包独立存储(解绑不抹)

**Level:** L3
**Level Rationale:** 持久化模型变更(SaveGroups/GetGroups/loadGroups 三处 + Runtime 快照失效语义),改变 AC-5 的行为语义,跨 store/schema 边界。
**Linked Acceptance Items:** PAC-3
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 3(同文件,validateExtRows 先行);下游=无(前端零改动,存储透明)。**设计决策:单文件 `extgroups.json` = map[packID]map[groupKey]GroupConfig;换绑/换版本时 Runtime 清内存组快照,GetGroups 回读磁盘合并——前端流程零改动。**
**重要性:** **P0(M2 半场)**——"解绑后在标准 tab 保存会把扩展组配置抹掉"是 P3 文档化的已知限制,本任务把它消解为"配置随包保存、随绑定生命周期恢复"。

**Files:**
- Modify: `bridge/runtime.go`(SetConnCfg 换绑/换版本清快照)
- Modify: `bridge/message_service.go`(extGroupsFile/extKeys/saveExtGroups/loadAllGroups;SaveGroups/GetGroups/NewMessageService 接线)
- Test: `bridge/message_service_ext_test.go`(追加)

**Interfaces:**
- Consumes: `store.Load/Save`、`s.rt.Pack()/Groups()/SetGroups()`、Task 3 的 SaveGroups 结构
- Produces: `extGroupsFile = "extgroups.json"`、`extKeys(p) map[string]bool`、`saveExtGroups(groups) error`、`loadAllGroups() map[string]schema.GroupConfig`(两文件都无数据返回 nil,保 DefaultGroups 兜底)

- [ ] **Step 1: 写失败测试**

`bridge/message_service_ext_test.go` 追加:

```go
func TestExtGroupsPersistAcrossUnbind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 42, "volt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}
	// 解绑:前端此时只提交标准组载荷
	rt.SetConnCfg(&ConnectionConfig{Version: "2016"})
	std := schema.FromMap(map[string]schema.GroupConfig{
		"vehicle": {Enabled: true, Rows: m["vehicle"].Rows},
	}, []string{"vehicle"})
	if err := ms.SaveGroups(*std); err != nil {
		t.Fatal(err)
	}
	// 重绑:命令组配置应从 extgroups.json 恢复,而非默认值
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	got, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	gm := got.ToMap()
	g, ok := gm["extData09"]
	if !ok || g.Rows[0]["seq"] != float64(42) {
		t.Fatalf("重绑后命令组配置应恢复, got %+v", gm["extData09"])
	}
	if v, ok := gm["vehicle"]; !ok || !v.Enabled {
		t.Fatal("标准组配置应保持")
	}
}
```

- [ ] **Step 1b: 补标准组保存不覆盖命令组的测试**(评审 P0-1,模拟实时面板保存)

`bridge/message_service_ext_test.go` 追加:

```go
func TestExtGroupsSurviveStandardOnlySave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 42, "volt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}
	// 实时面板保存:order 仅标准组键,payload 不含命令组——不得覆盖命令组配置
	std := schema.FromMap(map[string]schema.GroupConfig{
		"vehicle": {Enabled: true, Rows: m["vehicle"].Rows},
	}, []string{"vehicle"})
	if err := ms.SaveGroups(*std); err != nil {
		t.Fatal(err)
	}
	got, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	gm := got.ToMap()
	if g, ok := gm["extData09"]; !ok || g.Rows[0]["seq"] != float64(42) {
		t.Fatalf("标准组保存不得覆盖命令组配置, got %+v", gm["extData09"])
	}
}
```

Run: `go test ./bridge/ -run TestExtGroupsSurviveStandardOnlySave -v`
Expected: PASS(合并写入保留 extData09)

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./bridge/ -run "TestExtGroups" -v`
Expected: FAIL(重绑后 gm["extData09"] 缺失——现行为即"解绑抹配置")

- [ ] **Step 3: 实现**

`bridge/runtime.go` 的 SetConnCfg 替换为:

```go
// SetConnCfg 保存连接配置快照,并按其 ExtensionPack 解析激活扩展包。
// 换绑/换版本时内存组快照失效:后续 GetGroups 回读磁盘并按包合并扩展组配置。
func (rt *Runtime) SetConnCfg(cfg *ConnectionConfig) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	prev := rt.connCfg
	if cfg == nil { // 执行期修订:既有测试传 nil,防 cfg.Version 解引用 panic
		rt.connCfg = nil
		rt.pack = nil
		rt.syncExtCommands(nil)
		return
	}
	rt.connCfg = cfg
	rt.pack = resolvePack(rt.packs, cfg)
	rt.syncExtCommands(rt.pack)
	if prev == nil || prev.Version != cfg.Version || prev.ExtensionPack != cfg.ExtensionPack {
		rt.groups = nil
	}
}
```

`bridge/message_service.go`:

(注意:`extGroupsFile` 常量已由 Task 1 定义在 ext_service.go,**本任务不重复定义**。)

SaveGroups 替换为(在 Task 3 基础上追加 saveExtGroups):

```go
// SaveGroups 校验(标准组装 + 扩展行干跑)、持久化并快照报文配置。
func (s *MessageService) SaveGroups(payload schema.GroupsPayload) error {
	groups := payload.ToMap()
	if _, err := schema.Assemble(s.version(), groups, time.Now()); err != nil {
		return err
	}
	if err := s.validateExtRows(groups); err != nil {
		return err
	}
	if err := s.saveExtGroups(groups); err != nil {
		return err
	}
	s.rt.SetGroups(groups)
	return store.Save(groupsFile, &groups)
}
```

loadGroups 替换为:

```go
// loadAllGroups 读标准组(message.json)并叠加激活包的扩展组配置(extgroups.json)。
// 两个文件都无数据时返回 nil,触发 GetGroups 的默认值兜底。
func (s *MessageService) loadAllGroups() map[string]schema.GroupConfig {
	var g map[string]schema.GroupConfig
	if err := store.Load(groupsFile, &g); err != nil || g == nil {
		g = map[string]schema.GroupConfig{}
	}
	if p := s.rt.Pack(); p != nil {
		var all map[string]map[string]schema.GroupConfig
		if err := store.Load(extGroupsFile, &all); err == nil && all != nil {
			for k, v := range all[p.Meta.ID] {
				g[k] = v
			}
		}
	}
	if len(g) == 0 {
		return nil
	}
	return g
}
```

文件内所有 `s.loadGroups()` 调用点(GetGroups、NewMessageService)改为 `s.loadAllGroups()`。

**执行期修订(实现者发现 Step 1b 与原实现矛盾,控制器已裁定)**:GetGroups 在内存快照非 nil 时也须叠加磁盘扩展组(标准组保存会把快照写为仅标准组,之后 GetGroups 若不叠加,Step 1b 场景命令组消失)——新增只读合并:快照非 nil 时把 `loadExtGroupsOf(packID)` 的键叠加进 src(内存键优先,不回写 rt.groups,保持 GetGroups 无副作用)。

同文件追加:

```go
// extKeys 激活包的扩展组键集合(实时追加单元 + 命令组),与 GetSchema 的键命名一致。
func (s *MessageService) extKeys(p *ext.Pack) map[string]bool {
	if p == nil {
		return nil
	}
	out := map[string]bool{}
	for _, u := range p.Realtime.AppendUnits {
		out[u.Key] = true
	}
	for _, c := range p.Commands {
		switch c.Body.Type {
		case "fields":
			out[c.Key] = true
		case "realtimeLike":
			for _, u := range c.Body.Units {
				out[c.Key+":"+u.Key] = true
			}
		}
	}
	return out
}

// saveExtGroups 把激活包的扩展组配置按包独立存储。
// 未绑包时直接返回(解绑后的保存不触碰扩展组配置);绑包时以包 id 为键
// **逐键合并**(评审 P0-1:禁止整体覆盖——实时面板保存的 payload 只含标准组与
// 追加单元、不含命令组,整体覆盖会抹掉命令组配置),part 为空则不写。
func (s *MessageService) saveExtGroups(groups map[string]schema.GroupConfig) error {
	keys := s.extKeys(s.rt.Pack())
	if len(keys) == 0 {
		return nil
	}
	var all map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &all); err != nil || all == nil {
		all = map[string]map[string]schema.GroupConfig{}
	}
	part := map[string]schema.GroupConfig{}
	for k, g := range groups {
		if keys[k] {
			part[k] = g
		}
	}
	if len(part) == 0 {
		return nil
	}
	if all[s.rt.Pack().Meta.ID] == nil {
		all[s.rt.Pack().Meta.ID] = map[string]schema.GroupConfig{}
	}
	for k, v := range part {
		all[s.rt.Pack().Meta.ID][k] = v
	}
	return store.Save(extGroupsFile, &all)
}
```

(import 区确认:`ext`、`store`、`schema`、`fmt`、`time` 均已存在,无新增。)

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./bridge/ -run "TestExtGroups" -v && go test ./... -count=1`
Expected: PASS;既有 GetGroups/DefaultGroups/PAC 测试全绿(未绑包/首启行为不变:loadAllGroups 空返回 nil → DefaultGroups 兜底)。**若既有测试因 SetConnCfg 清快照失败,核对测试是否依赖"同 cfg 二次 SetConnCfg 保留快照"——仅在版本或绑包变化时清空,同 cfg 不清,既有用法不受影响;确需调整的既有测试属计划外变更,最小修复并在报告中申报。** **评审 P0 强调:`prev == nil` 也清快照是启动链正确性的必要条件**(app.go:28 构造顺序 NewMessageService 先于 SetConnCfg,构造时 Pack() 为 nil 快照缺扩展合并,靠启动时 SetConnCfg 的 prev==nil 清空强制回读)——实现时不可"优化"掉。

- [ ] **Step 5: 提交**

```bash
git add bridge/runtime.go bridge/message_service.go bridge/message_service_ext_test.go
git commit -m "feat(ext): 扩展组配置按包独立存储,解绑/换绑后配置不丢(AC-5 强化)"
```

---

### Task 5: 扩展命令区打磨(版本可见性 + 多行表单 + enabled 对齐)

**Level:** L3
**Level Rationale:** 三处用户可见行为变更(空态文案、tab 禁用条件、多行表单)跨 ExtensionCommands/RealTimePanel/state 三个前端文件。
**Linked Acceptance Items:** PAC-5、PAC-6
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=Task 2(store.packs);下游=Task 6 验收(PAC-6 用手动步骤验证)。**三处小改同属"命令区无静默"主题,合并为单任务;多行表单复用 RealTimePanel 的 addRow/removeRow 模式,fields 体 g.multiple 恒 false 天然单行。**
**重要性:** **P1(M3)**——消化 oracle 业务审核的 D(版本静默)、F(多行契约不可达)、Watch-out#3(enabled 硬编码)。

**Files:**
- Modify: `frontend/src/components/ExtensionCommands.vue`(空态三态/ensureGroup 对齐/多行模板)
- Modify: `frontend/src/components/RealTimePanel.vue`(tab 禁用条件)

**Interfaces:**
- Consumes: `store.packs/ store.config/ store.extSchema/ store.groups`(Task 2)、useFieldHelpers、`GroupSchema.multiple/maxRows`
- Produces: 三态空态文案;multiple 单元加行/删行;tab 禁用仅由"未绑包"决定

- [ ] **Step 1: ExtensionCommands script 区修改**

import 区(第 1-10 行)替换为:

```ts
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { GroupSchema } from '../api/backend'
import { stateToPayload } from '../api/backend'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import { store } from '../state'
import {
  bitOptions, bitsArrayOf, defaultFor, hexOf, numOf, setBitsArray, setEnum, setHex, setNum,
} from '../composables/useFieldHelpers'
```

ensureGroup 替换为(enabled 对齐 g.enabled,与 RealTimePanel 同款):

```ts
function ensureGroup(g: GroupSchema) {
  if (!store.groups[g.key]) {
    const row: Record<string, unknown> = {}
    for (const f of g.fields) row[f.key] = defaultFor(f)
    store.groups[g.key] = { enabled: g.enabled, rows: [row] }
  }
  return store.groups[g.key]
}
```

ensureGroup 之后追加多行助手与三态文案:

```ts
function addRow(g: GroupSchema) {
  const grp = ensureGroup(g)
  if (!g.multiple || (g.maxRows && grp.rows.length >= g.maxRows)) return
  const row: Record<string, unknown> = {}
  for (const f of g.fields) row[f.key] = defaultFor(f)
  grp.rows.push(row)
}

function removeRow(g: GroupSchema, idx: number) {
  const grp = store.groups[g.key]
  if (!grp || (g.multiple && grp.rows.length <= 1)) return
  grp.rows.splice(idx, 1)
}

// extHint 空态三态文案:未绑定 / 版本不匹配 / 未声明 commands(消化 oracle 审核 D 的静默问题)。
const extHint = computed(() => {
  const packId = store.config?.extensionPack
  if (!packId) return '绑定含 commands 的扩展包后,在此配置与发送私有命令'
  const info = store.packs.find((p) => p.id === packId)
  if (!info) return `扩展包「${packId}」未找到,请在设置页重新导入`
  if (info.baseVersion !== (store.config?.version ?? '2016')) {
    return `扩展包「${info.label}」基准版本 ${info.baseVersion} 与当前档案版本 ${store.config?.version} 不匹配,请调整档案版本或换绑其他包`
  }
  return `扩展包「${info.label}」未声明 commands 段,无可配置的私有命令`
})
```

- [ ] **Step 2: ExtensionCommands 模板区修改**

空态(第 73 行)改为:

```html
    <a-empty v-if="store.extSchema.length === 0" :description="extHint" />
```

字段区(第 83-128 行的 group-row 块)整体替换为多行渲染(每行独立字段组,multiple 显示行号与删除):

```html
          <div v-for="(row, ri) in ensureGroup(g).rows" :key="ri" class="group-row">
            <div class="row-head">
              <span v-if="g.multiple" class="row-label">第 {{ ri + 1 }} 行</span>
              <a-button v-if="g.multiple && ensureGroup(g).rows.length > 1" size="small" type="text" danger @click="removeRow(g, ri)">删除行</a-button>
            </div>
            <div class="fields-grid">
              <template v-for="f in g.fields" :key="f.key">
                <div v-if="f.kind === 'enum'" class="field">
                  <span class="field-label">{{ f.label }}</span>
                  <a-select
                    :value="numOf(row, f.key)"
                    size="small"
                    :options="f.enum?.map((e) => ({ value: e.value, label: e.label })) ?? []"
                    @change="(v: unknown) => setEnum(row, f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'int' || f.kind === 'float'" class="field">
                  <span class="field-label">{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span>
                  <a-input-number
                    :value="numOf(row, f.key)"
                    size="small"
                    :step="f.kind === 'int' ? 1 : 0.1"
                    :min="f.min"
                    :max="f.max"
                    style="width: 100%"
                    @change="(v: number | string | null | undefined) => setNum(row, f.key, v)"
                  />
                </div>
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
                <div v-else-if="f.kind === 'bitgroup'" class="field field-bits">
                  <span class="field-label">{{ f.label }}</span>
                  <a-checkbox-group
                    :value="bitsArrayOf(row, f)"
                    :options="bitOptions(f)"
                    class="bits-group"
                    @change="(vals: Array<string | number | boolean>) => setBitsArray(row, f, vals)"
                  />
                </div>
              </template>
            </div>
          </div>
          <a-button v-if="g.multiple" size="small" type="dashed" block @click="addRow(g)">＋ 添加一行</a-button>
```

style 区追加:

```css
.row-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
}
.row-label {
  font-size: 12px;
  color: #888;
}
```

- [ ] **Step 3: RealTimePanel tab 禁用条件**

模板中 `<a-tab-pane key="custom" tab="自定义数据" :disabled="store.extSchema.length === 0">` 改为:

```html
      <a-tab-pane key="custom" tab="自定义数据" :disabled="!store.config?.extensionPack">
        <ExtensionCommands />
      </a-tab-pane>
```

(未绑包仍禁用=现状;绑包后即使版本不匹配/无 commands 也可点开看到三态文案——PAC-5。)

- [ ] **Step 4: 构建验证**

Run: `cd frontend && npm run build`
Expected: vue-tsc 零错误 + vite 构建成功

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/ExtensionCommands.vue frontend/src/components/RealTimePanel.vue
git commit -m "feat(ext): 命令区三态空态可见性与 multiple 多行表单,ensureGroup 对齐组定义 enabled"
```

---

### Task 6: 《插件包编写指南》+ 内置示例包 + 契约回写

**Level:** L1
**Level Rationale:** 无代码行为变更;仅文档 + 一个"示例包合法性"回归测试(纯读取断言)。
**Linked Acceptance Items:** PAC-7
**Task Gate:** task reviewer + linked AC
**关联性:** 上游=全部任务(指南覆盖 P1~P4 能力);下游=Master 收尾。**示例包必须经 ext.LoadFile 全过——用一个单测把它锁死,防止指南与校验器漂移。**
**重要性:** **P1(AC-6 唯一载体)**——没有指南与示例,实施人员无从下手,整个扩展机制对目标用户不可交付。

**Files:**
- Create: `docs/extpack-guide.md`、`docs/extpack/sample-pack.json`、`internal/ext/sample_test.go`
- Modify: `docs/superpowers/specs/2026-08-31-extpack-design.md`(§4.3/§4.4 契约回写)

**Interfaces:**
- Consumes: 设计契约 §4(全部已落地能力)
- Produces: 指南 + 示例包;`TestDocsSamplePackValid`

- [ ] **Step 1: 建示例包**

`docs/extpack/sample-pack.json`(内含 appendUnits 一个 + commands 两个,realtimeLike 单元带 multiple):

```json
{
  "meta": { "id": "sample-private", "label": "示例·私有遥测包", "vendor": "示例", "baseVersion": "2016" },
  "realtime": {
    "appendUnits": [
      {
        "key": "customTelemetry", "title": "私有遥测单元", "unitCode": 128, "enabled": false, "multiple": false, "maxRows": 10,
        "fields": [
          { "key": "soc2", "label": "SOC2", "type": "u8", "unit": "%" },
          { "key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V" }
        ]
      }
    ]
  },
  "commands": [
    {
      "key": "extData09", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual+periodic",
      "body": { "type": "fields", "fields": [
        { "key": "seq", "label": "流水号", "type": "u16" },
        { "key": "volt", "label": "电压", "type": "u16", "scale": 0.1, "unit": "V" }
      ] }
    },
    {
      "key": "extReport0A", "label": "扩展报表", "code": 10, "direction": "up", "trigger": "manual",
      "body": { "type": "realtimeLike", "units": [{
        "key": "telemetry", "title": "私有遥测", "unitCode": 129, "enabled": true, "multiple": true, "maxRows": 5,
        "fields": [
          { "key": "soc", "label": "SOC", "type": "u8", "unit": "%" },
          { "key": "temp", "label": "温度", "type": "i8", "offset": 40, "unit": "°C" }
        ]
      }] }
    }
  ]
}
```

- [ ] **Step 2: 示例包合法性回归锁**

`internal/ext/sample_test.go`:

```go
package ext

import (
	"path/filepath"
	"testing"
)

// TestDocsSamplePackValid 锁死内置示例包与校验器的一致性:指南示例永不过期。
func TestDocsSamplePackValid(t *testing.T) {
	p, err := LoadFile(filepath.Join("..", "..", "docs", "extpack", "sample-pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.ID != "sample-private" || len(p.Commands) != 2 || len(p.Realtime.AppendUnits) != 1 {
		t.Fatalf("示例包结构 = %+v", p.Meta)
	}
}
```

Run: `go test ./internal/ext/ -run TestDocsSamplePackValid -v`
Expected: PASS(示例包一次写对;若失败按错误路径修正示例包,不改校验器)

- [ ] **Step 3: 写《插件包编写指南》**

`docs/extpack-guide.md`,内容要点(全文写入,不少于下列各节):
- 概览:什么是扩展包(一个 JSON = 实时追加单元 + 私有命令字);目标读者(实施人员)
- 快速开始:复制 sample-pack.json → 设置页导入 → 连接档案绑定 → 实时数据/自定义数据 tab 填表 → 发送,控制台 TX hex 验证
- 包格式:meta(id 规则/label/vendor/baseVersion)、realtime.appendUnits(key/title/unitCode 0x80~0xFE/enabled/multiple/maxRows/fields)、commands(key/label/code/direction=up/trigger/body.type=fields|realtimeLike)
- 字段 DSL 类型表:u8/u16/u32、i8/i16/i32、f32、bits、bytes;scale/offset 数值变换契约(物理值=线值×scale+offset,编码非整数报错);scale≤0 非法
- 码位规则(§4.4 表):unitCode 标准占用拒绝、私有可用 0x80~0xFE;命令码 2016=0x09~0x7F / 2025=0x0C~0x7F;同包 unitCode/key/命令码唯一;key 禁冒号、禁与标准组键同名(vehicle/motor/fuelcell/engine/location/extremum/alarm/voltage/temperature/minparallel/batterytemp/fcstack/supercap/supercapextremum)
- 错误信息解读:所有校验错误带 JSON 路径(如 `realtime.appendUnits[0].fields[3].scale`),按路径定位修改
- 已知限制:重启后周期开关需重新开启(会话级);解绑后扩展组配置随包保存、重绑恢复;覆盖导入同 id 会清除该包已保存的扩展组配置(新旧布局行值不兼容);**切换协议版本后扩展组配置重置为默认值**(前端版本切换走默认值重载,评审 P1-3 文档化口径);标准组与扩展组分存两个文件,极端崩溃下可能各停在最近成功版本(下次保存收敛);fields 体命令单行、realtimeLike 单元 multiple 时多行(每行一个 TLV)
- 常见问题(FAQ):导入无反应(检查路径/非法字段)、tab 灰色(未绑定)、tab 点开提示版本不匹配(改档案版本或换包)

- [ ] **Step 4: 设计契约回写**

`docs/superpowers/specs/2026-08-31-extpack-design.md`:

§4.4 末尾追加一段:

```markdown
- **执行期补强(2026-09-01,修复批 A+B)**:命令码同包唯一(校验器强制);命令/单元 key 禁含冒号(与命令组键命名空间冲突);命令/单元 key 不得与标准报文组键同名(保留表:vehicle/motor/fuelcell/engine/location/extremum/alarm/voltage/temperature/minparallel/batterytemp/fcstack/supercap/supercapextremum)
```

§4.3 的 `multiple: true` 条目下追加一句:

```markdown
  - 命令体 realtimeLike 单元同样支持 multiple(每行独立 TLV);P4 起前端命令表单提供加行/删行(此前仅单行)
```

- [ ] **Step 5: 提交**

```bash
git add docs/extpack-guide.md docs/extpack/sample-pack.json internal/ext/sample_test.go docs/superpowers/specs/2026-08-31-extpack-design.md
git commit -m "docs(ext): 《插件包编写指南》与内置示例包,设计契约回写命令码唯一与多行支持"
```

---

## 任务依赖图

```
Task 1(后端包管理)─▶ Task 2(设置页 UI + store.packs)─┐
                                                       ├─▶ Task 5(命令区打磨)─▶ Task 6(指南示例)
Task 3(SaveGroups 干跑)─▶ Task 4(按包独立存储)────────┤          ▲
                                                       └──────────┘
                                          Task 6 指南"已知限制"依赖 Task 4 的存储语义(评审 P1-4)
```

关键耦合点(评审重点):Task 1 的 PackInfo.commandCount 经 Task 2 的 wails 再生成落地,Task 5 消费 store.packs 时依赖该字段与 id 字段;Task 1 定义 `extGroupsFile` 常量与 `clearExtGroupsFor`(覆盖导入清配置),Task 4 复用且**不重复定义**;Task 3 与 Task 4 同改 SaveGroups(顺序执行,先干跑后存储);Task 4 的 extKeys 键规则与 GetSchema/compileCommandGroups 一致(fields=`key`、realtimeLike=`key:unitkey`);Task 4 的 SetConnCfg 清快照仅当版本或绑包变化(同 cfg 不清,保既有测试;prev==nil 清空为启动链必要条件,勿删)。

## Self-Review 记录

1. **Spec 覆盖**:AC-4 的 UI 面 = Task 1/2(PAC-1/2);AC-5 强化 = Task 3/4(PAC-3/4);oracle D/F/Watch-out#3 = Task 5(PAC-5/6);AC-6 = Task 6(PAC-7)。设计 §5 范围边界内,无越界(下行命令/私有加密/解析页解码均不在本阶段)。
2. **占位符扫描**:无 TBD;所有代码步骤含完整代码。Task 6 指南为内容要点式(文档任务,由实现者成文,要点即验收清单)。
3. **类型一致性**:`ImportPack(path string) (PackInfo, error)` Task 1 定义、Task 2 前端消费(wails 再生成);`validateExtRows` Task 3 定义、Task 4 沿用;`extGroupsFile/extKeys/saveExtGroups/loadAllGroups` Task 4 定义、内部自洽;`store.packs: bridge.PackInfo[]` Task 2 定义、Task 5 消费;`addRow/removeRow` Task 5 定义、模板消费。
4. **拆分决策**:P4 独立可验收(M1~M4),符合 Master 分解;Task 3/4 因同文件顺序串行,Task 1/2 与 3/4 可并行(执行时按序即可)。
5. **Level/Gate 完整**:2 个 L2 + 3 个 L3 + 1 个 L1,均带 rationale 与 gate。
6. **L3 AC 绑定**:每个 L3 任务 Linked AC 非空且存在于 PAC 清单。
7. **AC 覆盖**:PAC-1/2←Task 1/2;PAC-3←Task 4;PAC-4←Task 3;PAC-5/6←Task 5;PAC-7←Task 6。
8. **AC 可执行**:每条 PAC 带测试名/手动步骤。
9. **已知执行期笔误风险**:Go 代码经逐字推演可编译(ExtService 现有结构 + 签名核对过);Vue 模板经 vue-tsc 语义核对;风险点 Task 4 的既有测试兼容性已在 Step 4 给出口径(同 cfg 不清快照)。
10. **既有债务消化核对**:packs 目录自建(Task 1)、ListPacks 下拉重扫(Task 2)、SaveGroups 扩展行干跑(Task 3)、扩展包 key 与标准键黑名单(已由 P3 修复批 B 完成,本计划不重复)、按包独立存储(Task 4)、版本可见性/多行/enabled 对齐(Task 5)、指南+示例(Task 6)。
11. **Oracle 执行前评审修订(2026-09-01)**:已落实 P0-1(saveExtGroups 改逐键合并 + part 空不写,并补 TestExtGroupsSurviveStandardOnlySave)、P0-2(ImportPack 同 id 覆盖时 clearExtGroupsFor 清除该包扩展组配置,并补测试)、P1-1(P3 终审 Watch-out×3 处置: #1 key 字符集未校验→修复批 6bd25ea、#2 SetExtAutoReport 无版本门禁→修复批 4f84413、#3 ensureGroup 硬编码 enabled→本计划 Task 5)、P1-2(store.packs 初始化进 loadInitialData)、P1-3(版本切换重置扩展组配置为文档化口径,写入指南已知限制)、P1-4(依赖图补 Task 4→Task 6 边)。可选 P2(导入成功但 ReloadPacks 报错、删除后 cfg 残留悬空 id、extgroups 删除包条目未清)不阻塞,留待执行期按实际需要处理。
