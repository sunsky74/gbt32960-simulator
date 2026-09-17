package servermode

// 平台侧深度应答深测:
//   - 拒绝语义一律 0x03 VIN重复(表4):重复登入(车辆直连)、平台链路 VIN 数超限;
//   - 正常 0x01 车辆登入 / 0x05 平台登入 → 0x01 成功;0x02/0x03 数据 → 0x01;
//   - 0x07 心跳应答空体、0x08 校时应答 6 字节时间(6.3.2 应答保留报文时间);
//   - 2025 帧只读:不应答、不注册会话,帧事件带只读摘要。

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	mdl25 "github.com/sunsky74/gb32960/model/gbt2025"
	"github.com/sunsky74/gb32960/types"
)

// deepReadFrame 读一帧应答并拆出 (cmd, resp, vin, payload)。
func deepReadFrame(t *testing.T, conn net.Conn, fr *framing.FrameReader, timeout time.Duration) (byte, byte, string, []byte) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	raw, err := fr.Next()
	if err != nil {
		t.Fatalf("读应答帧: %v", err)
	}
	return raw[2], raw[3], string(raw[4:21]), raw[24 : len(raw)-1]
}

// deepLoginFrame 指定 VIN 的真实 2016 车辆登入帧(结构与引擎 sendLogin 同构)。
func deepLoginFrame(t *testing.T, vin string) []byte {
	t.Helper()
	raw, _, err := engine.BuildFrame(api.V2016, vin, 0x01, &mdl.VehicleLogin{
		BeanTime:  model.BeanTime{Year: 26, Month: 9, Day: 3, Hour: 10, Minute: 0, Second: 0},
		SerialNum: 1,
		ICCID:     "12345678901234567890",
		Count:     1,
		Length:    1,
		Codes:     []string{"1"},
	})
	if err != nil {
		t.Fatalf("登入帧 %s: %v", vin, err)
	}
	return raw
}

// emptyBody2025 2025 心跳(0x07)空体。
type emptyBody2025 struct{}

func (emptyBody2025) Version() api.GBTVersion { return api.V2025 }
func (emptyBody2025) Bytes() ([]byte, error)  { return nil, nil }

// TestDeepDuplicateLoginVINDup 车辆直连连接:首次 0x01 → 0x01 成功;
// 同 VIN 重复登入 / 换 VIN 再登入 → 一律 0x03 VIN重复(表4),原会话不受影响。
func TestDeepDuplicateLoginVINDup(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)

	// 正常登入 → 0x01 成功,空体
	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal(err)
	}
	cmd, resp, vin, body := deepReadFrame(t, conn, fr, 2*time.Second)
	if cmd != 0x01 || resp != byte(types.ResponseSuccess) || vin != vin17 || len(body) != 0 {
		t.Fatalf("登入应答 = %02X/%02X/%q body=%d", cmd, resp, vin, len(body))
	}

	// 同 VIN 重复登入 → 0x03 VIN重复(表4)
	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal(err)
	}
	if _, resp, vin, _ = deepReadFrame(t, conn, fr, 2*time.Second); resp != byte(types.ResponseVINDup) || vin != vin17 {
		t.Fatalf("重复登入应答 = %02X/%q, want 03/%q", resp, vin, vin17)
	}

	// 换 VIN 再登入 → 同样 0x03(车辆直连只允许一次登入)
	other := "LSV00000000000009"
	if _, err := conn.Write(deepLoginFrame(t, other)); err != nil {
		t.Fatal(err)
	}
	if _, resp, vin, _ = deepReadFrame(t, conn, fr, 2*time.Second); resp != byte(types.ResponseVINDup) || vin != other {
		t.Fatalf("换 VIN 登入应答 = %02X/%q, want 03/%q", resp, vin, other)
	}

	// 拒绝不得污染会话:仅首个 VIN 在线
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		online := 0
		for _, s := range c.sessions {
			if s.Online {
				online++
			}
		}
		return online == 1
	})
	for _, s := range srv.Sessions() {
		if s.VIN != vin17 {
			t.Fatalf("被拒登入不应注册会话: %+v", s)
		}
	}
}

// TestDeepPlatformVINLimitVINDup 平台链路(0x05)车辆 VIN 数上限:
// 平台标识不占额度,超限的第 2 个车辆登入 → 0x03 VIN重复(表4)。
func TestDeepPlatformVINLimitVINDup(t *testing.T) {
	h, _ := newCollector(time.Now())
	cfg := DefaultConfig("127.0.0.1:0")
	cfg.MaxVinsPerConn = 1 // 仅允许 1 个车辆 VIN
	srv := startTestServer(t, cfg, h)

	conn, err := net.Dial("tcp", srv.Status().ListenAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)

	// 0x05 平台登入 → 0x01 成功,回显平台 VIN(表22)
	sendReq(t, conn, pltVIN17, 0x05, []byte{1, 2, 3})
	cmd, resp, vin, _ := deepReadFrame(t, conn, fr, 2*time.Second)
	if cmd != 0x05 || resp != byte(types.ResponseSuccess) || vin != pltVIN17 {
		t.Fatalf("平台登入应答 = %02X/%02X/%q", cmd, resp, vin)
	}

	// 车辆 A 登入(占满 1 个额度)→ 0x01;数据帧 → 0x01
	if _, err := conn.Write(deepLoginFrame(t, vinA17)); err != nil {
		t.Fatal(err)
	}
	cmd, resp, vin, _ = deepReadFrame(t, conn, fr, 2*time.Second)
	if cmd != 0x01 || resp != byte(types.ResponseSuccess) || vin != vinA17 {
		t.Fatalf("车辆 A 登入应答 = %02X/%02X/%q", cmd, resp, vin)
	}
	sendReq(t, conn, vinA17, 0x02, []byte{9, 9})
	cmd, resp, vin, _ = deepReadFrame(t, conn, fr, 2*time.Second)
	if cmd != 0x02 || resp != byte(types.ResponseSuccess) || vin != vinA17 {
		t.Fatalf("数据应答 = %02X/%02X/%q", cmd, resp, vin)
	}

	// 车辆 B 超限 → 0x03 VIN重复,且不得注册
	if _, err := conn.Write(deepLoginFrame(t, vinB17)); err != nil {
		t.Fatal(err)
	}
	if _, resp, vin, _ = deepReadFrame(t, conn, fr, 2*time.Second); resp != byte(types.ResponseVINDup) || vin != vinB17 {
		t.Fatalf("超限登入应答 = %02X/%q, want 03/%q", resp, vin, vinB17)
	}
	for _, s := range srv.Sessions() {
		if s.VIN == vinB17 {
			t.Fatalf("超限 VIN 不得注册: %+v", s)
		}
	}
	if n := len(srv.Sessions()); n != 2 {
		t.Fatalf("会话数 = %d, want 2(平台+A)", n)
	}
}

// TestDeepHeartbeatClockReplies 心跳/校时应答体形态:
// 0x07 → 0x01 且应答体为空;0x08 → 0x01 且应答体 = 6B 校时时间(hooks.Now)。
func TestDeepHeartbeatClockReplies(t *testing.T) {
	// hooks.Now 用「当前时刻」而非过去时间:serve 循环以 Now+IdleTimeout 设读超时,
	// 过去时间会让服务端立即空闲超时关闭(DefaultConfig IdleTimeout=60s)。
	fixed := time.Now().Truncate(time.Second)
	h, _ := newCollector(fixed)
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)

	if _, err := conn.Write(loginFrame(t)); err != nil {
		t.Fatal(err)
	}
	if _, resp, _, _ := deepReadFrame(t, conn, fr, 2*time.Second); resp != byte(types.ResponseSuccess) {
		t.Fatalf("登入应答 = %02X", resp)
	}

	// 0x07 心跳:空应答体
	if _, err := conn.Write(heartbeatFrame(t)); err != nil {
		t.Fatal(err)
	}
	cmd, resp, vin, body := deepReadFrame(t, conn, fr, 2*time.Second)
	if cmd != 0x07 || resp != byte(types.ResponseSuccess) || vin != vin17 || len(body) != 0 {
		t.Fatalf("心跳应答 = %02X/%02X/%q body=%d", cmd, resp, vin, len(body))
	}

	// 0x08 校时:应答体 = 6B 时间(表5,与 hooks.Now 一致)
	clockRaw, _, err := engine.BuildFrame(api.V2016, vin17, 0x08, emptyBody{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(clockRaw); err != nil {
		t.Fatal(err)
	}
	cmd, resp, vin, body = deepReadFrame(t, conn, fr, 2*time.Second)
	if cmd != 0x08 || resp != byte(types.ResponseSuccess) || vin != vin17 {
		t.Fatalf("校时应答 = %02X/%02X/%q", cmd, resp, vin)
	}
	want := ext.EncodeBeanTime(fixed)
	if !bytes.Equal(body, want[:]) {
		t.Fatalf("校时应答体 = %x, want %x", body, want)
	}
}

// TestDeepV2025FramesReadOnly 2025 帧只读:不应答、不注册会话,
// 帧事件带「2025 只读」摘要(d.Version==V2025 分支)。
func TestDeepV2025FramesReadOnly(t *testing.T) {
	h, c := newCollector(time.Now())
	srv := startTestServer(t, DefaultConfig("127.0.0.1:0"), h)
	conn := dial(t, srv)
	defer func() { _ = conn.Close() }()
	fr := framing.NewFrameReader(conn)

	login, _, err := engine.BuildFrame(api.V2025, vin17, 0x01, &mdl25.VehicleLoginV2025{
		BeanTime:  model.BeanTime{Year: 26, Month: 1, Day: 2, Hour: 3, Minute: 4, Second: 5},
		SerialNum: 1,
		ICCID:     "12345678901234567890",
		Count:     1,
		Lengths:   []int{1},
		Codes:     []string{"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	hb, _, err := engine.BuildFrame(api.V2025, vin17, 0x07, emptyBody2025{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append(login, hb...)); err != nil {
		t.Fatal(err)
	}

	// 只读:400ms 内不得有应答
	_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	if raw, err := fr.Next(); err == nil {
		t.Fatalf("2025 帧不应答,却收到 cmd=0x%02X resp=0x%02X", raw[2], raw[3])
	}

	// 帧事件:两条 2025 帧均带只读摘要
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		n := 0
		for _, f := range c.frames {
			if strings.HasPrefix(f.Summary, "2025 只读") {
				n++
			}
		}
		return n >= 2
	})
	if n := len(srv.Sessions()); n != 0 {
		t.Fatalf("2025 帧不得注册会话,当前 %d 条: %+v", n, srv.Sessions())
	}
}
