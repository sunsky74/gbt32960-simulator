package engine

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
)

// TestConnectFailureClearsCancel 回归 ⑦:拨号失败后必须清空 c.cancel/c.lifeCtx,
// 否则随后的 Disconnect 会误判"存在活动连接",在 nil conn 上发登出,
// 冒出一条假的 "发送登出报文失败" EventError。
func TestConnectFailureClearsCancel(t *testing.T) {
	// 找一个必然拒绝的端口(先监听再关闭,复用 TestClientConnectRefused 手法)。
	// 仅当受限环境连本地回环监听端口都分配不到时跳过;正常环境不应命中 skip。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("受限环境:本地监听端口分配失败,跳过: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	bus := NewBus()
	events, stop := bus.Subscribe(64)
	defer stop()

	client := NewClient(Options{
		Host: "127.0.0.1", Port: port, Version: api.V2016,
		VIN: "LSV00000000000001", ICCID: "89860000000000000001",
	}, bus)
	if err := client.Connect(context.Background()); err == nil {
		t.Fatal("connect should fail")
	}

	// (b) 白盒:失败连接不得残留 cancel/lifeCtx。
	client.mu.Lock()
	cancel, lifeCtx := client.cancel, client.lifeCtx
	client.mu.Unlock()
	if cancel != nil || lifeCtx != nil {
		t.Errorf("失败 Connect 后应清空 cancel/lifeCtx: cancel=%v lifeCtx=%v", cancel != nil, lifeCtx != nil)
	}

	client.Disconnect()

	// (a) 失败态 Disconnect 必须是幂等空操作:不产生登出失败事件。
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case e := <-events:
			if e.Kind == EventError && strings.Contains(e.Message, "发送登出报文失败") {
				t.Fatalf("失败态 Disconnect 发出了假登出错误事件: %+v", e)
			}
		case <-deadline:
			return
		}
	}
}
