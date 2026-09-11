package bridge

import (
	"encoding/hex"
	"testing"
	"time"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
)

func TestAssembleRemoteAck(t *testing.T) {
	rt := NewRuntime()
	p, err := ext.LoadFile("../internal/ext/testdata/remote-ack-fixture.json")
	if err != nil {
		t.Fatalf("加载夹具: %v", err)
	}
	rt.SetPacks([]*ext.Pack{p})
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: p.Meta.ID})
	cmd := p.Commands[0] // 0x09 锁车应答
	// 模拟用户填表:流水号 0x1234、锁车状态=0x05(请求锁车+限速中)、等级状态=0、速度状态=0、故障=00(无故障)
	rt.SetGroups(map[string]schema.GroupConfig{
		cmd.Key: {Enabled: true, Rows: []schema.RowValue{{
			"serialNumber": float64(0x1234),
			"lockStatus": map[string]any{
				"bit0": true, "bit1": false, "bit2": true,
				"bit3": false, "bit4": false, "bit5": false,
			},
			"speedLevelStatus": map[string]any{},
			"speedValueStatus": map[string]any{},
			"faultList":        "",
		}}},
	})
	ms := &MessageService{rt: rt}
	payload, err := ms.assembleCommandBody(cmd, time.Date(2026, 9, 2, 10, 30, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	hexStr := hex.EncodeToString(payload)
	// 表21头: 时间 1A09020A1E00(26-09-02 10:30:00) + 流水号 1234 + N=01 + 子指令 09
	// 应答体: 锁车状态 05 + 等级 00 + 速度 00 (故障区空)
	want := "1a09020a1e00" + "1234" + "01" + "09" + "050000"
	if hexStr != want {
		t.Fatalf("payload = %s, want %s", hexStr, want)
	}
}
