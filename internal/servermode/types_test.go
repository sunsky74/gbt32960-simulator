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
	if cfg.MaxVinsPerConn != 128 || cfg.LogLines != 500 {
		t.Fatalf("默认扩展参数错误: %+v", cfg)
	}
	if !cfg.IdleEnabled || cfg.IdleTimeout != 60*time.Second {
		t.Fatalf("默认空闲配置错误: %+v", cfg)
	}
}

func TestNewFillsNonPositiveConfig(t *testing.T) {
	srv := New(Config{}, Hooks{})
	cfg := srv.cfgSnapshot()
	if cfg.MaxConns != 64 || cfg.MaxFrameBytes != 8192 || cfg.MaxVinsPerConn != 128 || cfg.LogLines != 500 {
		t.Fatalf("非正数配置未回落默认值: %+v", cfg)
	}
	if srv.buf.cap != cfg.LogLines {
		t.Fatalf("环形缓冲容量 = %d, want cfg.LogLines=%d", srv.buf.cap, cfg.LogLines)
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
