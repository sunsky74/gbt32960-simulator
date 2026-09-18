# 待办与设计草案(Backlog)

> 本文件收录已形成结论、尚未排期实施的功能设计与未来工作项。
> 启动实施时应升级为 `docs/superpowers/specs/` 下的正式 spec,并按既有流程(计划 → 双门评审 → 验收)执行。

最后更新:2026-09-17

## 1. 服务端模式:本地终端测试增强(断言 / 用例 / 报告)

### 1.1 背景与目标

服务端模式(`internal/servermode`)的产品定位是**给车端终端做本地测试**:真实或模拟 T-Box 连上来,
验证其行为是否符合 GB/T 32960。现状能力是"监控台"——会话管理、报文流/详情、导出、空闲检测、
平台下发——缺三件事:**断言(通过/失败结论)、用例编排、测试报告输出**。

目标:把"连上来跑一圈"升级为"跑完有结论、有证据、可出报告",且不改变任何既有自动应答行为。

### 1.2 能力模型(方案三层)

| 层 | 能力 | 说明 |
|---|---|---|
| Phase 1 | 被动检查清单 | 固定规则集持续评估,每个登入 VIN 一份报告;零操作自动跑 |
| Phase 2 | 主动单步探测 | 人工触发 0x80 参数查询(及扩展包 down 命令),校验终端应答 |
| Phase 3 | 内置多步用例 | Go 内置固定用例(等登入→等心跳→等上报→探测);暂不做脚本语言/DSL |

### 1.3 v1 检查规则(6 条)

| RuleID | 检查点 | 判定 | 依据 |
|---|---|---|---|
| `login.ack` | 登入应答 | RX 0x01/0x05 后 ≤2s 收到同命令 TX,应答标志 0x01;收到 0x02/超时 = fail | FrameEvent(新增 `Resp` = raw[3]) |
| `login.duplicate` | 重复 VIN 登入 | 同 VIN 二次登入被拒 → warn | WarnEvent(新增 Code/VIN) |
| `heartbeat.cadence` | 心跳周期/漂移 | 相邻 RX 0x07 间隔在容差内;样本 <2 时显示"观察中" | FrameEvent |
| `report.cadence` | 实时上报周期 | 针对 RX 0x02;**0x03 补发不参与周期判定** | FrameEvent |
| `frame.integrity` | 帧完整性 | 解码失败/超长帧计数 >0 → fail(附原始 hex 证据) | WarnEvent(Code/VIN) |
| `frame.unknown` | 未知命令/加密帧 | unknown/encrypted → warn(未知命令可配为 fail) | FrameEvent.Kind |

v1 明确**不做**车速/SOC/坐标等数值语义断言(车型差异大、误报率高;留作 v2 可选 warn-only 规则)。

### 1.4 架构

- 新包 `internal/checks`:纯状态机(model/engine/rules/probe/scripts),依赖 `servermode` 单向;
  单测直接构造事件驱动,无网络。
- `bridge/server_service.go` 扇出 Hooks(现有 Hooks 是单消费者):
  `OnSession/OnFrame/OnWarn` → `emit(...)` + `checks.OnXxx(...)`。
- 新事件 `server:check`:低频、全量、按 VIN 替换(仅在结论变化 / 会话上下线 / 探测完成 / 用例步进时 emit,绝不逐帧)。
- 新 RPC:`CheckReports` / `ResetChecks` / `ExportReport` / `CheckConfig` / `SetCheckConfig`;
  Phase 2 增加 `RunProbe`;Phase 3 增加 `RunCase`。
- servermode 增量改动(4 处,全部向后兼容):
  1. `FrameEvent` 增 `Resp string`(取 `raw[3]`,结构化应答标志,避免解析中文 Summary);
  2. `WarnEvent` 增 `Code string` + `VIN string`(机器可匹配、可按 VIN 归因);
  3. `removeConn` 对未登入连接补 warn(补齐"连上永不登入"盲区);
  4. (Phase 2)`WriteFrameVIN` 从 `c.vin == vin` 改为遍历 `c.vins`(平台链路多车可下发)。
- 持久化:检查阈值随 `server.json` 存(扩展 `ServerConfig`;**注意 `UpdateIdle` 重建结构体时须保留新字段**);
  报告仅内存 + 手动导出 TXT/JSON,**不自动落盘**(与既有 D3 决策一致)。
- UI:会话行加结论徽标;底部区改 Tabs(报文详情 / 检查清单);顶栏加汇总与导出。

### 1.5 关键决策

1. **结论单调恶化(粘性)**:窗口内一次违规即锁定,后续正常不"洗白";`重置` 或重新登入开新报告。
2. **周期容差**:默认 auto(前 3 个间隔中位数作基线,报告注明"学习基线"),容差 = 基线×pct + 200ms 固定余量;兼容本地加速联调。
3. **0x08 不可作服务端主动探测**:本项目中校时是终端→平台方向(`conn.go handleClock` 为收侧应答);
   主动探测主用 0x80(参数查询),0x81 缓做,扩展包命令复用现有 down 模板。
4. **自测终端不自动应答**:模拟器客户端收到下行需人工点「应答」,探测超时文案需区分"未收到应答"与协议违规。

### 1.6 分期与工作量

| 阶段 | 内容 | 工作量 | 验收摘要 |
|---|---|---|---|
| P1 | 被动清单 + 实时 UI + 导出 | ≈3-4 天 | 每个 VIN 自动出检查报告;结论粘性;导出 TXT/JSON 含证据帧;既有测试全绿、go.mod 零变化 |
| P2 | 主动单步探测(0x80) | ≈2 天 | 下发/关联(Dir=rx + 同 VIN + 同 cmd + RS≠0xFE)/RTT/超时记 fail;平台链路可下发任意登入 VIN |
| P3 | 内置多步用例 + 报告时间线 | ≈2-3 天 | 内置用例可运行;步骤级 pass/fail/timeout;报告含时间线;取消无残留 |

### 1.7 非目标

- 非一致性/认证测试套件(失败是"疑似"线索,不逐条覆盖标准条款、不构成权威判定)
- 非车队/负载测试(MaxConns=64 不变,不做多车聚合)
- 无脚本语言/DSL,用例仅 Go 内置
- 不做 2025 主动探测(2025 维持只读)
- v1 不做数值语义断言
- 报告不自动落盘、无数据库
- 不改任何自动应答行为(检查器只观察;探测仅用户显式触发)
- 不做跨重连历史对比(每次登入一份报告)

### 1.8 主要风险

| 风险 | 对策 |
|---|---|
| 应答关联误配 | 仅匹配 Dir=rx + 同 VIN + 同 cmd + RS≠0xFE;FIFO waiter;乱序应答保留原文 |
| 时序容差误报 | auto 基线 + 可配容差;0x03 不计入周期 |
| 平台链路双 VIN 语义 | 规则带适用性(心跳看平台 VIN、上报看车辆 VIN);P2 修 WriteFrameVIN |
| 未登入连接盲区 | v1 以 warn 可见;若需"连接→登入时延",触发条件 = 新增 OnConn Hook(明确不在 v1) |
| 报告生命周期 | Stop 保留内存可导出、再次 Start 清空(重启前提示导出) |

## 2. 未来工作:客户端多连接与压测

现状与结论:**保持单活动连接不变**(`bridge/connection_service.go`:连接新档案前先断开旧客户端,
Runtime 持有单个 `*engine.Client`)。多连接/压测作为未来工作,待需求出现时先评估再排期。

未来若评估,可能的方向与待决问题:

- 方向 A(多客户端编排):N 个 `engine.Client` 实例并行 + 每实例生命周期/状态/日志隔离;UI 表达(多标签或表格)。
- 方向 B(压测):连接数扩展、事件总线(容量 512 非阻塞扇出)与 Forwarder 批量转发的吞吐上限、
  内存上界复核、UI 事件洪峰处理(可顺带做 server 侧帧事件批处理)。
- 待决问题:多连接与扩展包/轨迹回放的交互;单窗口 UI 信息密度;失败隔离与重连策略。
- 触发条件:平台上量联调、多车在线仿真、容量验证需求出现时启动评估(先出设计再排期)。

## 3. 工具链:GitLab CI(自建实例)构建与下载

**现状(2026-09-17)**:`.gitlab-ci.yml` 已提交;首次实测(v0.1.0)全部 job 卡 pending——**公司 runner 的标签未知,
未打标签的 job 无 runner 匹配,需先向运维确认**。当前流水线范围:**Linux 二进制 + Windows exe / NSIS 安装包**
(Linux runner 交叉编译);**macOS(.app.zip)本轮暂不构建**(需 macOS runner,恢复方式见下)。
构建命令与 `.github/workflows/release.yml` 同源;产物上传 Generic Package,Release 页给出永久下载链接。

**待确认(阻塞首次跑通)**:

1. **公司 runner 的标签与可用性(最关键,待确认)**
   - 项目 Settings → CI/CD → Runners:有哪些可用 runner、各自标签、是否 Docker executor;
   - 二选一对齐:① 把 job 的 `tags:` 改为公司 runner 实际标签(拿到标签名后改 CI);② 让 runner 开启 "Run untagged jobs"(CI 不动)。
2. **实例能力**:Package Registry 是否开启(管理员设置);GitLab 版本支持 `release:` 关键字(≥13.x)与 dotenv 变量透传。
3. **内网网络可达性(自建环境最常见阻塞点)**
   - Docker 镜像:`golang:1.25-bookworm`、`alpine:3`、`registry.gitlab.com/gitlab-org/release-cli:latest`;
   - Linux job 内的外部源:`deb.nodesource.com`(Node 22 安装源)、`dl-cdn.alpinelinux.org`(apk)、npm registry(`wails build` 会自动执行 `npm install`)、Go 模块代理(`go install` wails CLI);
   - 不可达时:同步镜像到内网仓库并替换 `image:`,npm / GOPROXY 配内网源(必要时加 `.npmrc`)。
4. **触发条件与生效方式**:默认仅 `v*` 标签触发(与 GitHub Actions 共用同一批 tag);流水线使用 tag 指向提交中的
   `.gitlab-ci.yml`——**修改 CI 后需删除并重打 tag(或打新 tag)才生效**。

**恢复 macOS 的前提**:一台 Mac 注册 shell executor runner(tag `macos`;需 Xcode CLT / Go 1.25 / Node 22);
恢复 `build_macos` job 与 upload / release 中对应条目(参考 git 历史 `ccd462b`)。

**非阻塞备注**:`internal/updater` 硬编码 GitHub Releases(`release.go` 中 `Repo`/`BaseURL` 常量),
GitLab Release 仅作"仓库内直接下载"渠道;若要改为从 GitLab 自动更新,需另立任务改代码。

---

> 注:条目 1 源自 2026-09-10 的架构设计咨询(基于 `internal/servermode` 现有 Hooks/事件流与
> `bridge/server_service.go` 的实地代码分析);实施前建议升级为正式 spec(AC 编号、决策记录等按既有约定)。
