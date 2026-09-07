package bridge

import (
	"testing"

	"gbt32960-simulator/internal/servermode"
)

func TestServerServiceStartStopPersist(t *testing.T) {
	tempHome(t) // store 持久化落到临时目录,不污染真实用户配置
	svc := NewServerService(nil)
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
}

func TestServerServiceRestartOnNewPort(t *testing.T) {
	tempHome(t)
	svc := NewServerService(nil)
	st1, err := svc.Start(ServerConfig{IP: "127.0.0.1", Port: 0, IdleEnabled: true, IdleSeconds: 60}, true)
	if err != nil || !st1.Running {
		t.Fatalf("首次启动: %v %+v", err, st1)
	}
	oldAddr := st1.ListenAddr
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	// 再启动必须绑定新的临时端口(重建 Server,不残留旧 Addr)
	st2, err := svc.Start(ServerConfig{IP: "127.0.0.1", Port: 0, IdleEnabled: true, IdleSeconds: 60}, true)
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
