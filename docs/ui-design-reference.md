# UI 全局参考设置参数 V2.0(正式版)

> 来源基准:客户端模拟页体系 + 全仓审计(`docs/frontend-ui-spec.md`)。
> 用途:**后续前端开发的唯一准则**——新增页面/组件一律以本文档为准;与本文冲突的存量实现按 §13 迁移清单收敛。
> 双主题:所有颜色参数提供 Dark / Light 两套,由 `<html data-theme="dark|light">` 驱动。
> 修订记录:V2.1(2026-09-18)——①圆角/字号 token 化(`--radius-*` 38 处 / `--fs-*` 128 处);②文字级语义色 `--*-text` + `--focus-ring`,tertiary 0.45→0.55;③antd 文字 token 与浅色预设标签覆盖;④键盘可达性(`:focus-visible`、JsonTree / CollapsibleCard);⑤类名单源化与残留(§13)。
> V2.0(2026-09-07)——①分割条统一规范(废除 9px/12px 双标);②新增 §5.1 SettingRow;③新增 §11 弹窗与抽屉;④新增 §12 扩展包/应用网格卡片。
> V1.0:客户端模拟页基准首版提炼。

---

## 1. 布局骨架与尺寸

| 参数 | 值 | 说明 / 使用处 |
|---|---|---|
| 顶部工具栏高度 | `48px` | `.topbar`(页面级 Header) |
| 侧导航栏宽度(展开/折叠) | `168px` / `56px` | `.sidenav`,brand 区 44px 高 |
| 二级导航栏宽度(设置页) | `220px` | `.settings-nav` |
| 右侧卡片栈宽度(客户端页) | `320px` | `.right`(flex-shrink:0, overflow-y:auto) |
| 上下 Panel 最小高度 | `160px` | 客户端页 MIN_PANEL_PX |
| 上下 Panel 初始占比 | `55% / 45%` | 拖拽分割条初始位置 |
| **分割条判定区(统一)** | **`12px`**(上下条高 / 左右条宽) | 见 §8 统一规范 |
| 字号基准 | `14px` | body(antd fontSize:14) |

---

## 2. 间距体系(4px 网格:2 / 4 / 6 / 8 / 12 / 16)

### 2.1 容器内边距(padding)

| 容器 | padding | 备注 |
|---|---|---|
| 滚动型页面根 | `12px 16px` | `.page-root`(解析/服务端/设置页) |
| 工作台页主区 | `12px 16px` | `.server-workbench` / 客户端页 `.left` |
| 顶栏 | `0 16px` | `.topbar` |
| Panel 内容区 | `16px` | `.zone-body` |
| Panel 头部 | `8px 16px`(min-height 40px) | `.zone-head` |
| 控制台工具条 | `8px 16px 12px` | `.console-toolbar` |
| 可折叠卡片 | 头 `12px 16px` / 内容 `8px 16px 16px` | `.cc-head` / `.cc-content` |
| 分组折叠体 / 数据行 | `6px 8px 8px` / `8px 10px` | `.group-body` / `.group-row`(1px dashed) |
| 列表行 | `4~8px 16px` | `.console-row` 4px / `.sess-row` 8px |

### 2.2 元素间距(gap / margin)

| 场景 | 值 |
|---|---|
| 顶栏控件横向 / 卡片栈纵向 | `12px` |
| 工具条、Panel 头、按钮组(a-space) | `8px` |
| 表单项纵向 / 数据行之间 / 分组卡片之间 | `8px` |
| 开关行 | `gap 8px + margin-bottom 6px` |
| 字段网格 `6px 14px` / 字段内 label↔控件 `6px` | 紧凑级 |
| 位组网格 `2px 10px`;标题↔副标题 `4px` | 微级 |

**选用规则**:结构级 12px / 组件级 8px / 紧凑 6px / 微 2~4px;水平留白统一 16px;面板间距 100% 由容器 gap 或分割条占位提供,**禁止手写 margin**。

**Token 边界(V2.1 明确)**:间距**不做** CSS 变量 token 化,继续沿用上表 4px 网格字面值;`--radius-*` / `--fs-*` 只覆盖圆角与字号。新增间距按 2/4/6/8/12/16 书写,勿以"未 token 化"为由改造。

---

## 3. 盒子(容器)边框与外观

### 3.1 边框颜色

| Token | Dark | Light | 用途 |
|---|---|---|---|
| `--border-subtle` | `rgba(255,255,255,.08)` | `rgba(0,0,0,.06)` | 分隔线、卡片次级边框 |
| `--border-strong` | `rgba(255,255,255,.15)` | `rgba(0,0,0,.12)` | 主面板边框、悬浮卡边框 |

### 3.2 使用规则

| 场景 | 边框 |
|---|---|
| 主面板 `.zone` | `1px solid var(--border-strong)` |
| 卡片 `.side-card` / 分组 `.group-collapse` | `1px solid var(--border-subtle)` |
| 可编辑数据行 `.group-row` | `1px dashed var(--border-strong)`(虚线=可编辑语义) |
| 结构分隔线 / 行分隔 | `1px solid var(--border-subtle)` |
| 列表选中行 | 背景 `--hl-bg` + `inset 2px 0 0 var(--primary)` 左指示条 |

### 3.3 圆角与阴影

| 级别 | Token | 值 | 使用处 |
|---|---|---|---|
| 盒子级 | `--radius-lg` | `8px` | zone / side-card / group-collapse / antd 全局 / §12 网格卡片 |
| 子容器级 | `--radius-md` | `6px` | group-row / row-detail |
| 小元素级 | `--radius-sm` | `4px` | raw-hex / byte-card / dd-json-label / 滚动条 thumb |
| 微标签级 | `--radius-xs` | `2px` | server-proto / st / sr-badge |
| 胶囊 | `--radius-pill` | `999px` | 分割条手柄(§8) |
| 阴影 | — | `--shadow`(dark `0 1px 2px rgba(0,0,0,.3)` / light `.06`) | 仅主面板与悬浮卡;卡片栈不加;§12 网格卡 hover 态加 shadow-md |

单值圆角一律取 `--radius-*`(style.css:14-18);多值方向性圆角(如 SideNav.vue:155 `0 2px 2px 0`)保留字面值。

### 3.4 背景三级分层

| 层级 | Dark | Light |
|---|---|---|
| 页面底 `--bg-page` | `#141414` | `#f0f2f5` |
| 面板 `--bg-panel` | `#1f1f1f` | `#ffffff` |
| 抬升层 `--bg-elevated` | `#262626` | `#ffffff` |

---

## 4. 按钮体系

**全站按钮 `size="small"`**;默认尺寸仅两类例外:模态框确认按钮、解析页 Hero 主输入区按钮组与扩展包 select。

| 场景 | 规格 |
|---|---|
| 顶栏/主操作 | `small + primary`;危险 `small + danger`;次操作 `small + default` |
| Panel 头操作组 | `small`,primary+default 混排,a-space :size="8" |
| 工具条筛选组 | `small` button 组,选中 `primary`(禁止 radio-button) |
| 行内删除 / 链接操作 / 添加 | `small + text + danger` / `small + link`(padding 0 4px) / `small + dashed` |
| 组内间距 | `8px`;语义区之间用 `bar-sep` 或 `spacer` |
| 默认尺寸例外 | **仅**模态框确认按钮 + 解析页 Hero 操作区(解析/清空/复制 + 扩展包 select,ParserInputHero.vue:51-70);其余一律 `small`(RealTimePanel「发送补发 (0x03)」已回归,RealTimePanel.vue:330) |

---

## 5. 控件规格(Ant Design)

| 控件 | 规格 |
|---|---|
| 输入/选择/数字/开关/Tabs | 一律 `size="small"`(高 24px) |
| 表单布局 | `layout="vertical"`,label padding-bottom 2px |
| 固定宽度档位 | 档案选择 300 / 搜索 220 / zone 内搜索 160 / 端口 110 / 心跳 180 / 补发数字 200 / 间隔 130 / 数组项 84 / 预览弹窗 select 200 |
| 弹性宽度 | 字段行内 `flex:1; min-width:0` |
| 字段网格 | `repeat(auto-fill, minmax(210px, 1fr))`;位组 `minmax(170px, 1fr)` |
| 状态徽标 | a-tag(语义色) / a-badge / 圆点 8×8+脉冲;自绘徽标 11px 字号下限 |

### 5.1 设置项行(SettingRow)规范

**结构**:水平 flex 容器,`justify-content: space-between`,`align-items: center`(多行描述时 `flex-start`),`padding: 12px 0`,行底 `1px solid var(--border-subtle)`,**每组最后一行去除 border-bottom**。

| 区 | 规范 |
|---|---|
| 左侧信息区 | `flex:1; min-width:0`,**最大宽度限制 65%~70%**,超长自动换行(`overflow-wrap: break-word`) |
| 标题行 | 标题 `font-size: 14px; font-weight: 500; color: var(--text-primary)`,与 Badge 同行 `gap: 8px`;禁用/规划中态标题降为 `--text-secondary` |
| 副说明 | `12px; var(--text-secondary); margin-top: 4px; line-height: 1.6`;数据类说明走 `--font-mono` 11px |
| 右侧控件区 | `flex-shrink:0`,右对齐,内部多控件 `gap: 8px`;**Select / Input 固定宽度 180px~240px,禁止拉伸或压扁**(开关/单选不受限) |

---

## 6. 字号与文本层级

| 层级 | Token | 字号×字重 | 颜色 |
|---|---|---|---|
| Hero 标题 | `--fs-22` | 22×600 | --text-primary |
| 页面标题 | `--fs-18` | 18×600 | --text-primary |
| 品牌/卡片标题 | `--fs-16` / `--fs-14` | 16×600 / 14×600 | --text-primary |
| 正文 | `--fs-14` | 14×400 | --text-primary |
| 次要说明 | `--fs-13` | 13×400 | --text-secondary |
| 辅助标签/数据 | `--fs-12` | 12×400 | --text-secondary / tertiary |
| 微标签 | `--fs-11` | 11×400~600 | --text-tertiary |

等宽:数据文本统一 `--font-mono` + `font-variant-numeric: tabular-nums`。**字号一律取 `--fs-*`(style.css:20-26),下限 11px**;梯度外字号(10 / 15 / 17px)已清零,18px 为页面标题档(如 `.ext-title`,ExtensionsPage.vue:237)。`--text-tertiary` 双主题 0.45→0.55(WCAG ≥4.5:1)。

---

## 7. 状态色(双主题)

| 语义 | Dark | Light |
|---|---|---|
| 主色 `--primary` | `#1677ff` | `#1677ff` |
| 成功 `--success` | `#52c41a` | `#389e0d` |
| 警告 `--warning` | `#faad14` | `#d46b08` |
| 错误 `--error` | `#ff4d4f` | `#cf1322` |

辅助:`--primary-hover-bg`(0.08~0.1)、`--row-hover-bg`(0.06)、`--item-hover-bg`(0.04)、`--hl-bg`+`--hl-shadow`(选中)、`--success-glow`(状态点光晕)。特殊语义色须提供 light 覆盖(参照 `--c-encrypted` `#b37feb→#6424c2` 先例)。

### 7.1 文字级语义色与焦点轮廓(V2.1 新增)

| 用途 | CSS 变量 | Dark | Light | antd token |
|---|---|---|---|---|
| 主色文字 | `--primary-text` | `#4096ff` | `#0958d9` | colorPrimaryText / colorInfoText / colorLink |
| 成功文字 | `--success-text` | `#52c41a` | `#237804` | colorSuccessText |
| 警告文字 | `--warning-text` | `#faad14` | `#ad4e00` | colorWarningText |
| 错误文字 | —(antd 独有) | `#FF4D4F` | `#CF1322` | colorErrorText |
| 焦点轮廓 | `--focus-ring` | `#4096ff` | `#1677ff` | —(自绘 `:focus-visible`) |

**规则**:文字用 `-text` 变体,填充/边框/图标保留 `--primary/--success/--warning`;antd 由 colorInfoText 派生 colorLink,两者显式声明(theme/index.ts:29-35 / 57-63)。键盘焦点:全局 `:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }`(style.css:151),antd 控件自带焦点样式不受影响。浅色预设标签可读性覆盖:`:root[data-theme='light'] .ant-tag-blue/.ant-tag-geekblue { color: #0958d9; }`(style.css:943-947),其余预设标签沿 antd 默认。

---

## 8. 分割条统一规范(V2.0 起,废除 9px/12px 双标)

**底层抽象**:统一分割组件 `ResizableDivider`(横向,上下拖,cursor `ns-resize`/`row-resize`)与 `ResizableDividerCol`(纵向,左右拖,cursor `col-resize`),共用同一视觉契约。

| 项 | 统一规范 |
|---|---|
| 判定区(Hit Area) | **12px**(上下条高度 / 左右条宽度),透明、无视觉线 |
| 手柄(居中胶囊) | 横向拖拽条:`36px × 4px`(宽×高);纵向拖拽条:`4px × 36px`;`border-radius: 999px`;常态色 `--divider-grip`,**常显** |
| Hover / 拖拽中 | 手柄切换 `var(--primary)`(允许 ≤40px 的轻微加长动效),条底 `var(--primary-hover-bg)` 淡色 |
| 全局锁定 | 拖拽中 `body.dragging-ns/ew`:禁文字选中 + 锁定全局光标 |
| 提示 | 仅 `aria-label`(无原生 tooltip、无悬浮文字) |
| 钳制常量同步 | 页面级 `DIVIDER_PX` 等钳制常量必须与 12px 同步(代码注释标注联动) |
| 占位语义 | 面板间距由分割条 12px 占位构成,两侧 0 额外 margin |

组件契约:拖拽数学动态读取自身 `offsetWidth/offsetHeight`,不硬编码尺寸。

---

## 9. 交互动效

| 场景 | 时长/曲线 |
|---|---|
| hover 微交互 | `0.15s` |
| 主题切换 / 侧栏宽度 | `0.2s ease` |
| 卡片折叠(grid 0fr↔1fr) | `0.3s ease` |
| 悬浮卡入场 | `0.1s ease-out` |
| 状态点脉冲 | `2s ease-out infinite` |
| reduced-motion / 设置关动画 | 全局压至 `0.01ms` |

---

## 10. 滚动条

宽/高 `8px`,thumb 圆角 4px、色 `--scrollbar-thumb`(hover 加深),轨道透明。

---

## 11. 弹窗与抽屉(Modal & Drawer)规范

### 11.1 抽屉(Drawer)

| 项 | 规范 |
|---|---|
| 宽度 | **统一 440px**(placement right);特殊宽内容(表格级)可 520px,须注明理由 |
| 蒙层 | `rgba(0,0,0,0.45)` + 轻微 `backdrop-filter: blur(2px)` |
| 内容区 | **轻卡片化排版**:内层分组用 `var(--bg-panel)` 底 + `1px var(--border-subtle)` 细边框卡片,**禁用纯白刺眼底色直铺**;表单行 gap 12~16px |
| 头部/操作区 | 标题 16×600;操作区右下对齐(`justify-content: flex-end`),主操作 primary |

### 11.2 模态框(Modal)

宽度档位:420(轻提示)/ 440(单表单)/ 520(双列表单)/ 760(宽内容预览);蒙层同 11.1;说明文字 13px secondary;确认按钮默认尺寸 primary;危险确认走 `Modal.confirm`。

### 11.3 弹层内代码预览区(JSON / 原始报文 HEX)

- **强制独立深色代码块容器**:不随主题切换的固定深底(如 `#0f0f0f` 系)+ 自配语法前景;`border-radius: var(--radius-md)`(6px);`max-height: 360px; overflow-y: auto`(PackJsonPanel 当前保留 6 处固定 hex 未 token 化,见 §13#10 残留)
- 字体 `--font-mono` `12px`,`white-space: pre-wrap; word-break: break-all`
- **右上角必须配备快捷复制按钮**(antd Typography `copyable` 或自绘 icon 按钮),复制成功给 `message.success` 反馈
- 禁止将 JSON 直接倾倒在正文文本流中

---

## 12. 扩展包 / 应用网格卡片规范

适用于扩展包管理页、卡片墙类内容区(ExtensionsPage 等)。

| 项 | 规范 |
|---|---|
| 容器栅格 | `repeat(auto-fill, minmax(320px, 1fr))`,`gap: 12px` |
| 卡片外观 | `border-radius: 8px`;`1px solid var(--border-subtle)`;底 `var(--bg-panel)`;**hover:`shadow-md` 悬浮动效**(`0.2s` 过渡,不位移) |
| 头部左侧 | 固定 **40px × 40px** 扩展图标;无图标时用**首字母彩色徽章**(底色由包 id 哈希取主题色盘,文字白/黑按对比度) |
| 头部右侧 | **唯一 `a-switch` 启用开关**;**杜绝卡片底部重复显示"已启用"文字状态**(状态语义由开关唯一承载) |
| 卡片内容 | 名称 14×600;元信息(vendor/版本/单元数)12px tertiary;操作(详情/删除)收进 hover 显现或下拉,不与 Switch 抢位 |

---

## 13. 存量迁移清单(V2.0 生效,按此收敛)

| # | 现状 | 迁移动作 | 状态 |
|---|---|---|---|
| 1 | 客户端上下分割条 9px + 36×3 长条 grip(`.resizable-divider` 基类) | 统一至 §8:12px + 胶囊 36×4 | ✅ 已完成 |
| 2 | 服务端胶囊 32×4 / 4×32(hover 40) | 尺寸对齐 §8 的 36×4 / 4×36 | ✅ 已完成(tight 变体整体删除) |
| 3 | SettingRow 现值 14px 0 / 72% / 13px 标题 / desc tertiary | 对齐 §5.1:12px 0 / ≤70% / 14px 标题 / desc secondary | ✅ 已完成 |
| 4 | 服务端配置抽屉 360px、无 blur | 对齐 §11.1:440px + 蒙层 blur | ✅ 已完成 |
| 5 | ExtensionsPage 网格与卡片未按 §12(底部"已启用"文字态、硬编码 JSON 深色块) | 按 §12 重排;JSON 块对齐 §11.3(补复制按钮) | ✅ 已完成(40×40 徽章/唯一 Switch/复制按钮原已就位;网格 320/gap12、padding 12 16、JSON 块 360px/pre-wrap、success/error token 化) |
| 6 | `frontend-ui-spec.md` §5 所列 9 项坏味道 | 按该清单逐项修复(硬编码色 → token、内联宽度 → 档位类) | ✅ 已完成(#888→token、重复 row-head/bc-issue/modal-hint 删除、内联宽度→w-* 档位类(余 4 处 100% 豁免)、sn-item 网格归位、三色提 --c-unknown/--c-encrypted 变量、字号倒挂修正) |
| 7 | 单值圆角/字号散落字面量(含 3px 圆角与 10/15/17px 梯度外字号) | 统一 `--radius-*` / `--fs-*`(11px 下限、18px 页面标题档) | ✅ 已完成(125+ 处;多值方向性圆角保留字面值) |
| 8 | 文字语义色缺失:文字直接用填充色、tertiary 0.45、无键盘焦点轮廓 | 新增 `--*-text` / `--focus-ring`;tertiary→0.55;antd 文字 token 同步;浅色 preset tag 文字覆盖 | ✅ 已完成 |
| 9 | 自绘行不可键盘操作(JsonTree 行 / CollapsibleCard 头部) | `tabindex="0"` + `aria-expanded` + Enter/Space;全局 `:focus-visible` | ✅ 已完成 |
| 10 | 跨组件类名冲突与 scoped 副本(`.group-title`、`.byte-card`、`.modal-hint` 等) | 单源化:设置面板 `.group-title`→`.settings-group-title`;共享类只留全局定义;`.track-file` 收 ellipsis;`CTRL_W`→`w-180` | ⚠️ 主体完成:残留 4 项详见 `frontend-ui-spec.md` §5#10(PacketDetail 逻辑副本 / CTRL_W / PackJsonPanel 固定色块 / 其余预设标签) |

---

## 14. 快速检查单(新页面/新组件)

1. 容器:滚动页 `.page-root`(12px 16px + gap 12);工作台页 `.topbar` + `.zone` 体系。
2. 盒子:主面板 strong 边框 + 8px + shadow;子卡片 subtle 边框无阴影;可编辑行 dashed。
3. 内边距:头 8px 16px / 体 16px / 卡片内容 8 16 16 / 行 4~8px 16px。
4. 按钮:一律 small(仅模态确认 + Hero 操作区默认尺寸例外);语义档位见 §4;筛选 = button 组。
5. 间距:12/8/6/2-4;水平线 16px;零手写 margin 于面板间。
6. 文本:字号走 `--fs-*`(下限 11px,18px 页面标题);三级透明度(tertiary 0.55);数据 mono。
7. 颜色:仅 CSS 变量与状态 token;文字用 `--*-text` 变体;双主题各验 ≥4.5:1;特殊语义色成对定义。
8. 分割条:一律 §8 统一组件与规格;钳制常量同步。
9. 设置行:§5.1;弹窗/抽屉:§11;网格卡片:§12。
10. 动效 0.15/0.2/0.3s;尊重 reduced-motion。
11. 键盘:自绘可交互/可展开元素补 `tabindex` + `aria-*`;焦点轮廓走全局 `:focus-visible`(`--focus-ring`)。
