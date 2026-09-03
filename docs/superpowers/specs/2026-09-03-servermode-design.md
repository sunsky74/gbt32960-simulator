# GB/T 32960 模拟器 · 服务端模式(被动接收器) 设计文档

> 状态:已确认(设计讨论结论固化)
> 日期:2026-09-03
> 关联调研:GitHub 技术选型调研(2026-09-03,证据:darkinno-tech/gb32960-go-sdk、SuperAlways/gbt32960-server、本地 Java 原版 gateway-connect-gbt32960、golang/go#66956、CloudWeGo netpoll 实测)
> Three Pillars applicability: yes - 纯技术编码工作(Go TCP 服务 + Vue 前端接线),按 three-pillars §1 判定
> Project type: general-backend(混合前端)- 主交付物为内嵌 Wails 桌面应用的 TCP 服务组件 + 配套页面;不确定时按更严路径取 general-backend

---

## 1. Final Intent(最终意图)

用户(车端/BOX/网关开发者)在本机一键启动一个 GB/T 32960 **平台侧接收服务**,用于观察与验证车端实现的上报行为:

1. 接收车端 TCP 连接,按协议**全自动应答**(登入/实时/补发/心跳/校时/登出),车端流程可完整走通;
2. 会话与报文**实时展示**在服务端模式页面(会话表 + 报文表 + 字段级解析);
3. 报文可**手动导出**为文本日志,便于离线分析。

**明确不做**:平台下行命令(0x80/0x81/0x82/0x8A 的主动下发)、VIN 白名单、自动落盘、TLS、2025 平台侧密钥交换。

## 2. Three Pillars

### Overall Business Flow

服务端模式是模拟器的第三种工作形态,与"客户端模拟"(本工具扮演车端连平台)互补:服务端模式下**本工具扮演平台**,接收真实车端/BOX 或另一个模拟器实例的上报。它复用报文解析资产(`internal/parser`、扩展包解码)做字段级展示,但不与客户端引擎(`internal/engine`)发生任何耦合——同一时刻两种模式可并存(客户端模拟连远端平台,服务端模式收本机车端)。触发者:用户在服务端模式页点击"启动服务";消费者:联调/测试人员,通过页面观察车端行为是否合规。

### Current Requirement Flow

车端连接 → 服务端 accept(连接数上限内)→ 等待 0x01 车辆登入 → 全放行:注册会话(VIN 索引)+ 回成功应答 → 之后车端可发 0x02 实时 / 0x03 补发 / 0x07 心跳 / 0x08 校时,服务端逐帧解码、发事件、自动应答 → 0x04 登出回应答后延迟断开;空闲检测开关开启时,空闲超出配置时长的连接被关闭并在控制台输出 → 会话注销、offline 事件。异常分支:BCC/长度校验失败丢帧重同步(不断连);未登入连接发受限命令丢弃并告警;2025($$)帧只读展示不应答;加密帧原样 hex 展示并提示不支持。所有收到 的帧进入 500 条环形缓冲,页面实时渲染最近 200 条,用户可手动导出全部缓冲。

### Current Requirement Technical Architecture

#### Development Architecture

新增包 `internal/servermode/`(与 `internal/engine` 零相互 import):

```
internal/servermode/
├── server.go      # Server:Start(ctx, addr)/Stop();accept loop、maxConns=64、
│                  #   per-conn goroutine、WaitGroup 优雅停机
├── conn.go        # 单连接帧循环 + 空闲检测(开关+时长可配,默认开/60s);
│                  #   连接状态 {VIN, authed, version, loginAt, lastSeen}
├── registry.go    # SessionRegistry:RWMutex + map[VIN]*Conn;putIfAbsent 语义(重复登入后到者拒绝)
├── handlers.go    # 0x01~0x08 处理器矩阵 map[cmd]Handler
├── reply.go       # 唯一应答出口:同命令码 + 应答标志 0xFE;校时回 BeanTime(时间源可注入)
├── decode.go      # 帧分流:## → 2016 闭环;$$ → 2025 只读(不应答)
└── ringbuffer.go  # 报文环形缓冲(500 条)+ 事件发射

bridge/server_service.go   # ServerService:Start(addr)/Stop()/Status()/Sessions()/
                           # ExportLog(path);EventsEmit 出口(server:* 事件)
```

前端:`ServerModePage.vue` 激活(表单/会话表/报文表/导出),报文详情抽屉复用 `ByteGridView` + `FieldTableView`。

#### Existing Architecture Fit

| 现有资产 | 复用方式 |
|---|---|
| `gb32960` 协议库 | 帧同步/编解码/BeanTime;`api.GetCodec(version, type)` 分发 |
| `internal/parser` | 2025 只读解析、报文字段级解析(含 `ParseWithPack` 扩展包解码) |
| `internal/store` | 监听地址持久化(键 `server.json`) |
| `bridge/forwarder.go` 模式 | EventsEmit 事件总线接线惯例 |
| `internal/engine` 的 Client | **仅测试复用**:集成测试里扮演真实车端驱动 |

缺口:无——标准库 `net` 覆盖监听/连接/goroutine 需求,无需新增任何中间件或依赖。

#### New Architecture Enablement

**不引入任何新架构元素**。理由(证据链):1~50 连接规模下 goroutine-per-conn 内存开销 ≈ 200KB(golang-nuts:~4KB/goroutine);Go 官方在 golang/go#66956 明确 std `net` 定位足够;CloudWeGo 实测 goroutine-per-conn 的 p99 优于事件驱动库;唯一同类 Go 实现 darkinno-tech/gb32960-go-sdk 用纯标准库压测 10k 并发。若引入 gnet/ants:event-loop 不可阻塞戒律、外部依赖、Windows 桌面场景打折,而收益区间(10k+ 连接)远超本期——"不引入会怎样"的答案:没有任何功能或性能损失。

## 3. Final Acceptance Checklist (Draft)

- [AC-1] (Source: Overall Business Flow) 启动服务后,车端(本模拟器客户端引擎或真实设备)可完成 0x01 登入 → 0x02/0x07/0x08 → 0x04 登出全流程,页面实时展示会话上线/离线与每条报文
- [AC-2] (Source: Current Requirement Flow) 0x01 自动回成功应答;同 VIN 第二连接登入回失败应答且原会话不受影响;0x08 校时应答体含当前系统时间(BeanTime)
- [AC-3] (Source: Current Requirement Flow) 0x04 登出:车端收到应答后连接才被服务端关闭(延迟 ~200ms)
- [AC-4] (Source: Current Requirement Flow) BCC 校验失败/垃圾字节流不断连,重新同步后可继续收帧并产生告警事件;2025($$)帧解析展示并标注"只读",不产生应答;加密帧原样展示并提示不支持
- [AC-5] (Source: Current Requirement Flow) 手动导出生成文本日志(时间/VIN/命令/hex),内容与环形缓冲一致
- [AC-6] (Source: Development Architecture) `internal/servermode` 不 import `internal/engine`;`bridge.ServerService` 提供 Start/Stop/Status/Sessions/ExportLog 绑定;重复 Start 幂等
- [AC-7] (Source: Existing Architecture Fit) 复用 gb32960/parser/store/forwarder 现有资产,`go.mod` 零变化(零新依赖)
- [AC-8] (Source: Existing Architecture Fit) 服务未启动时,现有功能逐字节不变(现有测试全绿,其他页面无任何 UI/行为变化)
- [AC-9] (Source: New Architecture Enablement) 全部安全边界生效:默认 127.0.0.1(非环回绑定前端二次确认)、maxConns=64、单帧 8KB 上限、环形缓冲 500 条、停机 ≤2s
- [AC-10] (Source: Current Requirement Flow) 空闲检测开关:开启时(默认 60s,可配置并即时生效)空闲超出时长的连接被关闭,产生 offline 事件与控制台「空闲超时关闭」输出;关闭时不做任何空闲剔除,连接可长期挂起

## 4. 端到端业务流

```
联调人员                        模拟器(平台角色)                    车端/BOX/另一实例
   │  服务端模式页,填端口           │                                     │
   │────── 启动服务 ──────────▶│ net.Listen(127.0.0.1:port)          │
   │                           │◀──────── TCP 连接 ──────────────────│
   │                           │◀── 0x01 车辆登入 ───────────────────│
   │                           │ 注册会话 + server:session 事件       │
   │                           │─── 0xFE 成功应答 ──────────────────▶│
   │  页面:会话表上线             │◀── 0x02/0x03/0x07/0x08 ────────────│
   │  页面:报文表+字段解析         │ 解码 + server:frame 事件 + 自动应答  │
   │                           │◀── 0x04 车辆登出 ───────────────────│
   │                           │ 应答 → 延迟 200ms → close + offline  │
   │  导出日志(可选) ─────────▶│ ExportLog → 文本文件                  │
```

## 5. 技术契约

### 5.1 协议行为矩阵(2016,##)

| 命令 | 行为 | 应答 |
|---|---|---|
| 0x01 车辆登入 | 全放行;注册会话(putIfAbsent,后到者失败);authed=true | 0xFE + 结果字节(成功 0x01/失败 0x02) |
| 0x02 实时 | authed 门禁(未登入:丢弃 + warn,不应答);解码发事件 | 0xFE + 结果字节 |
| 0x03 补发 | 同 0x02(逐帧应答) | 同上 |
| 0x04 车辆登出 | 回应答 → 注销会话 → 延迟 ~200ms close | 0xFE + 结果字节 |
| 0x07 心跳 | 刷新读超时 | 0xFE + 结果字节(空体) |
| 0x08 校时 | 时间源可注入(测试固定时钟) | 0xFE + BeanTime 当前时间(6B BCD) |
| 0x05/0x06 平台链路 | warn 事件,不应答 | — |

### 5.2 超时与容错矩阵

| 场景 | 行为 |
|---|---|
| 空闲检测开关 ON(默认) | 空闲时长(默认 60s,可配 5~3600)内无任何帧的连接(登录前后统一)→ close + offline 事件 + 控制台输出(server:warn「空闲超时关闭」) |
| 空闲检测开关 OFF | 不校验空闲连接(可无限期挂起;用于观察车端在无服务端干预下的重连/保活行为) |
| BCC/长度校验失败 | 丢弃该帧重新同步(不断连)+ warn 事件 |
| 单帧 > 8KB | 丢弃重同步 + warn |
| 非法命令码 | 展示 + warn,不应答 |
| 连接数 > 64 | 新连接立即 close |

### 5.3 事件契约(server:*)

| 事件 | 载荷 |
|---|---|
| `server:status` | `{running, listenAddr, error?}` |
| `server:session` | `{vin, peer, state: online/offline, lastSeen}` |
| `server:frame` | `{time, vin, cmd, hex, summary}` |
| `server:warn` | `{note, hex?}` |

### 5.4 前端契约

- 启动表单:IP(默认 127.0.0.1)+ 端口(默认 32960)+ 空闲检测开关(默认开)+ 空闲时长秒数(默认 60,范围 5~3600),均持久化 `store/server.json`;启动/停止按钮状态机;非环回地址二次确认
- 会话表:VIN / IP / 端口 / 状态 / 最后活跃;报文表:时间 / 来源 / 命令 / hex(前端渲染最近 200 条)
- 报文行点击 → 详情抽屉:`ByteGridView` + `FieldTableView`(走 `ParseWithPack`,支持扩展包自定义单元解码)
- 导出:wails 保存对话框 → 文本行 `[时间] [VIN] [命令] [hex]`

### 5.5 全局约束

- `internal/servermode` 禁止 import `internal/engine`;不共享 Runtime/Bus
- `go.mod` 零变化;`app.shutdown → Server.Stop()`(≤2s)
- 服务端模式不影响客户端模拟的任何行为(两模式可并存)

## 6. 范围边界

**本期做**:2016 完整接收闭环、2025 只读展示、事件推送、会话/报文展示、手动导出、安全边界全套。

**本期不做(预留演进)**:平台下行命令、VIN 白名单、自动落盘/滚动日志、TLS 监听、2025 应答与平台侧密钥交换、多实例集群。

## 7. 关键技术决策记录(已确认)

| # | 决策 | 依据 |
|---|---|---|
| D1 | 定位:被动接收器(无下行) | 用户裁决(2026-09-03) |
| D2 | VIN 全放行 + 全自动应答;同 VIN 重复登入后到者拒绝 | 用户裁决;Java 原版 putIfAbsent 语义 |
| D3 | 展示 + 手动导出(不自动落盘) | 用户裁决 |
| D4 | 2016 闭环 + 2025 只读 | 用户裁决;加密平台侧角色未验证,风险隔离 |
| D5 | 零新第三方依赖;TLS/密钥交换排除 | 用户裁决;调研证据链 |
| D6 | 事件推送型数据流(Go 持状态 + EventsEmit + 挂载快照) | 用户裁决;与 forwarder/ConsolePanel 同构 |
| D7 | 标准库 net goroutine-per-conn(非 gnet/evio/ants) | 调研:golang/go#66956、CloudWeGo p99 实测、darkinno 10k 压测 |
| D8 | 登出应答后延迟 ~200ms 断开 | SuperAlways 实践:车端须先收到应答 |
| D9 | 集成测试以 internal/engine Client 为车端驱动 | 资产红利:现成 2016 客户端 |
| D10 | 空闲检测:可配置开关(默认开,时长默认 60s/范围 5~3600,登录前后统一),关闭时完全不校验;替代原固定两级超时(登录前 30s/登录后 5min) | 用户裁决(2026-09-03 spec 评审) |
