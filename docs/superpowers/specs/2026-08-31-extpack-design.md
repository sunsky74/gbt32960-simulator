# GB/T 32960 模拟器 · 插件包扩展机制 设计文档(Master Design)

> 状态:已确认(设计讨论结论固化)
> 日期:2026-08-31
> 关联计划:`docs/superpowers/plans/2026-08-31-extpack-master-plan.md`
> 说明:设计决策来自 2026-08-31 的架构调研(Java project4j 基线扩展机制对照 + 本仓库现状审计),方向已经用户确认("方向没问题")。

---

## 1. Final Intent(最终意图)

实施人员通过在 APP 中导入一个 JSON 扩展包(零代码、零重编译),即可让模拟器:

1. 在 0x02 实时报文尾部**追加私有数据单元**(字段级声明,自动表单 + 自动编码)
2. 发送**私有命令帧**(如上行预留区 0x09~0x7F 的扩展数据)

开发者保留 Go 接口作为复杂场景(私有加密/条件时序)的逃生口(本期不实现)。

## 2. 三支柱与全局验收标准(Global AC Draft)

| AC | 内容 | 来源 |
|---|---|---|
| AC-1 | 导入含 `realtime.appendUnits` 的包并绑定 Profile 后,实时面板出现私有组表单;发送 0x02 后帧尾含正确 TLV,hex 预览与 golden 一致 | 业务主流程 |
| AC-2 | 导入含 `commands` 的包并绑定后,扩展命令区出现表单;手动发送产生正确命令帧;周期开关按间隔发送 | 业务主流程 |
| AC-3 | **未绑定扩展包时,现有行为逐字节不变**(现有 golden/单测全绿,UI 无变化) | 当前需求 |
| AC-4 | 导入非法包(语法/结构/语义/干跑任一失败)得到带 JSON 路径的错误,且不落盘 | 当前需求 |
| AC-5 | 换绑/解绑扩展包通过切换连接档案生效;报文配置持久化不丢失标准组 | 当前需求 |
| AC-6 | 内置示例包 + 《插件包编写指南》;实施人员按指南可在短时间内完成第一个自定义字段扩展 | 交付 |

## 3. 端到端业务流

```
实施人员                              模拟器                                平台/网关
   │  拿到厂商协议文档                   │                                     │
   │  填写/修改 extension.json          │                                     │
   │────────── 导入(设置页) ──────────▶│ 加载:反序列化→静态校验→干跑           │
   │                                   │ 通过 → 存用户数据目录 packs/          │
   │  连接档案绑定扩展包                 │                                     │
   │────────── 绑定 Profile ──────────▶│ GetSchema = 标准组 + 私有组           │
   │                                   │ 前端自动渲染私有字段表单               │
   │  填值,点发送                       │                                     │
   │────────── 发送 ──────────────────▶│ 标准体(typed) + 私有TLV(DSL) ──0x02──▶│
   │                                   │ 或 私有命令帧(rawBody) ──0x09───────▶│
```

## 4. 技术契约(跨 Phase 共享,实现不得偏离)

### 4.1 插件包格式(单 JSON 文件)

```jsonc
{
  "meta": { "id": "private-telemetry", "label": "私有远控 私有遥测", "vendor": "私有远控", "baseVersion": "2016" },
  "realtime": {
    "appendUnits": [
      {
        "key": "customTelemetry", "title": "私有遥测单元",
        "unitCode": 128, "enabled": false, "multiple": false, "maxRows": 10,
        "fields": [
          { "key": "soc2", "label": "SOC2", "type": "u8", "unit": "%" },
          { "key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V" }
        ]
      }
    ]
  },
  "commands": [
    {
      "key": "extData09", "label": "扩展数据上报", "code": 9,
      "direction": "up", "trigger": "manual+periodic",
      "body": { "type": "fields", "fields": [ /* 同上字段 DSL */ ] }
    },
    {
      "key": "extReport0A", "label": "扩展报表", "code": 10,
      "direction": "up", "trigger": "manual",
      "body": { "type": "realtimeLike", "units": [ /* 与 appendUnits 同构 */ ] }
    }
  ]
}
```

注:`code` / `unitCode` 均为 JSON 数字(Go 侧 `int`)。

### 4.2 字段 DSL 类型集(最小集,冻结)

| type | 线格式 | 编译为 FieldSchema.Kind | 说明 |
|---|---|---|---|
| u8/u16/u32 | 无符号大端 | `int`(scale=1)/ `float`(scale≠1,遵循仓库 ×0.1→float 约定);Min/Max=物理量程 | |
| i8/i16/i32 | 有符号补码大端 | `int`(scale=1)/ `float`(scale≠1) | |
| f32 | IEEE754 大端 | `float` | 不允许 scale/offset |
| bits | 位段(按最大 index 取 1/2/4 字节) | `bitgroup` | index 0~31 |
| bytes | 原样字节 | `bytes`(新 kind,前端 hex 输入,长度经 `FieldSchema.Length` 透传) | length 1~255 |

**数值变换契约**:`物理值 = 线值 × scale + offset`;编码反向计算,结果非整数必须报错(禁止静默截断);`scale` 缺省 1,`offset` 缺省 0,`scale ≤ 0` 非法。

### 4.3 编码规则

- **追加单元**:`TLV = unitCode(u8) + len(u16,大端) + EncodeFields(数据)`;unitCode 限于 0x80~0xFE(见 §4.4),恰与协议库的自定义数据 TLV 区重合(库可解码);拼接到标准 0x02 体字节之后,整体交 `rawBody` 走既有 `BuildFrame`(BCC 由帧层重算,协议库零改动)
- **`multiple: true` 语义 = 每行独立一个完整 TLV**(同一 unitCode 重复);已确认默认
- **realtimeLike 命令体**:`6B 时间(十进制字节,与本库 BeanTime codec 一存,非 BCD——golden 已验证)+ TLV×N`(Phase 3 实现,须复用 BeanTime 语义)

### 4.4 码位合法性(导入校验规则;依协议库实际语义,经 Oracle 评审修正)

| 对象 | 2016 | 2025 |
|---|---|---|
| unitCode 标准占用(拒绝) | 0x01~0x09 | 0x01~0x08、0x30(燃料电池电堆)、0x31(超级电容)、0x32(超容极值)、0xFF(签名) |
| unitCode 私有可用(**自定义数据区**) | **0x80~0xFE** | **0x80~0xFE** |
| 命令码标准占用(拒绝) | 0x01~0x08 | 0x01~0x0B |
| 命令码本期可用(上行预留区) | 0x09~0x7F | 0x0C~0x7F |
| 其余命令区(0x80~0x82 / 0x83~0xBF / 0xC0~0xFF) | 本期全部拒绝(下行扩展后续版本) | 同左 |

- **unitCode 仅允许 0x80~0xFE**:该区间与协议库的自定义 TLV 编解码区完全重合(`realtime_data_codec` 对 0x80~0xFE 按 `类型u8+长度u16+数据` 解码),扩展单元因此天然可被本仓库解析页/服务端模式解码;0x0A~0x7F 属标准预留且库解码直接报 `ErrUnknownTLVType`,0xFF 在 2016 同样不可解码——一律拒绝
- 同包内 unitCode / key 不得重复(校验器强制);跨包不校验(同时仅激活一个包)
- `direction` 仅支持 `up`;`trigger ∈ {manual, periodic, manual+periodic}`;`maxRows ≥ 0`

### 4.5 四个接入点(现状 → 目标)

| # | 接入点 | 现状 | 目标 |
|---|---|---|---|
| 1 | 命令码表 | `commandFor()` 内 私有远控 硬编码 map | 泛化注册表 `(版本, code, name)`,包激活时注册。**硬契约:每个扩展命令帧必须构造 `Min=Max=code` 的精确命令副本(同 私有远控 hack),禁止直接用 `CommandVXXByCode`——库把 0x09~0x7F 折叠为 `Code=0x09` 区间条目,线字节取 `rt.Code`,0x0A 会静默发出 0x09**;P3 必须含"帧命令字节等于声明 code"的 golden 断言 |
| 2 | 组装管线 | `schema.Assemble` 一把梭;`SendReissue` 直调 2016 专用组装(2025 连接下为既有 bug) | 标准体(不动) + `ext.EncodeUnit` 追加 TLV → `rawBody`;**四路同管线:Preview / SendRealtime / SetAutoReport / SendReissue**;P2 顺带把 SendReissue 统一到 `schema.Assemble(version,...)` 修复既有 2025 bug |
| 3 | Schema | `GetSchema(version)` | `GetSchema(version, pack)` = 标准 + `ext.CompileUnit`(前端表单零新增概念,仅新增 `bytes` kind 渲染);`schema.FieldSchema` 增加 `Length int` 字段(bytes 长度透传,标准组不使用,零行为影响);**扩展组的默认值由 `ext.DefaultsFor` 提供**(`bridge.defaultFieldValue` 无 bytes/物理默认值能力);P2 须同步处理 GetGroups/DefaultGroups 硬编码 "2016" 的 order 问题与 message.json 残留扩展键过滤 |
| 4 | Profile | `ConnectionConfig` 无扩展字段 | 增加 `extensionPack`;`Runtime` 持有激活包 |

### 4.6 全局约束

- 协议库 gb32960-go **零改动**
- 标准路径现有 golden 测试与单测**必须保持全绿(逐字节)**
- 技术栈不变:Wails v2 / Go / Vue 3 / Ant Design Vue;前端继续由 `GroupSchema` 驱动
- 包文件为单 JSON,存 `packs/` 子目录(`store.Dir()` 即 `os.UserConfigDir()/gbt32960-simulator` 之下,P2 新增子目录逻辑)
- 同时仅激活一个扩展包(按 Profile 绑定)
- 错误信息一律带 JSON 路径定位(如 `realtime.appendUnits[0].fields[3].scale`)

## 5. 范围边界

**本期做**:realtime.appendUnits 端到端、commands(上行私有命令)端到端、包管理(导入/列表/删除/绑定)、示例包与指南。

**本期不做(留后续)**:下行命令(0x83~0xBF)处理与私有应答时序 DSL、私有加密(Go 逃生口)、报文解析页对私有单元的解码(复用同一 DSL 的 decode 方向,P3 后评估)、标准单元内部字段修改。

## 6. Phase 分解(业务闭环优先,非技术分层)

| Phase | 名称 | 交付(可独立验收) | 依赖 | 覆盖 Global AC |
|---|---|---|---|---|
| **P1** | 扩展包核心引擎 | `internal/ext` 纯 Go 包:类型定义、加载、静态校验、干跑、字段 DSL 编码器、GroupSchema 编译器;全部单测。含 `schema.FieldSchema` 增加 `Length` 字段(零行为影响) | 无 | AC-4 全量;AC-1/2 的字节级地基 |
| **P2** | 实时数据扩展端到端 | Profile/Runtime 绑定、GetSchema 合并、组装管线**四路**拼接(Preview/SendRealtime/SetAutoReport/SendReissue,顺带修复 SendReissue 2025 既有 bug)、前端 `bytes` kind 渲染、最小 ExtService(目录扫描+列表)、示例包 golden | P1 | AC-1、AC-3、AC-5 |
| **P3** | 扩展命令字 | 命令注册表泛化、SendExtension/SetExtAutoReport、前端扩展命令区、realtimeLike 体、golden | P2 | AC-2、AC-3 |
| **P4** | 管理与交付面 | 设置页包管理 UI(导入/删除/换绑)、《插件包编写指南》、内置示例包 | P2(P3 可并行) | AC-6、AC-4 的 UI 面 |

**依赖关系**:P1 → P2 → P3;P4 依赖 P2,与 P3 可并行。

## 7. 关键技术决策记录(已确认)

| 决策 | 结论 | 依据 |
|---|---|---|
| 扩展的消费者 | 实施人员,APP 内导入 JSON 包(零代码);Go 接口仅为开发者逃生口 | 用户画像与交付链路(2026-08-31 讨论) |
| multiple 语义 | 每行独立一个完整 TLV | 厂商私有单元常见写法;必要时后续加 `rowsLayout` 开关 |
| 扩展包存放 | 用户数据目录 `packs/*.json`,设置页管理 | 与 `internal/store` 目录约定一致 |
| 标准体与扩展体的边界 | 字节层拼接,标准部分继续走 typed struct | 保 golden 全绿,风险隔离 |
| 协议库 | 零改动(rawBody 通道已验证) | 调研结论(2026-08-31) |
| 私有单元码区 = 0x80~0xFE | 与库自定义 TLV 区重合:可解码、不冒充标准单元 | Oracle 评审 P0-1(2026-09-01) |
| 扩展命令帧必须精确码副本(Min=Max=code) | 库区间条目折叠会改写线字节(0x0A→0x09),静默错误必须以契约+golden 拦截 | Oracle 评审 P1-2 |
| 0x03 补发同走扩展管线 | 体格式与 0x02 同构;顺带修复 SendReissue 的 2025 版既有 bug | Oracle 评审 P1-3 |

## 8. 子设计索引

P2~P4 的差异化细节(如 golden 用例数据、前端组件结构、UI 文案)在各自 Phase plan 中定稿——四阶段共享同一技术契约(本文 §4),故不单设 Phase design 文件,避免重复漂移;若某 Phase 实现中发现契约缺口,回写本文档并同步 Master plan。

## 9. 契约缺口回写(Phase 1 终审发现,待 P2/P3 裁决)

| # | 缺口 | 现状 | 裁决时机 |
|---|---|---|---|
| G-1 | offset 非整数 × Kind=int:校验器不限制 offset 为整数,`offset: 0.5` 会编译出 Kind=int 且默认值 0.5,P2 前端 int 输入框不兼容;且与仓库既有"纯偏移字段为 float"约定不一致 | Phase 1 按 PAC-3 冻结(scale=1→int)实现 | **P2 开工前**:二选一——校验 offset 整性,或 offset 非整数时 Kind=float |
| G-2 | realtimeLike 命令体单元与 realtime.appendUnits 共享 unitCode 命名空间:命令体 TLV 与 0x02 追加单元分属不同帧、线序无歧义,Phase 1 按"同包唯一"字面实现会拒绝跨场景同码 | validate.go 已按共享命名空间实现(含注释) | **P3 开工前**:确认是否有意共享;若厂商确需同码分离,拆分两张 unitCode 表 |
