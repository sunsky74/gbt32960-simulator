package bridge

import (
	"testing"

	"gbt32960-simulator/internal/servermode"
)

func TestServerServiceStartStopPersist(t *testing.T) {
	tempHome(t) // store 持久化落到临时目录,不污染真实用户配置
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
}

func TestServerServiceNonLoopbackNeedsForce(t *testing.T) {
	svc := NewServerService()
	if _, err := svc.Start(ServerConfig{IP: "0.0.0.0", Port: 0}, false); err == nil {
		t.Fatal("非环回地址未 force 应拒绝")
	}
	// 引用守卫:载荷类型来自 servermode,绑定面签名依赖它们
	_ = servermode.Status{}
	_ = []servermode.Session(nil)
}
