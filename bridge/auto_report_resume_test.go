package bridge

// auto_report_resume_test.go:手动重连后周期上报恢复的回归测试。
//
// 缺陷背景:engine.Disconnect 会停止周期上报 ticker(client.go),但 bridge 的
// MessageService.reportOn 记忆保持 true(AutoReportState 仍上报 "on" 给 UI)。
// 手动重连时 ConnectionService.Connect 创建全新客户端,新客户端没有 ticker——
// 断开→重连后开关显示 ON,0x02 却永不发送。修复:app 装配注入 onConnected
// 回调,登录成功后按记忆状态重新装载 ticker。
//
// 假平台复用 connection_concurrency_test.go 的 delayedPlatform(0x01 延迟
// 200ms 应答,其余立即应答,0x04 应答后关闭连接)。

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
)

func TestAutoReportResumesAfterReconnect(t *testing.T) {
	tempHome(t) // Connect → SaveConfig 会持久化,隔离到临时目录
	platform := startDelayedPlatform(t)
	rt := NewRuntime()
	cs := NewConnectionService(rt)
	ms := NewMessageService(rt)
	cs.setOnConnected(func() { _ = ms.resumeAutoReport() }) // 模拟 app 装配
	t.Cleanup(func() { _ = cs.Disconnect() })

	ch, cancel := rt.Bus().Subscribe(4096)
	defer cancel()
	var reports atomic.Int64
	go func() {
		for ev := range ch {
			if ev.Kind == engine.EventTx && strings.HasPrefix(ev.Cmd, "0x02") {
				reports.Add(1)
			}
		}
	}()

	cfg := *DefaultConnectionConfig()
	cfg.Name = "resume"
	cfg.Host, cfg.Port = "127.0.0.1", platform.port()

	// 1) 首次连接 + 开启周期上报(1s),等待首个 0x02
	if err := cs.Connect(cfg); err != nil {
		t.Fatalf("首次 Connect: %v", err)
	}
	// 组快照必须在 Connect 之后注入:Connect→SaveConfig→SetConnCfg 在无旧
	// 配置(prev == nil)时会清空 rt.groups,提前设置会被抹掉。
	rt.SetGroups(ms.DefaultGroups("2016").ToMap())
	if err := ms.SetAutoReport(true, 1); err != nil {
		t.Fatalf("SetAutoReport(true, 1): %v", err)
	}
	c1 := waitReportCount(t, &reports, 1, 2*time.Second)
	t.Logf("重连前首个 0x02 已发出(计数 %d)", c1)

	// 2) 断开:引擎停 ticker,bridge 记忆必须保持 ON
	if err := cs.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if !ms.AutoReportState().On {
		t.Fatal("断开不应把周期上报开关记忆扭转为 OFF")
	}
	// 引擎停 ticker 不等待在途 tick 协程结束:先等迟到 0x02 落地再取基线,
	// 否则旧连接的迟到帧可能让「重连后计数增长」的断言假阳性。
	time.Sleep(500 * time.Millisecond)
	baseline := reports.Load()

	// 3) 手动重连:必须按记忆状态自动恢复周期上报,否则开关 ON 但 0x02 永不发送
	if err := cs.Connect(cfg); err != nil {
		t.Fatalf("重连 Connect: %v", err)
	}
	if !ms.AutoReportState().On {
		t.Fatal("重连后周期上报开关记忆不应变化")
	}
	if got := waitReportCount(t, &reports, baseline+1, 3*time.Second); got <= baseline {
		t.Fatalf("重连后周期上报未恢复: 0x02 计数停在 %d (重连前基线 %d)", got, baseline)
	}
}

// waitReportCount 轮询等待 0x02 计数达到 want;超时判失败并报告当前计数。
func waitReportCount(t *testing.T, count *atomic.Int64, want int64, timeout time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n := count.Load(); n >= want {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 0x02 计数达到 %d 超时(当前 %d)", want, count.Load())
	return 0
}
