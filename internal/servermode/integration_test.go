package servermode

// 集成测试:engine.Client 作为真实车端驱动本地 servermode.Server,
// 端到端证明 AC-1(全流程闭环)/AC-2(重复 VIN 拒绝)/AC-3(登出延迟断开)。
// 测试代码 import engine 不违反运行时隔离(go list -deps 不含测试文件依赖)。

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
)

// itICCID 登入报文 ICCID(定长 20,与 decode_test 的 golden 登入帧同构)。
const itICCID = "12345678901234567890"

// splitHostPort 用 net.SplitHostPort 拆监听地址,port 转整数供 engine.Options。
func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("解析监听地址 %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("端口 %q 非数字: %v", portStr, err)
	}
	return host, port
}

// frameCount 统计 collector 中指定命令(如 "0x01")的帧事件数。
func frameCount(c *collector, cmd string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, f := range c.frames {
		if f.Cmd == cmd {
			n++
		}
	}
	return n
}

// TestIntegrationFullFlow AC-1 主干:engine.Client 完成
// 0x01 登入 → session online → 0x08 校时(AutoClockSync)→ 0x07 心跳(200ms 加速)
// → 0x04 登出(Disconnect)→ session offline;登入帧先于登出帧。
func TestIntegrationFullFlow(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	host, port := splitHostPort(t, srv.Status().ListenAddr)

	bus := engine.NewBus()
	client := engine.NewClient(engine.Options{
		Host:              host,
		Port:              port,
		Version:           api.V2016,
		VIN:               vin17,
		ICCID:             itICCID,
		SubsystemCodes:    []string{"1"},
		HeartbeatInterval: 200 * time.Millisecond, // 加速心跳闭环
		LoginTimeout:      2 * time.Second,
		AutoClockSync:     true, // 登录成功后自动 0x08 校时
	}, bus)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("车端连接+登入失败: %v", err)
	}

	// online 会话事件 + 0x01 登入帧
	waitFor(t, 2*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.sessions) > 0 && c.sessions[0].Online && c.sessions[0].VIN == vin17
	})
	waitFor(t, time.Second, func() bool { return frameCount(c, "0x01") >= 1 })
	// 0x08 校时请求帧(登录后自动发出,server 应回 BeanTime 应答)
	waitFor(t, time.Second, func() bool { return frameCount(c, "0x08") >= 1 })
	// 0x07 心跳帧(引擎 200ms 周期自动发出)
	waitFor(t, 2*time.Second, func() bool { return frameCount(c, "0x07") >= 1 })

	// 0x04 登出:Disconnect 触发(发 0x04 → 收 ack → 关连接)→ server 发 offline
	client.Disconnect()
	waitFor(t, 2*time.Second, func() bool { return frameCount(c, "0x04") >= 1 })
	waitFor(t, 2*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if len(c.sessions) < 2 {
			return false
		}
		last := c.sessions[len(c.sessions)-1]
		return !last.Online && last.VIN == vin17
	})

	// 顺序断言:首帧 0x01 的索引必须先于首帧 0x04
	c.mu.Lock()
	defer c.mu.Unlock()
	idx01, idx04 := -1, -1
	for i, f := range c.frames {
		if f.Cmd == "0x01" && idx01 < 0 {
			idx01 = i
		}
		if f.Cmd == "0x04" && idx04 < 0 {
			idx04 = i
		}
	}
	if idx01 < 0 || idx04 < 0 || idx01 >= idx04 {
		t.Fatalf("帧顺序异常: 0x01@%d 0x04@%d", idx01, idx04)
	}
}

// TestIntegrationLogoutDelayedClose AC-3:server 对 0x04 先回应答、200ms 后才
// 断开连接——offline 事件必须晚于车端收到 ack ≥150ms(200ms 延迟的容差下限)。
//
// 注意:不能由 client.Disconnect 驱动——引擎收到 ack 后立即关闭本地连接,
// server 会先观察到 EOF 而非延迟关闭,时序断言无从成立。故用 Send(0x04) 发
// 登出并保持连接,让 server 的延迟断开成为唯一关闭源(与计划实现要求一致)。
func TestIntegrationLogoutDelayedClose(t *testing.T) {
	h, c := newCollector(time.Now())

	// ack/offline 独立时间记录:ack 取车端总线 RX 事件时间(引擎读循环解码即
	// 打点),offline 在 OnSession 回调内打点。必须在建 Server 前包装 Hooks。
	tt := struct {
		mu        sync.Mutex
		ackAt     time.Time
		offlineAt time.Time
	}{}
	innerSession := h.OnSession
	h.OnSession = func(e SessionEvent) {
		innerSession(e)
		tt.mu.Lock()
		if !e.Online && tt.offlineAt.IsZero() {
			tt.offlineAt = time.Now()
		}
		tt.mu.Unlock()
	}

	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	host, port := splitHostPort(t, srv.Status().ListenAddr)

	bus := engine.NewBus()
	client := engine.NewClient(engine.Options{
		Host:           host,
		Port:           port,
		Version:        api.V2016,
		VIN:            vin17,
		ICCID:          itICCID,
		SubsystemCodes: []string{"1"},
		LoginTimeout:   2 * time.Second,
		// 心跳间隔 0(不发),保证 200ms 窗口内连接静默,唯一事件源是登出应答
	}, bus)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("车端连接+登入失败: %v", err)
	}
	defer client.Disconnect() // server 已断开后引擎自身已清理,此调用幂等无害

	events, cancelSub := bus.Subscribe(64)
	defer cancelSub()
	ackSeen := make(chan struct{}, 1)
	go func() {
		for ev := range events {
			if ev.Kind == engine.EventRx && strings.HasPrefix(ev.Cmd, "0x04") {
				tt.mu.Lock()
				if tt.ackAt.IsZero() {
					tt.ackAt = ev.Time
				}
				tt.mu.Unlock()
				select {
				case ackSeen <- struct{}{}:
				default:
				}
			}
		}
	}()

	// 发 0x04 登出(与引擎 sendLogout 同构的报文体),连接保持不动
	if err := client.Send(context.Background(), 0x04, &mdl.VehicleLogout{
		BeanTime:  engine.BeanTimeNow(),
		SerialNum: 1,
	}); err != nil {
		t.Fatalf("发送登出报文: %v", err)
	}

	select {
	case <-ackSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("2s 内未观察到 0x04 应答(车端侧)")
	}
	// server 侧同样处理到了登出帧
	waitFor(t, time.Second, func() bool { return frameCount(c, "0x04") >= 1 })

	// 连接由 server 在 ack 后 ~200ms 主动断开 → removeConn 发 offline
	waitFor(t, 2*time.Second, func() bool {
		tt.mu.Lock()
		defer tt.mu.Unlock()
		return !tt.offlineAt.IsZero()
	})

	tt.mu.Lock()
	ackAt, offlineAt := tt.ackAt, tt.offlineAt
	tt.mu.Unlock()
	if ackAt.IsZero() || offlineAt.IsZero() {
		t.Fatalf("时间记录缺失: ack=%v offline=%v", ackAt, offlineAt)
	}
	if !offlineAt.After(ackAt) {
		t.Fatalf("offline(%v)未晚于 ack(%v),顺序错误", offlineAt, ackAt)
	}
	if gap := offlineAt.Sub(ackAt); gap < 150*time.Millisecond {
		t.Fatalf("offline 仅晚于 ack %v,应 ≥150ms(200ms 延迟断开容差下限)", gap)
	}
}

// TestIntegrationDuplicateVIN AC-2 闭环:第二个同 VIN 车端登入被拒(收到失败
// 应答,Connect 报错),server 侧原会话不受影响(唯一会话保留、无 offline、
// 首车心跳持续、状态仍在线)。
func TestIntegrationDuplicateVIN(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	host, port := splitHostPort(t, srv.Status().ListenAddr)

	firstBus := engine.NewBus()
	first := engine.NewClient(engine.Options{
		Host:              host,
		Port:              port,
		Version:           api.V2016,
		VIN:               vin17,
		ICCID:             itICCID,
		SubsystemCodes:    []string{"1"},
		HeartbeatInterval: 200 * time.Millisecond, // 拒绝后用心跳证明原会话仍活着
		LoginTimeout:      2 * time.Second,
	}, firstBus)
	if err := first.Connect(context.Background()); err != nil {
		t.Fatalf("首车登入应成功: %v", err)
	}
	defer first.Disconnect()
	waitFor(t, 2*time.Second, func() bool { return frameCount(c, "0x01") >= 1 })

	// 第二个同 VIN 车端:LoginRetries=1/LoginTimeout=500ms,即使最坏路径也快速失败
	second := engine.NewClient(engine.Options{
		Host:           host,
		Port:           port,
		Version:        api.V2016,
		VIN:            vin17,
		ICCID:          itICCID,
		SubsystemCodes: []string{"1"},
		LoginTimeout:   500 * time.Millisecond,
		LoginRetries:   1,
	}, engine.NewBus())
	err := second.Connect(context.Background())
	if err == nil {
		second.Disconnect()
		t.Fatal("重复 VIN 登入应失败")
	}
	if !strings.Contains(err.Error(), "拒绝登录") {
		t.Fatalf("应报平台拒绝登录,got: %v", err)
	}
	second.Disconnect() // 失败路径引擎已清理,幂等无害

	// server 侧:显式拒绝告警
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, w := range c.warns {
			if strings.HasPrefix(w.Note, "重复登入拒绝") {
				return true
			}
		}
		return false
	})

	// 原会话不受影响:快照唯一、无额外 online/offline、首车仍在线、心跳持续
	snap := srv.Sessions()
	if len(snap) != 1 || snap[0].VIN != vin17 {
		t.Fatalf("原会话应保留且唯一: %+v", snap)
	}
	c.mu.Lock()
	online, offline := 0, 0
	for _, s := range c.sessions {
		if s.Online {
			online++
		} else {
			offline++
		}
	}
	c.mu.Unlock()
	if online != 1 || offline != 0 {
		t.Fatalf("被拒连接不应产生 online/offline 变化: online=%d offline=%d", online, offline)
	}
	if first.State() != engine.StateOnline {
		t.Fatalf("首车状态应仍在线: %s", first.State())
	}
	base := frameCount(c, "0x07")
	waitFor(t, 2*time.Second, func() bool { return frameCount(c, "0x07") > base })
}
