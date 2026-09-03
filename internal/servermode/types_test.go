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
	h.OnStatus(Status{})        // 不 panic
	h.OnSession(SessionEvent{}) //
	h.OnFrame(FrameEvent{})     //
	h.OnWarn(WarnEvent{})       //
	if h.Now().IsZero() {
		t.Fatal("Now 未填充默认值")
	}
	fixed := time.Unix(1_700_000_000, 0)
	h2 := normalizeHooks(Hooks{Now: func() time.Time { return fixed }})
	if !h2.Now().Equal(fixed) {
		t.Fatal("自定义 Now 被覆盖")
	}
}
