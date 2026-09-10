package bridge

// connection_lifecycle_test.go:ConnectionService 的档案 CRUD(隔离 store)与
// 连接生命周期(真实本地假平台)测试。validateConn 的覆盖见 connection_platform_test.go。
//
// 假平台选择:复用 internal/servermode,而非手写 fake。理由——servermode 是仓库内
// 真实的平台侧实现,按规范自动应答 0x01 车辆登入 / 0x04 车辆登出(conn.go
// handleLogin/handleLogout + reply.go),零新增依赖,bridge 生产代码本就依赖该包
// (runtime.go),无循环依赖;import 面只有 New/DefaultConfig/Start/Status/Stop。
// internal/engine/client_test.go 的 fakePlatform 需手写应答组帧,重复了 servermode
// 的既有能力,仅当测试需要断言服务端接收行为时才值得自建。

import (
	"context"
	"net"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gbt32960-simulator/internal/servermode"
	"gbt32960-simulator/internal/store"
)

// startLocalPlatform 启动绑定 127.0.0.1:0 的本地假平台(动态端口),测试结束停服。
func startLocalPlatform(t *testing.T) *servermode.Server {
	t.Helper()
	srv := servermode.New(servermode.DefaultConfig("127.0.0.1:0"), servermode.Hooks{})
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("启动本地假平台: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

// connCfgForPlatform 以 DefaultConnectionConfig 为基线,指向假平台动态端口。
func connCfgForPlatform(t *testing.T, srv *servermode.Server, name string) ConnectionConfig {
	t.Helper()
	addr := srv.Status().ListenAddr
	if addr == "" {
		t.Fatal("假平台未就绪: ListenAddr 为空")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("解析监听地址 %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("端口 %q 非数字: %v", portStr, err)
	}
	cfg := *DefaultConnectionConfig()
	cfg.Name = name
	cfg.Host, cfg.Port = host, port
	return cfg
}

// closedLoopbackPort 占用 127.0.0.1:0 后立即释放,得到一个必然拒绝连接的端口。
// 内核理论上有极小概率把该端口复用给其他进程,但该分支仅需「连接失败」,
// 只要没有监听者即确定性成立(拒绝发生在毫秒级,不依赖 3s 拨号超时)。
func closedLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用临时端口: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("释放临时端口: %v", err)
	}
	return port
}

// ---------------------------------------------------------------- 档案 CRUD

// TestProfileCRUDEmptyAndSave 空 store 列表为空;保存后出现且激活;
// 同名保存为 upsert(不新增),激活跟随最后一次保存。
func TestProfileCRUDEmptyAndSave(t *testing.T) {
	tempHome(t) // store 落到临时目录
	svc := NewConnectionService(NewRuntime())

	// Given 空 store
	// When GetProfiles
	profiles, err := svc.GetProfiles()
	// Then 返回空列表、无错误
	if err != nil {
		t.Fatalf("GetProfiles(空 store): %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("空 store 应无档案, got %+v", profiles)
	}

	// When 保存 A
	cfgA := *DefaultConnectionConfig()
	cfgA.Name = "A"
	if err := svc.SaveConfig(cfgA); err != nil {
		t.Fatalf("SaveConfig(A): %v", err)
	}
	// Then A 在列表中且激活
	profiles, err = svc.GetProfiles()
	if err != nil {
		t.Fatalf("GetProfiles(A 后): %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "A" || !profiles[0].Active {
		t.Fatalf("保存 A 后列表 = %+v, want [A active]", profiles)
	}

	// When 保存 B(激活切到 B),再以同名 A(改 Host)覆盖保存
	cfgB := *DefaultConnectionConfig()
	cfgB.Name = "B"
	cfgB.Port = 33999
	if err := svc.SaveConfig(cfgB); err != nil {
		t.Fatalf("SaveConfig(B): %v", err)
	}
	cfgA2 := cfgA
	cfgA2.Host = "10.1.2.3"
	if err := svc.SaveConfig(cfgA2); err != nil {
		t.Fatalf("SaveConfig(A 覆盖): %v", err)
	}
	// Then 仍是 2 条(upsert 不新增),A 内容已覆盖且激活为 A
	profiles, err = svc.GetProfiles()
	if err != nil {
		t.Fatalf("GetProfiles(覆盖后): %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("同名保存应 upsert, got %+v", profiles)
	}
	byName := make(map[string]ProfileSummary, len(profiles))
	for _, p := range profiles {
		byName[p.Name] = p
	}
	if !byName["A"].Active || byName["B"].Active {
		t.Fatalf("最后保存的 A 应为激活: %+v", profiles)
	}
	if byName["A"].Host != "10.1.2.3" {
		t.Fatalf("A 未按 upsert 覆盖: %+v", byName["A"])
	}
}

// TestProfileCRUDSwitch 切换不存在档案报错;成功时返回完整配置、激活切换、Runtime 回填。
func TestProfileCRUDSwitch(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewConnectionService(rt)

	cfgA := *DefaultConnectionConfig()
	cfgA.Name = "A"
	cfgB := *DefaultConnectionConfig()
	cfgB.Name = "B"
	cfgB.Port = 33001
	for _, cfg := range []ConnectionConfig{cfgA, cfgB} {
		if err := svc.SaveConfig(cfg); err != nil {
			t.Fatalf("SaveConfig(%s): %v", cfg.Name, err)
		}
	}

	// When 切换不存在的档案
	if _, err := svc.SwitchProfile("nope"); err == nil {
		t.Fatal("切换不存在的档案应报错")
	}

	// When 切回 A(保存顺序使 B 为激活)
	got, err := svc.SwitchProfile("A")
	// Then 返回 A 完整配置 + 激活为 A + Runtime 回填
	if err != nil {
		t.Fatalf("SwitchProfile(A): %v", err)
	}
	if got == nil || got.Name != "A" || got.Port != cfgA.Port {
		t.Fatalf("SwitchProfile 返回值 = %+v, want A(%d)", got, cfgA.Port)
	}
	profiles, err := svc.GetProfiles()
	if err != nil {
		t.Fatalf("GetProfiles: %v", err)
	}
	active := ""
	for _, p := range profiles {
		if p.Active {
			active = p.Name
		}
	}
	if active != "A" {
		t.Fatalf("切换后激活 = %q, want A: %+v", active, profiles)
	}
	if rt.ConnCfg() == nil || rt.ConnCfg().Name != "A" {
		t.Fatalf("Runtime 未回填切换的档案: %+v", rt.ConnCfg())
	}
}

// TestProfileCRUDDelete 删除激活档案后激活回落到第一条剩余档案;
// 删除最后一条后激活清空并持久化。
func TestProfileCRUDDelete(t *testing.T) {
	tempHome(t)
	svc := NewConnectionService(NewRuntime())

	for _, name := range []string{"A", "B", "C"} {
		cfg := *DefaultConnectionConfig()
		cfg.Name = name
		if err := svc.SaveConfig(cfg); err != nil {
			t.Fatalf("SaveConfig(%s): %v", name, err)
		}
	}

	// When 删除激活的 C
	if err := svc.DeleteProfile("C"); err != nil {
		t.Fatalf("DeleteProfile(C): %v", err)
	}
	// Then 剩余 [A B],激活回落到第一条 A
	profiles, err := svc.GetProfiles()
	if err != nil {
		t.Fatalf("GetProfiles(C 删除后): %v", err)
	}
	if len(profiles) != 2 || profiles[0].Name != "A" || !profiles[0].Active {
		t.Fatalf("删除激活后应回落到 A: %+v", profiles)
	}

	// When 删除激活的 A(第一条)
	if err := svc.DeleteProfile("A"); err != nil {
		t.Fatalf("DeleteProfile(A): %v", err)
	}
	// Then 激活回落到 B
	profiles, err = svc.GetProfiles()
	if err != nil {
		t.Fatalf("GetProfiles(A 删除后): %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "B" || !profiles[0].Active {
		t.Fatalf("删除 A 后激活应为 B: %+v", profiles)
	}

	// When 删除最后一条 B
	if err := svc.DeleteProfile("B"); err != nil {
		t.Fatalf("DeleteProfile(B): %v", err)
	}
	// Then 持久化内容为空列表且 Active 清空
	var pd profilesData
	if err := store.Load(profilesFile, &pd); err != nil {
		t.Fatalf("回读 %s: %v", profilesFile, err)
	}
	if len(pd.Items) != 0 || pd.Active != "" {
		t.Fatalf("删除最后一条后应为空档案: %+v", pd)
	}
}

// TestProfileGetConfig 空 store 返回默认值并回填 Runtime;有档案时返回激活档案并回填 Runtime。
func TestProfileGetConfig(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewConnectionService(rt)

	// When 空 store 读取
	got, err := svc.GetConfig()
	// Then 默认值 + Runtime 已更新
	if err != nil {
		t.Fatalf("GetConfig(空 store): %v", err)
	}
	if want := DefaultConnectionConfig(); !reflect.DeepEqual(got, want) {
		t.Fatalf("空 store 应返回默认配置: got %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(rt.ConnCfg(), got) {
		t.Fatalf("GetConfig 未回填 Runtime: %+v", rt.ConnCfg())
	}

	// Given 有激活档案
	cfg := *DefaultConnectionConfig()
	cfg.Name = "A"
	cfg.Host = "10.9.9.9"
	cfg.Port = 33002
	if err := svc.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig(A): %v", err)
	}

	// When 再读取
	got, err = svc.GetConfig()
	// Then 返回激活档案 + Runtime 回填
	if err != nil {
		t.Fatalf("GetConfig(A): %v", err)
	}
	if got.Name != "A" || got.Host != "10.9.9.9" || got.Port != 33002 {
		t.Fatalf("GetConfig 应返回激活档案: %+v", got)
	}
	if rt.ConnCfg() == nil || rt.ConnCfg().Name != "A" {
		t.Fatalf("GetConfig 未回填激活档案到 Runtime: %+v", rt.ConnCfg())
	}
}

// ---------------------------------------------------------------- 连接生命周期

// TestConnectionLifecycleRoundTrip 真实本地假平台:Connect 阻塞至登录成功(state=online),
// Disconnect 后 state=idle。
func TestConnectionLifecycleRoundTrip(t *testing.T) {
	tempHome(t) // Connect → SaveConfig 会持久化
	srv := startLocalPlatform(t)
	svc := NewConnectionService(NewRuntime())
	t.Cleanup(func() { _ = svc.Disconnect() })

	cfg := connCfgForPlatform(t, srv, "本地")

	// When Connect(servermode 立即应答 0x01)
	if err := svc.Connect(cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	// Then 登录成功 = online
	if got := svc.State(); got != "online" {
		t.Fatalf("Connect 后 State = %q, want online", got)
	}

	// When Disconnect(发 0x04,servermode 立即应答)
	if err := svc.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	// Then idle
	if got := svc.State(); got != "idle" {
		t.Fatalf("Disconnect 后 State = %q, want idle", got)
	}
}

// TestConnectionLifecycleUnreachable 不可达端口:Connect 报错且状态保持 idle。
func TestConnectionLifecycleUnreachable(t *testing.T) {
	tempHome(t) // Connect → SaveConfig 会持久化
	svc := NewConnectionService(NewRuntime())

	cfg := *DefaultConnectionConfig()
	cfg.Name = "dead"
	cfg.Port = closedLoopbackPort(t)

	// When 连接无监听端口
	err := svc.Connect(cfg)
	// Then 报错,状态仍 idle
	if err == nil {
		t.Fatal("连接不可达端口应报错")
	}
	if got := svc.State(); got != "idle" {
		t.Fatalf("失败后 State = %q, want idle", got)
	}
}

// TestConnectProbe TestConnect 只做 TCP/TLS 可达性检测,不发协议帧。
func TestConnectProbe(t *testing.T) {
	srv := startLocalPlatform(t)
	svc := NewConnectionService(NewRuntime())

	t.Run("监听中端口可达", func(t *testing.T) {
		res := svc.TestConnect(connCfgForPlatform(t, srv, "live"))
		if !res.OK {
			t.Fatalf("TestConnect(监听中) = %+v, want OK", res)
		}
	})

	t.Run("关闭端口失败且带消息", func(t *testing.T) {
		cfg := *DefaultConnectionConfig()
		cfg.Port = closedLoopbackPort(t)
		res := svc.TestConnect(cfg)
		if res.OK {
			t.Fatalf("TestConnect(关闭端口) = %+v, want 失败", res)
		}
		if strings.TrimSpace(res.Message) == "" {
			t.Fatal("失败结果应带错误消息")
		}
	})

	t.Run("TLS 材料损坏不 panic", func(t *testing.T) {
		cfg := connCfgForPlatform(t, srv, "tls")
		cfg.TLS.Enabled = true
		// 非 PEM 文本且文件不存在 → Build 报错(TCP 已可达,命中 TLS 分支)
		cfg.TLS.ClientCert = filepath.Join(t.TempDir(), "missing-cert.pem")
		cfg.TLS.ClientKey = filepath.Join(t.TempDir(), "missing-key.pem")
		res := svc.TestConnect(cfg)
		if res.OK {
			t.Fatalf("TLS 材料损坏应失败: %+v", res)
		}
		if !strings.Contains(res.Message, "TLS") {
			t.Fatalf("应报 TLS 配置错误(TCP 已可达), got %q", res.Message)
		}
	})
}

// TestConnectionStateWithoutClient 无客户端时 State 为 idle。
func TestConnectionStateWithoutClient(t *testing.T) {
	svc := NewConnectionService(NewRuntime())
	if got := svc.State(); got != "idle" {
		t.Fatalf("State(无客户端) = %q, want idle", got)
	}
}
