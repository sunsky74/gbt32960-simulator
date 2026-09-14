# 项目前端 UI 与布局设计规范手册

> 反推自真实代码（2026-09-14 复核基准）。技术栈事实：**Vue 3 + Ant Design Vue 4 + 自定义 CSS 变量体系**，
> 无 Tailwind / 无 components/ui 原子目录——设计系统由 `frontend/src/style.css`（设计 Token + 全局布局类）
> 与 `frontend/src/theme/index.ts`（antd ConfigProvider Token）双层驱动，主题经 `<html data-theme="dark|light">` 切换。
> 参数速查版见 `docs/ui-design-reference.md`；本手册为完整规范 + 坏味道清零核对。

---

## 1. 基础设计系统 Token

### 1.1 色彩体系（双主题，`style.css` `:root` 变量 + `theme/index.ts` antd token 同源）

| 语义 | CSS 变量 | Dark | Light | antd token 对应 |
|---|---|---|---|---|
| 主色 Primary | `--primary` | `#1677ff` | `#1677ff` | colorPrimary |
| 页面底色 | `--bg-page` | `#141414` | `#f0f2f5` | colorBgLayout |
| 面板/卡片底色 | `--bg-panel` | `#1f1f1f` | `#ffffff` | — |
| 抬升层(输入/悬浮) | `--bg-elevated` | `#262626` | `#ffffff` | colorBgContainer/`#2a2a2a` elevated |
| 边框(次级/分隔线) | `--border-subtle` | `rgba(255,255,255,.08)` | `rgba(0,0,0,.06)` | colorBorderSecondary |
| 边框(主级/盒子) | `--border-strong` | `rgba(255,255,255,.15)` | `rgba(0,0,0,.12)` | colorBorder ≈.12 |
| 文本主/次/弱/禁用 | `--text-primary…disabled` | 白 0.88/0.65/0.45/0.35 | 黑 0.88/0.65/0.45/0.3 | colorText 系 |
| 成功/警告/错误 | `--success/--warning/--error` | `#52c41a/#faad14/#ff4d4f` | `#389e0d/#d46b08/#cf1322` | colorSuccess 系 |

辅助交互色：`--primary-hover-bg`(主色 0.08~0.1 淡底)、`--row-hover-bg`(行 hover 0.06)、`--item-hover-bg`(卡片头 hover 白/黑 0.04)、`--hl-bg`+`--hl-shadow`(选中态)。
**规则：组件样式一律引用变量，双主题各验一遍；特殊语义色需提供 light 覆盖**（如 `--c-encrypted`：dark `#9254de` → light `#6424c2`；`--c-unknown`：dark `#d46b08` → light `#ad4e00`）。

### 1.2 字体与排版

- 家族：`-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif`；数据文本（hex/JSON/VIN/时间）统一 `--font-mono` + `font-variant-numeric: tabular-nums`
- 字号梯度（实测）：**11 / 12 / 13 / 14 / 16 / 22**

| 层级 | 字号×字重 | 实例 |
|---|---|---|
| 页面 Hero 标题 | 22×600 | `.hero-title`（解析页输入区,ParserInputHero.vue:104） |
| 品牌位/大标题 | 16×600 | `.brand`（style.css:166） |
| 梯度外特例 | 15×600 | 扩展包区两处：抽屉标题（PackDetailDrawer.vue:339）、卡片统计数字 mono（PackCard.vue:230） |
| 正文/卡片标题 | 14×(400/500/600) | body、`.cc-title`、`.sb-title`、SettingRow `.sr-title`（14×500） |
| 次要说明 text-muted | 13×400/600 | `.toolbar-label`、`.group-title` |
| 辅助标签/数据 | 12×400 | `.zone-title`、`.field-label`、`.sr-desc` |
| 微标签 | 11×400~600 | `.dir-badge`、`.st`、`.server-proto` |

### 1.3 圆角与阴影

| 级别 | 值 | 使用处 |
|---|---|---|
| 盒子级 | 8px | `.zone`/`.side-card`/`.group-collapse` + antd 全局 borderRadius:8 |
| 子容器 | 6px | `.group-row`(虚线数据行)、`.row-detail` |
| 小元素 | 4px | `.raw-hex`、`.byte-card`、滚动条 thumb |
| 微标签 | 2px | `.server-proto`、`.st`、`.sr-badge`；孤例：`.dd-json-label` 3px（PackJsonPanel.vue:61） |
| 胶囊 | 999px | 分割条手柄（§2.3，V2.0 起统一） |
| 阴影 | `--shadow`:dark `0 1px 2px rgba(0,0,0,.3)` / light `.06` | **仅**主面板 zone 与悬浮卡片；卡片栈不加重影 |

弹窗圆角/蒙层走 antd 默认（borderRadius token 全局 8 生效）。

---

## 2. 布局与栅格系统

### 2.1 整体骨架（`App.vue` → `.app` flex 行）

```
.app(flex,100%)
├─ SideNav(.sidenav, width var(--nav-w), border-right subtle, 0.2s width 过渡)
└─ main.page-content(flex:1,min-width:0)
   └─ 页面组件(KeepAlive)
```

| 区 | 尺寸 | 备注 |
|---|---|---|
| 左侧主导航 | **168px 展开 / 56px 折叠**（brand 区点击切换，localStorage 持久化） | brand 区 44px 高；主菜单 navItems(4 项) + 底部 `mt-auto` 第二菜单 bottomNavItems(设置,border-top 分隔) |
| 二级导航（设置页） | **220px** `.settings-nav`（bg-panel + border subtle 盒子,SettingsPage.vue:174） | |
| 顶栏 | 48px `.topbar`（padding 0 16px,gap 12px,border-bottom subtle） | 客户端/服务端页共用;`bar-sep` 1×16 竖线 + `spacer` 弹性 |
| 主工作区外层 padding | 工作台页 `.server-workbench` **12px 16px**;滚动页 `.page-root` **12px 16px**(gap 12px) | 解析页 `.parser-page`/扩展页 `.ext-page` 复用 page-root（ext-page 的 padding 已对齐 12px 16px，原 16px 覆盖见 §5） |

### 2.2 卡片与面板

| 类 | 背景 | 边框 | 圆角 | 阴影 | 内边距 |
|---|---|---|---|---|---|
| 主面板 `.zone` | `--bg-panel` | 1px `--border-strong` | 8px | `--shadow` | 头 `.zone-head` 8px 16px(min-height 40,gap 8);体 `.zone-body` 16px |
| 右侧卡片 `.side-card` | `--bg-panel` | 1px `--border-subtle` | 8px | 无 | 头 `.cc-head` 12px 16px;内容 `.cc-content` 8px 16px 16px |
| 分组折叠 `.group-collapse` | `--bg-panel` | 1px subtle | 8px | 无 | header 6px 12px/13px;体 `.group-body` 6px 8px 8px |
| 数据行 `.group-row` | 透明 | **1px dashed strong** | 6px | 无 | 8px 10px,行距 8px |
| 右侧卡片栈 `.right` | `--bg-page` 页底 | — | — | — | padding 12px,gap 12px,卡片 `flex-shrink:0` |

面板间距规则：**结构级 12px / 组件级 8px / 紧凑 6px / 微 2~4px**；水平对齐线 16px；间距 100% 由分割条或容器 gap 提供（面板间不手写 margin）。

### 2.3 分割条（ResizableDivider / -Col，`style.css` 分割条段，V2.0 统一规范）

| 项 | 统一规范（已废除 9px / 双标体例） |
|---|---|
| 判定区 | **12px** 透明判定区（上下条高 / 左右条宽，同时构成面板间呼吸间距） |
| 手柄 | 常显居中胶囊：上下条 `36×4`、左右条 `4×36`，`border-radius:999px`，色 `--divider-grip`（style.css:261） |
| hover / 拖拽 | 手柄切 `var(--primary)` 并轻微加长至 40px，条底 `--primary-hover-bg` |
| 提示 | 仅 `aria-label`（无原生 tooltip） |
| 拖拽锁定 | `body.dragging-ns/ew`：禁文字选中 + 锁定全局光标 |

组件契约：拖拽数学动态读取自身 `offsetWidth/Height`，页面级钳制常量须与 CSS 尺寸同步（`DIVIDER_PX=12`）。

---

## 3. 常用控件交互规范

### 3.1 控件尺寸（Ant Design，无自绘控件库）

- **全站 `size="small"`**（工具风）：Button/Input/Select/InputNumber/Switch/Tabs 一律 small（antd small 控件高 24px）
- 默认尺寸仅用于：模态框确认按钮、解析页 Hero 主输入区
- 固定宽度：模板统一走 `.w-NNN` 档位类（style.css:916，档位 84/110/130/160/180/200/220/300），如端口 110 / 心跳 180 / 间隔 130 / 数组项 84 / 补发数字 200；内联固定像素宽度已清零
- 其余固定宽走全局类：档案选择 300（`.profile-select`）、搜索框 220（`.search-input`）、zone 内嵌搜索 160（`.zone-search`）、报文预览 select 200（`.pack-select`）
- 弹性规则：字段行内控件 `flex:1; min-width:0`；字段网格 `repeat(auto-fill,minmax(210px,1fr))`
- 按钮语义：主操作 primary / 危险 danger / 行内删除 `text+danger` / 添加 dashed / 行内链接 link(padding 压 0 4px)；筛选组 = button 组(选中 primary,**禁 radio-button**)

### 3.2 状态徽标（Badge / Tag）

| 形态 | 规格 | 实例 |
|---|---|---|
| antd `a-tag`（卡片头 extra） | 默认尺寸 + 语义色字符串（"red"/"blue"/无色） | 报警 red / 状态 blue / 轨迹:回放中 blue、未导入无色(已导入显示格式 GPX/XLSX/CSV) |
| antd `a-badge`（连接状态） | status 点 + 文本 | success/processing/default |
| 自绘 `.sr-badge`（SettingRow） | **11px** 字号（全站下限）、padding 2px 6px、radius 2px、tertiary 字 + elevated 底 + subtle 边框 | "规划中" |
| 自绘 `.dd-ok/.dd-off`（扩展页详情） | ●/○ 前缀 + primary/tertiary 色 | "● 已启用 / ○ 已禁用" |
| 在线状态点 | 8×8 圆(50%) + `--success` + 2s 脉冲光晕(`--success-glow`) | `.server-dot/.sess-dot` |

### 3.3 列表/设置行（`SettingRow.vue`，设置中心 8 分类强制复用）

- 布局：flex 两端对齐，左（标题+徽标行 → 描述 12px secondary，max-width 70%）右（action 插槽，flex-shrink 0，gap 8）
- 行高：`padding: 12px 0`；**分隔线** `border-bottom 1px subtle`（`.row-group` 末行隐藏）；内容列 **max-width 880px**（`.sb-content`,padding 6px 24px 28px）
- 禁用态（规划中）：标题降为 secondary
- 二级导航项 `.sn-item`：gap 8px / padding 8px 10px（已归位 4px 网格）；active 态 = `--primary-hover-bg` 底 + 主色文字 + 500 字重（SettingsPage.vue:223）

---

## 4. 弹窗与抽屉（Modal / Drawer）规范

- **蒙层/毛玻璃**：antd 默认（无自定义 backdrop-filter）；圆角由全局 token 8px 生效
- **宽度实测**：ConsolePanel 应答 520 / 导出 420；RealTimePanel 报文预览 760（`:footer="null"` 只读 + copyable）；ExtensionsPage 导入 640；**Drawer 仅服务端配置一处：`placement="right"` / 440px**
- 弹窗内排版约定：说明文字 `.modal-hint`（13px secondary,margin-bottom 12）；表单行 gap 8~12；应答码 radio-group `button-style="solid"`；操作右对齐（`.cfg-actions flex-end`）；危险确认用 `Modal.confirm`
- 抽屉：`cfg-form` gap 16px、`.cfg-item` 行 gap 12、label 宽 80px 13px secondary
- 悬浮信息卡 `.byte-card`（非模态）：fixed 定位随鼠标、min 200/max 250、padding 8 12、bg-elevated + border-strong + radius 4 + shadow、0.1s 入场、屏幕边缘翻转

---

## 5. 坏味道清零核对（原九项，2026-09-14 复扫）

> 下列九项为本文档首版（`a8670a6`）依据当时代码列出的不一致清单，同批次已按设计规范 V2.0 清零；
> 下表为复扫结果——位置更新为当前文件与行号，状态注明清零方式或残留。

| # | 当前位置（原引用） | 原问题 | 复扫状态 |
|---|---|---|---|
| 1 | `ExtensionCommands.vue`（原 `:188`） | `.row-label color:#888` 硬编码灰 + `.row-head` 同名类两套实现 | ✅ 已清零：scoped 重写删除，复用全局 `group-row` 体系（`ExtensionCommands.vue:203` 注释） |
| 2 | `PacketParserPage.vue`（原 `:1014-1019`） | `#ff7875/#cf1322` 硬编码红（双主题手写第二套） | ✅ 已清零：硬编码删除（页面零硬编码色）；字节告警复用全局主题化 `.bc-issue`（style.css:1547，`HoverInfoCard.vue:159` 注明不再重写） |
| 3 | 扩展包组件（原 `ExtensionsPage.vue:834-1059`，已拆分） | success 色双主题手写、`rgba(255,77,79,.35)!important`、JSON 预览块自成深色主题 | ✅ 已清零并重新规范：色值全部 token 化；JSON 块按 reference §11.3 定位为**固定深色代码块**（`PackJsonPanel.vue:77`，`#0f0f0f` 底 + 复制按钮 + `max-height:360px`，声明为不随主题切换的独立代码区） |
| 4 | 全仓（原 12 处） | 内联 `style="width:NNNpx"` 散落模板 | ✅ 已清零：收敛为 `.w-*` 档位类（style.css:916）；固定像素内联 0 处（余 `width:100%` 属豁免） |
| 5 | `ExtensionsPage.vue`（原 `:629`） | `.ext-page` 覆盖 padding 为 16px，偏离 12px 基准 | ✅ 已清零：现 `padding: 12px 16px`（`ExtensionsPage.vue:216`） |
| 6 | `SettingsPage.vue` / `SettingRow.vue`（原 `:406`） | `.sn-item` gap 9px / padding 7px 10px；`.sr-badge` 10px 字号 + radius 3px | ✅ 已清零：`.sn-item` 归位 `gap:8px / padding:8px 10px`（`SettingsPage.vue:204`）；`.sr-badge` 11px / radius 2px（`SettingRow.vue:61`） |
| 7 | `SettingsPage.vue`（原 `:391 / :450`） | `.sn-head` 14×600 与 `.sn-item.active` 15×600 字号倒挂 | ✅ 已清零：`.sn-title` 14×600（`SettingsPage.vue:191`），active 不再另设字号（`:223`） |
| 8 | 弹层共用类（原 `ServerModePage.vue` 下发弹窗） | `modal-hint/extcmd-fields/hex-input` scoped 重写，与 ConsolePanel 重复实现 | ⚠️ 部分清零：全局 `.modal-hint` 已建立（style.css:909，注明"禁止 scoped 重写"）；**残留两处同内容 scoped 副本未删**（`ConsolePanel.vue:331`、`ExtCommandModal.vue:159`，无视觉差异） |
| 9 | `style.css`（原 `:1325` 等三色行） | 未知橙/加密紫 dark/light 各写两份 rgba 原值（6 处硬编码） | ✅ 已清零：提为 `--c-unknown` / `--c-encrypted` 语义变量（style.css:47-48 / 77-78），`st-*` / `frame-*` / `dir-*` 全量引用变量 |

**共性结论**：全局体系（style.css 变量 + zone/card 类）覆盖良好；九项坏味道已于设计规范 V2.0 批次清零，唯一残留为第 8 项两处无视觉差异的 scoped 副本。新增页面仍须零 scoped 颜色、布局类从全局取；弹层共用类禁止 scoped 重写。
