package engine

// 流水号边界与连接级空载荷帧深测:
//   - 未登入直接登出仍须带可用流水号且不 panic(表20:登出体 = 时间 6B + 流水号 2B);
//   - 登入重试按次消耗流水号,登出复用当次登入值(表6/表20);
//   - 0x07 心跳 / 0x08 校时请求的数据单元长度为 0(帧级断言,哑平台收集)。

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	"github.com/sunsky74/gb32960/types"

	"gbt32960-simulator/internal/framing"
)

// TestDeepSerialWithoutLoginUsable 未登入直接登出:
// 流水号账本须惰性分配(不 panic、不返回 0),且重复取值稳定;组帧可用。
func TestDeepSerialWithoutLoginUsable(t *testing.T) {
	vin := "LSV00000000000001"
	bus := NewBus()
	events, stop := bus.Subscribe(16)
	defer stop()

	c := NewClient(Options{
		Host: "127.0.0.1", Port: 1, Version: api.V2016,
		VIN: vin, ICCID: "89860000000000000001",
	}, bus)

	// 未连接直接登出:走 sendLogout 的真实路径(内部先取流水号再写帧)。
	c.sendLogout(context.Background())

	s1 := c.currentLoginSerial(vin)
	if s1 == 0 {
		t.Fatal("未登入登出未分配可用流水号(表20 要求登出仍带流水号)")
	}
	if s2 := c.currentLoginSerial(vin); s2 != s1 {
		t.Fatalf("重复取登出流水号 = %d, want 稳定为 %d", s2, s1)
	}
	// 未连接时写帧必须失败并发出错误事件(受控失败,非 panic)。
	select {
	case e := <-events:
		if e.Kind != EventError || !strings.Contains(e.Message, "发送登出报文失败") {
			t.Fatalf("事件 = %+v, want 发送登出报文失败", e)
		}
	default:
		t.Fatal("未连接登出应发出错误事件")
	}

	// 流水号确实可组帧(表20:体=时间 6B + 流水号 2B)。
	body := &mdl.VehicleLogout{BeanTime: BeanTimeNow(), SerialNum: int(s1)}
	raw, _, err := BuildFrame(api.V2016, vin, 0x04, body)
	if err != nil {
		t.Fatalf("登出组帧: %v", err)
	}
	if got := frameSerial(raw); got != int(s1) {
		t.Fatalf("登出帧流水号 = %d, want %d", got, s1)
	}
}

// TestDeepLoginRetrySerialIncrements 登入重试场景:第 1 次不应答触发超时重试,
// 第 2 次成功。同一链接同一 VIN 的登入流水号按次递增(1→2);登出复用最后一次。
func TestDeepLoginRetrySerialIncrements(t *testing.T) {
	vin := "LSV00000000000001"
	var mu sync.Mutex
	var logins []int
	var logout int
	attempts := 0

	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			switch raw[2] {
			case 0x01:
				mu.Lock()
				attempts++
				a := attempts
				logins = append(logins, frameSerial(raw))
				mu.Unlock()
				if a >= 2 { // 第 1 次有意静默 → 驱动 waitAck 超时重试
					_, _ = conn.Write(replyFrameRaw(0x01, 0x01, nil))
				}
			case 0x04:
				mu.Lock()
				logout = frameSerial(raw)
				mu.Unlock()
				_, _ = conn.Write(replyFrameRaw(0x04, 0x01, nil))
				return
			}
		}
	})

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: vin, ICCID: "89860000000000000001",
		LoginTimeout: 300 * time.Millisecond, LoginRetries: 2,
	}, nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("重试后应登录成功: %v", err)
	}
	c.Disconnect()

	mu.Lock()
	defer mu.Unlock()
	if len(logins) != 2 || logins[0] != 1 || logins[1] != 2 {
		t.Fatalf("登入流水号 = %v, want [1 2](重试按次递增)", logins)
	}
	if logout != logins[len(logins)-1] {
		t.Fatalf("登出流水号 = %d, want 复用当次登入 %d(表20)", logout, logins[len(logins)-1])
	}
}

// TestDeepClockHeartbeatFramesEmptyPayload 帧级断言:客户端发出的 0x07/0x08
// 数据单元长度为 0(总长 25B = 24 头 + BCC),应答标志 0xFE,起始符 ##,BCC 正确。
func TestDeepClockHeartbeatFramesEmptyPayload(t *testing.T) {
	vin := "LSV00000000000001"
	var mu sync.Mutex
	got := map[byte][][]byte{}

	fp := startLifecyclePlatform(t, func(conn net.Conn, n int64) {
		fr := framing.NewFrameReader(conn)
		for {
			raw, err := fr.Next()
			if err != nil {
				return
			}
			cp := append([]byte(nil), raw...)
			mu.Lock()
			got[raw[2]] = append(got[raw[2]], cp)
			mu.Unlock()
			switch raw[2] {
			case 0x01, 0x04:
				_, _ = conn.Write(replyFrameRaw(raw[2], 0x01, nil))
				if raw[2] == 0x04 {
					return
				}
			}
		}
	})

	c := NewClient(Options{
		Host: "127.0.0.1", Port: fp.port(), Version: api.V2016,
		VIN: vin, ICCID: "89860000000000000001",
		HeartbeatInterval: 100 * time.Millisecond,
		AutoClockSync:     true,
		LoginTimeout:      time.Second, LoginRetries: 1,
	}, nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n7, n8 := len(got[0x07]), len(got[0x08])
		mu.Unlock()
		if n7 > 0 && n8 > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.Disconnect()

	mu.Lock()
	defer mu.Unlock()
	if len(got[0x07]) == 0 || len(got[0x08]) == 0 {
		t.Fatalf("未观察到 0x07/0x08: 心跳=%d 校时=%d", len(got[0x07]), len(got[0x08]))
	}
	for _, cmd := range []byte{0x07, 0x08} {
		for _, raw := range got[cmd] {
			if len(raw) != 25 {
				t.Fatalf("0x%02X 帧长 = %d, want 25(空载荷)", cmd, len(raw))
			}
			if pl := int(raw[22])<<8 | int(raw[23]); pl != 0 {
				t.Fatalf("0x%02X 数据单元长度 = %d, want 0", cmd, pl)
			}
			if raw[2] != cmd || raw[3] != byte(types.ResponseCommand) {
				t.Fatalf("0x%02X 帧头 = %02X/%02X", cmd, raw[2], raw[3])
			}
			var bcc byte
			for _, b := range raw[2 : len(raw)-1] {
				bcc ^= b
			}
			if bcc != raw[len(raw)-1] {
				t.Fatalf("0x%02X BCC = %02X, 计算 %02X", cmd, raw[len(raw)-1], bcc)
			}
		}
	}
}
