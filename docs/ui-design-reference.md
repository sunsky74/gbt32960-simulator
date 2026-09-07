# UI 全局参考设置参数(客户端模拟页基准)

> 来源:以 `frontend/src/pages/ClientSimulatorPage.vue` 及其全部子组件(RealTimePanel / ConsolePanel / CollapsibleCard / 右侧卡片栈)+ `frontend/src/style.css` + `frontend/src/theme/index.ts` 为基准提炼。
> 用途:新增页面/组件时的间距、边框、按钮、字号等全局参考,保证与客户端模拟页视觉一致。
> 双主题:所有颜色参数均提供 Dark / Light 两套,由 `<html data-theme>` 驱动。

---

## 1. 布局骨架与尺寸

| 参数 | 值 | 说明 / 使用处 |
|---|---|---|
| 顶部工具栏高度 | `48px` | `.topbar`(页面级 Header) |
| 侧导航栏宽度(展开) | `168px` | `.sidenav` EXPANDED_W |
| 侧导航栏宽度(折叠) | `56px` | `.sidenav` COLLAPSED_W |
| 侧栏品牌区高度 | `44px` | `.sidenav-brand` |
| 右侧卡片栈宽度 | `320px` | `.right`(flex-shrink: 0, overflow-y: auto) |
| 左侧面板最小高度 | `160px` | MIN_PANEL_PX(上下 Panel 拖拽钳制) |
| 上下 Panel 初始占比 | `55% / 45%` | 拖拽分割条初始位置 |
| 上下拖拽条高度 | `9px`(客户端基类) / `12px`(服务端 tight) | `.resizable-divider` / `.tight` |
| 左右拖拽条宽度 | `12px`(服务端 Session↔报文流、HEX↔解析) | `.resizable-divider-col` |
| 字号基准 | `14px` | body(antd fontSize: 14) |

---

## 2. 间距体系(4px 网格:2 / 4 / 6 / 8 / 12 / 16)

### 2.1 容器内边距(padding)

| 容器 | padding | 备注 |
|---|---|---|
| 滚动型页面根 | `12px 16px` | `.page-root`(解析/服务端/设置页) |
| 客户端页左区 | `12px 16px` | `.left`(含 border-right 分隔) |
| 客户端页右区(卡片栈) | `12px` | `.right` |
| 顶栏 | `0 16px` | `.topbar` |
| Panel 内容区 | `16px` | `.zone-body` |
| Panel 头部 | `8px 16px`(min-height 40px) | `.zone-head` |
| 控制台工具条 | `8px 16px 12px` | `.console-toolbar` |
| Tabs 头部 | `8px 16px 0`(zone)/ `12px 16px 0`(console) | `.zone-tabs .ant-tabs-nav` |
| 可折叠卡片头部 | `12px 16px` | `.cc-head` |
| 可折叠卡片内容 | `8px 16px 16px` | `.cc-content`(上窄下宽,贴头部) |
| 分组折叠体 | `6px 8px 8px` | `.group-body` |
| 数据行(虚线盒) | `8px 10px` | `.group-row` |
| 控制台事件行 | `4px 16px` | `.console-row` |
| 列表上下留白 | `4px 0` | `.console-list` / `.stream-list` |

### 2.2 元素间距(gap / margin)

| 场景 | 值 |
|---|---|
| 顶栏控件横向间距 | `12px`(gap) |
| 右侧卡片栈纵向间距 | `12px`(gap) |
| 工具条/Panel 头部控件间距 | `8px`(gap) |
| 按钮组间距(a-space) | `8`(默认)/ `small` |
| 表单项纵向间距 | `8px`(.mini-form .ant-form-item margin-bottom) |
| 开关行间距 | `gap: 8px` + `margin-bottom: 6px`(.switch-row) |
| 字段网格间距 | `6px 14px`(行 × 列,.fields-grid) |
| 字段内 label ↔ 控件 | `6px`(gap) |
| 数据行之间 | `8px`(margin-bottom,.group-row) |
| 分组卡片之间 | `8px`(margin-bottom,.group-collapse item) |
| 位组网格间距 | `2px 10px`(.bits-group / .alarm-bits) |
| 标题 ↔ 副标题 | `4px`(console-empty-title margin-bottom) |
| 行头 ↔ 内容 | `4px`(row-head margin-bottom) |

**间距选用规则**:
- 结构级分隔(区块之间、卡片栈)→ `12px`
- 组件级分隔(按钮组、表单项、行)→ `8px`
- 紧凑内容(字段行内、位复选)→ `6px` 或 `2~4px`
- 水平留白统一 `16px`(左右 padding 对齐),紧凑场景可降为 `12px`

---

## 3. 盒子(容器)边框与外观

### 3.1 边框颜色(CSS 变量,双主题)

| Token | Dark | Light | 用途 |
|---|---|---|---|
| `--border-subtle` | `rgba(255,255,255,0.08)` | `rgba(0,0,0,0.06)` | 分隔线、卡片边框(次级) |
| `--border-strong` | `rgba(255,255,255,0.15)` | `rgba(0,0,0,0.12)` | 主面板(zone)边框、悬浮卡片边框 |

antd token 对应:`colorBorderSecondary` = subtle、`colorBorder` = 0.12/0.12。

### 3.2 边框使用规则

| 场景 | 边框 | 场景类 |
|---|---|---|
| 主面板盒(zone) | `1px solid var(--border-strong)` | `.zone` |
| 右侧卡片 / 分组卡片 | `1px solid var(--border-subtle)` | `.side-card` / `.group-collapse .ant-collapse-item` |
| 可编辑数据行 | `1px dashed var(--border-strong)` | `.group-row`(虚线=可编辑语义) |
| 结构分隔线 | `1px solid var(--border-subtle)` | 顶栏下边线、Panel 头下边线、行下边线 |
| 选中行 | 背景高亮 + `inset 2px 0 0 var(--primary)` | `.sess-row.selected`(左侧 2px 主色指示条) |

### 3.3 圆角(4 级)

| 级别 | 值 | 使用处 |
|---|---|---|
| 盒子级 | `8px` | `.zone`、`.side-card`、`.group-collapse`、antd 全局 borderRadius |
| 子容器级 | `6px` | `.group-row`、`.row-detail` |
| 小元素级 | `4px` | `.raw-hex`、`.byte-card`、滚动条 thumb |
| 微标签级 | `2px` | `.server-proto`、`.st` 状态小标签 |
| 圆形 | `50%` | 状态圆点(.server-dot / .sess-dot 8×8) |

### 3.4 阴影与背景分层

| Token | 值(Dark / Light) |
|---|---|
| `--shadow` | `0 1px 2px rgba(0,0,0,0.3)` / `0 1px 2px rgba(0,0,0,0.06)` |

三级背景分层(页面 → 面板 → 输入框):

| 层级 | Dark | Light | 对应 |
|---|---|---|---|
| 页面底 | `#141414` | `#f0f2f5` | `--bg-page` / colorBgLayout |
| 面板 | `#1f1f1f` | `#ffffff` | `--bg-panel` |
| 抬升层(输入/悬浮) | `#262626` | `#ffffff` | `--bg-elevated` / colorBgContainer #262626→#2a2a2a elevated |

阴影仅用于主面板 zone 与悬浮卡片(byte-card),卡片栈不加重影(靠边框分层)。

---

## 4. 按钮体系

**核心规则:全站按钮统一 `size="small"`(紧凑工具风),仅模态框确认类用默认尺寸。**

| 场景 | 规格 | 示例 |
|---|---|---|
| 顶栏主操作 | `small + type="primary"` | 连接并登录 |
| 顶栏次操作 | `small + default` | 测试连接、新建连接 |
| 顶栏危险操作 | `small + danger` | 断开连接 |
| Panel 头操作组 | `small`,primary(主)+ default(次)混排,a-space :size="8" | 发送(0x02)/ 预览 HEX / 保存配置 |
| 工具条过滤器 | `small`,选中态 `type="primary"`,未选中 `default`(button 组;客户端控制台与服务端报文流同款,禁止 radio-button) | 控制台/报文流:全部/发送(TX)/接收(RX)/链路(Error) |
| 行内删除 | `small + type="text" + danger` | 删除行、数组项 × |
| 行内链接操作 | `small + type="link"`,padding 压缩为 `0 4px` | 应答按钮 |
| 添加类操作 | `small + type="dashed"`,列表级用 `block` 拉满 | ＋ 添加一行 / ＋ 添加项 |
| 弹窗确认 | 默认尺寸 + primary | 发送应答 / 选择位置并导出 |

按钮间距:同一操作组内 `8px`(a-space :size="8" 或容器 gap: 8px);不同语义区之间靠 `bar-sep` 竖线或 `spacer` 弹性空隙分隔。

---

## 5. 控件规格(Ant Design)

| 控件 | 规格 |
|---|---|
| 输入框 / 选择器 / 数字输入 | 一律 `size="small"`(表单/工具条内) |
| 开关 | `size="small"` |
| 复选框组 | 网格排列,checkbox 字号 `12px`(bits)/ `13px`(alarm) |
| Tabs | `size="small"`,nav padding `8px 16px 0` |
| 表单布局 | `layout="vertical"`,label 下 padding-bottom `2px` |
| 固定宽度参考 | 档案选择 `300px`、搜索框 `220px`、端口输入 `110px`、心跳输入 `180px`、补发数字输入 `200px`、间隔输入 `130px`、数组项输入 `84px` |
| 弹性宽度 | 字段行内控件 `flex: 1; min-width: 0` 填满剩余 |
| 字段网格 | `repeat(auto-fill, minmax(210px, 1fr))`;位组 `minmax(170px, 1fr)`;报警位两列 `1fr 1fr` |

**已收录的场景变体**(有意偏离基准,新组件同场景应沿用变体而非基准值):

| 变体 | 值 | 场景 |
|---|---|---|
| Panel 头部内嵌搜索框 | `160px`(`.zone-search`) | zone-head 单行内的紧凑搜索,区别于工具条级 220px |
| 原始报文内容区 padding | `8px 16px`(`.raw-body`) | 不做字段解析的紧凑展示区,区别于 zone-body 16px |
| 抽屉/弹窗表单间距 | `gap: 16px`(`.cfg-form`) | 弹层内表单比页面结构级 12px 略宽松 |

---

## 6. 字号与文本层级

| 层级 | 字号 | 字重 | 颜色 Token | 使用处 |
|---|---|---|---|---|
| 页面标题(品牌) | 16px | 600 | --text-primary | .brand |
| 卡片标题 | 14px | 600 | --text-primary | .cc-title |
| 正文/主要文本 | 14px | 400 | --text-primary | body 默认 |
| 次要说明 | 13px | 400/600 | --text-secondary | .toolbar-label、.group-title、.modal-hint |
| 辅助标签/数据 | 12px | 400 | --text-secondary / --text-tertiary | .zone-title、.field-label、.row-time、hex 数据 |
| 微标签 | 11px | 400~600 | --text-tertiary | .dir-badge、.st、.col-head、.server-proto |

- 文本透明度阶梯(Dark,Light 同构):primary `0.88` / secondary `0.65` / tertiary `0.45` / disabled `0.35`
- 等宽字体:hex/JSON/VIN/时间等数据文本统一 `--font-mono`(SFMono-Regular, Consolas, Menlo…)+ `font-variant-numeric: tabular-nums`
- 系统字体栈:-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif

---

## 7. 状态色(双主题)

| 语义 | Token | Dark | Light |
|---|---|---|---|
| 主色(品牌/交互/TX) | `--primary` | `#1677ff` | `#1677ff` |
| 成功(RX/在线) | `--success` | `#52c41a` | `#389e0d` |
| 警告(链路) | `--warning` | `#faad14` | `#d46b08` |
| 错误(危险) | `--error` | `#ff4d4f` | `#cf1322` |

辅助交互色:

| Token | Dark | Light | 用途 |
|---|---|---|---|
| `--primary-hover-bg` | rgba(22,119,255,0.1) | 0.08 | 分割条 hover、主色淡底 |
| `--row-hover-bg` | rgba(22,119,255,0.06) | 0.06 | 列表行 hover |
| `--item-hover-bg` | rgba(255,255,255,0.04) | rgba(0,0,0,0.04) | 卡片头 hover |
| `--hl-bg` | rgba(22,119,255,0.32) | 0.15 | 选中行高亮 |
| `--hl-shadow` | 0 0 0 1px 主色描边 + 外发光 | 同构弱化 | 选中强调 |
| `--success-glow` | rgba(82,194,26,0.4) | rgba(56,158,13,0.35) | 在线状态点脉冲光晕起始色 |

特殊语义色(未知橙 `#d46b08`/`#ad4e00`、加密紫 `#9254de`/`#6424c2`、告警红)双主题各自覆盖。

---

## 8. 交互动效

| 场景 | 时长/曲线 |
|---|---|
| hover 微交互(行/卡片头/分割条) | `0.15s` |
| 主题切换过渡(背景/文字/边框/阴影) | `0.2s ease` |
| 卡片折叠动画(grid-template-rows 0fr↔1fr) | `0.3s ease` |
| 侧导航宽度切换 | `0.2s ease` |
| 悬浮信息卡入场 | `0.1s ease-out`(opacity) |
| 在线状态点脉冲 | `2s ease-out infinite` box-shadow 扩散 |
| reduced-motion / 设置关动画 | 全局压到 `0.01ms` |

分割条拖拽:客户端基类 = grip 常显长条(36×3,hover 48×3 主色);服务端变体 = 12px 透明命中区 + 常显胶囊手柄(tight 32×4 / col 4×32,hover 变主色并轻微加长),同时构成 Panel 间 12px 呼吸间距;拖拽中变主色并锁定光标(body.dragging-ns/ew + user-select: none);无视觉线、无悬浮文字,交互意图纯靠手柄与光标表达。

---

## 9. 滚动条

| 参数 | 值 |
|---|---|
| 宽/高 | `8px` |
| thumb 圆角 | `4px` |
| thumb 颜色 | `--scrollbar-thumb`(Dark: rgba(255,255,255,0.18)) |
| thumb hover | `--scrollbar-thumb-hover`(0.3) |
| 轨道 | 透明 |

---

## 10. 快速对照:新页面/新组件检查单

1. **容器**:滚动页用 `.page-root`(12px 16px + gap 12px);工作台页自管高度,用 `.topbar` + `.zone` 体系。
2. **盒子**:主面板 `1px solid --border-strong + 8px 圆角 + --shadow`;子卡片 `1px solid --border-subtle + 8px 圆角`,不加阴影。
3. **内边距**:头部 `8px 16px`、内容 `16px`、卡片内容 `8px 16px 16px`、行 `4~8px 16px`。
4. **按钮**:一律 `size="small"`;主操作 primary、危险 danger/text+danger、添加 dashed、行内链接 link。
5. **间距**:区块 12px、组件 8px、紧凑 6px、微 2~4px;水平对齐线 16px。
6. **文本**:数据用 mono + tabular-nums;层级 14/13/12/11 + 三级透明度。
7. **颜色**:只用 CSS 变量与状态 token,禁止硬编码;需双主题各验一遍。
8. **动效**:hover 0.15s、结构动画 0.2~0.3s,尊重 reduced-motion。
