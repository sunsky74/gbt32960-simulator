package bridge

import (
	"net"
	"testing"
	"time"

	"gbt32960-simulator/internal/servermode"
	"gbt32960-simulator/internal/store"
)

// testServerCfg 合法服务端表单(端口 0 = 系统分配临时端口)。
func testServerCfg() ServerConfig {
	return ServerConfig{
		IP: "127.0.0.1", Port: 0, IdleEnabled: true, IdleSeconds: 60,
		MaxConns: 64, MaxFrameBytes: 8192, LogLines: 500, MaxVinsPerConn: 128,
	}
}

func TestServerServiceStartStopPersist(t *testing.T) {
	tempHome(t) // store 持久化落到临时目录,不污染真实用户配置
	svc := NewServerService(nil)
	cfg := testServerCfg()
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
}

func TestServerServiceRestartOnNewPort(t *testing.T) {
	tempHome(t)
	svc := NewServerService(nil)
	st1, err := svc.Start(testServerCfg(), true)
	if err != nil || !st1.Running {
		t.Fatalf("首次启动: %v %+v", err, st1)
	}
	oldAddr := st1.ListenAddr
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	// 再启动必须绑定新的临时端口(重建 Server,不残留旧 Addr)
	st2, err := svc.Start(testServerCfg(), true)
	if err != nil || !st2.Running {
		t.Fatalf("换端口重启: %v %+v", err, st2)
	}
	if st2.ListenAddr == oldAddr {
		t.Fatalf("重启后仍绑定旧地址 %s(应绑定新临时端口)", oldAddr)
	}
	_ = svc.Stop()
}

func TestServerServiceNonLoopbackNeedsForce(t *testing.T) {
	svc := NewServerService(nil)
	if _, err := svc.Start(ServerConfig{IP: "0.0.0.0", Port: 0}, false); err == nil {
		t.Fatal("非环回地址未 force 应拒绝")
	}
	// 引用守卫:载荷类型来自 servermode,绑定面签名依赖它们
	_ = servermode.Status{}
	_ = []servermode.Session(nil)
}

func TestServerServiceAdvancedParamsPersist(t *testing.T) {
	tempHome(t)
	svc := NewServerService(nil)
	cfg := testServerCfg()
	cfg.MaxConns, cfg.MaxFrameBytes, cfg.LogLines, cfg.MaxVinsPerConn = 8, 4096, 100, 4
	st, err := svc.Start(cfg, true)
	if err != nil || !st.Running {
		t.Fatalf("start: %v %+v", err, st)
	}
	defer func() { _ = svc.Stop() }()
	if got := svc.LoadConfig(); got.MaxConns != 8 || got.MaxFrameBytes != 4096 || got.LogLines != 100 || got.MaxVinsPerConn != 4 {
		t.Fatalf("高级参数未持久化: %+v", got)
	}
}

func TestServerServiceUpdateIdlePreservesAdvanced(t *testing.T) {
	tempHome(t)
	svc := NewServerService(nil)
	cfg := testServerCfg()
	cfg.MaxConns, cfg.MaxFrameBytes, cfg.LogLines, cfg.MaxVinsPerConn = 8, 4096, 100, 4
	if _, err := svc.Start(cfg, true); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Stop() }()
	if err := svc.UpdateIdle(false, 120); err != nil {
		t.Fatal(err)
	}
	got := svc.LoadConfig()
	if got.IdleEnabled || got.IdleSeconds != 120 {
		t.Fatalf("空闲参数未更新: %+v", got)
	}
	if got.MaxConns != 8 || got.MaxFrameBytes != 4096 || got.LogLines != 100 || got.MaxVinsPerConn != 4 {
		t.Fatalf("UpdateIdle 丢失高级参数: %+v", got)
	}
}

func TestServerServiceStartValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ServerConfig)
		want   string
	}{
		{"maxConnsLow", func(c *ServerConfig) { c.MaxConns = 0 }, "最大连接数须在 1~512"},
		{"maxConnsHigh", func(c *ServerConfig) { c.MaxConns = 513 }, "最大连接数须在 1~512"},
		{"maxFrameBytesLow", func(c *ServerConfig) { c.MaxFrameBytes = 511 }, "帧上限须在 512~65536 字节"},
		{"maxFrameBytesHigh", func(c *ServerConfig) { c.MaxFrameBytes = 65537 }, "帧上限须在 512~65536 字节"},
		{"logLinesLow", func(c *ServerConfig) { c.LogLines = 99 }, "日志保留行数须在 100~10000"},
		{"logLinesHigh", func(c *ServerConfig) { c.LogLines = 10001 }, "日志保留行数须在 100~10000"},
		{"maxVinsLow", func(c *ServerConfig) { c.MaxVinsPerConn = 0 }, "平台链路 VIN 数上限须在 1~1024"},
		{"maxVinsHigh", func(c *ServerConfig) { c.MaxVinsPerConn = 1025 }, "平台链路 VIN 数上限须在 1~1024"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewServerService(nil)
			cfg := testServerCfg()
			tc.mutate(&cfg)
			if _, err := svc.Start(cfg, true); err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestServerServiceLoadConfigFallsBack(t *testing.T) {
	tempHome(t)
	// 旧版 server.json:不含新键 → 新键取默认
	legacy := map[string]any{"ip": "127.0.0.1", "port": 32960, "idleEnabled": true, "idleSeconds": 60}
	if err := store.Save(serverCfgFile, legacy); err != nil {
		t.Fatal(err)
	}
	svc := NewServerService(nil)
	got := svc.LoadConfig()
	if got.MaxConns != 64 || got.MaxFrameBytes != 8192 || got.LogLines != 500 || got.MaxVinsPerConn != 128 {
		t.Fatalf("旧文件缺新键应回落默认: %+v", got)
	}
	// 越界值 → 回落默认
	bad := testServerCfg()
	bad.MaxConns, bad.MaxFrameBytes, bad.LogLines, bad.MaxVinsPerConn = 9999, 1, 1, 99999
	if err := store.Save(serverCfgFile, bad); err != nil {
		t.Fatal(err)
	}
	got = svc.LoadConfig()
	if got.MaxConns != 64 || got.MaxFrameBytes != 8192 || got.LogLines != 500 || got.MaxVinsPerConn != 128 {
		t.Fatalf("越界值应回落默认: %+v", got)
	}
}

func TestServerServiceMaxConnsClosesExtra(t *testing.T) {
	tempHome(t)
	svc := NewServerService(nil)
	cfg := testServerCfg()
	cfg.MaxConns = 1
	st, err := svc.Start(cfg, true)
	if err != nil || !st.Running {
		t.Fatalf("start: %v %+v", err, st)
	}
	defer func() { _ = svc.Stop() }()

	first, err := net.DialTimeout("tcp", st.ListenAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	time.Sleep(150 * time.Millisecond) // 等 accept 循环登记首个连接

	second, err := net.DialTimeout("tcp", st.ListenAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := second.Read(make([]byte, 1)); err == nil {
		t.Fatal("超过 MaxConns 的连接应被服务端立即关闭")
	}
}
