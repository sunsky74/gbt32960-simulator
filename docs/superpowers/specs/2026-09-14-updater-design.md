# GB/T 32960 模拟器 · 应用内更新(App Auto-Update) 设计文档

> 状态:已确认(2026-09-14 grilling 评审:18 项决议 + 11 项修正固化;2026-09-15 P2 修订:审计修订落地——白名单 host、Ed25519 离线签名、SHA256SUMS 取址等)
> 日期:2026-09-14(草案同日评审通过)
> 关联调研:Wails 自更新生态调研(2026-09-14)。关键证据:Wails v2 无官方 updater 且已明确不做(wailsapp/wails#1178,关闭于 v3);社区无成熟第三方库(全部 0~15 star 或停更 2 年以上);Wails v3 内建 `app.Updater`(仍 beta)采用 helper swap 模式与 Sparkle 同构;minio/selfupdate 的 rename-aside 替换算法经生产验证;GitHub Releases API 未认证限额 60 次/时/IP。
> 决策 ADR:`docs/adr/0001-updater-architecture.md`、`docs/adr/0002-windows-per-user-install.md`;术语:`CONTEXT.md`
> Three Pillars applicability: yes - 纯技术编码工作(Go 更新服务 + Vue 设置页接线),按 three-pillars §1 判定
> Project type: general-backend(混合前端)- 主交付物为内嵌 Wails 桌面应用的更新服务组件 + 配套设置页;不确定时按更严路径取 general-backend

---

## 1. Final Intent(最终意图)

用户(联调/测试人员)在本机使用模拟器桌面应用时,可在**应用内**完成版本更新,无需手动访问 GitHub 下载安装包:

1. 在设置中查看当前版本,手动检查更新;
2. 发现新版本后一键下载(带进度),下载产物经 SHA-256 校验;
3. 一键安装:应用自动替换自身并重启,升级完成;失败自动回滚并拉起原版本,启动时告知;
4. 启动后自动检查(默认开、可关),发现新版本时轻量提示,可"跳过此版本"。

**明确不做**:差分更新、断点续传、无感静默安装(不询问用户)、平台代码签名/公证、镜像/代理配置、指定版本回滚、更新频道(beta/stable 分离)。

## 2. Three Pillars

### Overall Business Flow

模拟器以 GitHub Releases 为唯一分发渠道(现有 release.yml 已就绪)。更新功能是"发布→分发→升级"闭环的应用侧最后一环:发布者打 `v*` tag → CI 三平台构建并发布 Release → 用户应用在设置页(或启动时)检查 `releases/latest` → 发现新版本 → 下载对应平台资产 → 校验 → 替换自身 → 重启完成升级。触发者:用户手动点击或启动时自动;消费者:使用桌面应用做联调测试的开发者。更新能力与协议功能完全解耦,不影响客户端/服务端任何运行状态。

### Current Requirement Flow

打开"设置 → 关于"面板 → 显示当前版本(如 `v0.1.0`)→ 点击"检查更新" → 有新版:展示最新版本号/发布说明(纯文本截断+展开)/资产大小 → 点击"下载"→ 进度条(可取消;无进展 120s 自动判失败)→ 完成并校验通过 → 点击"安装并重启"→ 二次确认(动态提示运行态中断)→ 应用退出 → helper 完成替换 → 应用以新版本重新启动。

分支:已是最新 → 文案提示;网络失败/限流/无发布 → 分类错误提示且不阻塞任何既有功能;校验失败 → 拒绝安装并删除产物;替换失败 → 自动回滚、拉起旧版本、下次启动 toast 告知;macOS 从 translocation 路径运行或无写权限 → 明确指引(移到 /Applications 后重试);Windows 安装目录不可写(旧 machine 范围安装)→ 提示使用安装包手动更新;用户可"跳过此版本"(自动检查对其静默、手动检查仍展示、更高版本发布后解锁)。

### Current Requirement Technical Architecture

#### Development Architecture

新增包 `internal/updater/`(纯逻辑,不 import wails):

```
internal/updater/
├── semver.go        # 极简语义化版本解析/比较(vX.Y.Z,容忍 v 前缀与不可解析输入)
├── release.go       # GitHubClient:GET releases/latest;baseURL 与 *http.Client 可注入(测试缝)
├── asset.go         # 按 GOOS/GOARCH 匹配资产(排除 installer 等干扰项)
├── download.go      # 流式下载 + 进度回调(≥100ms 节流)+ 停滞检测(120s)
├── verify.go        # SHA256SUMS 解析 + Ed25519 验签与哈希比对(内嵌公钥;验签先于解析,fail-closed)
├── helper.go        # helper 模式入口 RunHelperIfRequested():等父退出 → 同卷暂存 → 替换 →
│                    #   结果文件 → 拉起实例(失败回滚并拉起旧版)
├── apply.go         # 平台无关:定位运行路径、同卷暂存、备份/回滚算法
├── state.go         # 更新结果文件读写(last-result.json)与缓存清理
├── apply_darwin.go  # ditto 解压 + 整体替换 .app(保留一代 .bak)
├── apply_windows.go # 自身复制到 %TEMP% + rename-aside(锁重试 3 次)
└── apply_linux.go   # rename 覆盖 + 权限保留

bridge/updater_service.go  # UpdaterService:CurrentVersion()/CheckUpdate()/DownloadUpdate()/
                           #   ApplyUpdate()/CancelDownload()/ConsumeLastResult();
                           #   启动清理缓存;update:progress 事件出口
main.go                    # var version = "dev" + helper 早期拦截钩子 + Bind 追加
app.go                     # 装配、ctx 注入(shutdown 时取消下载)
```

#### Existing Architecture Fit

| 现有资产 | 复用方式 |
|---|---|
| release.yml 的 `-ldflags -X main.version=${GITHUB_REF_NAME}` | 补齐 main.go `var version` 声明完成接线(当前注入被静默忽略) |
| release.yml / wails.json 发布链 | 追加 SHA256SUMS 生成(CI)、`-installscope user`、构建前注入 wails.json info 段版本;`SHA256SUMS.sig` 由发布者离线签名后上传 |
| `bridge/forwarder.go` 事件模式 | `update:progress` 采用同款"nil-ctx 守卫 + ≥100ms 节流"推送 |
| `app.go` 的 `fwdCancel` 模式 | 下载 ctx 取消(shutdown 先行取消,emit 前查 `ctx.Err()`) |
| `bridge/wiring.go` WireContexts | UpdaterService ctx 注入(不导出 SetContext,避免进入 RPC 绑定面) |
| `SettingsCommonPanel` 规划占位行 | "启动时检查更新"由占位转正为真实开关 |
| `SettingsPage` categories 注册表 | 新增"关于"分类(第 9 项)+ `SettingsAboutPanel.vue` |
| `api/events.ts` WailsEventMap | 登记 `update:progress` 事件类型 |
| `useAppSettings`(localStorage) | 新增 `checkUpdateOnStartup` / `lastUpdateCheckAt` / `skippedVersion` 字段与持久化 |
| `navigation.ts` 导航机制 | 追加轻量"打开设置→关于"函数(toast 跳转用) |

**缺口**:全仓当前无 `net/http` 使用——标准库自足(下载/校验/重命名均标准库;macOS 解压复用系统 `ditto` 以保证符号链接/xattr 保真),**`go.mod` 零变化**。

#### New Architecture Enablement

**不引入任何新架构元素与第三方依赖**。理由(证据链):Wails v2 官方无更新器且明确不做(wailsapp/wails#1178 关闭于 v3);社区无成熟库(相关项目全部 0~15 star 或停更);`go-selfupdate` 资产命名规则与现有资产不兼容且不处理 `.app` 整体替换;`minio/selfupdate` 仅提供替换原语——算法可借鉴(rename-aside + 回滚),但不值得为此引入依赖;"helper 等待父进程退出→替换→重启"与 Wails v3 内建实现、Sparkle 流程同构(失败回滚 + 拉起旧版语义亦与 Sparkle 一致),为本场景事实标准。"不引入会怎样"的答案:无任何功能损失。

## 3. Final Acceptance Checklist

- [AC-1] (Source: Overall Business Flow) release 构建版本号经 ldflags 生效:关于面板显示 `vX.Y.Z`;未注入(dev/本地构建)显示"开发构建";系统层版本(Finder/文件属性)经发布流程注入后与应用内一致
- [AC-2] (Source: Current Requirement Flow) 手动"检查更新":有新版展示 版本号/发布说明(纯文本截断+展开)/资产大小;无新版提示"已是最新";404/限流/网络错误分类提示,且不影响任何既有功能
- [AC-3] (Source: Current Requirement Flow) 自动检查:默认开(可关)、启动后延迟 ~3s、24h 冷却、失败静默;"跳过此版本"持久化生效(自动检查对其静默、手动检查仍展示、更高版本解锁);发现新版本 toast 可点击直达"设置→关于"
- [AC-4] (Source: Current Requirement Flow) 下载:进度事件驱动进度条(≥100ms 节流),支持取消(取消后清理临时文件);无字节进展 120s 判失败;失败可重试
- [AC-5] (Source: Current Requirement Flow) 校验:Ed25519 验签失败、SHA256SUMS 缺失或哈希不匹配 → 拒绝安装 + 明确提示 + 删除下载产物(fail-closed);验证顺序=先验签后解析哈希
- [AC-6] (Source: Current Requirement Flow) macOS:helper 等父进程退出 → ditto 解压 → 同卷暂存 → 整体替换 `.app`(保留一代 `.bak`)→ `open` 重启;任一环节失败自动回滚并拉起旧版本;translocation/无写权限给出明确指引
- [AC-7] (Source: Current Requirement Flow) Windows:per-user 安装下 rename-aside 替换(失败退避重试 3 次)+ 自动重启;失败回滚并拉起旧版;`.old` 残留由下次启动清理;不可写目录给出手动更新提示
- [AC-8] (Source: Current Requirement Flow) Linux:rename 覆盖 + `0755` 权限保留 + 自动重启;失败回滚拉起旧版;目录不可写时明确报错(不尝试提权)
- [AC-9] (Source: Development Architecture) 新增 `internal/updater` 与 `bridge/UpdaterService`;事件契约 `update:progress` 按规范登记;绑定面不暴露任意 URL/路径参数;`ConsumeLastResult()` 供启动消费更新结果
- [AC-10] (Source: Existing Architecture Fit) 既有功能逐字节不变(全量测试绿);无更新场景下应用行为与现状完全一致
- [AC-11] (Source: New Architecture Enablement) `go.mod` 零变化;下载仅允许 HTTPS 白名单域名(`api.github.com` / `github.com` / `release-assets.githubusercontent.com` / `objects.githubusercontent.com`,重定向逐跳校验);SHA-256 + Ed25519 强制校验
- [AC-12] (Source: Overall Business Flow) release.yml 增强:SHA256SUMS 生成上传;Windows 构建追加 `-installscope user`;构建前注入 wails.json info 段版本;发布清单含 `SHA256SUMS.sig` 离线签名上传步骤
- [AC-13] (Source: Current Requirement Flow) 安装确认框动态提示运行态中断(客户端连接中/服务端运行中 → "将断开连接/停止服务并退出")
- [AC-14] (Source: Current Requirement Flow) 失败闭环:替换失败回滚后自动拉起旧版本;新实例启动读取结果文件,失败时 toast 告知原因与日志位置(成功静默)
- [AC-15] (Source: Existing Architecture Fit) 启动时清理更新缓存(不跨会话复用);替换仅涉及程序主体,用户配置与数据(settings.json/packs/轨迹)不受影响

## 4. 端到端业务流

```
发布者                       CI(release.yml)                用户应用                           GitHub Releases
  │  push tag v0.1.1 ──────▶│ 三平台构建 + SHA256SUMS 生成     │                                    │
  │  离线签名 ─────────────▶│ 上传 SHA256SUMS.sig             │                                    │
  │                         │ wails.json 版本注入              │                                    │
  │                         │────────── 创建 Release ─────────────────────────────────────────────▶│
  │                         │                                 │                                    │
  │                         │       启动延迟 ~3s / 手动检查 ──▶│ GET /releases/latest               │
  │                         │                                 │◀──── tag / notes / assets ────────│
  │                         │                                 │ semver 比较 → 发现新版(检查跳过版本)│
  │                         │                                 │ 下载资产(直链,进度事件;120s 停滞防护)│
  │                         │                                 │ 验签 + SHA256SUMS 校验                    │
  │                         │                                 │ 用户确认"安装并重启"                 │
  │                         │                                 │ helper:等退出 → 同卷暂存 → 替换     │
  │                         │                                 │ 成功:拉起新版 → 新版本运行          │
  │                         │                                 │ 失败:回滚 + 拉起旧版 + 结果文件     │
  │                         │                                 │ 下次启动:消费结果(失败则 toast)    │
```

## 5. 技术契约

### 5.1 检查契约

- 端点:`GET https://api.github.com/repos/sunsky74/gbt32960-simulator/releases/latest`
- 请求头:`User-Agent: gbt32960-simulator/<version>`(GitHub 必填,缺失直接 403)、`Accept: application/vnd.github+json`、`X-GitHub-Api-Version: 2022-11-28`;整体超时 10s
- 错误分类与文案:

| 场景 | 行为 |
|---|---|
| 200 且有新版本 | 返回 `{hasUpdate: true, latest, notes, publishedAt, assetName, assetSize}` |
| 200 且已是最新 | `{hasUpdate: false, latest}` |
| 404 | "暂无发布版本" |
| 403/429(限流) | "接口限流,请稍后再试"(静态文案;2026-09-15 修订:移除 retry-after 契约以对齐 P1 实现) |
| 网络错误/超时 | "无法访问 GitHub,请检查网络"(P2 起附代理提示:如使用代理请确认 TUN 模式或 HTTPS_PROXY 生效) |

- 版本比较:剥离 `v` 前缀后按 X.Y.Z 数值比较;prerelease/draft 由 `releases/latest` 语义天然排除
- `version` 为空或 `dev`:不参与检查,UI 显示"开发构建"(检查按钮禁用并说明)

### 5.2 资产匹配表

| GOOS/GOARCH | 期望资产名 | 排除规则 |
|---|---|---|
| darwin/*(universal) | `gbt32960-simulator.app.zip` | — |
| windows/amd64 | `gbt32960-simulator.exe` | 排除 `*-installer.exe` |
| linux/amd64 | `gbt32960-simulator`(无扩展名) | 排除 `.zip` / `.exe` |

匹配不到(含未收录平台如 windows/arm64)→ 返回"当前平台暂不支持自动更新"(不进入下载流程)。

### 5.3 下载与校验契约

- 下载目录:`os.UserCacheDir()/gbt32960-simulator/updates/<tag>/`;**启动时全清该缓存目录**(不跨会话复用)
- 进度事件:`update:progress` `{phase: "downloading", received, total, percent}`,≥100ms 节流;`total` 优先取响应 `Content-Length`,缺失时用 API 返回的 `assetSize`
- **停滞检测**:重置式计时(每收到数据块重置),持续 120s 无字节进展 → 判失败;失败清理后可重试
- 取消(shutdown 或用户取消)立即生效并清理未完成文件;磁盘空间不足/写入失败 → 分类错误提示
- 校验(fail-closed,两步):① 下载同 tag `SHA256SUMS.sig`,以内嵌 Ed25519 公钥验签(覆盖 `SHA256SUMS` 全文,验签先于解析);② 下载 `SHA256SUMS`(优先取 API 资产列表 `browser_download_url`,并做"非 HTML"形态校验)→ 解析(兼容 `hash  name` / `hash *name`)→ 定位目标资产行 → 比对 SHA-256。任一步失败一律拒绝安装,删除产物并提示
- 发布侧:`SHA256SUMS` 由 CI 生成(覆盖 4 个资产、排除自身);发布者用**离线私钥**本地签名生成 `SHA256SUMS.sig`(单行 base64 + keyid,为轮换留缝)并上传至 Release;签名脚本与发布清单随 P2 实施落地

### 5.4 应用与重启契约(helper 协议)

- 触发链:`ApplyUpdate()`(前置:已完成下载与校验)→ 前端二次确认(含运行态中断提示)→ Go spawn helper 进程(带 sentinel 参数 `--updater-helper` + 父 PID / 产物路径 / 目标路径 / 日志与结果文件路径)→ 延迟 ~1s(留出 UI 收尾)→ `runtime.Quit(ctx)`
- helper 逻辑(三平台统一):
  1. 等待父 PID 退出(100ms 轮询;超时 60s → 写失败结果并退出)
  2. **将产物从缓存暂存到目标同目录/同卷**(macOS 解压暂存目录置于 `.app` 父目录),保证后续 rename 为同卷原子操作
  3. 平台替换(见下表)
  4. 写结果文件(成功:目标版本;失败:原因)→ **拉起对应实例**(成功拉起新版本;失败先回滚再拉起旧版本)→ 尽力清理缓存
  5. 全程写日志至 user cache 目录(`updates/helper.log`)
- 平台替换:

| 平台 | 替换方式 | 备份/回滚 |
|---|---|---|
| macOS | `ditto -x -k` 解压到与目标同卷的暂存目录 → 校验 `.app` 结构完整 → `mv 原 .app → .bak-<ts>` → `mv 新 .app → 原位` → `open`(不带 `-n`)重启;检测到 `/AppTranslocation/` 路径时拒绝执行并指引用户移入 `/Applications` | 失败时 `mv .bak → 原位`;保留一代备份,新实例启动成功后清理 |
| Windows | helper 先将自身复制到 `%TEMP%`(避免占用目标文件)→ `rename 目标 → .old` → `rename 新 → 目标`(**失败退避重试 3 次:200ms/500ms/1s**,覆盖杀软临时锁)→ 启动新实例 | 失败回滚;`.old` 由下次启动清理;目标目录不可写 → 明确提示手动更新(不尝试提权) |
| Linux | `rename` 覆盖运行中二进制(unix 语义允许)+ `chmod 0755` → 启动新实例 | 失败回滚;保留 `.old` 至新实例启动成功 |

- 结果文件:`updates/last-result.json`(ok / targetVersion / reason? / logPath);新实例启动经 `ConsumeLastResult()` 读取并清除;失败 → toast「上次更新未成功,已回滚在 vX,详情见日志」
- 重启顺序保证:helper 仅在父进程**完全退出后**才启动实例,避免双实例重叠;无需额外单实例锁
- 数据边界:替换仅涉及程序主体;用户配置与数据(settings.json、扩展包、轨迹等)不在替换范围
- 排障日志:helper 全程写日志到 user cache 目录;失败时以结果文件携带的路径提示用户

### 5.5 事件契约(update:*)

| 事件 | 载荷 |
|---|---|
| `update:progress` | `{phase: "downloading" \| "verifying", received, total, percent}` |

命名沿用 `namespace:event` 小写约定;前端经 `api/events.ts` 的 `onWailsEvent` 订阅(与 `server:*` 同款幂等清理)。

### 5.6 前端契约

- 新增第 9 分类"关于"(`SettingsAboutPanel.vue`):当前版本、检查更新按钮、更新状态区(最新版本 / 发布说明[纯文本截断+展开] / 下载进度条+取消 / "安装并重启" / "跳过此版本")、GitHub 仓库链接
- 状态机:`idle → checking → (up-to-date | available) → downloading → ready → applying`;各态按钮与文案互斥
- 自动检查:`useAppSettings` 新增 `checkUpdateOnStartup`(默认开)、`lastUpdateCheckAt`(24h 冷却)、`skippedVersion`(跳过版本),均 localStorage 持久化;App 启动后延迟 ~3s 触发;失败静默;发现新版本且未被跳过时 `message.info` 轻提示一次,可点击跳转"设置→关于"(`navigation.ts` 轻量函数)
- 启动消费结果:App 启动调用 `ConsumeLastResult()`;失败 → toast(原因 + 日志路径);成功静默
- 安装确认框:动态检测运行态(客户端已连接 / 服务端运行中)→ 文案追加"将断开连接 / 停止服务并退出"
- 绑定面:`CurrentVersion()` / `CheckUpdate()` / `DownloadUpdate()` / `ApplyUpdate()` / `CancelDownload()` / `ConsumeLastResult()`;`ApplyUpdate` 不使用前端传入的路径(仅用服务内部状态,防绑定面路径注入)
- wailsjs 绑定随构建自动再生成;`api/events.ts` 的 `WailsEventMap` 登记 `update:progress`

### 5.7 全局约束

- `go.mod` 零变化;不新增第三方依赖(HTTP/JSON/哈希/zip 全部标准库;macOS 解压用系统 `ditto`)
- 不改变既有服务与页面行为;不影响客户端/服务端模式的任何运行状态
- 仅 HTTPS 访问白名单域名:`api.github.com` / `github.com` / `release-assets.githubusercontent.com` / `objects.githubusercontent.com`(后两者为下载 302 目标,2026-09-15 实测锁定),重定向逐跳校验
- 遵循系统代理环境变量(`HTTPS_PROXY` 等,Go 默认行为),不做 UI 配置
- 检查/下载/应用为单飞(互斥),重复触发幂等;启动时清理更新缓存(不跨会话复用)
- helper 模式在 `main()` 最早期拦截(环境变量或参数判定)后直接 `os.Exit`,不初始化 Wails

### 5.8 验证策略

- 单元(表驱动):semver 比较(含 v 前缀/非法输入)、资产匹配(三平台 + 排除项)、SHA256SUMS 解析(两种格式)、Ed25519 验签(有效/被篡改/缺失/未知 keyid)、结果文件编解码
- 桥接层:HTTP 客户端可注入(`baseURL` + `*http.Client` 注入缝,对齐 forwarder 的 `emit` 注入惯例),覆盖 200/404/限流/网络错误/校验失败/停滞 120s 路径;补 2 例:403+限流 JSON 体(无 `tag_name`)、200 缺字段——锁定"状态码优先"分类
- 真实链路:集成测试覆盖真实 Release 302 链(github.com → release-assets.githubusercontent.com;≥2 host 组合),白名单逐跳校验路径
- 替换算法:临时目录模拟应用布局执行 rename-aside/同卷暂存/回滚用例(Windows 逻辑在 CI windows runner 覆盖)
- 端到端演练:发布 `v0.1.1` 后从 `v0.1.0` 真机升级(macOS 主路径),覆盖:下载中断重试 / 校验失败 / 替换失败回滚并拉起旧版 / 跳过版本

## 6. 范围边界

**本期做**:检查/下载/校验/三平台替换与重启、"跳过此版本"、失败闭环(回滚拉起旧版 + 启动告知)、运行态中断提醒、关于面板、SHA256SUMS + Ed25519 签名发布链(离线密钥)、release.yml 增强(per-user 切换 + wails.json 版本注入)、启动缓存清理。

**本期不做(预留演进)**:差分/增量更新、断点续传、静默无感安装、平台代码签名/公证、镜像源与代理配置、指定版本回滚、更新频道(beta/stable)、全局通知中心。

**迁移说明**:Windows 自本期起安装器切换为 per-user(`%LOCALAPPDATA%\Programs\...`,见 ADR-0002);v0.1.0 时代的 machine 范围旧装机需手动重装一次(v0.1.0 发布仅数日,装机量极小,可接受)。

**文档随动**:`docs/frontend-ui-spec.md`("设置中心 8 分类"→ 9 分类 + 关于面板说明);README 特性清单;`docs/backlog.md` 收录。

## 7. 关键技术决策记录(已确认)

> D1–D18 经 2026-09-14 grilling 评审固化(D16 起为工程修正项);D19–D21 为 2026-09-15 P2 审计修订(依据 oracle 行业对标审计与用户裁决)。

| # | 决策 | 依据 | 状态 |
|---|---|---|---|
| D1 | 定位:应用内全量更新(检查→下载→校验→替换→重启),非"仅通知" | 用户需求 | 已确认 |
| D2 | 自研薄层 + 零新依赖;不引 go-selfupdate/minio,不迁 Wails v3 | 调研 + grilling;ADR-0001 | 已确认 |
| D3 | 版本单一来源 = ldflags `main.version`;系统层版本由发布流程注入保持一致 | release.yml 既有接线 + grilling A8 | 已确认 |
| D4 | 自动检查默认开、启动延迟 ~3s、24h 冷却、失败静默、可关闭 | grilling D4 | 已确认 |
| D5 | 校验:SHA256SUMS + Ed25519 签名(先验签后哈希)强制 fail-closed | grilling A9;2026-09-15 修订:落地 | 已确认 |
| D6 | 三平台统一 helper 模式;macOS 整体替换 .app(保留一代备份,失败回滚并拉起旧版) | grilling D6/A2;ADR-0001 | 已确认 |
| D7 | Windows 安装范围切换 per-user(免 UAC 自替换) | grilling D7;ADR-0002 | 已确认 |
| D8 | UI:新增"关于"分类 + Common 占位行转正 | grilling | 已确认 |
| D9 | 下载产物不跨会话复用;启动时全清缓存 | grilling A5 | 已确认 |
| D10 | "跳过此版本":持久化 skippedVersion;自动静默/手动展示/更高版本解锁 | grilling A1 | 已确认 |
| D11 | 失败闭环:helper 回滚后拉起旧版;结果文件供新实例启动告知 | grilling A2 | 已确认 |
| D12 | 安装确认框动态提示运行态中断(连接中/服务运行中) | grilling A3 | 已确认 |
| D13 | 下载停滞:无字节进展 120s 判失败(重置式计时) | grilling A4 | 已确认 |
| D14 | 轻提示可点击跳转"设置→关于"(navigation 轻机制) | grilling A6 | 已确认 |
| D15 | 发布说明纯文本截断 + 展开(不做 markdown 渲染,零依赖) | grilling A7 | 已确认 |
| D16 | 替换前将产物暂存到目标同目录/同卷,再原子交换(跨卷 rename 不可行) | 工程修正 | 已确认 |
| D17 | 域名白名单 + 重定向逐跳校验;遵循系统代理环境变量 | 工程修正 | 已确认 |
| D18 | Windows rename 退避重试(AV 锁);错误分类补齐(磁盘/写入) | 工程修正 | 已确认 |
| D19 | Ed25519 密钥托管=离线密钥+本地签名(发布清单增补签名步骤;密钥丢失=更新链断裂、轮换需旧钥引导) | 用户裁决(2026-09-15,oracle 审计) | 已确认 |
| D20 | 下载白名单补 `release-assets.githubusercontent.com`(实测 302 目标;缺此项会 fail-closed 阻断全部下载) | oracle 审计实测(2026-09-15) | 已确认 |
| D21 | SHA256SUMS 优先取 `browser_download_url` + 非 HTML 形态校验;移除 retry-after 契约;网络失败文案附代理提示 | oracle 审计建议(2026-09-15) | 已确认 |
