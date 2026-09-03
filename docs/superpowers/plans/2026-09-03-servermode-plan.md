# 服务端模式(被动接收器)实施计划

> **For agentic workers:** Recommended execution: use superpowers:ltdd for Quality-LTDD (Subagent-Driven Development + Acceptance Gates + Final Intent Guard). Alternatives: use superpowers:subagent-driven-development for lighter subagent execution, or superpowers:executing-plans for inline checkpoint execution. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Wails 模拟器中实现 GB/T 32960 平台侧接收服务(接收+展示+自动应答,无下行),激活服务端模式页面。

**Architecture:** 新增 `internal/servermode` 包(标准库 net goroutine-per-conn,与 `internal/engine` 零耦合),经 `bridge/server_service.go` 绑定前端并以 `server:*` 事件推送;帧拆包复用从 engine 提取的 `internal/framing.FrameReader`,协议解码复用 gb32960 库 `codec.ProtocolCodec`。

**Tech Stack:** Go 标准库 net / gb32960 协议库(frame/codec/types/model)/ Vue 3 + ant-design-vue(既有)/ internal/store 持久化。零新第三方依赖。

**关联 spec:** `docs/superpowers/specs/2026-09-03-servermode-design.md`(D1~D11 决策,AC-1~AC-11)

**分解判定:** 不做 Master/Phase 分解——单一业务流(接收闭环),9 个任务,同质风险模型,预估行数低于 2000 上限,无独立发布阶段(acceptance-driven-plan §0.1 触发条件均不满足)。

## Global Constraints

- `internal/servermode` 禁止 import `internal/engine`(spec §5.5;AC-6 用 `go list -deps` 验证)
- `go.mod` 零变化(spec D5/AC-7)
- 默认监听 127.0.0.1;maxConns=64;单帧上限 8KB(超长丢弃重同步不断连);报文环形缓冲 500 条;停机 ≤2s(spec §5.5/AC-9)
- 空闲检测:开关默认开,时长默认 60s 可配 5~3600,登录前后统一;关闭时完全不校验(D10/AC-10)
- 应答语义:请求帧应答标志=0xFE(types.ResponseCommand),应答帧=0x01 成功/0x02 失败(types.ResponseSuccess/ResponseFailed);校时应答体=BeanTime 6B 十进制(ext.EncodeBeanTime)(spec §5.1)
- 同 VIN 重复登入:后到者回失败应答,原会话保留(D2/AC-2)
- 0x04 登出:回应答后延迟 200ms 断开(D8/AC-3)
- 2025($$)帧只读展示不应答;加密帧(加密标志≠0)原样展示(D4)
- 未知命令字 kind=unknown、加密帧 kind=encrypted,前端特殊颜色(D11/AC-11)
- 服务未启动时现有功能逐字节不变(AC-8:既有 `go test ./...` 全绿)
- 提交信息:SEMANTIC 前缀 + 中文(仓库惯例),每任务至少 1 次提交

## Final Acceptance Checklist (Refined from Spec) - MUST

- [AC-1] (Source: Overall Business Flow) 车端全流程闭环
  Refinement: `go test ./internal/servermode/ -run TestIntegrationFullFlow -v` — engine.Client 对本地 server 完成 0x01→online→0x02/0x07/0x08→0x04,期间 server 发出 session online/offline 与每帧 frame 事件
- [AC-2] (Source: Current Requirement Flow) 应答与准入
  Refinement: `go test ./internal/servermode/ -run 'TestBuildReply|TestRegistryDuplicateLogin' -v` — golden 字节断言(0x01 成功/失败、0x07 空体、0x08 固定时钟 BeanTime);同 VIN 第二连接登入收到 ResponseFailed 且第一会话不受影响
- [AC-3] (Source: Current Requirement Flow) 登出延迟断开
  Refinement: `go test ./internal/servermode/ -run TestIntegrationLogoutDelayedClose -v` — client 的 0x04 ack 先于连接关闭被收到(事件顺序断言)
- [AC-4] (Source: Current Requirement Flow) 容错与 2025 只读
  Refinement: `go test ./internal/framing/ ./internal/servermode/ -run 'TestFrameReaderResync|TestDecode|TestFrameTooLarge' -v` — 垃圾流重同步不断连(重同步用例已随 Task 1 迁至 framing 包);未知命令 kind=unknown;加密帧 kind=encrypted(Decode 前短路);V2025 帧产生事件但无应答字节写出
- [AC-5] (Source: Current Requirement Flow) 手动导出
  Refinement: `go test ./internal/servermode/ -run TestRingBufferExport -v` — 导出行数=缓冲帧数,行格式 `[时间] [VIN] [命令] [hex]`
- [AC-6] (Source: Development Architecture) 架构隔离与绑定面
  Refinement: `go list -deps ./internal/servermode | grep -c gbt32960-simulator/internal/engine` 输出 0;`go build ./...` 通过(bridge.ServerService 编译即证明绑定面存在)
- [AC-7] (Source: Existing Architecture Fit) 零新依赖
  Refinement: `git diff --stat go.mod go.sum` 为空;`go build ./...` 通过
- [AC-8] (Source: Existing Architecture Fit) 现有行为不变
  Refinement: `go test ./...` 全绿(不含新增用例的原有套件)
- [AC-9] (Source: New Architecture Enablement) 安全边界
  Refinement: `go test ./internal/servermode/ -run 'TestMaxConns|TestFrameTooLarge|TestStopDeadline|TestDefaultConfig' -v` — DefaultConfig.Addr 以 127.0.0.1 开头;第 65 连接被立即关闭;8KB+ 帧丢弃且连接存活;Stop() 2s 内返回
- [AC-10] (Source: Current Requirement Flow) 空闲检测开关
  Refinement: `go test ./internal/servermode/ -run 'TestIdle' -v` — 开(200ms 配置)空闲连接被关+offline+warn;关时空闲 500ms 连接仍存活
- [AC-11] (Source: Current Requirement Flow) 特殊颜色标识
  Refinement: `cd frontend && npm run build`(vue-tsc)通过;Go 侧 `server:frame` 载荷含 kind 字段(decode 单测断言);页面 CSS 类 `.frame-unknown`/`.frame-encrypted` 存在且色相不同(人工验收核对亮暗两主题)

---

### Task 1: 提取 FrameReader 到 internal/framing

**Level:** L2
**Level Rationale:** 代码移动改变 engine 包结构但不改变任何运行行为;后续任务(servermode)依赖它且 engine 自身行为不变——按 L2 走实现者+评审+聚焦测试(既有测试迁移后全绿),不满足 L1(纯移动也需 diff 审查确认无语义漂移)。
**Linked Acceptance Items:** AC-7(go.mod 不变)、AC-8(既有测试全绿)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/framing/framing.go`(git mv 自 `internal/engine/stream.go`)
- Create: `internal/framing/framing_test.go`(测试自 `internal/engine/client_test.go` 迁移)
- Modify: `internal/engine/client.go`(NewFrameReader → framing.NewFrameReader)

**Interfaces:**
- Consumes: 无(纯提取)
- Produces: `framing.NewFrameReader(r io.Reader) *FrameReader`、`(*FrameReader).Next() ([]byte, error)`、`framing.ErrFrameTooLarge`、`framing.MaxFrameLen`(=65535+25);Task 6 的 conn 帧循环消费

- [ ] **Step 1: 移动文件并改包名**

```bash
mkdir -p internal/framing
git mv internal/engine/stream.go internal/framing/framing.go
```

`internal/framing/framing.go` 头部改为:

```go
// Package framing 提供 GB/T 32960 帧流式拆包(半包/粘包/重同步),
// 供客户端引擎与服务端模式共用,不含业务语义。
package framing
```

同时:
1. 把常量 `maxFrameLen` 导出为 `MaxFrameLen`(=65535+24+1=65560,含头 24B+BCC 1B);
2. **新增可配置上限(评审 B4)**:`NewFrameReader(r io.Reader)` 保持 64KB 缺省,新增

```go
// NewFrameReaderLimit 创建带帧总长上限的读取器:声明长度超限时丢弃该帧头
// 并重同步(返回 ErrFrameTooLarge 一次),不阻塞等待超长帧体凑齐。
// 服务端模式用 8KB 上限实现 spec §5.2 的超长防护。
func NewFrameReaderLimit(r io.Reader, maxTotal int) *FrameReader {
	return &FrameReader{r: r, maxTotal: maxTotal, buf: make([]byte, 0, 1024)}
}
```

`FrameReader` 增加 `maxTotal int` 字段(0 = 用 MaxFrameLen);`tryParse` 中长度判定改为:

```go
total := frameHeaderLen + payloadLen + bccLen
limit := fr.maxTotal
if limit <= 0 {
    limit = MaxFrameLen
}
if total > limit {
    fr.buf = fr.buf[2:] // 跳过该伪起始头,重新同步
    return nil, 0, ErrFrameTooLarge
}
```

并在 `internal/framing/framing_test.go` 新增用例:

```go
func TestFrameReaderLimitRejectsOversize(t *testing.T) {
	hdr := append([]byte{0x23, 0x23, 0x07, 0xFE}, []byte("LVBV3J7B0LY000001")...)
	hdr = append(hdr, 0x00, 0x23, 0x28) // 声明 payload=9000
	good := append([]byte{}, hdr...)     // 后续跟一帧合法帧(此处用伪帧仅验证跳过)
	good = append(good, 0x23, 0x23)
	fr := NewFrameReaderLimit(bytes.NewReader(append(hdr, good[24:]...)), 8192)
	if _, err := fr.Next(); err != ErrFrameTooLarge {
		t.Fatalf("超长声明应报 ErrFrameTooLarge, got %v", err)
	}
}
```

(断言目标:第一次 Next 返回 ErrFrameTooLarge 而非阻塞;实现后可再补一帧真实合法帧验证重同步成功。)

- [ ] **Step 2: 修改 engine 引用**

`internal/engine/client.go`:import 增加 `"gbt32960-simulator/internal/framing"`,`readLoop` 中 `fr := NewFrameReader(conn)` 改为 `fr := framing.NewFrameReader(conn)`;删除 engine 内残留的同名引用。

- [ ] **Step 3: 迁移重同步测试**

把 `internal/engine/client_test.go` 中 `TestFrameReaderResyncOnGarbage` 整体移入 `internal/framing/framing_test.go`,包名 `framing`(引用去掉 `engine.` 前缀),函数与断言原样保留。

- [ ] **Step 4: 验证**

```bash
go build ./... && go test ./internal/framing/ ./internal/engine/ -v
```
Expected: 全部 PASS(含迁移后的 ResyncOnGarbage)。

- [ ] **Step 5: Commit**

```bash
git add internal/framing/ internal/engine/
git commit -m "refactor: FrameReader 提取为 internal/framing 供引擎与服务端模式共用"
```

---

### Task 2: servermode 类型基座(Config/Hooks/事件)

**Level:** L2
**Level Rationale:** 纯新增类型与默认值,无行为;但构成后续所有任务的契约面,需评审签名一致性。
**Linked Acceptance Items:** AC-9(DefaultConfig 默认值)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/servermode/types.go`
- Test: `internal/servermode/types_test.go`

**Interfaces:**
- Consumes: 无
- Produces(后续任务全部依赖,签名冻结):
  - `type FrameKind string`;常量 `KindNormal FrameKind = "normal"` / `KindUnknown = "unknown"` / `KindEncrypted = "encrypted"`
  - `type Status struct { Running bool; ListenAddr string }`
  - `type Session struct { VIN, Peer string; LoginAt, LastSeen time.Time }`(会话快照行,带 json tag)
  - `type SessionEvent struct { VIN, Peer string; Online bool; LastSeen time.Time }`
  - `type FrameEvent struct { Time time.Time; VIN, Cmd, Hex, Summary string; Kind FrameKind }`
  - `type WarnEvent struct { Note, Hex string }`
  - `type Hooks struct { OnStatus func(Status); OnSession func(SessionEvent); OnFrame func(FrameEvent); OnWarn func(WarnEvent); Now func() time.Time }`
  - `type Config struct { Addr string; MaxConns int; MaxFrameBytes int; IdleEnabled bool; IdleTimeout time.Duration }`
  - `func DefaultConfig(addr string) Config`(MaxConns=64, MaxFrameBytes=8192, IdleEnabled=true, IdleTimeout=60s)
  - `func normalizeHooks(h Hooks) Hooks`(nil 回调替换为 no-op;Now 缺省 time.Now)

- [ ] **Step 1: 写失败测试**

`internal/servermode/types_test.go`:

```go
package servermode

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("127.0.0.1:32960")
	if cfg.MaxConns != 64 || cfg.MaxFrameBytes != 8192 {
		t.Fatalf("默认上限错误: %+v", cfg)
	}
	if !cfg.IdleEnabled || cfg.IdleTimeout != 60*time.Second {
		t.Fatalf("默认空闲配置错误: %+v", cfg)
	}
}

func TestNormalizeHooksFillsDefaults(t *testing.T) {
	h := normalizeHooks(Hooks{})
	h.OnStatus(Status{})           // 不 panic
	h.OnSession(SessionEvent{})    //
	h.OnFrame(FrameEvent{})        //
	h.OnWarn(WarnEvent{})          //
	if h.Now().IsZero() {
		t.Fatal("Now 未填充默认值")
	}
	fixed := time.Unix(1_700_000_000, 0)
	h2 := normalizeHooks(Hooks{Now: func() time.Time { return fixed }})
	if !h2.Now().Equal(fixed) {
		t.Fatal("自定义 Now 被覆盖")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/servermode/ -run 'TestDefaultConfig|TestNormalizeHooks' -v
```
Expected: FAIL(包/函数不存在,编译错误)。

- [ ] **Step 3: 实现 types.go**

```go
// Package servermode 实现 GB/T 32960 平台侧接收服务(被动接收器):
// 接收车端连接、自动应答、事件推送。与 internal/engine(客户端引擎)零耦合。
package servermode

import "time"

type FrameKind string

const (
	KindNormal    FrameKind = "normal"    // 普通帧
	KindUnknown   FrameKind = "unknown"   // 未知命令字
	KindEncrypted FrameKind = "encrypted" // 加密帧(不支持解密)
)

// 事件结构一律带 json tag:wails EventsEmit 序列化后前端按小写字段消费。
type Status struct {
	Running    bool   `json:"running"`
	ListenAddr string `json:"listenAddr"`
}

type SessionEvent struct {
	VIN      string    `json:"vin"`
	Peer     string    `json:"peer"`
	Online   bool      `json:"online"`
	LastSeen time.Time `json:"lastSeen"`
}

type FrameEvent struct {
	Time    time.Time `json:"time"`
	VIN     string    `json:"vin"`
	Cmd     string    `json:"cmd"`
	Hex     string    `json:"hex"`
	Summary string    `json:"summary"`
	Kind    FrameKind `json:"kind"`
}

type WarnEvent struct {
	Note string `json:"note"`
	Hex  string `json:"hex"`
}

// Session(Sessions 快照)同样带 tag 供前端会话表消费。
type Session struct {
	VIN      string    `json:"vin"`
	Peer     string    `json:"peer"`
	LoginAt  time.Time `json:"loginAt"`
	LastSeen time.Time `json:"lastSeen"`
}

// Hooks 是 servermode 对外的唯一事件出口与时间源;bridge 层注入 EventsEmit,
// 测试注入采集切片与固定时钟。
type Hooks struct {
	OnStatus  func(Status)
	OnSession func(SessionEvent)
	OnFrame   func(FrameEvent)
	OnWarn    func(WarnEvent)
	Now       func() time.Time
}

type Config struct {
	Addr          string
	MaxConns      int
	MaxFrameBytes int
	IdleEnabled   bool
	IdleTimeout   time.Duration
}

func DefaultConfig(addr string) Config {
	return Config{Addr: addr, MaxConns: 64, MaxFrameBytes: 8192, IdleEnabled: true, IdleTimeout: 60 * time.Second}
}

func normalizeHooks(h Hooks) Hooks {
	if h.OnStatus == nil {
		h.OnStatus = func(Status) {}
	}
	if h.OnSession == nil {
		h.OnSession = func(SessionEvent) {}
	}
	if h.OnFrame == nil {
		h.OnFrame = func(FrameEvent) {}
	}
	if h.OnWarn == nil {
		h.OnWarn = func(WarnEvent) {}
	}
	if h.Now == nil {
		h.Now = time.Now
	}
	return h
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/servermode/ -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/servermode/
git commit -m "feat(servermode): 类型基座——Config/Hooks/事件载荷与安全默认值"
```

---

### Task 3: 帧解码分流(decode.go)

**Level:** L2
**Level Rationale:** 单模块行为(输入 raw 帧 → 输出分类结果),聚焦单测可覆盖;kind 判定是 AC-11 的数据基础但本任务不改跨模块行为。
**Linked Acceptance Items:** AC-4(kind 判定)、AC-11(载荷含 kind)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/servermode/decode.go`
- Test: `internal/servermode/decode_test.go`

**Interfaces:**
- Consumes: `codec.ProtocolCodec.Decode`(gb32960 库)、`frame.CommandCode/PayloadType`、`types.ResponseByCode`、Task 2 的 `FrameKind`
- Produces:
  - `type Decoded struct { Raw []byte; PM *frame.ProtocolMessage; Version api.GBTVersion; Cmd byte; VIN string; Kind FrameKind; Encrypted bool; Err error }`
  - `func decodeFrame(raw []byte) Decoded` — Err 仅表示帧结构/BCC 解码失败(PM=nil);Kind: 加密→KindEncrypted、PayloadType(v,cmd)==nil→KindUnknown、否则 KindNormal;2025 判定 `PM.Version == api.V2025`

- [ ] **Step 1: 写失败测试**

`internal/servermode/decode_test.go`(测试内用 engine.BuildFrame 造真实帧——**仅测试**依赖 engine 不违反运行时隔离):

```go
package servermode

import (
	"testing"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/model/gbt2016"
)

const vin17 = "LVBV3J7B0LY000001"

// emptyBody 0x07/0x08 等无载荷体的测试空体(自包含,不依赖 Task 4 的 rawBody)。
type emptyBody struct{}

func (emptyBody) Version() api.GBTVersion { return api.V2016 }
func (emptyBody) Bytes() ([]byte, error)  { return nil, nil }

// loginFrame 造一帧真实 2016 登入(载荷体与引擎 sendLogin 同构:
// 库内 gbt2016.VehicleLogin,ICCID 定长 20,Codes 须与 Count×Length 匹配)。
func loginFrame(t *testing.T) []byte {
	t.Helper()
	raw, _, err := engine.BuildFrame(api.V2016, vin17, 0x01, &gbt2016.VehicleLogin{
		BeanTime:  model.BeanTime{Year: 26, Month: 9, Day: 3, Hour: 10, Minute: 0, Second: 0},
		SerialNum: 1,
		ICCID:     "12345678901234567890",
		Count:     1,
		Length:    1,
		Codes:     []string{"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func heartbeatFrame(t *testing.T) []byte {
	t.Helper()
	raw, _, err := engine.BuildFrame(api.V2016, vin17, 0x07, emptyBody{})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDecodeNormal(t *testing.T) {
	d := decodeFrame(loginFrame(t))
	if d.Err != nil || d.Kind != KindNormal || d.Cmd != 0x01 || d.VIN != vin17 {
		t.Fatalf("decode 异常: %+v err=%v", d, d.Err)
	}
}

func TestDecodeHeartbeatIsNormal(t *testing.T) {
	// 心跳(0x07)载荷类型在库中为 nil("no decodable body"),但属正常命令——
	// 必须判 normal,不得落入 unknown(评审 B3)
	if d := decodeFrame(heartbeatFrame(t)); d.Kind != KindNormal {
		t.Fatalf("心跳 kind = %s, want normal", d.Kind)
	}
}

func TestDecodeUnknownCommand(t *testing.T) {
	// 0x30 在上行预留区但不在服务端已知命令白名单 → unknown
	raw := loginFrame(t)
	raw[2] = 0x30
	d := decodeFrame(raw)
	if d.Err != nil {
		t.Fatalf("预留区命令应可帧解码: %v", d.Err)
	}
	if d.Kind != KindUnknown {
		t.Fatalf("kind = %s, want unknown", d.Kind)
	}
}

func TestDecodeEncryptedShortCircuit(t *testing.T) {
	// 加密帧(加密标志 ≠ 0x01)必须在进入 ProtocolCodec.Decode 前短路分类:
	// 库对非 0x01 加密直接报 ErrEncryptionNotSupported(评审 B2)
	raw := loginFrame(t)
	raw[21] = 0x02 // 加密方式字节位于偏移 21(2 起始 + 1 cmd + 1 resp + 17 VIN)
	d := decodeFrame(raw)
	if d.Err != nil {
		t.Fatalf("加密帧不应报解码错误: %v", d.Err)
	}
	if d.Kind != KindEncrypted || !d.Encrypted {
		t.Fatalf("加密帧 kind = %s, want encrypted", d.Kind)
	}
}

func TestDecodeBCCFail(t *testing.T) {
	raw := loginFrame(t)
	raw[len(raw)-1] ^= 0xFF // 破坏 BCC
	d := decodeFrame(raw)
	if d.Err == nil || d.PM != nil {
		t.Fatal("BCC 损坏应解码失败")
	}
}
```

注:decode_test.go 末尾不再需要额外占位;`import "gbt32960-simulator/internal/engine"` 仅测试用(运行时隔离不破坏)。

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/servermode/ -run TestDecode -v
```
Expected: FAIL(decodeFrame 未定义)。

- [ ] **Step 3: 实现 decode.go**

```go
package servermode

import (
	"fmt"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

// encryptByteOffset 加密方式字节在帧内的偏移:起始符2 + 命令1 + 应答1 + VIN17。
const encryptByteOffset = 21

// encryptionNone 协议规定"不加密"的线上值是 0x01(types.EncryptionNone),不是 0。
const encryptionNone = 0x01

// knownCmds2016 服务端已知(会正常处理/应答)的 2016 命令白名单。
// 注意不能用 frame.PayloadType==nil 判未知:0x07/0x08 载荷类型即 nil(评审 B3)。
var knownCmds2016 = map[byte]bool{
	0x01: true, 0x02: true, 0x03: true, 0x04: true, 0x07: true, 0x08: true,
}

// Decoded 是单帧的解码与分类结果。
type Decoded struct {
	Raw       []byte
	PM        *frame.ProtocolMessage
	Version   api.GBTVersion
	Cmd       byte
	VIN       string
	Kind      FrameKind
	Encrypted bool
	Err       error
}

// decodeFrame 解码一帧并分类。加密判定在进入 ProtocolCodec.Decode 之前完成
// (库对加密标志≠0x01 的帧直接报 ErrEncryptionNotSupported,事后分支不可达,评审 B2):
// raw[21] ≠ 0x01 → KindEncrypted,不解析。BCC/结构错误返回 Err(PM=nil),
// 调用方丢弃该帧并告警(重同步由 FrameReader 保证)。
func decodeFrame(raw []byte) Decoded {
	d := Decoded{Raw: raw}
	if len(raw) > encryptByteOffset && raw[encryptByteOffset] != encryptionNone {
		d.Encrypted = true
		d.Kind = KindEncrypted
		d.VIN = string(raw[4 : 4+17]) // 头部字段直接截取,足够展示用
		d.Cmd = raw[2]
		return d
	}
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		d.Err = fmt.Errorf("帧解码失败: %w", err)
		return d
	}
	pm := msg.(*frame.ProtocolMessage)
	_ = pm.DecodePayload() // 尽力解码载荷,失败不影响帧级分类
	d.PM = pm
	d.Version = pm.Version
	d.VIN = pm.VIN
	if code, ok := frame.CommandCode(pm.RequestType); ok {
		d.Cmd = code
	}
	if pm.Version == api.V2016 && !knownCmds2016[d.Cmd] {
		d.Kind = KindUnknown
		return d
	}
	d.Kind = KindNormal // 2025 全部只读展示,不再细分 unknown
	return d
}

var _ = types.ResponseCommand // 若 types 未被他处使用则移除该行与 import
```

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/servermode/ -run TestDecode -v
```
Expected: 5 个 PASS(Normal/Heartbeat/Unknown/Encrypted/BCC)。

- [ ] **Step 5: Commit**

```bash
git add internal/servermode/decode.go internal/servermode/decode_test.go
git commit -m "feat(servermode): 帧解码分流——加密/未知命令/普通三态分类"
```

---

### Task 4: 应答构造(reply.go)

**Level:** L2
**Level Rationale:** 单模块纯函数(帧字节构造),golden 测试全覆盖;协议正确性由独立拼装的期望字节证明。
**Linked Acceptance Items:** AC-2(golden 字节)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/servermode/reply.go`
- Test: `internal/servermode/reply_test.go`

**Interfaces:**
- Consumes: `frame.ProtocolMessage.Bytes()`、`types.ResponseSuccess/ResponseFailed/ResponseCommand`、`ext.EncodeBeanTime`、`api.V2016`
- Produces:
  - `func buildReply(v api.GBTVersion, vin string, cmd byte, resp types.ResponseType, body []byte) ([]byte, error)` — Task 6 消费
  - `func clockBody(now time.Time) []byte` — 0x08 应答体(BeanTime 6B 十进制)

- [ ] **Step 1: 写失败测试(golden 独立拼装 + 测试内 XOR)**

`internal/servermode/reply_test.go`:

```go
package servermode

import (
	"bytes"
	"testing"
	"time"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

// xorBCC 测试内手写异或(不走 utils.CalcBCC,保证 golden 独立于生产路径,评审 m6):
// BCC = 帧去掉起始符 2B 与末位 BCC 后逐字节异或。
func xorBCC(b []byte) byte {
	var x byte
	for _, v := range b {
		x ^= v
	}
	return x
}

// assemble 独立拼装期望帧。加密字节 = 0x01(协议"不加密"线上值,评审 B1)。
func assemble(cmd byte, resp types.ResponseType, vin string, body []byte) []byte {
	out := []byte{0x23, 0x23, cmd, resp.Code()}
	out = append(out, []byte(vin)...)
	out = append(out, 0x01, byte(len(body)>>8), byte(len(body)))
	out = append(out, body...)
	return append(out, xorBCC(out[2:]))
}

func TestBuildReplyLoginSuccess(t *testing.T) {
	got, err := buildReply(api.V2016, vin17, 0x01, types.ResponseSuccess, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x01, types.ResponseSuccess, vin17, nil); !bytes.Equal(got, want) {
		t.Fatalf("login success reply:\n got %x\nwant %x", got, want)
	}
	// 双保险(评审 B1 回归锚):应答帧必须能被协议库自身 Decode 接受——
	// 车端引擎 readLoop 用的正是这个解码器,解不了等于应答无效。
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(got))
	if err != nil {
		t.Fatalf("应答帧被协议库拒收(加密字节/BCC 错误): %v", err)
	}
	if pm := msg.(*frame.ProtocolMessage); pm.ResponseType != types.ResponseSuccess {
		t.Fatalf("回读应答标志 = 0x%02X", pm.ResponseType)
	}
}

func TestBuildReplyLoginFailed(t *testing.T) {
	got, err := buildReply(api.V2016, vin17, 0x01, types.ResponseFailed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x01, types.ResponseFailed, vin17, nil); !bytes.Equal(got, want) {
		t.Fatalf("login failed reply:\n got %x\nwant %x", got, want)
	}
}

func TestBuildReplyHeartbeat(t *testing.T) {
	got, err := buildReply(api.V2016, vin17, 0x07, types.ResponseSuccess, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x07, types.ResponseSuccess, vin17, nil); !bytes.Equal(got, want) {
		t.Fatalf("heartbeat reply:\n got %x\nwant %x", got, want)
	}
}

func TestBuildReplyClock(t *testing.T) {
	fixed := time.Date(2026, 9, 3, 10, 30, 5, 0, time.Local)
	body := clockBody(fixed)
	if want := ext.EncodeBeanTime(fixed); !bytes.Equal(body, want[:]) {
		t.Fatalf("clock body = %x, want %x", body, want)
	}
	got, err := buildReply(api.V2016, vin17, 0x08, types.ResponseSuccess, body)
	if err != nil {
		t.Fatal(err)
	}
	if want := assemble(0x08, types.ResponseSuccess, vin17, body); !bytes.Equal(got, want) {
		t.Fatalf("clock reply:\n got %x\nwant %x", got, want)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/servermode/ -run TestBuildReply -v
```
Expected: FAIL(buildReply 未定义)。

- [ ] **Step 3: 实现 reply.go**

```go
package servermode

import (
	"time"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/types"
)

// rawBody 以原始字节实现 model.MessageBody(应答场景:结果空体/校时时间)。
type rawBody struct {
	v api.GBTVersion
	b []byte
}

func (r rawBody) Version() api.GBTVersion { return r.v }
func (r rawBody) Bytes() ([]byte, error)  { return r.b, nil }

// buildReply 构造平台应答帧:同命令码 + 应答标志(0x01 成功/0x02 失败)+ body。
// 加密方式必须显式置 EncryptionNone(0x01)——零值 0x00 会被协议库 Decode 拒收(评审 B1)。
func buildReply(v api.GBTVersion, vin string, cmd byte, resp types.ResponseType, body []byte) ([]byte, error) {
	msg := frame.ProtocolMessage{
		Version:     v,
		RequestType: &types.CommandV2016{Code: cmd},
		ResponseType: resp,
		VIN:         vin,
		Encryption:  types.EncryptionNone,
		Payload:     rawBody{v: v, b: body},
	}
	return msg.Bytes()
}

// clockBody 校时应答体:BeanTime 6 字节十进制(年-2000/月/日/时/分/秒)。
func clockBody(now time.Time) []byte {
	b := ext.EncodeBeanTime(now)
	return b[:]
}

var _ model.MessageBody = rawBody{} // 接口守卫
```

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/servermode/ -run 'TestBuildReply|TestClock' -v
```
Expected: 4 个 PASS。若 golden 与协议库 Bytes() 输出不一致,以协议库帧布局(gb32960-go/frame/protocol.go)为准修正 assemble,并在该测试注释记录差异原因。

- [ ] **Step 5: Commit**

```bash
git add internal/servermode/reply.go internal/servermode/reply_test.go
git commit -m "feat(servermode): 平台应答帧构造——0x01/0x07 空体与 0x08 校时 BeanTime,golden 覆盖"
```

---

### Task 5: 会话注册表(registry.go)

**Level:** L2
**Level Rationale:** 单模块并发数据结构,-race 聚焦测试覆盖。
**Linked Acceptance Items:** AC-2(重复登入)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/servermode/registry.go`
- Test: `internal/servermode/registry_test.go`

**Interfaces:**
- Consumes: Task 2 `SessionEvent`(仅语义)
- Produces:
  - `type Session struct { VIN, Peer string; LoginAt, LastSeen time.Time }`
  - `type Registry struct{ ... }` + `func NewRegistry() *Registry`
  - `(*Registry) Register(vin, peer string, now time.Time) (ok bool)` — putIfAbsent 语义
  - `(*Registry) Touch(vin string, now time.Time)`
  - `(*Registry) Remove(vin string) (Session, bool)`
  - `(*Registry) Snapshot() []Session`(按 VIN 排序,bridge Sessions() 消费)

- [ ] **Step 1: 写失败测试**

`internal/servermode/registry_test.go`:

```go
package servermode

import (
	"sync"
	"testing"
	"time"
)

func TestRegistryDuplicateLogin(t *testing.T) {
	r := NewRegistry()
	now := time.Unix(1_700_000_000, 0)
	if !r.Register(vin17, "1.2.3.4:5", now) {
		t.Fatal("首次登入应成功")
	}
	if r.Register(vin17, "5.6.7.8:9", now.Add(time.Second)) {
		t.Fatal("同 VIN 重复登入应被拒绝")
	}
	s := r.Snapshot()
	if len(s) != 1 || s[0].Peer != "1.2.3.4:5" {
		t.Fatalf("重复登入不应覆盖原会话: %+v", s)
	}
}

func TestRegistryRemoveAndTouch(t *testing.T) {
	r := NewRegistry()
	now := time.Unix(1_700_000_000, 0)
	r.Register(vin17, "p", now)
	r.Touch(vin17, now.Add(time.Minute))
	if got := r.Snapshot()[0].LastSeen; !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("Touch 未刷新: %v", got)
	}
	if _, ok := r.Remove(vin17); !ok {
		t.Fatal("Remove 应成功")
	}
	if _, ok := r.Remove(vin17); ok {
		t.Fatal("重复 Remove 应失败")
	}
	if r.Register(vin17, "p2", now) != true {
		t.Fatal("登出后应可重新登入")
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Register(vin17, "p", time.Now())
			r.Touch(vin17, time.Now())
			r.Snapshot()
		}()
	}
	wg.Wait()
	if len(r.Snapshot()) != 1 {
		t.Fatal("并发下会话数应为 1")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/servermode/ -run TestRegistry -v
```
Expected: FAIL(NewRegistry 未定义)。

- [ ] **Step 3: 实现 registry.go**

```go
package servermode

import (
	"sort"
	"sync"
	"time"
)

// Session 结构定义见 types.go(Task 2,带 json tag)。

// Registry VIN→会话索引;putIfAbsent 语义:后到登入被拒,原会话不受影响。
type Registry struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func NewRegistry() *Registry { return &Registry{sessions: map[string]Session{}} }

func (r *Registry) Register(vin, peer string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[vin]; exists {
		return false
	}
	r.sessions[vin] = Session{VIN: vin, Peer: peer, LoginAt: now, LastSeen: now}
	return true
}

func (r *Registry) Touch(vin string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[vin]; ok {
		s.LastSeen = now
		r.sessions[vin] = s
	}
}

func (r *Registry) Remove(vin string) (Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[vin]
	if ok {
		delete(r.sessions, vin)
	}
	return s, ok
}

func (r *Registry) Snapshot() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VIN < out[j].VIN })
	return out
}
```

- [ ] **Step 4: 跑测试(含 -race)**

```bash
go test ./internal/servermode/ -run TestRegistry -race -v
```
Expected: 3 个 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/servermode/registry.go internal/servermode/registry_test.go
git commit -m "feat(servermode): VIN 会话注册表——putIfAbsent 重复登入拒绝与并发安全"
```

---

### Task 6: 服务生命周期与连接帧循环(server.go/conn.go + 环形缓冲)

**Level:** L3
**Level Rationale:** 跨模块核心行为(网络生命周期+协议状态机+事件流),直接承载多条 AC;必须以任务级验收证明。
**Linked Acceptance Items:** AC-1(事件序列,与 Task 7 共同)、AC-4(重同步/加密/未知在线行为)、AC-9(maxConns/8KB/Stop)、AC-10(空闲开关)
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `internal/servermode/server.go`(Server/Start/Stop/Status/accept loop)
- Create: `internal/servermode/conn.go`(连接状态与帧循环/处理器矩阵)
- Create: `internal/servermode/ringbuffer.go`
- Test: `internal/servermode/server_test.go`(容错/上限/空闲/停机单测;全流程集成在 Task 7)

**Interfaces:**
- Consumes: Task 1 `framing`、Task 2 `Config/Hooks`、Task 3 `decodeFrame/Decoded`、Task 4 `buildReply/clockBody`、Task 5 `Registry`
- Produces(Task 8 消费):
  - `func New(cfg Config, hooks Hooks) *Server`
  - `(*Server) Start(ctx context.Context) error`(幂等;端口占用返回错误)、`(*Server) Stop() error`(≤2s)、`(*Server) Status() Status`、`(*Server) Sessions() []Session`
  - `(*Server) UpdateIdle(enabled bool, d time.Duration)`(运行中即时生效,AC-10)、`(*Server) cfgSnapshot() Config`
  - `(*Server) ExportLines() []string`(环形缓冲 → `[时间] [VIN] [命令] [hex]` 文本行)

- [ ] **Step 1: 写失败测试**

`internal/servermode/server_test.go`:

```go
package servermode

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// 采集 Hooks:事件收进切片供断言。回调运行在连接 goroutine、断言运行在测试
// goroutine——必须加锁(评审 M5,-race 门禁)。
type collector struct {
	mu       sync.Mutex
	status   []Status
	sessions []SessionEvent
	frames   []FrameEvent
	warns    []WarnEvent
	now      time.Time
}

func newCollector(fixed time.Time) (Hooks, *collector) {
	c := &collector{now: fixed}
	return normalizeHooks(Hooks{
		OnStatus:  func(s Status) { c.mu.Lock(); c.status = append(c.status, s); c.mu.Unlock() },
		OnSession: func(e SessionEvent) { c.mu.Lock(); c.sessions = append(c.sessions, e); c.mu.Unlock() },
		OnFrame:   func(e FrameEvent) { c.mu.Lock(); c.frames = append(c.frames, e); c.mu.Unlock() },
		OnWarn:    func(e WarnEvent) { c.mu.Lock(); c.warns = append(c.warns, e); c.mu.Unlock() },
		Now:       func() time.Time { return c.now },
	}), c
}

// waitFor 轮询断言辅助:fn 返回 true 前最多等 timeout(fn 自行持锁)。
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("条件在 %v 内未满足", timeout)
}

func startTestServer(t *testing.T, cfg Config, hooks Hooks) *Server {
	t.Helper()
	srv := New(cfg, hooks)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

func dial(t *testing.T, srv *Server) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", srv.Status().ListenAddr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func TestDefaultAddrLoopback(t *testing.T) {
	cfg := DefaultConfig("127.0.0.1:0")
	if len(cfg.Addr) < 9 || cfg.Addr[:9] != "127.0.0.1" {
		t.Fatalf("默认地址必须环回: %s", cfg.Addr)
	}
}

func TestStartIdempotentAndPortInUse(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("重复 Start 应幂等: %v", err)
	}
	srv2 := New(DefaultConfig(srv.Status().ListenAddr), h)
	defer func() { _ = srv2.Stop() }()
	if err := srv2.Start(context.Background()); err == nil {
		t.Fatal("端口占用应报错")
	}
}

func TestFrameTooLargeDroppedConnAlive(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer conn.Close()
	// 声明 payload=9000(>8KB)的伪帧头 + 紧随一帧合法登入。
	// framing 层限长立即丢弃伪头并重同步(不阻塞等 9025 字节凑齐,评审 B4)
	hdr := append([]byte{0x23, 0x23, 0x07, 0xFE}, []byte(vin17)...)
	hdr = append(hdr, 0x00, 0x23, 0x28) // len=9000
	if _, err := conn.Write(append(hdr, loginFrame(t)...)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if w.Note == "帧超长丢弃(>8KB)" {
				return true
			}
		}
		return false
	})
	// 重同步成功:伪帧后的合法登入仍被处理(帧事件含 0x01)
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, f := range c.frames {
			if f.Cmd == "0x01" {
				return true
			}
		}
		return false
	})
}

func TestIdleEnabledCloses(t *testing.T) {
	h, c := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.IdleTimeout = 200 * time.Millisecond
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	waitFor(t, 2*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if strings.HasPrefix(w.Note, "空闲超时关闭") {
				return true
			}
		}
		return false
	})
}

func TestIdleDisabledKeeps(t *testing.T) {
	h, _ := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.IdleEnabled = false
	cfg.IdleTimeout = 100 * time.Millisecond
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	time.Sleep(400 * time.Millisecond)
	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal("空闲检测关闭时连接不应被剔除")
	}
}

func TestIdleUpdateRuntime(t *testing.T) {
	h, c := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.IdleEnabled = false // 初始关闭:连接挂 150ms 不被剔
	srv := startTestServer(t, cfg, h)
	conn := dial(t, srv)
	defer conn.Close()
	time.Sleep(150 * time.Millisecond)
	// 运行中开启(200ms)→ UpdateIdle 逐连接重设 deadline,静默连接在 ~200ms 后
	// 被服务端关闭(评审 M1:靠 poke 生效,不再依赖帧到达)。断言关闭发生在
	// 客户端 2s 读超时**之前**(即确系服务端关闭,非本地超时)。
	srv.UpdateIdle(true, 200*time.Millisecond)
	start := time.Now()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	if err == nil {
		t.Fatal("运行中开启空闲检测后连接应被关闭")
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("连接由客户端读超时关闭而非服务端空闲剔除(耗时 %v)", elapsed)
	}
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if strings.HasPrefix(w.Note, "空闲超时关闭") {
				return true
			}
		}
		return false
	})
}

func TestMaxConns(t *testing.T) {
	h, _ := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.MaxConns = 2
	srv := startTestServer(t, cfg, h)
	c1, c2, c3 := dial(t, srv), dial(t, srv), dial(t, srv)
	defer c1.Close()
	defer c2.Close()
	time.Sleep(200 * time.Millisecond)
	// 第 3 个连接应被立即关闭:读到 EOF
	c3.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := c3.Read(buf); err == nil {
		t.Fatal("超限连接应被关闭")
	}
	c3.Close()
}

func TestStopDeadline(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	done := make(chan error, 1)
	go func() { done <- srv.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 超过 2s")
	}
}

func TestRingBufferExport(t *testing.T) {
	h, _ := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	_, _ = conn.Write(loginFrame(t))
	time.Sleep(200 * time.Millisecond)
	conn.Close()
	lines := srv.ExportLines()
	if len(lines) == 0 {
		t.Fatal("导出不应为空")
	}
	// 行格式: [时间] [VIN] [命令] [hex]
	wantPrefix := "[" + vin17 + "]"
	found := false
	for _, l := range lines {
		if len(l) > len(wantPrefix) && l[1:len(wantPrefix)+1] == vin17 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("导出行缺少 VIN: %q", lines)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/servermode/ -run 'TestDefaultAddr|TestStartIdempotent|TestFrameTooLarge|TestIdle|TestMaxConns|TestStop|TestRingBuffer' -v
```
Expected: FAIL(New 未定义)。

- [ ] **Step 3: 实现 ringbuffer.go**

```go
package servermode

import "sync"

// ring 环形缓冲:超出容量丢最旧。
type ring struct {
	mu   sync.Mutex
	cap  int
	vals []string
}

func newRing(capacity int) *ring { return &ring{cap: capacity} }

func (r *ring) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.vals) >= r.cap {
		r.vals = r.vals[len(r.vals)-r.cap+1:]
	}
	r.vals = append(r.vals, line)
}

func (r *ring) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.vals))
	copy(out, r.vals)
	return out
}
```

- [ ] **Step 4: 实现 conn.go**

```go
package servermode

import (
	"context"
	"fmt"
	"net"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
)

// conn 单个车端连接:帧循环 + 协议状态(authed)。
// serve/nextFrame 见本任务末尾(空闲判定走 cfgSnapshot 支持运行中更新)。
type conn struct {
	nc     net.Conn
	fr     *framing.FrameReader
	srv    *Server
	authed bool
	vin    string
}

func (c *conn) handleRaw(ctx context.Context, raw []byte) {
	// 超长防护在 framing 层完成(NewFrameReaderLimit + ErrFrameTooLarge,见 serve);
	// 此处不再重复判长。
	d := decodeFrame(raw)
	if d.Err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: d.Err.Error(), Hex: fmt.Sprintf("%x", raw)})
		return // 重同步由 FrameReader 保证,不断连
	}
	now := c.srv.hooks.Now()

	// 帧事件(kind 三态;2025 只读标注)
	cmdName := fmt.Sprintf("0x%02X", d.Cmd)
	if d.PM != nil && d.PM.Payload != nil {
		cmdName = fmt.Sprintf("0x%02X", d.Cmd) // 摘要后续可扩展为解码要点
	}
	sum := ""
	if d.Version == api.V2025 {
		sum = "2025 只读,应答未支持"
	}
	c.srv.hooks.OnFrame(FrameEvent{Time: now, VIN: d.VIN, Cmd: cmdName, Hex: fmt.Sprintf("%x", raw), Summary: sum, Kind: d.Kind})
	if c.vin == "" && d.VIN != "" {
		c.vin = d.VIN
	}

	// 加密帧:不解析不应答
	if d.Encrypted {
		c.srv.hooks.OnWarn(WarnEvent{Note: "加密帧不支持解密,仅展示", Hex: ""})
		return
	}
	// 2025:只读,不应答
	if d.Version == api.V2025 {
		return
	}
	// 2016 处理器矩阵
	switch d.Cmd {
	case 0x01:
		c.handleLogin(d, now)
	case 0x02, 0x03:
		c.handleData(d, now)
	case 0x04:
		c.handleLogout(d, now)
	case 0x07:
		c.handleHeartbeat(d, now)
	case 0x08:
		c.handleClock(d, now)
	default: // 0x05/0x06/未知
		c.srv.hooks.OnWarn(WarnEvent{Note: fmt.Sprintf("命令 0x%02X 不在服务端支持范围(平台链路/未知命令)", d.Cmd)})
	}
}

func (c *conn) reply(cmd byte, resp types.ResponseType, body []byte) {
	raw, err := buildReply(api.V2016, c.vin, cmd, resp, body)
	if err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: "应答构造失败: " + err.Error()})
		return
	}
	if _, err := c.nc.Write(raw); err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: "应答发送失败: " + err.Error()})
	}
}

func (c *conn) handleLogin(d Decoded, now time.Time) {
	if ok := c.srv.registry.Register(d.VIN, c.nc.RemoteAddr().String(), now); !ok {
		c.srv.hooks.OnWarn(WarnEvent{Note: "重复登入拒绝: " + d.VIN})
		c.reply(0x01, types.ResponseFailed, nil)
		return
	}
	c.authed = true
	c.vin = d.VIN
	c.reply(0x01, types.ResponseSuccess, nil)
	c.srv.hooks.OnSession(SessionEvent{VIN: d.VIN, Peer: c.nc.RemoteAddr().String(), Online: true, LastSeen: now})
}

func (c *conn) handleData(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的数据帧被丢弃: " + d.VIN})
		return
	}
	c.srv.registry.Touch(c.vin, now)
	c.reply(d.Cmd, types.ResponseSuccess, nil)
}

func (c *conn) handleLogout(d Decoded, now time.Time) {
	// 只回应答;会话注销与 offline 事件由 removeConn 在连接真正关闭后发出
	// (车端先收到 ack、后观察到掉线——与 Task 7 集成断言一致,评审 M2)。
	c.reply(0x04, types.ResponseSuccess, nil)
	time.AfterFunc(200*time.Millisecond, func() { _ = c.nc.Close() }) // 等 ack flush(D8)
}

func (c *conn) handleHeartbeat(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的心跳被丢弃"})
		return
	}
	c.srv.registry.Touch(c.vin, now)
	c.reply(0x07, types.ResponseSuccess, nil)
}

func (c *conn) handleClock(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的校时被丢弃"})
		return
	}
	c.reply(0x08, types.ResponseSuccess, clockBody(now))
}
```

`conn.go` 的 import 含 `"github.com/sunsky74/gb32960/types"`。`serve` 循环(空闲判定走 cfgSnapshot 支持运行中更新;超长帧经 framing 层限长直接告警续读,评审 B4;读超时以 net.Error.Timeout() 识别并输出 spec 指定文案,评审 m2):

```go
func (c *conn) serve(ctx context.Context) {
	defer c.srv.removeConn(c)
	c.fr = framing.NewFrameReaderLimit(c.nc, c.srv.cfgSnapshot().MaxFrameBytes)
	for {
		if ctx.Err() != nil {
			return
		}
		if cfg := c.srv.cfgSnapshot(); cfg.IdleEnabled {
			_ = c.nc.SetReadDeadline(c.srv.hooks.Now().Add(cfg.IdleTimeout))
		} else {
			_ = c.nc.SetReadDeadline(time.Time{})
		}
		raw, err := c.fr.Next()
		if err != nil {
			if ctx.Err() != nil {
				return // 主动停机
			}
			if errors.Is(err, framing.ErrFrameTooLarge) {
				c.srv.hooks.OnWarn(WarnEvent{Note: "帧超长丢弃(>8KB)"})
				continue // 已重同步,继续读后续帧
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && c.srv.cfgSnapshot().IdleEnabled {
				c.srv.hooks.OnWarn(WarnEvent{Note: "空闲超时关闭: " + c.vin}) // spec §5.2 指定文案
				return
			}
			c.srv.hooks.OnWarn(WarnEvent{Note: "连接断开: " + err.Error(), Hex: c.vin})
			return
		}
		c.handleRaw(ctx, raw)
	}
}
```

`conn` 结构体字段:`nc net.Conn; fr *framing.FrameReader; srv *Server; authed bool; vin string`;import 增加 `"errors"`。

- [ ] **Step 5: 实现 server.go**

```go
package servermode

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// Server 平台侧接收服务:accept loop + per-conn goroutine。
type Server struct {
	cfg      Config
	hooks    Hooks
	registry *Registry
	buf      *ring

	mu      sync.Mutex
	ln      net.Listener
	conns   map[*conn]struct{}
	running bool
	cancel  context.CancelFunc
}

// New 创建服务(未启动)。/hooks 经 normalizeHooks 填充默认值。
func New(cfg Config, hooks Hooks) *Server {
	return &Server{
		cfg:      cfg,
		hooks:    normalizeHooks(hooks),
		registry: NewRegistry(),
		buf:      newRing(500),
		conns:    map[*conn]struct{}{},
	}
}

// Start 监听并进入 accept loop。幂等:已运行返回 nil。
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("监听失败: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s.ln, s.running, s.cancel = ln, true, cancel
	s.mu.Unlock()

	s.hooks.OnStatus(Status{Running: true, ListenAddr: ln.Addr().String()})
	go s.acceptLoop(ctx)
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		nc, err := s.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.hooks.OnWarn(WarnEvent{Note: "accept: " + err.Error()})
				continue
			}
		}
		s.mu.Lock()
		if len(s.conns) >= s.cfg.MaxConns {
			s.mu.Unlock()
			_ = nc.Close() // 超限立即拒绝(AC-9)
			continue
		}
		c := &conn{nc: nc, srv: s}
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		go c.serve(ctx)
	}
}

// Stop 停止监听、主动关闭全部连接(阻塞读被唤醒 → removeConn 发 offline)并等待
// 退出(≤2s)。幂等:未运行时直接返回 nil(评审 M4:去掉 stopOnce,支持 停止→再启动→再停止)。
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	ln, cancel := s.ln, s.cancel
	s.running = false
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	var err error
	if ln != nil {
		err = ln.Close()
	}
	if cancel != nil {
		cancel()
	}
	for _, c := range conns {
		_ = c.nc.Close() // 唤醒阻塞读;authed 连接经 removeConn 发 offline + 注销
	}
	deadline := time.After(2 * time.Second) // 停机上限(AC-9)
	for {
		s.mu.Lock()
		n := len(s.conns)
		s.mu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-deadline:
			s.hooks.OnStatus(Status{Running: false})
			return err
		case <-time.After(20 * time.Millisecond):
		}
	}
	s.hooks.OnStatus(Status{Running: false})
	return err
}

// Status 当前运行状态。
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.ln == nil {
		return Status{Running: false}
	}
	return Status{Running: true, ListenAddr: s.ln.Addr().String()}
}

// Sessions 会话快照(bridge 下发前端)。
func (s *Server) Sessions() []Session { return s.registry.Snapshot() }

// ExportLines 导出环形缓冲为文本行: [时间] [VIN] [命令] [hex](AC-5)。
func (s *Server) ExportLines() []string { return s.buf.snapshot() }

// removeConn 连接退出清理 + offline 事件 + 环形行落地。
func (s *Server) removeConn(c *conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	_ = c.nc.Close()
	if c.vin != "" {
		if _, ok := s.registry.Remove(c.vin); ok {
			s.hooks.OnSession(SessionEvent{VIN: c.vin, Peer: c.nc.RemoteAddr().String(), Online: false, LastSeen: s.hooks.Now()})
		}
	}
}

// cfgSnapshot 并发安全地读取当前配置(serve 循环每轮调用)。
func (s *Server) cfgSnapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// UpdateIdle 运行中更新空闲检测(AC-10「即时生效」,评审 M1):改配置后逐连接
// 主动重设读超时——阻塞在 Next() 的静默连接立即被唤醒:OFF→ON 时超时倒计时开始;
// ON→OFF 时 deadline 清零。serve 循环下一轮按新配置继续。
func (s *Server) UpdateIdle(enabled bool, d time.Duration) {
	s.mu.Lock()
	s.cfg.IdleEnabled = enabled
	if enabled {
		s.cfg.IdleTimeout = d
	}
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		if enabled {
			_ = c.nc.SetReadDeadline(s.hooks.Now().Add(d))
		} else {
			_ = c.nc.SetReadDeadline(time.Time{})
		}
	}
}
```

同时在 `conn.go` 的 OnFrame 处把导出行写入环形(供 AC-5):在 `handleRaw` 中 OnFrame 之后追加:

```go
c.srv.buf.add(fmt.Sprintf("[%s] [%s] [%s] [%s]", now.Format("2006-01-02 15:04:05"), d.VIN, cmdName, fmt.Sprintf("%x", raw)))
```

- [ ] **Step 6: 跑全部单测(含 -race)**

```bash
go test ./internal/servermode/ -race -v
```
Expected: 全部 PASS。空闲类用例若偶发超时,检查 `serve` 中 deadline 刷新逻辑(每帧后重设)。

- [ ] **Step 7: 任务级验收(逐条核对 Linked AC)**

```bash
go test ./internal/servermode/ -run 'TestFrameTooLarge|TestIdle|TestMaxConns|TestStopDeadline|TestRingBuffer' -v   # AC-4/9/10/5 后端侧
go list -deps ./internal/servermode | grep -c internal/engine   # AC-6: 输出 0
go test ./...   # AC-8: 既有套件全绿
```

- [ ] **Step 8: Commit**

```bash
git add internal/servermode/
git commit -m "feat(servermode): 服务生命周期与连接帧循环——accept/空闲开关/处理器矩阵/环形缓冲"
```

---

### Task 7: 集成测试(engine.Client 作为真实车端)

**Level:** L3
**Level Rationale:** 端到端业务流验证,直接证明 AC-1/2/3;测试代码 import engine 不违反运行时隔离(仅测试)。
**Linked Acceptance Items:** AC-1、AC-2(闭环)、AC-3
**Task Gate:** task reviewer + linked AC

**Files:**
- Test: `internal/servermode/integration_test.go`

**Interfaces:**
- Consumes: Task 6 `Server`;`engine.NewClient/engine.Options/engine.NewBus`、`Client.Connect/Disconnect`(测试驱动)
- Produces: 无(验收测试)

- [ ] **Step 1: 写集成测试**

先核对引擎选项构造:`grep -n "type Options" internal/engine/client.go`,按实际字段(Host/Port/Version/VIN/自动登录等)填充。骨架:

```go
package servermode

import (
	"context"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
)

func TestIntegrationFullFlow(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := New(DefaultConfig("127.0.0.1:0"), h)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	addr := srv.Status().ListenAddr
	host, port := splitHostPort(t, addr)

	bus := engine.NewBus()
	client := engine.NewClient(engine.Options{
		Host: host, Port: port, Version: api.V2016, VIN: vin17,
		// 其余字段按 Options 实际定义补齐(自动登录/心跳等默认开启)
	}, bus)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("车端连接+登入失败: %v", err)
	}
	defer client.Disconnect()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(c.sessions) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(c.sessions) == 0 || !c.sessions[0].Online {
		t.Fatalf("未观察到上线事件: %+v", c.sessions)
	}
	if len(c.frames) == 0 {
		t.Fatal("未观察到登入帧事件")
	}
	// 0x07 心跳:等待引擎自动心跳产生 frame 事件(或调 engine 的手动心跳入口)
	// 0x08/0x04 的闭环断言见 Logout 用例;此处断言 0x01 闭环(AC-1 主干)
}

func TestIntegrationLogoutDelayedClose(t *testing.T) {
	// 同上启动;client.Connect 后调 client.Disconnect()(触发 0x04 登出);
	// 断言:server 先发出 0x04 应答帧(可从 server 侧 frame 事件或 client 侧
	// ack 观察),随后 sessions 收到 offline —— 即车端收到应答后连接才关闭(AC-3)。
}
```

实现要求:
- `splitHostPort` 用 `net.SplitHostPort`;
- 心跳闭环:若 Options 有心跳周期字段,设为 200ms 加速测试;否则轮询 frame 事件中出现 `0x07`;
- Logout 用例断言顺序:收集 `[]string` 事件序列(`"tx-ack:04"`、`"offline"`),offline 必须晚于 ack ≥150ms(200ms 延迟的容差下限)。

- [ ] **Step 2: 跑集成测试**

```bash
go test ./internal/servermode/ -run TestIntegration -v -count=1
```
Expected: PASS。collector 已内置互斥锁(Task 6),integration 直接复用 newCollector 即可。

- [ ] **Step 3: 任务级验收**

AC-1:`TestIntegrationFullFlow` PASS;AC-3:`TestIntegrationLogoutDelayedClose` PASS;AC-2 闭环部分:重复登入集成(第二个 Client 同 VIN Connect 失败)补一个小用例 `TestIntegrationDuplicateVIN`。

- [ ] **Step 4: Commit**

```bash
git add internal/servermode/integration_test.go
git commit -m "test(servermode): 引擎客户端驱动集成测试——登入/心跳/登出延迟断开/重复 VIN"
```

---

### Task 8: bridge 服务与装配(server_service.go + app/main 接线)

**Level:** L3
**Level Rationale:** 跨层接线(bridge→前端绑定→生命周期钩子),改变应用装配行为。
**Linked Acceptance Items:** AC-6(绑定面)、AC-9(默认 127.0.0.1 经前端表单传入)
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `bridge/server_service.go`
- Test: `bridge/server_service_test.go`
- Modify: `app.go`(App 字段 + NewApp + startup/shutdown)
- Modify: `main.go`(Bindings 追加)

**Interfaces:**
- Consumes: Task 6 `servermode.New/Start/Stop/Status/Sessions/ExportLines`、`internal/store`、wails `runtime.EventsEmit`
- Produces(前端消费):
  - `type ServerConfig struct { IP string; Port int; IdleEnabled bool; IdleSeconds int }`(持久化 `store/server.json`)
  - `func NewServerService() *ServerService`
  - `(*ServerService) SetContext(ctx context.Context)`
  - `(*ServerService) LoadConfig() ServerConfig`
  - `(*ServerService) Start(cfg ServerConfig) (servermode.Status, error)` — IP 非环回时返回需确认错误(`ErrLoopbackConfirm`,前端二次确认后带 `Force bool` 的 StartRequest 重试;简化:签名改 `Start(cfg ServerConfig, force bool)`)
  - `(*ServerService) Stop() error`
  - `(*ServerService) UpdateIdle(enabled bool, idleSeconds int) error`(运行中即时生效,AC-10;**wailsjs 手写绑定时此方法不得遗漏**,评审 m7)
  - `(*ServerService) Status() servermode.Status`
  - `(*ServerService) Sessions() []servermode.Session`
  - `(*ServerService) ExportLog() (string, error)` — wails 保存对话框选路径,写 ExportLines 文本,返回路径
  - 事件:`server:status` / `server:session` / `server:frame` / `server:warn`(EventsEmit,载荷即 Task 2 结构 JSON)

- [ ] **Step 1: 写失败测试**

`bridge/server_service_test.go`(不依赖 wails runtime——Hooks 直接采集,EventsEmit 经 ctx 为 nil 守卫跳过):

```go
package bridge

import (
	"context"
	"testing"
	"time"

	"gbt32960-simulator/internal/servermode"
)

func TestServerServiceStartStopPersist(t *testing.T) {
	svc := NewServerService()
	cfg := ServerConfig{IP: "127.0.0.1", Port: 0, IdleEnabled: true, IdleSeconds: 60}
	st, err := svc.Start(cfg, true)
	if err != nil || !st.Running {
		t.Fatalf("start: %v %+v", err, st)
	}
	if got := svc.LoadConfig(); got.IdleSeconds != 60 || got.IP != "127.0.0.1" {
		t.Fatalf("配置未持久化: %+v", got)
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	if svc.Status().Running {
		t.Fatal("stop 后状态应为停止")
	}
	_ = servermode.Status{}
	_ = time.Now
	_ = context.Background
}

func TestServerServiceNonLoopbackNeedsForce(t *testing.T) {
	svc := NewServerService()
	if _, err := svc.Start(ServerConfig{IP: "0.0.0.0", Port: 0}, false); err == nil {
		t.Fatal("非环回地址未 force 应拒绝")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./bridge/ -run TestServerService -v
```
Expected: FAIL(类型未定义)。

- [ ] **Step 3: 实现 server_service.go**

```go
package bridge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"gbt32960-simulator/internal/servermode"
	"gbt32960-simulator/internal/store"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const serverCfgFile = "server.json"

// ServerConfig 服务端模式页面表单持久化结构。
type ServerConfig struct {
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	IdleEnabled bool   `json:"idleEnabled"`
	IdleSeconds int    `json:"idleSeconds"`
}

// ServerService 服务端模式的前端绑定面。
type ServerService struct {
	ctx     context.Context
	mu      sync.Mutex
	srv     *servermode.Server
	lastIP  string // 最近一次启动的地址(UpdateIdle 持久化用)
	lastPort int
}

func NewServerService() *ServerService { return &ServerService{} }

func (s *ServerService) SetContext(ctx context.Context) { s.ctx = ctx }

func (s *ServerService) emit(name string, data any) {
	if s.ctx == nil {
		return // 测试环境无 wails 上下文
	}
	wailsRuntime.EventsEmit(s.ctx, name, data)
}

func (s *ServerService) LoadConfig() ServerConfig {
	cfg := ServerConfig{IP: "127.0.0.1", Port: 32960, IdleEnabled: true, IdleSeconds: 60}
	_ = store.Load(serverCfgFile, &cfg) // 文件缺失/损坏用默认值
	if cfg.IdleSeconds < 5 || cfg.IdleSeconds > 3600 {
		cfg.IdleSeconds = 60
	}
	return cfg
}

// Start 启动服务端;force=false 且 IP 非环回时拒绝(安全默认,spec §5.4)。
func (s *ServerService) Start(cfg ServerConfig, force bool) (servermode.Status, error) {
	if !force && cfg.IP != "127.0.0.1" && cfg.IP != "localhost" {
		return servermode.Status{}, fmt.Errorf("监听地址 %s 非环回,需在页面二次确认", cfg.IP)
	}
	// 端口 0 = 系统分配临时端口(集成/桥接测试用);UI 层由 a-input-number min=1 挡住(评审 M3)
	if cfg.Port < 0 || cfg.Port > 65535 {
		return servermode.Status{}, fmt.Errorf("端口非法: %d", cfg.Port)
	}
	if cfg.IdleSeconds < 5 || cfg.IdleSeconds > 3600 {
		return servermode.Status{}, fmt.Errorf("空闲时长须在 5~3600 秒")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		s.srv = servermode.New(servermode.DefaultConfig(fmt.Sprintf("%s:%d", cfg.IP, cfg.Port)), servermode.Hooks{
			OnStatus:  func(st servermode.Status) { s.emit("server:status", st) },
			OnSession: func(e servermode.SessionEvent) { s.emit("server:session", e) },
			OnFrame:   func(e servermode.FrameEvent) { s.emit("server:frame", e) },
			OnWarn:    func(e servermode.WarnEvent) { s.emit("server:warn", e) },
		})
	}
	st := s.srv.Status()
	if st.Running {
		return st, nil // 幂等
	}
	if err := s.srv.Start(context.Background()); err != nil {
		return servermode.Status{}, err
	}
	s.lastIP, s.lastPort = cfg.IP, cfg.Port
	_ = store.Save(serverCfgFile, cfg)
	return s.srv.Status(), nil
}

func (s *ServerService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return nil
	}
	return s.srv.Stop()
}

// UpdateIdle 运行中更新空闲检测(透传 Server.UpdateIdle,AC-10 即时生效)并持久化。
func (s *ServerService) UpdateIdle(enabled bool, idleSeconds int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil || !s.srv.Status().Running {
		return fmt.Errorf("服务未启动")
	}
	if idleSeconds < 5 || idleSeconds > 3600 {
		return fmt.Errorf("空闲时长须在 5~3600 秒")
	}
	s.srv.UpdateIdle(enabled, time.Duration(idleSeconds)*time.Second)
	_ = store.Save(serverCfgFile, ServerConfig{IP: s.lastIP, Port: s.lastPort, IdleEnabled: enabled, IdleSeconds: idleSeconds})
	return nil
}

func (s *ServerService) Status() servermode.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return servermode.Status{}
	}
	return s.srv.Status()
}

func (s *ServerService) Sessions() []servermode.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return nil
	}
	return s.srv.Sessions()
}

// ExportLog 弹出保存对话框,导出环形缓冲文本(AC-5)。
func (s *ServerService) ExportLog() (string, error) {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil || !srv.Status().Running {
		return "", fmt.Errorf("服务未启动")
	}
	path, err := wailsRuntime.SaveFileDialog(s.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: fmt.Sprintf("servermode-%s.log", strings.ReplaceAll(time.Now().Format("20060102-150405"), ":", "")),
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 用户取消
	}
	lines := srv.ExportLines()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
```

注:`time` import 补上;`SaveFileDialog` 签名以仓库 wails 版本为准(参照 `bridge/ext_service.go` 中 PickPackFile 的既有用法)。

- [ ] **Step 4: app.go / main.go 接线**

`app.go`:

```go
// App struct 增加字段
server *bridge.ServerService
// NewApp 增加初始化
server: bridge.NewServerService(),
// startup 增加
a.server.SetContext(ctx)
// shutdown 增加(在引擎登出之后)
_ = a.server.Stop()
```

`main.go` Bind 列表追加 `app.server`(在 `app.sys` 之后)。

- [ ] **Step 5: 生成 wails 绑定并验证**

```bash
go build ./... && go test ./bridge/ -run TestServerService -v
wails generate module 2>/dev/null || true   # 若命令不可用,build/dev 时自动再生成
```
Expected: go 侧 PASS;`frontend/wailsjs/go/bridge/ServerService.js` 出现(手动按 SystemService.js 样例补亦可,方法名与 Go 导出一致)。

- [ ] **Step 6: 任务级验收**

AC-6:`go build ./...` 通过且绑定面五方法齐备(Start/Stop/Status/Sessions/ExportLog + LoadConfig);AC-9:非环回 force 测试 PASS。

- [ ] **Step 7: Commit**

```bash
git add bridge/ app.go main.go frontend/wailsjs/
git commit -m "feat(servermode): bridge 绑定面与装配——server:* 事件、配置持久化、安全默认与导出"
```

---

### Task 9: 前端页面激活(ServerModePage)

**Level:** L3
**Level Rationale:** 用户可见工作流变更(页面从占位到可用),绑定 AC-1/5/11 的可视层。
**Linked Acceptance Items:** AC-1(实时展示)、AC-5(导出交互)、AC-11(kind 着色)
**Task Gate:** task reviewer + linked AC

**Files:**
- Modify: `frontend/src/pages/ServerModePage.vue`(整体重写激活)
- Modify: `frontend/src/style.css`(追加 servermode 样式)

**Interfaces:**
- Consumes: Task 8 `ServerService.*` 绑定与 `server:*` 事件(wailsjs/runtime `EventsOn`);解析页组件 `ByteGridView/FieldTableView` 与 `ParserService.ParsePacket`(详情抽屉);`frontend/src/api/backend.ts` 的事件封装惯例
- Produces: 可用页面(无导出接口,消费端)

- [ ] **Step 1: 重写 ServerModePage.vue**

结构要点(完整实现,样式类名与后端载荷字段对齐):

```vue
<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  CaretRightOutlined, DownloadOutlined, ExportOutlined, StopOutlined,
} from '@ant-design/icons-vue'
import * as ServerService from '../../wailsjs/go/bridge/ServerService'
import { EventsOn } from '../../wailsjs/runtime/runtime'

interface ServerFrame {
  time: string
  vin: string
  cmd: string
  hex: string
  summary: string
  kind: 'normal' | 'unknown' | 'encrypted' | 'warn'
}
interface ServerSession {
  vin: string
  peer: string
  online: boolean
  lastSeen: string
}

const cfg = reactive({ ip: '127.0.0.1', port: 32960, idleEnabled: true, idleSeconds: 60 })
const running = ref(false)
const listenAddr = ref('')
const sessions = ref<ServerSession[]>([])
const frames = ref<ServerFrame[]>([])
const detail = ref<ServerFrame | null>(null)

const RENDER_CAP = 200 // 前端渲染上限(后端环形 500,spec §5.4)

let offs: Array<() => void> = []

function subscribe() {
  offs = [
    EventsOn('server:status', (st: { running: boolean; listenAddr: string }) => {
      running.value = st.running
      listenAddr.value = st.listenAddr ?? ''
    }),
    EventsOn('server:session', (e: ServerSession) => {
      const i = sessions.value.findIndex((s) => s.vin === e.vin && s.online)
      if (e.online && i < 0) sessions.value = [e, ...sessions.value].slice(0, RENDER_CAP)
      if (!e.online && i >= 0) sessions.value.splice(i, 1)
    }),
    EventsOn('server:frame', (e: ServerFrame) => {
      frames.value = [e, ...frames.value].slice(0, RENDER_CAP)
    }),
    EventsOn('server:warn', (e: { note: string }) => {
      // 告警行独立样式(评审 m3):不冒充 unknown,避免与未知命令橙色混淆
      frames.value = [{ time: new Date().toISOString(), vin: '', cmd: 'WARN', hex: '', summary: e.note, kind: 'warn' }, ...frames.value].slice(0, RENDER_CAP)
    }),
  ]
}

async function loadCfg() {
  try {
    const saved = await ServerService.LoadConfig()
    Object.assign(cfg, saved)
  } catch (e) {
    message.error('读取服务端配置失败: ' + String(e))
  }
}

async function start() {
  const loopback = cfg.ip === '127.0.0.1' || cfg.ip === 'localhost'
  const doStart = async (force: boolean) => {
    try {
      const st = await ServerService.Start({ ...cfg }, force)
      if (st.running) message.success('服务已启动 ' + st.listenAddr)
    } catch (e) {
      message.error(String(e))
    }
  }
  if (loopback) return doStart(false)
  Modal.confirm({
    title: '监听地址非环回',
    content: `即将监听 ${cfg.ip}:${cfg.port},局域网内任何设备都可连接(协议无认证)。确认继续?`,
    okText: '继续监听',
    cancelText: '取消',
    onOk: () => doStart(true),
  })
}

async function stop() {
  try {
    await ServerService.Stop()
  } catch (e) {
    message.error(String(e))
  }
}

async function exportLog() {
  try {
    const path = await ServerService.ExportLog()
    if (path) message.success('已导出: ' + path)
  } catch (e) {
    message.error(String(e))
  }
}

function kindClass(kind: string) {
  if (kind === 'unknown') return 'frame-unknown'
  if (kind === 'encrypted') return 'frame-encrypted'
  if (kind === 'warn') return 'frame-warn'
  return ''
}

onMounted(async () => {
  subscribe()
  await loadCfg()
  const st = await ServerService.Status().catch(() => null)
  if (st?.running) {
    running.value = true
    listenAddr.value = st.listenAddr
  }
  // 快照行全部是活会话(注册表只存在线会话),补 online: true 供状态列渲染(评审 m3)
  const snap = (await ServerService.Sessions().catch(() => [])) ?? []
  sessions.value = snap.map((s: Omit<ServerSession, 'online'>) => ({ ...s, online: true }))
})
onUnmounted(() => offs.forEach((off) => off()))
</script>
```

`<template>` 骨架(沿用现有 zone 结构,激活为真实交互):

- 监听表单:IP/端口/空闲开关(a-switch)/空闲秒数(a-input-number 5~3600,开关关闭时禁用);`running ? 停止按钮(StopOutlined, danger) : 启动按钮(CaretRightOutlined, primary)`;导出按钮(running 时可用)
- 状态条:`running ? <a-tag color="success">运行中 {{ listenAddr }}</a-tag> : <a-tag>未启动</a-tag>`
- 会话表:dataSource=sessions,列 **VIN / IP:端口 / 状态 / 最后活跃**(spec §5.4 状态列必须有);快照接口 `Sessions()` 返回的行全部是活会话,前端映射时 `online: true`(评审 m3);空态文案保留现有
- 报文表:dataSource=frames,行 `:class="kindClass(record.kind)"`,列 时间/VIN/命令/HEX(hex 列 monospace);点击行 `detail = record`;**`server:warn` 事件单独渲染为 `.frame-warn` 样式行(不冒充 unknown,避免与未知命令色混淆,评审 m3)**
- 详情抽屉:`<a-drawer v-model:open="detailVisible" :title="detail?.cmd">` 内嵌 ByteGridView(normalized-hex=detail.hex)与 FieldTableView;顶部一个扩展包下拉(选项 = `ParserService.ParserPacks()` + 默认「不使用扩展包」),字段表数据 = `ParserService.ParsePacket(detail.hex, packId)` 结果(spec §5.4:支持扩展包自定义单元解码);kind≠normal 时仅展示 hex 与说明(加密/未知不解析)
- 空闲开关/时长变更且 running 时,`watch` 调 `ServerService.UpdateIdle(cfg.idleEnabled, cfg.idleSeconds)`(AC-10 即时生效;失败 message.error)

- [ ] **Step 2: style.css 追加(亮暗主题 token,色相区分)**

```css
/* ---- 服务端模式:报文行特殊颜色(kind 区分,亮暗主题各自定义) ---- */
.frame-unknown td { color: #d46b08; }          /* 橙:未知命令字 */
.frame-encrypted td { color: #9254de; }        /* 紫:加密帧 */
.frame-warn td { color: #cf1322; }             /* 红:服务端告警(空闲超时/超长/断开) */
:root[data-theme='light'] .frame-unknown td { color: #ad4e00; }
:root[data-theme='light'] .frame-encrypted td { color: #6424c2; }
:root[data-theme='light'] .frame-warn td { color: #a8071a; }
```

- [ ] **Step 3: 类型检查与构建**

```bash
cd frontend && npm run build
```
Expected: vue-tsc 无错误,vite 构建成功。若 wailsjs 绑定文件缺失,按 `frontend/wailsjs/go/bridge/SystemService.js` 样例手写 ServerService.js/.d.ts(方法名一一对应)。

- [ ] **Step 4: 任务级验收(Linked AC 逐条)**

- AC-1 可视层:`wails dev` 启动,用本模拟器客户端页连 `127.0.0.1:<port>`,观察会话上线与报文行实时出现
- AC-5:点导出 → 选路径 → 文件内容为 `[时间] [VIN] [命令] [hex]` 行
- AC-11:向服务端发未知命令帧与加密帧,行分别呈橙/紫色;亮暗主题各核对一次

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/ServerModePage.vue frontend/src/style.css frontend/wailsjs/
git commit -m "feat(ui): 服务端模式页激活——启停表单/会话表/报文着色/详情抽屉/导出"
```

---

## 收尾验收(计划级)

全部任务完成后:

```bash
go build ./... && go test ./... -count=1          # AC-7/8
go list -deps ./internal/servermode | grep -c internal/engine   # AC-6 = 0
git diff --stat go.mod go.sum                     # AC-7 = 空
cd frontend && npm run build                      # AC-11 前置
```

人工验收(AC-1/5/11 可视层 + AC-9 非环回二次确认弹窗)按 Task 9 Step 4 清单执行。
