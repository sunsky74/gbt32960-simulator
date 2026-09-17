# 应用内更新 · Phase 3「替换与重启」实施计划

> **For agentic workers:** Recommended execution: use superpowers:ltdd for Quality-LTDD(推荐)。Alternatives: superpowers:subagent-driven-development(轻量)或 superpowers:executing-plans(内联检查点)。Steps use checkbox (`- [ ]`) syntax for tracking。

**Goal:** 交付"安装并重启"闭环:已就绪(下载 + 校验通过)的产物经前端二次确认后,由 helper 进程在三平台完成**自身替换**(macOS 整包 `.app` / Windows per-user rename-aside 退避重试 / Linux rename 覆盖 + 权限保留),成功后自动拉起新版本;失败自动回滚并拉起旧版本;新实例启动消费结果文件,失败 toast 告知(成功静默)。同时补齐"检查/下载/应用"单飞、`main()` 最早期 helper 拦截、启动清理扩展(缓存 + 陈旧备份 `.bak-*`/`.old`)、AC-13 运行态中断提醒,以及 Windows per-user 安装包切换(`-installscope user`)与平台替换逻辑的 CI 验证 + `v0.1.1` 真机演练证据。

**Architecture:** `internal/updater` 追加:`apply.go`(平台无关原语——helper 参数编解码、退避重试、父进程等待、依赖缝 `HelperDeps`、共享错误值)、`state.go`(结果文件 `last-result.json`/日志路径与编解码)、`apply_darwin.go`/`apply_windows.go`/`apply_linux.go`(平台暂存/交换/回滚/预检/启动清理)、`spawn_unix.go`/`spawn_windows.go`(helper 拉起与进程存活探测)、`helper.go`(`RunHelperIfRequested()`,由 `main()` 最早期调用,不初始化 Wails)。`bridge.UpdaterService` 追加 `ApplyUpdate()/ConsumeLastResult()`(均无参数,路径只来自服务内部状态),`WireUpdaterStartup` 扩展为"清缓存 + 清陈旧备份";`app.go` 无需新增行。前端"关于"面板补二次确认框(动态运行态文案)/`applying` 态/启动失败告知。发布链仅追加 `-installscope user` 与一个常规 CI 测试工作流。`go.mod` 零变化。

**Tech Stack:** Go 1.25(标准库 `os/exec`、`syscall`、`path/filepath`、`encoding/json`、`time`;macOS 复用系统 `ditto` 与 `open`)/ Wails v2.15(`runtime.Quit`、绑定再生成)/ Vue 3 + Ant Design Vue(`Modal.confirm`,已有先例 `SettingsAdvancedPanel.vue`)/ vitest + @vue/test-utils / GitHub Actions(ubuntu-latest + windows-latest)。

> Phase 引用:`docs/superpowers/plans/2026-09-14-updater-master-plan.md`(P3 行:AC-6/7/8/13/14 + AC-12 installscope);设计文档 `docs/superpowers/specs/2026-09-14-updater-design.md` §3(AC-6/7/8/9/12/13/14/15)、§5.4(应用与重启契约:helper 协议/平台替换表/结果文件/重启顺序/数据边界)、§5.6(前端契约:状态机 `idle→checking→(up-to-date|available)→downloading→ready→applying`、确认框、绑定面 `ApplyUpdate()`/`ConsumeLastResult()`)、§5.7(全局约束:helper 在 `main()` 最早期拦截)、§5.8(验证策略:替换算法临时目录矩阵 / CI windows runner / v0.1.1 真机端到端演练)、§6(范围边界)、§7(D6 helper 模式 / D7 per-user / D11 失败闭环 / D16 同卷暂存 / D18 Windows 退避重试);ADR:`docs/adr/0001-updater-architecture.md`、`docs/adr/0002-windows-per-user-install.md`、`docs/adr/0003-update-check-web-302.md`。

> 起点事实(2026-09-17,HEAD = `8aa9245`;写计划前逐项核实):P2 已完成并推送;`internal/updater/` 现有 `release.go`/`asset.go`/`download.go`/`verify.go`/`semver.go`/`signing_pubkey.go` 与配套测试,**无任何 apply/helper 代码**;`DownloadResult{tag,assetName,size,sha256}` 是下载返回契约,校验产物位于 `os.UserCacheDir()/gbt32960-simulator/updates/<tag>/<assetName>`;`CleanupCache()` 清 tag 子目录、保留根级 `last-result.json`/`helper.log`;`main.go` 的 `main()` 直接 `wails.Run`(无拦截钩子);`bridge.UpdaterService` 导出恰 4 方法(均无参数);`app.go startup()` 已调 `bridge.WireUpdaterStartup`;`frontend/src/components/settings/SettingsAboutPanel.vue` 已有 idle/checking/available/downloading/ready 五态;`frontend/src/api/events.ts` 已登记 `update:progress`;`.github/workflows/release.yml` 是仓库唯一工作流,Windows 构建行为 `wails build -platform windows/amd64 -clean -trimpath -nsis -webview2 download -ldflags ...`;**本机实测 Wails CLI v2.15.0 支持 `-installscope string`(machine 默认 / user)**;`wails.json` 无 installscope / 版本注入字段(P4 范围)。

## Global Constraints

> 继承自 Master Plan 全局约束(所有 Phase 隐含)与 P2 Global Constraints;以下为 P3 增补。

- `go.mod` 零变化;不新增第三方依赖(进程/文件/JSON 全部标准库;macOS 解压用系统 `ditto`、重启用 `open`)
- 不改变既有服务与页面行为;后端 `go vet ./...` + `go test ./... -count=1` + `go test -race ./...`;前端 `npm run lint` + `npm run typecheck` + `npm test -- --run` + `npm run build` 全绿
- **数据边界(AC-15)**:替换仅涉及程序主体;`<UserConfigDir>/gbt32960-simulator/**`(settings.json、packs、packstates.json 等)与轨迹文件绝不被 helper 读写或删除;替换后按 sha256 前后比对
- **helper 拦截(§5.7)**:`main()` 第一行调用 `updater.RunHelperIfRequested()`;判定 `os.Args[1] == "--updater-helper"`,命中即执行替换流程并 `os.Exit`,**绝不初始化 Wails**(不调用 `NewApp()`/`wails.Run`)
- **回滚唯一路径**:`HelperDeps.Swap` 只做两次 rename,**不自行回滚**;失败时把已产生的 `backup`(可能为空串)返回给 `HelperMain`,由 `HelperMain` 统一调用 `Rollback`——避免双回滚路径
- **拉起顺序**:helper 仅在父进程**完全退出后**(100ms 轮询 / 60s 超时)才启动实例,避免双实例重叠(无需单实例锁);重启目标**一律使用 `target` 路径**,严禁使用 helper 自身路径(macOS 替换后自身路径已指向新包,回滚必须从 target 拉起旧版)
- **同卷暂存(D16)**:暂存位置必须在目标同目录(同卷才可原子 rename);缓存根目录不新增持久文件——`CleanupCache` 白名单仍是 `last-result.json` / `helper.log` 两项
- **结果文件契约(§5.4)**:`updates/last-result.json` 固定 schema `{ok, targetVersion, reason?, logPath}`;原子写(临时文件 + rename);`ConsumeLastResult()` 读取即清除;helper 日志单文件 `updates/helper.log`,打开时 >1MiB 截断(无轮转)
- **故障注入仅构建期**:`var helperFault string` 只允许经 `-X gbt32960-simulator/internal/updater.helperFault=stage|swap|relaunch` 注入(仅演练/测试用);**不得**提供任何运行时开启途径(环境变量 / 命令行参数 / 绑定面);发布构建为空串(零效果),T9 演练命令为唯一注入点
- **绑定面无路径参数**:`ApplyUpdate()`/`ConsumeLastResult()` 均无参数;`ApplyUpdate` 返回 `error`(成功即已拉起 helper 并安排退出);前端进入 `applying` 的唯一条件 = `ApplyUpdate()` 成功返回
- 前端错误展示经既有 `errText(e)` 解包(Wails 将 Go error 包为 `Error`);**文案内容不得改写**
- Git:所有操作前缀 `GIT_MASTER=1`;Conventional Commits(中文摘要);逐任务提交;注释/提交信息/文档全中文
- Windows/Linux 构建标签下的用例无法在本机(darwin)执行:本机以 `GOOS=windows|linux go vet ./...` 保证编译,行为由 CI runner(T8)与真机演练(T9)覆盖

### P3 进程级契约(实现不得偏离)

- **helper 参数格式(唯一契约)**:
  `gbt32960-simulator --updater-helper --parent-pid <pid> --artifact <缓存产物路径> --target <运行中的 exe 或 .app 根> --tag <目标版本> --result <updates/last-result.json> --log <updates/helper.log>`
  解析:`IsHelperInvocation(args)` 仅认 `args[1] == "--updater-helper"`;其余 flag 用 `flag.NewFlagSet(..., ContinueOnError)` 解析 `args[2:]`;`Encode()` 生成除 argv[0] 外的完整参数表(首元素为 sentinel)
- **ApplyUpdate 触发链(§5.4)**:守卫(dev/单飞/就绪产物存在)→ `RunningTarget()` → `PreflightTarget()` → 清上次结果文件 → `SpawnHelper(args)` → 置 `applying` → goroutine `time.Sleep(~1s)` → `runtime.Quit(ctx)`;`ApplyUpdate` 在 **spawn 成功后立即返回 nil**(UI 有 ~1s 绘制 `applying` 态);spawn 失败**不退出应用**
- **helper 执行链(§5.4)**:等待父退出 → 平台预检 → 同卷暂存 → 交换(写结果文件先于拉起)→ 拉起新版本;任一环节失败 → 回滚(若已产生备份)→ 写失败结果 → 拉起旧版本(超时分支除外:父进程仍在,不拉起)
- **平台替换表(§5.4 原表)**:
  | 平台 | 暂存 | 替换 | 回滚 | 重启 | 备份清理 |
  |---|---|---|---|---|---|
  | macOS | `ditto -x -k` 解压到 `<父目录>/.gbt32960-update-<pid>.staging/` + `.app` 结构校验 | `mv 原 .app → .bak-<ts>`;`mv 新 .app → 原位` | `mv .bak → 原位` | `open`(不带 `-n`) | 新实例启动清 `.bak-*` 与 `.staging` |
  | Windows | 复制到 `<目录>/<base>.new-<pid>` | `rename 目标 → .old`;`rename 新 → 目标`(首次失败后按 200ms/500ms/1s 退避重试 3 次,共 4 次尝试) | rename 回位(同样重试) | `spawnDetached`(`DETACHED_PROCESS`) | 下次启动清 `.old`、`<base>.new-*`、`%TEMP%/gbt32960-helper-*.exe` |
  | Linux | 复制到 `<目录>/<base>.new-<pid>` + `chmod 0755` | `rename` 覆盖运行中二进制(unix 语义允许)+ `chmod 0755` | rename 回位 + `chmod 0755` | `spawnDetached`(`Setsid`) | 下次启动清 `.old` 与 `<base>.new-*` |

### P3 新增中文文案(错误值即用户可见文案;分类、fail-closed、可操作)

- Apply 守卫类(前端 toast,原文透出):
  - `更新已在进行中`(二次 `ApplyUpdate` / applying 期间下载或检查)
  - `更新正在下载中,请稍候`(下载中触发安装)
  - `更新包未就绪,请先下载`(无内存就绪产物,或产物文件已不在)
  - `已是最新版本,无需安装`(版本比较守卫)
  - `无法启动更新进程,请手动更新`(helper 拉起失败)
  - `无法定位应用位置,请手动更新`(三平台共用:可执行文件/bundle 定位失败)
- 平台预检指引(拒绝执行且**不退出应用**):
  - macOS:`应用正从临时位置运行,无法自动更新;请将应用移到 /Applications 后重试`(App Translocation)
  - macOS:`应用所在目录不可写,无法自动更新;请将应用移到 /Applications 后重试`
  - Windows:`安装目录不可写,无法自动更新;请下载安装包手动更新`(不尝试提权,ADR-0002)
  - Linux:`程序所在目录不可写,无法自动更新;请检查目录权限后重试`(不尝试提权)
- helper 结果文件 `reason`(仅落盘与日志,不经前端二次改写):
  `等待应用退出超时` / `更新包不存在` / `暂存失败` / `替换失败` / `无法启动新版本` / `回滚失败,请手动重新安装`
- 启动告知(toast,前端组合,§5.4 固定文案):
  `上次更新未成功,已回滚在 vX,详情见日志`(X = 当前运行版本;成功或无记录 → 静默)

---

## Phase Final Acceptance Checklist (Refined from Spec) - MUST

> 说明:各 Refinement 中的 `go test` 目标文件由本计划任务创建;**文件创建前命令不可运行**(计划中已逐条标注创建任务);`grep`/`GOOS=... go vet`/四件套等既有基线命令以当前仓库为准则可运行。所有命令均在仓库根目录执行(frontend 命令除外)。

- [ ] [PAC-1] (Source: Development Architecture;← Global AC-9/15) 平台无关原语与结果闭环:helper 参数编解码往返一致;重试语义 = 首次失败后按 200ms/500ms/1s 退避 3 次;父进程等待 100ms 轮询 / 60s 超时;结果文件 `{ok,targetVersion,reason?,logPath}` 原子读写与"读取即清除";`helper.log` 超限截断;替换仅涉程序主体(同目录用户数据逐字节不变)。
  Refinement: `go test ./internal/updater/ -run 'TestHelperArgs|TestRenameWithRetry|TestWaitParentExit|TestLastResult|TestUpdatesRoot|TestOpenHelperLog|TestHelperEntry' -count=1 -v`(T1 创建)→ 全 PASS:往返用例含**含空格路径**;`IsHelperInvocation` 对 `["app"]`/`["app","--updater-helper-x"]` 均 false;重试用例断言调用次数(第 3 次失败第 4 次成功 → 4 次且成功;全程失败 → `替换失败`);等待用例 alive 立即 false 时耗时 <100ms、恒活 + `timeout=120ms` → `等待应用退出超时`;结果文件缺文件 → `(nil,nil)`、非法 JSON → error、`Clear` 幂等、无 `.tmp` 残留。`go test ./internal/updater/ -run 'TestReleaseDirAndCleanup' -count=1 -v`(P2 回归:路径委托 `UpdatesRoot()` 后行为不变)。`go test ./internal/updater/ -run 'TestHelperMainDataBoundary' -count=1 -v`(T5 创建)→ 同目录 `settings.json`/`packs/x.json` 逐字节不变。
- [ ] [PAC-2] (Source: Current Requirement Flow;← Global AC-6) macOS:helper 等父进程退出 → `ditto -x -k` 解压到同卷暂存 → `.app` 结构校验 → `.bak-<ts>` 保留一代 → 整体替换 → `open` 重启;任一环节失败自动回滚并拉起旧版本;translocation / 无写权限给出精确指引且不退出应用。
  Refinement: `go test ./internal/updater/ -run 'TestDarwin' -count=1 -v`(T2 创建;本机 darwin 实跑)→ 全 PASS:用测试内 `ditto -c -k --keepParent` 生成的真实 `.app.zip` 经 Stage 得到 `<父目录>/.gbt32960-update-<pid>.staging/X.app` 且 `Contents/MacOS/x` 可执行;文本冒充 zip / 缺 `Contents/MacOS` → `暂存失败`;`bundleRootFrom("/Applications/A.app/Contents/MacOS/A") == "/Applications/A.app"`、裸二进制 → `无法定位应用位置,请手动更新`;`/AppTranslocation/` 路径 → 精确文案 `应用正从临时位置运行,无法自动更新;请将应用移到 /Applications 后重试`;父目录 `chmod 0555` → 精确文案 `应用所在目录不可写,无法自动更新;请将应用移到 /Applications 后重试`;Swap 产出备份名形如 `A.app.bak-YYYYMMDD-HHMMSS` 且 Rollback 还原、`Rollback("")` 幂等;`TestDarwinCleanupStaleBackups` 仅删 `.bak-*` 与 `.gbt32960-update-*.staging`、无关文件保留。`grep -n "AppTranslocation" internal/updater/apply_darwin.go` 非空。真机主路径证据见 T9(S1/S4)。
- [ ] [PAC-3] (Source: Current Requirement Flow;← Global AC-7) Windows:helper 自复制到 `%TEMP%` → `rename 目标 → .old` → `rename 新 → 目标`(200ms/500ms/1s 退避重试 3 次,覆盖杀软临时锁)→ 启动新实例;失败回滚;`.old` 由下次启动清理;不可写目录提示手动更新(不尝试提权)。
  Refinement: 本机(交叉编译检查)`GOOS=windows GOARCH=amd64 go vet ./...` 无输出;CI windows runner(`.github/workflows/test.yml` 的 `go-windows` job,T8 创建)`go test ./internal/updater/ -run 'TestWindows' -count=1 -v`(T3 创建)→ 全 PASS:Stage 复制到 `<目录>/<base>.new-<pid>`;`renameRetries` 与 `[200ms,500ms,1s]` 逐项相等;Swap 前清理陈旧 `.old`;Swap/Rollback 全矩阵(含 staged 缺失 → `替换失败`);`TestWindowsProcessAlive`(self → true;`cmd /c exit` 的 pid 轮询转 false);`TestWindowsCleanupStaleBackups` 覆盖 `.old`、`<base>.new-*`、`%TEMP%/gbt32960-helper-*.exe` 且无关文件保留。真机走查见 T9 第 3 步(`icacls` 制造不可写 → 精确文案 `安装目录不可写,无法自动更新;请下载安装包手动更新`;安装路径位于 `%LOCALAPPDATA%\Programs\...`)。
- [ ] [PAC-4] (Source: Current Requirement Flow;← Global AC-8) Linux:`rename` 覆盖运行中二进制 + `chmod 0755` + 自动重启;失败回滚拉起旧版;`.old` 保留至新实例启动成功;目录不可写明确报错(不尝试提权)。
  Refinement: 本机 `GOOS=linux GOARCH=amd64 go vet ./...` 无输出;CI ubuntu runner(`go-linux` job)`go test ./internal/updater/ -run 'TestLinux' -count=1 -v`(T4 创建)→ 全 PASS:Stage 复制 + 0755;Swap 后 `<target>.old` 存在(保留语义)且 target == 暂存内容、权限 0755;Rollback 还原 + 0755;父目录 `chmod 0555` → 精确文案 `程序所在目录不可写,无法自动更新;请检查目录权限后重试`;cleanup 仅删 `.old` 与 `<base>.new-*`。
- [ ] [PAC-5] (Source: Development Architecture;← Global AC-9/14/15) helper 早期拦截 + 绑定面扩展 + 启动清理/消费:`main()` 首行拦截且不初始化 Wails;`ApplyUpdate()`/`ConsumeLastResult()` 无参数;单飞与守卫文案;spawn 成功才进入 `applying` 并 ~1s 后 `runtime.Quit`;`ConsumeLastResult` 读取即清除;`WireUpdaterStartup` = 清缓存 + 清陈旧备份。
  Refinement: `grep -n "^func (s \*UpdaterService) [A-Z]" bridge/updater_service.go | wc -l` 输出 `6`(P3 前为 4,可作基线);`grep -n "RunHelperIfRequested" main.go` 恰 1 条且在 `NewApp()` 之前;`go test ./bridge/ -run 'TestUpdaterServiceApply|TestUpdaterServiceConsume|TestUpdaterServiceStartup' -count=1 -v`(T6 创建)→ 全 PASS:四类守卫文案(dev/未就绪/下载中/二次调用);成功路径断言 `args.ParentPID == os.Getpid()`、`args.Artifact == s.ready.Path`、`args.Tag == s.ready.Tag`、`args.Result/Log` 位于缓存根、quit 在注入 `quitDelay=10ms` 后恰 1 次;spawn 失败 → `无法启动更新进程,请手动更新`、quit 零调用、`applying` 保持 false;`ConsumeLastResult` 第二次返回 `Present=false`、缺文件 `Present=false`、损坏 JSON → error。`grep -n "CleanupStaleBackups" bridge/wiring.go` 非空;`go test ./bridge/ -run TestUpdaterService -count=1`(P2 回归)全过;`wails generate module && grep -n "ApplyUpdate\|ConsumeLastResult" frontend/wailsjs/go/bridge/UpdaterService.d.ts` 各 1 条。
- [ ] [PAC-6] (Source: Current Requirement Flow;← Global AC-13/14 前端) 前端:就绪态出现"安装并重启";二次确认框按运行态动态追加中断文案(四种组合精确匹配);确认后进入 `applying` 态(文案 + 按钮不可再点);`ApplyUpdate` 拒绝 → 纯文案 toast 且保持可重试;启动消费失败结果 → 固定 toast 文案,成功/无记录静默。
  Refinement: `cd frontend && npm test -- --run src/components/settings/SettingsAboutPanel.test.ts src/composables/useUpdater.test.ts`(T7 创建)→ 全 PASS 且精确断言:`安装过程中将退出。`(无运行态)/`安装过程中将断开连接并退出。`(客户端 `State()=="online"`)/`安装过程中将停止服务并退出。`(`Status().running==true`)/`安装过程中将断开连接、停止服务并退出。`(两者);确认后 `ApplyUpdate` 恰 1 次且出现 `正在安装并重启`;失败用例 `message.error('更新包未就绪,请先下载')` 且不出现 `正在安装并重启`;`lastResultToast`:present=false 或 ok=true → null,present&&!ok → `上次更新未成功,已回滚在 v0.1.0,详情见日志`。`grep -n "lastResultToast\|ConsumeLastResult" frontend/src/App.vue` 各 1 条。`cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build` 全 exit 0。
- [ ] [PAC-7] (Source: Overall Business Flow;← Global AC-12 installscope 部分 + AC-10/11) 发布链与 CI:Windows 构建追加 `-installscope user`;新增常规 CI 测试工作流并覆盖 windows/ubuntu runner;`go.mod` 零变化;既有功能回归全绿。
  Refinement: `grep -n "installscope user" .github/workflows/release.yml` 恰 1 条(Windows 构建行,T8);`grep -n "windows-latest\|ubuntu-latest" .github/workflows/test.yml` 覆盖两平台(T8);CI 记录:test.yml 的 `go-windows`/`go-linux`/`frontend` 三 job 通过(证据:Run URL + 关键日志截取);`GOOS=windows GOARCH=amd64 go vet ./... && GOOS=linux GOARCH=amd64 go vet ./... && go vet ./...` 无输出;`go test ./... -count=1 && go test -race ./...` exit 0;`git diff --stat $(git merge-base HEAD origin/main)..HEAD -- go.mod go.sum` 输出为空。
- [ ] [PAC-8] (Source: Current Requirement Flow;← Global AC-6/7/8/14) 真机端到端演练:发布 `v0.1.1`(含离线 Ed25519 签名)后完成 macOS 主路径升级(正常升级 / 下载中断重试 / 校验失败 / 注入基线下替换失败回滚 + 拉起旧版 / 跳过版本);数据指纹前后一致;Windows 真机走查完成或显式标注为遗留风险。
  Refinement: 按 T9 清单逐项执行并留存证据于 `.superpowers/sdd/updater-p3/rehearsal-checklist.md` + `.superpowers/sdd/updater-p3/evidence/<scenario>/`(命令 + 输出、`helper.log` 与 `last-result.json` 副本、版本截图、`.bak-*`/`.old` 清理前后 `ls -l`、数据文件 sha256 前后、时间戳);判定 = 每场景"预期 / 实际 / 证据路径"三列填写完整且 `判定=PASS`;Windows 未走查时必须在报告中写明外部前置缺失与风险等级。

---

### Task 1: `internal/updater/apply.go` + `state.go` — 平台无关原语与结果文件编解码

**Level:** L2
**Level Rationale:** 纯函数与文件编解码,无网络、无进程、无平台分支;但它是三平台实现与 helper 协议、结果闭环的共用契约(参数格式/重试语义/错误值错一处则全线返工),测试矩阵从宽。
**Linked Acceptance Items:** PAC-1(并作为 PAC-2/3/4/5 的共享契约)
**Task Gate:** task reviewer + focused checks(本任务单测矩阵)

**Files:**
- Create: `internal/updater/apply.go`
- Create: `internal/updater/apply_test.go`
- Create: `internal/updater/state.go`
- Create: `internal/updater/state_test.go`
- Modify: `internal/updater/download.go`(仅 `ReleaseDir`/`CleanupCache` 改为委托 `UpdatesRoot()`;行为逐字节不变,P2 既有测试为回归门)

**Interfaces:**
- Consumes: 标准库 `os`/`os/exec`/`path/filepath`/`encoding/json`/`time`;P2 的 `Artifact`(不变)。
- Produces(本任务定义的契约;T2–T6 依赖):
  ```go
  // ---- apply.go ----
  const HelperSentinel = "--updater-helper"

  type HelperArgs struct {
      ParentPID int
      Artifact  string
      Target    string
      Tag       string
      Result    string
      Log       string
  }
  func IsHelperInvocation(args []string) bool            // 仅认 args[1] == HelperSentinel
  func (a HelperArgs) Encode() []string                  // [sentinel, --parent-pid, N, --artifact, ...](不含 argv[0])
  func ParseHelperArgs(args []string) (HelperArgs, error) // 解析 args[2:] 起;必填缺失/非数字/未知 flag → error

  // HelperDeps helper 协议依赖缝(生产 = DefaultDeps();测试注入 fake)。Stage 的 cleanup 可为 nil。
  type HelperDeps struct {
      WaitParent func(pid int, poll, timeout time.Duration) error
      Preflight  func(target string) error
      Stage      func(artifact, target string) (staged string, cleanup func(), err error)
      Swap       func(staged, target string) (backup string, err error) // 失败返回已产生的 backup(""=未产生);不自行回滚
      Rollback   func(backup, target string) error                      // backup == "" → nil(幂等)
      Relaunch   func(target string) error
  }
  func DefaultDeps() HelperDeps                  // 平台实现(T2/T3/T4)
  func WaitParentExit(pid int, poll, timeout time.Duration, alive func(int) bool) error
  func RenameWithRetry(old, new string, retries []time.Duration) error // 首次失败后逐次退避重试;len(retries)=重试次数
  func RunningTarget() (string, error)           // 平台实现
  func PreflightTarget(target string) error      // 平台实现
  func SpawnHelper(args HelperArgs) error        // 平台实现(spawn_unix.go / spawn_windows.go)
  func CleanupStaleBackups() error               // 平台实现;启动清理(尽力而为)

  // 共享错误值(文案即用户可见文案 / 结果 reason)。
  var (
      ErrTargetNotFound    = errors.New("无法定位应用位置,请手动更新")
      ErrParentWaitTimeout = errors.New("等待应用退出超时")
      ErrArtifactMissing   = errors.New("更新包不存在")
      ErrStageFailed       = errors.New("暂存失败")
      ErrSwapFailed        = errors.New("替换失败")
      ErrRelaunchFailed    = errors.New("无法启动新版本")
      ErrRollbackFailed    = errors.New("回滚失败,请手动重新安装")
  )

  // ---- state.go ----
  type LastResult struct { // 文件 schema 固定(§5.4)
      OK            bool   `json:"ok"`
      TargetVersion string `json:"targetVersion"`
      Reason        string `json:"reason,omitempty"`
      LogPath       string `json:"logPath"`
  }
  type ApplyOutcome struct { // 绑定面返回(多一个 present 标志:区分"无记录"与"失败记录")
      Present       bool   `json:"present"`
      OK            bool   `json:"ok"`
      TargetVersion string `json:"targetVersion"`
      Reason        string `json:"reason"`
      LogPath       string `json:"logPath"`
  }
  func AsOutcome(r *LastResult) ApplyOutcome                   // nil → {Present:false}
  func UpdatesRoot() (string, error)                            // UserCacheDir()/gbt32960-simulator/updates
  func LastResultPath() (string, error)                         // <root>/last-result.json
  func HelperLogPath() (string, error)                          // <root>/helper.log
  func WriteLastResult(r LastResult) error                      // 临时文件 + rename 原子写;MkdirAll 根目录
  func ReadLastResult() (*LastResult, error)                    // 不存在 → (nil, nil)
  func ClearLastResult() error                                  // 不存在视为成功
  func OpenHelperLog() (*os.File, error)                        // O_CREATE|O_APPEND|O_WRONLY,>1MiB 先截断
  ```

- [ ] **Step 1: 写失败测试(完整矩阵)**

创建 `internal/updater/state_test.go` 与 `internal/updater/apply_test.go`(复用 P2 已在 `download_test.go` 声明的 `redirectUserCache(t)`;同包共享,**勿重复声明**):

```go
// ---- state_test.go 用例与断言 ----
// TestUpdatesRootPaths       : UpdatesRoot() 下 LastResultPath()==<root>/last-result.json、HelperLogPath()==<root>/helper.log
// TestLastResultCodec        : 写→读往返(成功:reason 空;失败:reason=替换失败);MkdirAll 缺失目录自动创建;
//                              缺文件 → (nil,nil);非法 JSON → error;字段名与 schema 逐字一致(读原始字节断言
//                              {"ok":..,"targetVersion":..,"reason":..,"logPath":..} 与 omitempty 行为)
// TestLastResultClearIdempotent: 写→Clear→再 Clear(均无 error);Clear 后 Read → (nil,nil);无 `.tmp` 残留(Glob)
// TestOpenHelperLogTruncates : 预置 >1MiB helper.log → OpenHelperLog → Stat().Size()==0;追加写后可读回
// TestAsOutcome              : nil → {Present:false};失败记录 → Present=true/OK=false/Reason/LogPath 透出

// ---- apply_test.go 用例与断言 ----
// TestHelperArgsRoundtrip    : Encode → exec 角度拼 args(前缀假 exe)→ IsHelperInvocation==true →
//                              ParseHelperArgs 与源值逐字段相等;路径含空格与中文不变形
// TestHelperArgsRejections   : 缺 sentinel(IsHelperInvocation false)/ 缺 --target / --parent-pid=abc /
//                              未知 flag → error
// TestRenameWithRetryRetries : 注入 rename 假函数(renameWithRetry 的内部缝):
//                              1 败 1 成 → 2 次调用且成功;3 败 1 成 → 4 次调用;4 败 → ErrSwapFailed;
//                              retries 为空 → 仅 1 次尝试
// TestWaitParentExit         : alive 立即 false → nil 且耗时 < poll;alive 恒 true + timeout=120ms →
//                              ErrParentWaitTimeout 且耗时 < 600ms;alive 第 3 次 false → nil
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/updater/ -run 'TestUpdatesRoot|TestLastResult|TestOpenHelperLog|TestAsOutcome|TestHelperArgs|TestRenameWithRetry|TestWaitParentExit' -v
```

预期:编译失败(`UpdatesRoot`/`HelperArgs`/`RenameWithRetry` 等未定义)。

- [ ] **Step 3: 实现 `internal/updater/apply.go` 与 `state.go`**

要点(骨架;完整实现由执行者按契约补齐):

```go
// apply.go
func IsHelperInvocation(args []string) bool { return len(args) > 1 && args[1] == HelperSentinel }

func (a HelperArgs) Encode() []string {
    return []string{HelperSentinel,
        "--parent-pid", strconv.Itoa(a.ParentPID),
        "--artifact", a.Artifact, "--target", a.Target,
        "--tag", a.Tag, "--result", a.Result, "--log", a.Log}
}

// RenameWithRetry = renameWithRetry(os.Rename, ...) 的导出包装;renameWithRetry 携带可注入 rename 缝(测试用,不导出)。
func RenameWithRetry(old, new string, retries []time.Duration) error {
    return renameWithRetry(os.Rename, old, new, retries)
}

// WaitParentExit 以 poll 间隔轮询 alive,直至父进程退出或 timeout 到期(§5.4:100ms / 60s)。
func WaitParentExit(pid int, poll, timeout time.Duration, alive func(int) bool) error {
    deadline := time.Now().Add(timeout)
    for {
        if !alive(pid) {
            return nil
        }
        if time.Now().After(deadline) {
            return ErrParentWaitTimeout
        }
        time.Sleep(poll)
    }
}

// state.go
func WriteLastResult(r LastResult) error {
    p, err := LastResultPath()
    // MkdirAll(根) → 临时文件 <root>/.last-result.json.tmp-<ts> → json.MarshalIndent → rename → 失败清理临时文件
}
```

`download.go` 的委托重构(行为不变):

```go
// ReleaseDir 委托 UpdatesRoot(),分片仅供测试与历史调用;tag 清洗逻辑不变。
func ReleaseDir(tag string) (string, error) {
    root, err := UpdatesRoot()
    if err != nil { return "", err }
    return filepath.Join(root, safeTag(tag)), nil
}
// CleanupCache 读取 root 的条目并保留 last-result.json / helper.log(逻辑不变,仅取 root 的方式改为 UpdatesRoot())。
```

- [ ] **Step 4: 运行确认通过 + 回归 + vet**

```bash
go test ./internal/updater/ -run 'TestUpdatesRoot|TestLastResult|TestOpenHelperLog|TestAsOutcome|TestHelperArgs|TestRenameWithRetry|TestWaitParentExit' -count=1 -v
go test ./internal/updater/ -count=1
go vet ./internal/updater/
GOOS=windows GOARCH=amd64 go vet ./internal/updater/
GOOS=linux GOARCH=amd64 go vet ./internal/updater/
```

预期:全部 PASS;`go vet` 无输出;跨平台 vet 无输出(此时尚无平台文件,应自然通过)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/apply.go internal/updater/apply_test.go internal/updater/state.go internal/updater/state_test.go internal/updater/download.go
GIT_MASTER=1 git commit -m "feat(updater): 替换原语与结果文件编解码——helper 参数契约/退避重试/父进程等待/原子结果文件"
```

---

### Task 2: macOS 替换(`apply_darwin.go` + `spawn_unix.go`)

**Level:** L3
**Level Rationale:** 整包替换是 P3 最高危路径(AC-6):ditto 解压、同卷暂存、bundle 结构校验、translocation/权限预检与 `.bak-<ts>` 一代备份任何一处出错都会损坏用户安装;且是本机唯一可全真验证的平台(macOS 主路径)。
**Linked Acceptance Items:** PAC-2、PAC-1(协议),PAC-8(真机证据)
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `internal/updater/apply_darwin.go`
- Create: `internal/updater/apply_darwin_test.go`
- Create: `internal/updater/spawn_unix.go`
- Create: `internal/updater/spawn_unix_test.go`

**Interfaces:**
- Consumes: T1 的 `HelperArgs`/`HelperDeps`/`RenameWithRetry`/错误值;系统 `ditto`、`open`;`os/exec`、`syscall`。
- Produces:
  ```go
  // ---- apply_darwin.go (//go:build darwin) ----
  var (
      ErrTranslocated      = errors.New("应用正从临时位置运行,无法自动更新;请将应用移到 /Applications 后重试")
      ErrTargetNotWritable = errors.New("应用所在目录不可写,无法自动更新;请将应用移到 /Applications 后重试")
  )
  func RunningTarget() (string, error)                 // os.Executable→EvalSymlinks→bundleRootFrom
  func bundleRootFrom(exePath string) (string, error)  // 向上取最外层 *.app;非 bundle → ErrTargetNotFound(纯函数,可直接测)
  func PreflightTarget(target string) error            // translocation 前缀 + 目录可写探针(os.CreateTemp 于父目录)
  func DefaultDeps() HelperDeps
  func CleanupStaleBackups() error                     // 删 <父目录>/*.bak-* 与 .gbt32960-update-*.staging(尽力而为)
  func validateBundle(app string) error                // Contents/Info.plist 存在 ∧ Contents/MacOS 至少 1 个可执行常规文件

  // ---- spawn_unix.go (//go:build unix) ----
  func processAlive(pid int) bool                      // syscall.Kill(pid,0):ESRCH→false;EPERM→true(有权限差异时保守视为存活)
  func spawnDetached(exe string, args []string) (*os.Process, error) // Setsid;Start 后 Release
  func SpawnHelper(args HelperArgs) error              // os.Executable() + args.Encode(),spawnDetached
  ```
  `DefaultDeps()` 组合(骨架):
  ```go
  func DefaultDeps() HelperDeps {
      return HelperDeps{
          WaitParent: func(pid int, poll, timeout time.Duration) error {
              return WaitParentExit(pid, poll, timeout, processAlive)
          },
          Preflight: PreflightTarget,
          Stage:     stageDarwinBundle, // ditto -x -k <zip> <stagingDir> → validateBundle → 返回内层 .app
          Swap:      swapDarwinBundle,  // rename target→<target>.bak-<20060102-150405>;rename staged→target;失败返回已产生 backup
          Rollback:  rollbackRename,    // 通用:backup=="" → nil;否则 rename backup→target
          Relaunch:  func(target string) error { return exec.Command("open", target).Run() },
      }
  }
  ```

- [ ] **Step 1: 写失败测试(断言清单;darwin 标签)。**测试自造 bundle 与 zip,不依赖网络与真实安装:

```
TestDarwinRunningTargetBundleRoot : bundleRootFrom("/Applications/A.app/Contents/MacOS/A")=="/Applications/A.app";
                                    bundleRootFrom("/Applications/A.app")=="/Applications/A.app";
                                    bundleRootFrom("/tmp/plain-bin") → ErrTargetNotFound
TestDarwinPreflight               : "/AppTranslocation/xxx" → 精确 ErrTranslocated 文案;
                                    t.TempDir() → nil;chmod 0555 父目录 → 精确 ErrTargetNotWritable 文案;
                                    target 不存在 → ErrTargetNotFound
TestDarwinStageExtracts           : 测试内 ditto -c -k --keepParent 打包 t.TempDir()/X.app(含 Contents/Info.plist
                                    与 Contents/MacOS/x 可执行)→ Stage 后 staged 位于 <父目录>/.gbt32960-update-<pid>.staging/X.app
                                    且 Contents/MacOS/x 仍可执行;cleanup() 后暂存目录消失
TestDarwinStageRejections         : 文本文件冒充 zip → ErrStageFailed;合法 zip 但缺 Contents/MacOS → ErrStageFailed
TestDarwinSwapAndRollback         : Swap:backup 名匹配 ^A\.app\.bak-\d{8}-\d{6}$、target 内容==staged、backup 内容==原 target;
                                    Rollback(backup,target) 还原;Rollback("",target) → nil;staged 缺失 → err 且 backup 语义正确
TestDarwinCleanupStaleBackups     : 造 A.app.bak-20260101-000000/、.gbt32960-update-999.staging/(含文件)、keep.txt
                                    → 仅 keep.txt 保留
TestSpawnUnixDetached             : spawnDetached("/bin/sleep", []string{"30"}) → 进程存在(processAlive(pid)==true),
                                    Kill 后 processAlive==false;返回的 Process 已 Release(不阻塞 Wait)
TestSpawnUnixProcessAliveSelf     : processAlive(os.Getpid()) == true
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/updater/ -run 'TestDarwin|TestSpawnUnix' -v
```

预期:编译失败(`bundleRootFrom`/`PreflightTarget`/`DefaultDeps`/`spawnDetached` 未定义)。

- [ ] **Step 3: 实现。**关键点(完整实现由执行者按契约补齐):
  - `stageDarwinBundle`:暂存目录 = `filepath.Join(filepath.Dir(target), fmt.Sprintf(".gbt32960-update-%d.staging", os.Getpid()))`(同卷,§5.4);`exec.Command("ditto", "-x", "-k", artifact, stagingDir)`(保留符号链接/xattr);解压后要求且仅要求存在 `<stagingDir>/<target 基名>` 是目录,再 `validateBundle`;失败一律 `ErrStageFailed`(日志由 T5 的 helper 记录)
  - `swapDarwinBundle`:`os.Rename(target, target+".bak-"+time.Now().Format("20060102-150405"))` → `os.Rename(staged, target)`;第二步失败返回 `(backup, ErrSwapFailed)`(不自行回滚)
  - 预检的"可写探针"实现为 `writableProbe(dir string) error`(`os.CreateTemp(dir, ".gbt32960-writecheck-*")` + Close + Remove);T3/T4 复用同一探针(T1 不含平台代码,故探针落在各平台文件;macOS 与 Linux 可共置 `apply_unix.go`?——**决定:探针为 3 行函数,在 darwin/linux 两文件内各自实现,不为此新增共享文件**)
  - `spawn_unix.go` 的 `spawnDetached`: `SysProcAttr{Setsid: true}`;`Start()` 成功后 `Process.Release()`,避免父进程退出前持有句柄

- [ ] **Step 4: 运行确认通过 + 全包回归 + vet**

```bash
go test ./internal/updater/ -run 'TestDarwin|TestSpawnUnix' -count=1 -v
go test ./internal/updater/ -count=1
go vet ./internal/updater/
GOOS=linux GOARCH=amd64 go vet ./internal/updater/   # spawn_unix.go 交叉编译检查
GOOS=windows GOARCH=amd64 go vet ./internal/updater/ # 此时 windows 平台符号仍缺 DefaultDeps,预期失败 → 由 T3 补齐
```

预期:`TestDarwin*`/`TestSpawnUnix*` 全 PASS;`go vet` 无输出;`GOOS=linux` vet 通过;`GOOS=windows` vet **预期失败**(`DefaultDeps`/`RunningTarget` 未定义)——T3 补 windows 实现后必须全绿,该命令已列入 PAC-3。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/apply_darwin.go internal/updater/apply_darwin_test.go internal/updater/spawn_unix.go internal/updater/spawn_unix_test.go
GIT_MASTER=1 git commit -m "feat(updater): macOS 整包替换——ditto 暂存/结构校验/bak 一代备份/translocation 与权限预检"
```

---

### Task 3: Windows 替换(`apply_windows.go` + `spawn_windows.go`)

**Level:** L3
**Level Rationale:** AC-7 高危路径:运行中 exe 被 OS 锁定,必须"自复制到 %TEMP% → rename-aside + 退避重试";杀软/索引器临时锁与 `.old` 残留是 Windows 特有失败面,且本机(darwin)无法执行,行为验证依赖 CI windows runner 与真机走查。
**Linked Acceptance Items:** PAC-3、PAC-1(协议),PAC-8(真机走查)
**Task Gate:** task reviewer + linked AC + CI windows runner 证据(在 T8 建立后回填)

**Files:**
- Create: `internal/updater/apply_windows.go`
- Create: `internal/updater/apply_windows_test.go`
- Create: `internal/updater/spawn_windows.go`

**Interfaces:**
- Consumes: T1 契约;`syscall`(Windows 分支)。
- Produces:
  ```go
  // ---- apply_windows.go (//go:build windows) ----
  var ErrInstallDirNotWritable = errors.New("安装目录不可写,无法自动更新;请下载安装包手动更新")
  var renameRetries = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second} // 1 次首试 + 3 次退避重试
  func RunningTarget() (string, error)   // os.Executable→EvalSymlinks
  func PreflightTarget(target string) error
  func DefaultDeps() HelperDeps          // Stage=复制到 <dir>/<base>.new-<pid>;Swap=_os.Remove(.old)→RenameWithRetry×2
                                         // Rollback=RenameWithRetry;Relaunch=spawnDetached(target)
  func CleanupStaleBackups() error       // <target>.old + glob <base>.new-* + %TEMP%/gbt32960-helper-*.exe(尽力而为)
  func helperCopyPath(pid int) string    // os.TempDir()/gbt32960-helper-<pid>.exe(纯函数,可测)
  func copySelf(dst string) error        // os.Executable → copyFile(dst, 0o755)

  // ---- spawn_windows.go (//go:build windows) ----
  func processAlive(pid int) bool        // syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION=0x400)+GetExitCodeProcess==259(STILL_ACTIVE)
  func spawnDetached(exe string, args []string) (*os.Process, error) // CreationFlags=DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP
  func SpawnHelper(args HelperArgs) error // copySelf(helperCopyPath(os.Getpid())) → spawnDetached(copy, args.Encode())
  ```

- [ ] **Step 1: 写失败测试(windows 标签;真实临时目录,不 mock 文件系统):**

```
TestWindowsStageCopy          : Stage 复制产物到 <dir>/<base>.new-<pid> 且字节一致;artifact 缺失 → ErrStageFailed
TestWindowsSwapAndRollback    : Swap → backup == target+".old" 且内容为原 target;target == staged 内容;
                                Rollback → 还原;Rollback("") → nil;staged 缺失 → err 且 backup 语义正确;
                                预置陈旧 .old → Swap 前被清理(不因目标存在而失败)
TestWindowsRenameRetryDefaults: renameRetries 逐项 == [200ms,500ms,1s](锁定规格,防实现漂移)
TestWindowsProcessAlive       : processAlive(os.Getpid())==true;spawnDetached(os.Executable(), ...) 反向用例见下注;
                                exec.Command("cmd","/c","exit") 启动后轮询至 processAlive(pid)==false(≤5s)
TestWindowsCleanupStaleBackups: 造 <base>.exe.old、<base>.exe.new-999、%TEMP%/gbt32960-helper-123.exe、keep.txt
                                → 仅 keep.txt 保留
TestWindowsHelperCopyPath     : helperCopyPath(42) == filepath.Join(os.TempDir(), "gbt32960-helper-42.exe")
TestWindowsSelfCopy           : copySelf(t.TempDir()/x.exe) 后字节与 os.Executable 一致,且文件存在
TestWindowsSpawnHelper        : 不做真实拉起(SpawnHelper 会启动进程);仅断言 copySelf+helperCopyPath 组合,
                                真实拉起由真机走查(T9 第 3 步)覆盖 —— 计划显式声明该取舍
```

> 注:Windows 只读目录无稳定的非提权构造方式(`os.Chmod` 在 Windows 不阻止建文件);"不可写 → `安装目录不可写…`"分支由 **darwin 的 `writableProbe` 失败用例 + T5 的 HelperMain 矩阵** 覆盖,Windows 侧由真机 `icacls /deny` 走查(T9)记录文案。

- [ ] **Step 2: 运行确认失败(本机不可执行,以下命令为 CI/Windows 目标)**

```bash
# 本机(darwin)只能做交叉编译检查:
GOOS=windows GOARCH=amd64 go vet ./internal/updater/   # 预期:此时失败(符号未定义)
# CI windows runner(T8 的 go-windows job):
go test ./internal/updater/ -run 'TestWindows' -v      # 预期:编译失败(未定义)
```

- [ ] **Step 3: 实现。**关键点:
  - `Stage` 复制用本地函数 `copyFile(src, dst string, mode os.FileMode) error`(`io.Copy` + `os.OpenFile`;Windows 下 mode 传 `0o755`,实际权限由 FS 语义决定)
  - `Swap`:`_ = os.Remove(target + ".old")`(陈旧残留先清,保证 `rename` 目标不存在)→ `RenameWithRetry(target, target+".old", renameRetries)` → `RenameWithRetry(staged, target, renameRetries)`;第二步失败返回 `(target+".old", ErrSwapFailed)`
  - `Rollback`:`os.Stat(backup)` 不存在 → nil;`RenameWithRetry(backup, target, renameRetries)`
  - `spawnDetached` 用 `syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}`(DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP)
  - `processAlive`:每次调用 `OpenProcess` 后必须 `defer syscall.CloseHandle(h)`(轮询不泄漏句柄)

- [ ] **Step 4: 本机交叉编译检查 + 提交**

```bash
GOOS=windows GOARCH=amd64 go vet ./internal/updater/   # T3/T4 完成后须无输出
gofmt -l internal/updater/                            # 空输出
```

预期:`GOOS=windows` vet 通过(T4 完成后一并确认 `GOOS=linux`/`GOOS=windows` 双通过)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/apply_windows.go internal/updater/apply_windows_test.go internal/updater/spawn_windows.go
GIT_MASTER=1 git commit -m "feat(updater): Windows rename-aside 替换——自复制 %TEMP%/退避重试/.old 清理"
```

---

### Task 4: Linux 替换(`apply_linux.go`)

**Level:** L2
**Level Rationale:** unix `rename` 语义最简(可覆盖运行中二进制),无平台特例、单文件、无新进程模型;但它是 AC-8 的落地点且行为只能在 Linux 主机验证(CI ubuntu runner),故要求聚焦用例 + CI 证据。
**Linked Acceptance Items:** PAC-4、PAC-1(协议)
**Task Gate:** task reviewer + focused checks(CI ubuntu job 证据在 T8 建立后回填)

**Files:**
- Create: `internal/updater/apply_linux.go`
- Create: `internal/updater/apply_linux_test.go`

**Interfaces:**
- Consumes: T1 契约;T2 的 `spawn_unix.go`(`processAlive`/`spawnDetached`,unix 标签已覆盖 linux)。
- Produces:
  ```go
  // ---- apply_linux.go (//go:build linux) ----
  var ErrProgramDirNotWritable = errors.New("程序所在目录不可写,无法自动更新;请检查目录权限后重试")
  func RunningTarget() (string, error)   // os.Executable→EvalSymlinks
  func PreflightTarget(target string) error // writableProbe(文件所在目录);目标不存在 → ErrTargetNotFound
  func DefaultDeps() HelperDeps          // Stage=复制到 <dir>/<base>.new-<pid>+Chmod 0755
                                         // Swap=RenameWithRetry(target,target+".old",nil)→RenameWithRetry(staged,target,nil)→Chmod 0755
                                         // Rollback=rename 回位 + Chmod 0755;Relaunch=spawnDetached(target)
  func CleanupStaleBackups() error       // 精确 <target>.old + glob <base>.new-*(尽力而为)
  ```

- [ ] **Step 1: 写失败测试(linux 标签):**

```
TestLinuxStageCopyAndMode  : Stage → 内容一致且 mode == 0755(umask 无关,显式 Chmod)
TestLinuxSwapKeepsOld      : Swap 后 <target>.old 存在(AC-8「保留 .old 至新实例启动成功」)、target == staged 内容、0755
TestLinuxSwapAndRollback   : staged 缺失 → err;Rollback 还原 + 0755;Rollback("") → nil
TestLinuxPreflightGuidance : chmod 0555 父目录 → 精确文案「程序所在目录不可写,无法自动更新;请检查目录权限后重试」
TestLinuxCleanupStaleBackups: 仅删 <target>.old 与 <base>.new-*,keep.txt 保留
```

- [ ] **Step 2: 运行确认失败(本机不可执行,CI ubuntu 目标)**

```bash
GOOS=linux GOARCH=amd64 go vet ./internal/updater/    # 预期:此时失败(符号未定义)
# CI ubuntu runner:
go test ./internal/updater/ -run 'TestLinux' -v       # 预期:编译失败
```

- [ ] **Step 3: 实现。**关键点:`os.Chmod(staged, 0o755)` 在 Stage 与 Swap 后各做一次(复制可能丢权限位);`rename` 对运行中二进制合法,无需停进程(设计文档 §5.4 表)。

- [ ] **Step 4: 本机交叉编译检查 + 提交**

```bash
GOOS=windows GOARCH=amd64 go vet ./... && GOOS=linux GOARCH=amd64 go vet ./...   # 三平台符号齐备后须双绿
go vet ./internal/updater/ && go test ./internal/updater/ -count=1
```

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/apply_linux.go internal/updater/apply_linux_test.go
GIT_MASTER=1 git commit -m "feat(updater): Linux rename 替换——权限保留/.old 保留至新实例启动"
```

---

### Task 5: helper 协议编排与 `main()` 早期拦截(`helper.go` + `main.go`)

**Level:** L3
**Level Rationale:** helper 是全链路收尾者——等待/预检/暂存/交换/回滚/结果/拉起顺序错一步即产生双实例、损坏安装或丢失失败告知;`main()` 拦截点属全局启动契约(§5.7,所有 Phase 继承);结果文件是跨会话失败闭环(AC-14)的唯一载体。
**Linked Acceptance Items:** PAC-1、PAC-5、AC-14(载体),PAC-8(S4 真机)
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `internal/updater/helper.go`
- Create: `internal/updater/helper_test.go`
- Modify: `main.go`(import + 首行拦截;其余不动)

**Interfaces:**
- Consumes: T1 契约(`HelperArgs`/`HelperDeps`/`LastResult`/`WriteLastResult`/`OpenHelperLog`/错误值);T2–T4 的 `DefaultDeps()`。
- Produces:
  ```go
  // ---- helper.go ----
  const (
      helperPollInterval  = 100 * time.Millisecond
      helperParentTimeout = 60 * time.Second
  )
  var helperFault string // 演练专用:仅 -X gbt32960-simulator/internal/updater.helperFault=stage|swap|relaunch 可注入

  func RunHelperIfRequested()                        // main() 最早期:if code, handled := helperEntry(os.Args, DefaultDeps()); handled { os.Exit(code) }
  func helperEntry(args []string, deps HelperDeps) (code int, handled bool) // 可测分发(不含 os.Exit)
  func HelperMain(args []string, deps HelperDeps) int // 0=成功;1=失败闭环已完成;2=参数/初始化失败
  ```
  `HelperMain` 顺序契约(骨架;日志每步一行):
  ```go
  a, err := ParseHelperArgs(args)            // 失败 → 2(无结果文件:参数不可信)
  log := OpenHelperLog(); if log != nil { defer log.Close() }
  if err := deps.WaitParent(a.ParentPID, helperPollInterval, helperParentTimeout); err != nil {
      writeResult(a, false, ErrParentWaitTimeout.Error()); return 1   // 父进程仍活:不拉起、不触碰文件
  }
  if _, err := os.Stat(a.Artifact); err != nil {
      writeResult(a, false, ErrArtifactMissing.Error()); relaunchOld(a, deps); return 1
  }
  if err := deps.Preflight(a.Target); err != nil {
      writeResult(a, false, ErrSwapFailed.Error()); relaunchOld(a, deps); return 1   // 细节在日志
  }
  staged, cleanup, err := deps.Stage(a.Artifact, a.Target)                    // helperFault=="stage" → 注入失败
  if err != nil { writeResult(a, false, ErrStageFailed.Error()); relaunchOld(a, deps); return 1 }
  if cleanup != nil { defer cleanup() }
  backup, err := deps.Swap(staged, a.Target)                                  // helperFault=="swap" → 注入失败
  if err != nil { return rollbackAndRelaunch(a, backup, ErrSwapFailed, deps) }
  writeResult(a, true, a.Tag)                                                 // §5.4:写结果先于拉起
  if err := deps.Relaunch(a.Target); err != nil {                             // helperFault=="relaunch" → 注入失败
      return rollbackAndRelaunch(a, backup, ErrRelaunchFailed, deps)
  }
  return 0
  // rollbackAndRelaunch:Rollback(backup,target) 失败 → 写 ErrRollbackFailed 并直接返回 1(不拉旧版:目标不可信);
  //                      成功 → 写对应 reason → Relaunch(target)(旧版);拉起失败仅记日志(用户可手动启动)。
  ```
- `main.go` 变更(唯一改动):
  ```go
  import "gbt32960-simulator/internal/updater"
  func main() {
      updater.RunHelperIfRequested() // helper 模式:最早拦截,不初始化 Wails(设计文档 §5.7)
      app := NewApp()
      ... // 以下逐字不变
  }
  ```

- [ ] **Step 1: 写失败测试(全 fake deps + 临时目录;不启动真实进程):**

```
TestHelperEntrySentinel      : helperEntry([exe,"--updater-helper","--parent-pid","0",...], fakeDeps) → handled==true;
                               helperEntry([exe,"--other"], fake) → handled==false, code==0(不触碰 deps)
TestHelperMainSuccess        : fake Stage/Swap/Relaunch 记录调用 → result 文件 {ok:true,targetVersion:vX};
                               Relaunch 恰 1 次且实参 == target;返回码 0;helper.log 含各阶段行
TestHelperMainStageFailure   : Stage 失败 → reason「暂存失败」、Rollback 零调用、Relaunch 1 次(旧版)、码 1
TestHelperMainSwapFailure    : Swap 返回 (backup, ErrSwapFailed) → Rollback(backup,target) 恰 1 次、
                               reason「替换失败」、Relaunch 1 次、码 1
TestHelperMainRelaunchFailure: Swap 成功 + Relaunch 第 1 次失败 → Rollback 1 次 → Relaunch 第 2 次(旧版)、
                               reason「无法启动新版本」、码 1
TestHelperMainRollbackFailure: Rollback 失败 → reason「回滚失败,请手动重新安装」且不 Relaunch、码 1
TestHelperMainParentTimeout  : WaitParent → ErrParentWaitTimeout → reason「等待应用退出超时」、
                               Stage/Swap/Rollback/Relaunch 零调用
TestHelperMainArtifactMissing: artifact 不存在 → reason「更新包不存在」、Stage 零调用、Relaunch 1 次(旧版)
TestHelperMainFaultInjection : helperFault="swap" → reason「替换失败」且 Rollback 被调用(演练语义锁定);
                               helperFault="bogus" → 行为等同正常成功路径;helperFault="" → 无影响
TestHelperMainDataBoundary   : 布局内同目录 settings.json/packs/x.json 全流程后逐字节不变(AC-15)
TestHelperMainResultSchema   : 原始 JSON 键恰为 ok/targetVersion/reason/logPath(omitempty 行为)
TestHelperMainArgFailure     : 缺 --target → 码 2 且无结果文件写入
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/updater/ -run 'TestHelperEntry|TestHelperMain' -v
```

预期:编译失败(`RunHelperIfRequested`/`helperEntry`/`HelperMain` 未定义)。

- [ ] **Step 3: 实现 `helper.go` 并修改 `main.go`。**`writeResult(a, ok, reason)` 内部:`LastResult{OK, TargetVersion: a.Tag, Reason, LogPath: a.Log}` → `WriteLastResult`(写失败仅记日志,不阻塞闭环)。`logf` 在 `log == nil` 时静默。

- [ ] **Step 4: 运行确认通过 + 全量回归 + vet + 人工冒烟**

```bash
go test ./internal/updater/ -run 'TestHelperEntry|TestHelperMain' -count=1 -v
go test ./internal/updater/ -count=1 && go test -race ./internal/updater/
go vet ./... && GOOS=windows GOARCH=amd64 go vet ./...
# 人工冒烟(本机;前置:frontend/dist 已存在,否则先 cd frontend && npm run build):
#   sleep 1 & go run . --updater-helper --parent-pid $! --artifact /nonexistent --target /nonexistent \
#     --tag v9.9.9 --result "$HOME/Library/Caches/gbt32960-simulator/updates/last-result.json" \
#     --log "$HOME/Library/Caches/gbt32960-simulator/updates/helper.log"
#   预期:~1s 后退出码 1,last-result.json reason=更新包不存在,helper.log 有启动/等待/失败行
```

预期:测试全 PASS;race 干净;vet 无输出;人工冒烟的输出与结果文件符合预期(证据留存于 T9 的 S0 冒烟)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/helper.go internal/updater/helper_test.go main.go
GIT_MASTER=1 git commit -m "feat(updater): helper 协议编排与 main 早期拦截——等待父进程/暂存/交换/回滚/结果/拉起"
```

---

### Task 6: `bridge.UpdaterService` 应用服务(`ApplyUpdate`/`ConsumeLastResult`/启动清理扩展)

**Level:** L3
**Level Rationale:** 绑定面扩展(AC-9)与"退出应用"这一不可逆动作的唯一入口;单飞/守卫/spawn/延迟退出必须确定性测试(`quit`/`spawn`/`target`/`preflight` 全部注入),否则测试会退出测试进程;跨包接线(启动清理扩展)且直接面向前端契约。
**Linked Acceptance Items:** PAC-5、PAC-6(接口契约)、PAC-8
**Task Gate:** task reviewer + linked AC

**Files:**
- Modify: `bridge/updater_service.go`(追加字段与方法;P2 方法保留)
- Modify: `bridge/updater_service_test.go`(追加)
- Modify: `bridge/wiring.go`(`WireUpdaterStartup` 追加陈旧备份清理)
- Regenerate: `frontend/wailsjs/**`(`wails generate module`)

**Interfaces:**
- Consumes: T5 的 `updater.HelperArgs`/`updater.SpawnHelper`/`updater.RunningTarget`/`updater.PreflightTarget`/`updater.CleanupStaleBackups`/`updater.ClearLastResult`/`updater.ReadLastResult`/`updater.AsOutcome`;P2 的下载状态;wails `runtime.Quit`。
- Produces:
  ```go
  // 绑定面(导出恰 6 方法,全部无参数)
  func (s *UpdaterService) ApplyUpdate() error
  func (s *UpdaterService) ConsumeLastResult() (updater.ApplyOutcome, error)

  // 新增内部字段(全部在 NewUpdaterService 中给出默认值,测试注入)
  ready          *updater.Artifact        // DownloadUpdate 成功后留存(仅内存,不跨会话;D9)
  applying       bool
  quitDelay      time.Duration            // 默认 time.Second
  spawn          func(updater.HelperArgs) error
  quit           func(context.Context)    // 默认 wruntime.Quit
  resolveTarget  func() (string, error)   // 默认 updater.RunningTarget
  preflight      func(string) error       // 默认 updater.PreflightTarget
  ```
  `ApplyUpdate` 流程(骨架;顺序即契约):
  ```go
  1  if dev → errors.New("开发构建不参与更新")
  2  mu:if downloading → "更新正在下载中,请稍候";if applying → "更新已在进行中"
     ready := s.ready; if ready == nil → "更新包未就绪,请先下载"
  3  if _, err := os.Stat(ready.Path); err != nil → "更新包未就绪,请先下载"
  4  rel := s.lastRelease
     if rel == nil || !updater.IsNewer(rel.TagName, s.version) → "已是最新版本,无需安装"
  5  target, err := s.resolveTarget(); if err != nil { return err }
  6  if err := s.preflight(target); err != nil { return err }   // 指引类文案原样透出,不退出应用
  7  _ = updater.ClearLastResult()                               // 清陈旧结果,防上次残留造成误报
  8  args := updater.HelperArgs{ParentPID: os.Getpid(), Artifact: ready.Path, Target: target,
         Tag: ready.Tag, Result: resultPath, Log: logPath}       // resultPath/logPath 经 LastResultPath()/HelperLogPath()
  9  if err := s.spawn(args); err != nil → "无法启动更新进程,请手动更新"(applying 保持 false)
  10 mu:applying = true
  11 go func(){ time.Sleep(s.quitDelay); s.quit(s.ctx) }()       // ~1s 留 UI 收尾,然后退出
  12 return nil
  ```
  `ConsumeLastResult`:`ReadLastResult()` → `(nil,nil)` → `ApplyOutcome{}`;非 nil → `AsOutcome(r)` 并**尽力** `ClearLastResult()`(清除失败不报错,避免重复 toast 由"清除失败"二次触发)。
  其余接线:`DownloadUpdate` 成功处 `s.ready = &art`;`CheckUpdate` 成功(取得新 `rel`)处 `s.ready = nil`(与前端 `check()` 复位一致);`CheckUpdate`/`DownloadUpdate` 的单飞判断纳入 `applying`(apply 期间一律 `更新已在进行中`)。

- [ ] **Step 1: 写失败测试(追加到 `bridge/updater_service_test.go`)**

```
TestUpdaterServiceApplyGuards        : 表驱动 —— dev →「开发构建不参与更新」;无 ready →「更新包未就绪,请先下载」;
                                       同版本(lastRelease.TagName 与 version 等值 → IsNewer 判 0)→「已是最新版本,无需安装」;
                                       二次调用(applying=true)→「更新已在进行中」;下载中 →「更新正在下载中,请稍候」
TestUpdaterServiceApplySpawnsAndQuits: 注入 resolveTarget=临时目录、preflight=nil、spawn 记录 args、
                                       quit=close(chan)、quitDelay=10ms → ApplyUpdate() == nil;
                                       断言关闭通道在 ≤1s 内收到信号且恰 1 次;args 字段逐一匹配
                                       (ParentPID==os.Getpid()/Artifact==ready.Path/Tag==ready.Tag/
                                       Result==LastResultPath()/Log==HelperLogPath());
                                       结果文件已在 spawn 前被清除(ClearLastResult 生效)
TestUpdaterServiceApplySpawnFailure  : spawn 返回 error → 文案「无法启动更新进程,请手动更新」;quit 零调用;
                                       applying 保持 false(可再次 Apply 或下载)
TestUpdaterServiceApplyPreflightGuidance: preflight 返回 updater.ErrTranslocated → 原文透出且 quit 零调用
TestUpdaterServiceConsumeLastResult  : 预置失败结果 → Present=true/OK=false/Reason/LogPath 正确;文件已被清除;
                                       第二次调用 → Present=false;预置成功结果 → OK=true(前端据此静默);
                                       无文件 → Present=false;损坏 JSON → error
TestUpdaterServiceStartupCleanup     : WireUpdaterStartup 后缓存 tag 子目录被清(P2 回归)+ 不因
                                       CleanupStaleBackups 报错(测试二进制非 .app → RunningTarget 失败须被吞掉)
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./bridge/ -run 'TestUpdaterServiceApply|TestUpdaterServiceConsume' -v
```

预期:编译失败(`ApplyUpdate`/`ConsumeLastResult` 未定义)。

- [ ] **Step 3: 实现 + 再生成绑定**

```bash
# 实现 bridge/updater_service.go 与 wiring.go 后:
wails generate module
grep -n "^func (s \*UpdaterService) [A-Z]" bridge/updater_service.go | wc -l   # 预期 6
grep -n "ApplyUpdate\|ConsumeLastResult" frontend/wailsjs/go/bridge/UpdaterService.d.ts
grep -n "ApplyOutcome" frontend/wailsjs/go/models.ts
```

预期:6 方法;`.d.ts` 含 `ApplyUpdate():Promise<void>` 与 `ConsumeLastResult():Promise<updater.ApplyOutcome>`;`models.ts` 含 `updater.ApplyOutcome`。

`bridge/wiring.go` 变更:
```go
// WireUpdaterStartup 装配更新服务启动行为:清理更新缓存(不跨会话复用)+ 清理陈旧替换备份
// (.bak-*/.old/暂存,上一会话失败或中断的残留;尽力而为,失败忽略)。
func WireUpdaterStartup(u *UpdaterService) {
    u.cleanupCache()
    _ = updater.CleanupStaleBackups()
}
```

- [ ] **Step 4: 运行确认通过 + P2 回归 + vet**

```bash
go test ./bridge/ -run 'TestUpdaterService' -count=1 -v
go test ./bridge/ -count=1 && go vet ./bridge/
```

预期:新用例全 PASS;P2 既有 `TestUpdaterService*` 全过(含 `TestUpdaterServiceDownloadAndVerify`/`DownloadCancel`/`CleanupCache`/`DownloadGuards`/`ErrorClassification`)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add bridge/updater_service.go bridge/updater_service_test.go bridge/wiring.go frontend/wailsjs
GIT_MASTER=1 git commit -m "feat(bridge): 应用更新服务——ApplyUpdate 单飞/helper 拉起/延迟退出/结果消费/启动清理扩展"
```

---

### Task 7: 前端二次确认 / `applying` 态 / 失败告知

**Level:** L3
**Level Rationale:** 用户可见的"安装并重启"闭环入口(AC-13/14):动态运行态文案必须精确、确认动作不可误触、失败不得退出应用;跨 api 绑定、composable 与设置面板。
**Linked Acceptance Items:** PAC-6、PAC-5(接口)
**Task Gate:** task reviewer + linked AC

**Files:**
- Modify: `frontend/src/components/settings/SettingsAboutPanel.vue`
- Modify: `frontend/src/components/settings/SettingsAboutPanel.test.ts`
- Modify: `frontend/src/composables/useUpdater.ts`(纯函数 `lastResultToast`)
- Modify: `frontend/src/composables/useUpdater.test.ts`
- Modify: `frontend/src/App.vue`(启动消费结果)

**Interfaces:**
- Consumes: T6 生成的绑定 `UpdaterService.ApplyUpdate()`/`ConsumeLastResult()`/`CurrentVersion()`;`ConnectionService.State()`(返回 `idle|connecting|loggingIn|online`);`ServerService.Status()`(返回 `{running, listenAddr}`);antd `Modal.confirm`(先例 `SettingsAdvancedPanel.vue`)。
- Produces:
  ```ts
  // useUpdater.ts(纯函数,无 IO)
  /** 上次更新失败的启动告知文案;成功/无记录返回 null(§5.4) */
  export function lastResultToast(
    res: { present: boolean; ok: boolean },
    currentVersion: string,
  ): string | null {
    if (!res.present || res.ok) return null
    return `上次更新未成功,已回滚在 ${currentVersion},详情见日志`
  }
  ```
  ```ts
  // SettingsAboutPanel.vue
  const applying = ref(false)
  // 运行态中断文案(AC-13;四种组合精确匹配,新增/修改均在下一行的枚举内)
  function interruptionHint(connOnline: boolean, serverRunning: boolean): string {
    const parts: string[] = []
    if (connOnline) parts.push('断开连接')
    if (serverRunning) parts.push('停止服务')
    return parts.length === 0 ? '安装过程中将退出。' : `安装过程中将${parts.join('、')}并退出。`
  }
  async function apply() {
    const [connState, serverStatus] = await Promise.all([ConnectionService.State(), ServerService.Status()])
    Modal.confirm({
      title: '安装并重启',
      content: `将安装 ${ready.value?.tag ?? ''}。${interruptionHint(connState === 'online', serverStatus.running)}`,
      okText: '安装并重启',
      cancelText: '取消',
      onOk: async () => {
        try { await UpdaterService.ApplyUpdate(); applying.value = true }
        catch (e) { message.error(errText(e)); throw e }   // 失败不退出应用,保持就绪态可重试
      },
    })
  }
  ```
  ```ts
  // App.vue onMounted 追加(启动消费,失败才提示;成功静默)
  onMounted(async () => {
    try {
      const [res, cur] = await Promise.all([
        UpdaterService.ConsumeLastResult(), UpdaterService.CurrentVersion(),
      ])
      const text = lastResultToast(res, cur)
      if (text) message.error(text)
    } catch { /* 静默:更新结果消费失败不影响启动 */ }
  })
  ```
  模板:`ready` 分支去掉"安装与重启将在后续阶段开放"提示,改为就绪文案 + `<a-button size="small" type="primary" @click="apply">安装并重启</a-button>`;新增 `applying` 分支(优先于 ready):`<span class="about-new">正在安装并重启,应用将在数秒内退出…</span>`,并令下载/跳过/检查按钮在 `applying` 时不可用(状态机互斥,§5.6)。

- [ ] **Step 1: 写失败测试(扩展两个测试文件)**

`SettingsAboutPanel.test.ts`:
```
mock 追加:mockedConnectionState/mockedServerRunning 变量 + vi.mock 两个绑定模块(ConnectionService/ServerService)
Modal 断言:const confirmSpy = vi.spyOn(Modal, 'confirm').mockImplementation((cfg) => { captured = cfg; return {} as never })
用例 1「确认框动态文案矩阵」:四组 (connState, serverRunning) → captured.content 精确包含
        「安装过程中将退出。」/「安装过程中将断开连接并退出。」/「安装过程中将停止服务并退出。」/
        「安装过程中将断开连接、停止服务并退出。」
用例 2「确认后进入 applying」:invoke captured.onOk() → ApplyUpdate 恰 1 次 → 文本含「正在安装并重启」;
        就绪按钮不再出现(或 disabled)
用例 3「取消不触发」:不调用 onOk → ApplyUpdate 零调用、仍为就绪态
用例 4「ApplyUpdate 拒绝」:mockRejectedValue(new Error('更新包未就绪,请先下载')) → message.error 精确文案、
        不出现「正在安装并重启」、就绪态可重试
```
`useUpdater.test.ts`:
```
lastResultToast 表驱动:{present:false,ok:false}/{present:true,ok:true} → null;
{present:true,ok:false} + 'v0.1.0' → 「上次更新未成功,已回滚在 v0.1.0,详情见日志」
```

- [ ] **Step 2: 运行确认失败**

```bash
cd frontend && npm test -- --run src/components/settings/SettingsAboutPanel.test.ts src/composables/useUpdater.test.ts
```

预期:失败(未定义 `lastResultToast`;无「安装并重启」按钮)。

- [ ] **Step 3: 实现。**注意:`Modal.confirm` 的 `onOk` 抛错是否保持弹窗由 antd 行为决定,**不作为验收断言**;验收断言为"错误 toast 出现且未进入 applying"。

- [ ] **Step 4: 前端四件套 + 提交**

```bash
cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build
grep -n "lastResultToast\|ConsumeLastResult" frontend/src/App.vue
```

预期:四件套全 exit 0;`App.vue` 两条命中。

```bash
GIT_MASTER=1 git add frontend/src/components/settings/SettingsAboutPanel.vue frontend/src/components/settings/SettingsAboutPanel.test.ts frontend/src/composables/useUpdater.ts frontend/src/composables/useUpdater.test.ts frontend/src/App.vue
GIT_MASTER=1 git commit -m "feat(frontend): 安装并重启闭环——运行态中断二次确认/applying 态/启动失败告知"
```

---

### Task 8: 发布链 `-installscope user` + CI 平台替换验证工作流

**Level:** L3
**Level Rationale:** AC-12 的 P3 部分(Windows per-user 切换)与 §5.8 的"Windows 逻辑在 CI windows runner 覆盖":本机无 Windows/Linux,行为验证只能靠 CI;工作流的正确性直接决定平台替换逻辑有没有回归门。
**Linked Acceptance Items:** PAC-7、PAC-3、PAC-4
**Task Gate:** task reviewer + linked AC + CI 运行记录

**Files:**
- Modify: `.github/workflows/release.yml`(Windows 构建行追加 `-installscope user`,仅此一行)
- Create: `.github/workflows/test.yml`(常规测试工作流;仓库目前只有 release.yml)

**Interfaces:**
- Consumes: T3/T4 的平台用例(`TestWindows*`/`TestLinux*`);仓库既有构建链(Go 1.25 / Node 22)。
- Produces: per-user NSIS 安装包(发布侧);`test.yml` 的 `go-linux`/`go-windows`/`frontend` 三 job。

- [ ] **Step 1: `release.yml` 追加 installscope(唯一行级改动)**

```yaml
      - name: 构建 (windows/amd64 + NSIS 安装包)
        if: runner.os == 'Windows'
        # per-user 安装(%LOCALAPPDATA%\Programs\...):免 UAC 自替换的前提,见 ADR-0002
        run: wails build -platform windows/amd64 -clean -trimpath -nsis -installscope user -webview2 download -ldflags "-X main.version=${GITHUB_REF_NAME}"
        shell: bash
```

- [ ] **Step 2: 新建 `.github/workflows/test.yml`**

```yaml
name: test

# 常规回归门(不参与发布):本机/PR 覆盖三平台编译与平台替换用例;
# Windows/Linux 的 apply 用例带构建标签,只能在对应 runner 实跑(spec §5.8)。
on:
  push:
    branches: [main]
  pull_request:
  workflow_dispatch:

jobs:
  go-linux:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with: { go-version: '1.25', cache-dependency-path: go.sum }
      - run: go vet ./...
      - run: go test ./... -count=1
      - run: go test -race ./internal/updater/ ./bridge/
  go-windows:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with: { go-version: '1.25', cache-dependency-path: go.sum }
      - run: go vet ./...
      - run: go test ./internal/updater/ ./bridge/ -count=1   # TestWindows* 实跑(§5.8)
  frontend:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: frontend } }
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-node@v7
        with: { node-version: '22', cache: npm, cache-dependency-path: frontend/package-lock.json }
      - run: npm ci
      - run: npm run lint && npm run typecheck && npm test -- --run && npm run build
```

> 取舍说明(为何新增工作流而非扩展 release.yml):`release.yml` 仅 tag 触发,无法作为常规回归门;而"Windows 逻辑在 CI windows runner 覆盖"是 §5.8 的明确验证策略。新工作流不修改发布链结构、不做 wails 打包(打包仍归 release.yml)。
> 不做跨平台 `GOOS=windows go test`(go test 无法在宿主外执行);本机以 `GOOS=windows|linux go vet ./...` 保证编译面,行为面交给本工作流与 T9。

- [ ] **Step 3: 本机可验证项**

```bash
GOOS=windows GOARCH=amd64 go vet ./... && GOOS=linux GOARCH=amd64 go vet ./...
ruby -ryaml -e 'YAML.load_file(".github/workflows/release.yml"); YAML.load_file(".github/workflows/test.yml")' && echo "YAML-OK"
grep -n "installscope user" .github/workflows/release.yml
```

预期:交叉 vet 双绿;YAML 可解析;installscope 恰 1 条。

- [ ] **Step 4: 触发一次 CI 并回填证据**

```bash
# 推送分支或开 PR 触发 test.yml;记录 Run URL 与 go-windows/go-linux/frontend 三 job 结果
gh run list --workflow=test.yml --limit 3
```

预期:三 job 通过(证据留存 `.superpowers/sdd/updater-p3/evidence/ci/`)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add .github/workflows/release.yml .github/workflows/test.yml
GIT_MASTER=1 git commit -m "ci(release): Windows 安装包切换 per-user(-installscope user);新增三平台测试工作流"
```

---

### Task 9: 端到端演练与验收证据(`v0.1.1` 发布 + 场景矩阵)

**Level:** L3
**Level Rationale:** P3 成败只能由真机替换验证(§5.8);回滚、失败闭环、数据边界与 per-user 安装均无自动化替代;含人工发布步骤(离线 Ed25519 签名)。
**Linked Acceptance Items:** PAC-8(并回填 PAC-2/3/4/5/6 的真机证据),AC-6/7/8/13/14
**Task Gate:** task reviewer + 全部证据留存 + 主控终验

**Files:**
- Create(证据,不入库;`.superpowers/` 已 gitignore):`.superpowers/sdd/updater-p3/rehearsal-checklist.md`、`.superpowers/sdd/updater-p3/evidence/**`、`.superpowers/sdd/updater-p3/p3-task-N-report.md`

- [ ] **Step 1: 前置与基线(发布 + 密钥仪式)**

```bash
# 1) macOS 基线 v0.1.0(无注入)与"故障注入基线"(仅演练用,不入 Release)
wails build -ldflags "-X main.version=v0.1.0"                                   # 正常基线
wails build -o gbt32960-simulator-fault -ldflags "-X main.version=v0.1.0 -X gbt32960-simulator/internal/updater.helperFault=relaunch"  # 注入基线(回滚场景)
# 2) 记录数据指纹(替换前后比对)
shasum -a 256 "$HOME/Library/Application Support/gbt32960-simulator/settings.json"
find "$HOME/Library/Application Support/gbt32960-simulator/packs" -type f -exec shasum -a 256 {} \;
# 3) 发布 v0.1.1:推送 tag → CI 三平台构建 + SHA256SUMS 生成
git tag v0.1.1 && git push origin v0.1.1
# 4) 离线签名(严格按 docs/release-signing.md §2,keyid 91fe6136):
#    a) GitHub Release 页下载 SHA256SUMS 与全部资产到临时目录
#    b) 本地重算核对(防盲签):go run ./tools/sign-release -sums -dir <临时目录> -out /tmp/SHA256SUMS.check && diff
#    c) go run ./tools/sign-release -sign -key <私钥路径> -in SHA256SUMS -out SHA256SUMS.sig
#    d) gh release upload v0.1.1 SHA256SUMS.sig
#    e) 自检:单行 <keyid> <base64>;keyid 与 signing_pubkey.go 一致
# 5) 确认 Release 资产齐备(4 资产 + SHA256SUMS + SHA256SUMS.sig)
gh release view v0.1.1 --json assets --jq '.assets[].name'
```

外部前置:私钥在密码管理器(丢失=更新链断裂,`docs/release-signing.md` §1);本步骤不含任何新密钥动作(仅对 v0.1.1 补签)。

- [ ] **Step 2: macOS 主路径场景矩阵(每场景:预期 / 实际 / 证据路径)**

外部前置(macOS 主路径):本机 macOS + Xcode Command Line Tools + Wails CLI v2.15.0;v0.1.1 的离线签名已完成(Step 1);无需额外硬件。这是 P3 的**必做主路径**(§5.8)。

```
S0 helper 独立冒烟      : T5 Step 4 的 --updater-helper 冒烟命令 → 退出码 1 + last-result.json reason=更新包不存在
S1 正常升级(主路径)    : v0.1.0 → 检查 v0.1.1 → 下载(进度)→ 校验 → 二次确认(无运行态文案)
                          → 应用退出 → helper.log 全阶段 → 新版本启动(关于面板 v0.1.1)
                          → .bak-* 已被新实例清理(ls -l 前后)→ 缓存 tag 目录被清 → 数据指纹一致
S2 运行态中断文案       : 客户端连接中 / 服务端运行中分别触发确认框 → 精确文案(两组合)
S3 下载中断重试         : 下载中取消(无错误弹窗、无 .part 残留)→ 断网/代理故障 → 分类文案 → 恢复后重试成功
S4 校验失败             : 临时上传**被篡改的 SHA256SUMS.sig**(改 1 字节)→ 下载被拒「更新包签名校验失败,已拒绝更新」
                          → 缓存无 .part 残留 → 立即恢复正确签名(演练窗口内他人手动更新会失败:记录开始/结束时间);
                          可选加强:用离线私钥重签一份哈希被改的 SHA256SUMS → 「更新包校验失败(哈希不匹配),已拒绝更新」
                          注:ADR-0003 后缺资产表现为 404 → 文案为「下载失败,请检查网络(…)」,属 P2 分类行为,本场景不做断言
S5 替换失败回滚+拉起旧版 : 安装"注入基线"(helperFault=relaunch)→ 升级 v0.1.1 → Swap 成功、拉起新版失败
                          → 回滚 → 旧版 v0.1.0 被拉起 → **重启后** toast「上次更新未成功,已回滚在 v0.1.0,详情见日志」
                          → last-result.json {ok:false,targetVersion:v0.1.1,reason:无法启动新版本}
                          → 应用内版本仍 v0.1.0、bundle 完好(open 可再启)
S6 跳过版本回归         : 发现 v0.1.1 → 跳过 → 自动检查静默、手动检查仍展示(等价 P1 场景)
S7 数据边界与回归       : settings.json/packs/轨迹 sha256 前后一致;连接/解析/服务端基本操作可用(AC-10/15)
```

证据留存要求(每个场景):命令 + 原始输出、`~/Library/Caches/gbt32960-simulator/updates/helper.log` 与 `last-result.json` 副本、应用内版本截图、`.bak-*`/`.old` 清理前后 `ls -l`、数据指纹前后 sha256、时间戳;目录 `.superpowers/sdd/updater-p3/evidence/<scenario>/`。

- [ ] **Step 3: Windows 真机走查(外部前置;不可用时显式标注风险)**

外部前置:Windows 10/11 机器、Go 1.25、Wails CLI v2.15.0、NSIS、WebView2 Runtime。
```
W0 基线安装:v0.1.0 官方安装包为 machine 范围,不能作 per-user 自更新基线(ADR-0002)→ 在 Windows 上本地构建
   wails build -platform windows/amd64 -nsis -installscope user -webview2 download -ldflags "-X main.version=v0.1.0"
   安装 → 确认路径位于 %LOCALAPPDATA%\Programs\...(per-user)
W1 正常升级:应用内升级到 v0.1.1 → 新版本运行;%TEMP%\gbt32960-helper-*.exe 已生成/清理;.old 已被下次启动清理
W2 不可写指引:icacls "<安装目录>" /deny "%USERNAME%":(W) → 点"安装并重启"
   → 精确文案「安装目录不可写,无法自动更新;请下载安装包手动更新」且应用不退出
   → icacls "<安装目录>" /remove:d "%USERNAME%" 还原
W3 回滚(可选):以注入基线(helperFault=relaunch)重复 S5,核对点同上
```
若 Windows 真机不可用:以 CI windows runner 的 `TestWindows*` 自动矩阵 + 报告中的**显式遗留风险**(Windows 真机未走查:不可写指引文案与 `.old` 清理时序无真机证据)作为替代,并由主控裁决是否放行 P4。

- [ ] **Step 4: 证据归档与台账**

```bash
# 清单页逐场景填写「预期/实际/证据路径/判定」;证据目录不入库(.superpowers/ 已 gitignore)
git status --short   # 不得出现 evidence 文件(确认 gitignore 生效)
```

本任务不产生入库文件(证据全部落在 gitignore 的 `.superpowers/sdd/updater-p3/`);Phase 出口由主控统一提交台账:

```bash
# 主控在 P3 全部任务与 PAC 终验通过后执行:
# GIT_MASTER=1 git add docs/superpowers/plans/2026-09-14-updater-master-plan.md
# GIT_MASTER=1 git commit -m "docs(updater): Master Plan 台账——P3 完成(commit 区间/证据位置)"
```

---

## Self-Review Record

- **Spec coverage:** §3 AC-6/7/8 → T2/T3/T4 + PAC-2/3/4;AC-9(绑定面扩展)→ T6 + PAC-5;AC-12(installscope 部分)→ T8 + PAC-7;AC-13(运行态中断)→ T7 + PAC-6;AC-14(失败闭环)→ T5/T6/T7 + PAC-5/6/8;AC-15(数据边界/清理扩展)→ T1/T5/T6 + PAC-1;AC-10/11(回归/零依赖)→ 各任务 gate + PAC-7。§5.4 全套(触发链/帮助逻辑五步/平台替换表/结果文件/重启顺序/数据边界)→ T2–T6;§5.7(`main()` 最早期拦截)→ T5;§5.8(临时目录矩阵 → T2/T3/T4/T5;CI windows runner → T8;v0.1.1 真机演练 → T9)。P4 内容(`wails.json` 版本注入、frontend-ui-spec 9 分类、README/backlog)→ **未纳入本计划**(AC-12/15 的 P4 部分)。
- **Placeholder scan:** 无 TBD/TODO;唯一"待填"是 T8 Step 4 的 CI Run URL 与 T9 的场景实测结果(证据类,由执行时填写)。
- **Type/interface consistency:** `HelperArgs{ParentPID,Artifact,Target,Tag,Result,Log}` 在 T1 定义、T5 解析/编码、T6 构造三处一致;`HelperDeps` 六函数签名在 T1 定义、T2/T3/T4 实现、T5 调用一致;`LastResult`(文件 schema)与 `ApplyOutcome`(绑定面,多 `present`)分工在 T1/T6/T7 三处一致;错误值/文案在 Global Constraints、T1–T8 实现与 PAC 三处一致;`helperFault` 取值 `stage|swap|relaunch` 在 T5 实现与 T9 演练命令一致;`renameRetries=[200ms,500ms,1s]` 在 T3 与 PAC-3 一致。
- **Level completeness:** 9 任务均含 Level 与 Level Rationale(T1/T4 = L2;T2/T3/T5/T6/T7/T8/T9 = L3)。
- **Task Gate completeness:** L2 = task reviewer + focused checks;L3 = task reviewer + linked AC(+证据),与 Level 匹配。
- **Decomposition decision:** 本 Phase 为已批准 4 阶段拆分之 P3;自身 9 个任务(≤8–10),不再二次拆分。三平台替换拆为 3 个任务的理由:验证宿主不同(darwin 本机 / CI windows / CI linux),失败面互不相同。
- **依赖完整性:** T1 → T2/T3/T4(平台实现)→ T5(编排)→ T6(bridge)→ T7(前端)→ T8(CI 固化)→ T9(真机);T3/T4 的 CI 证据依赖 T8 建立工作流后回填(Task Gate 已标注)。
- **跨 Phase 协调点:** ①`CleanupCache` 只清 tag 子目录、保留根级 `last-result.json`/`helper.log`(P2 已实现,P3 消费依赖此行为,不改动白名单);②`CheckUpdate`/`DownloadUpdate` 现有单飞语义扩展为含 `applying`,P2 文案与用例不变;③`WireUpdaterStartup` 只追加不修改既有清理调用;④P2 生成的 `Artifact.Path` 跨会话必失(D9),`ready` 仅存内存并在 `CheckUpdate` 成功时复位。
- **已解决的规格歧义(逐条记录):**
  1. **暂存位置 vs 缓存白名单**:P2 交接备忘称"helper 暂存目录必须置于 tag 子目录内",但设计 §5.4/D16 要求暂存与目标**同卷**(跨卷 rename 不可行),缓存目录不保证与安装目录同卷 → **以 D16/§5.4 为准**:暂存置于目标同目录;缓存根目录不新增持久文件(白名单不变)。
  2. **`ConsumeLastResult` 的返回类型**:文件 schema 固定为 `{ok,targetVersion,reason?,logPath}`,无法表达"无记录";绑定面指针返回在 wailsjs 生成中不可空 → 引入 `ApplyOutcome{Present,...}` 包装(文件 schema 不动)。
  3. **后台清理"新实例启动成功后清理"**:helper 无法可靠判定 `open` 后的新实例是否健康 → 由**新实例自身启动时**清理 `.bak-*`/`.old`(即 T6 的启动清理扩展),与 Windows 表"`.old` 由下次启动清理"一致。
  4. **AC-14 toast 文案**:§5.4 固定文案 `上次更新未成功,已回滚在 vX,详情见日志` 不含原因正文 → 原因与日志路径落在 `last-result.json`/`helper.log`(固定位置),toast 逐字使用规格文案;X 取 `CurrentVersion()`(回滚后在运行的版本)。
  5. **真机回滚的确定性触发**:§5.8 要求真机覆盖"替换失败回滚",但只读目录会被预检拦截(走指引而非回滚)→ 引入**仅构建期** `helperFault`(`-X`)注入 `relaunch`(交换成功、拉起失败 → 真备份还原),并在 T5 用例锁定语义;运行时无任何开启途径。
  6. **版本比较谓词(P2 交接备忘 3 更正)**:P2 备忘称 `IsNewer("v0.1.0","0.1.0")=true`,实测为 false(`semver.go` 对两侧统一 `TrimPrefix` 后数值比较,等值恒判 0);等值场景已被 `!IsNewer` 覆盖,故 ApplyUpdate 不引入额外等值守卫,与 `DownloadUpdate` 完全一致。
  7. **Windows 不可写分支的本地验证**:Windows 只读目录无稳定非提权构造 → 该分支由 `writableProbe`(darwin/Linux 用例)+ T5 矩阵覆盖,Windows 文案由真机 `icacls` 走查(T9 W2)取证。
  8. **`ApplyUpdate` 返回值**:最小化返回 `error`(UI 已有 `ready.tag`);成功 = helper 已拉起,失败 = 不退出应用。
- **可执行性自检:** PAC 中 `grep`/`go vet`/`GOOS=... go vet`/`go test ./...`/`git diff --stat`/四件套命令已在本仓库当前状态实测可运行(2026-09-17:`GOOS=windows|linux go vet ./internal/updater/` 通过;`grep -c "^func (s \*UpdaterService) [A-Z]" bridge/updater_service.go` = 4;`git merge-base HEAD origin/main` 可解析为 `4cb02d2`);引用"待创建"的测试命令均注明创建任务(T1–T7)。

## Execution Handoff

计划已保存至 `docs/superpowers/plans/2026-09-17-updater-phase-3-apply-plan.md`;Master Plan 台账 P3 行已更新(`Plan` → 本文件,`Status` → in-progress)。

> Phase 出口治理(主控):全部任务与 PAC 终验通过后,更新 Master Plan 台账 P3 行(status=done、commit 区间、证据位置),按 P1/P2 惯例独立 ledger 提交。

三种执行方式:

**1. Quality-LTDD(推荐)** - Subagent-Driven + 验收门 + Final Intent Guard。每个任务先过实现者自检与 Task Reviewer;L3 任务额外跑 linked AC;全部完成后 Final Reviewer 与 plan 级 AC 终验。含两处人工步骤(T8 Step 4 的 CI 触发确认、T9 真机演练发布与签名),由主控协调执行;T9 含 macOS 主路径与 Windows 真机两条外部依赖链。

**2. Subagent-Driven** - 每任务新 subagent + 两阶段评审(spec 合规 + 代码质量)。验收门较轻。

**3. Inline Execution** - 本会话内批量执行 + 检查点人工确认。适合逐任务在场审阅(T2–T5 的平台顺序建议逐个确认后继续)。

Quality-LTDD 与 Subagent-Driven 的关系:前者在后者之上增加 Final Intent、任务级 AC、Final Reviewer、plan 级 AC 与失败裁决。Inline 为手动检查点路径。

Which approach?
