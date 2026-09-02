package bridge

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
)

// newExtRT 构造绑定了 p2golden 包与 2016 配置的 Runtime(测试统一入口)。
func newExtRT(t *testing.T, packBound bool) *Runtime {
	t.Helper()
	p, err := ext.LoadFile(filepath.Join("testdata", "extpack.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{p})
	cfg := &ConnectionConfig{Version: "2016"}
	if packBound {
		cfg.ExtensionPack = "p2golden"
	}
	rt.SetConnCfg(cfg)
	return rt
}

func TestGetSchemaMerged(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)

	merged := ms.GetSchema("2016")
	if want := len(schema.V2016Groups()) + 1; len(merged) != want {
		t.Fatalf("合并后组数 = %d, want %d", len(merged), want)
	}
	last := merged[len(merged)-1]
	if last.Key != "telemetry" || last.Title != "私有遥测" || !last.Enabled {
		t.Fatalf("扩展组 = %+v", last)
	}
	if len(last.Fields) != 2 || last.Fields[0].Kind != "int" || last.Fields[1].Kind != "float" {
		t.Fatalf("字段 kind = %+v", last.Fields)
	}
}

func TestGetSchemaVersionGate(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	// 包 baseVersion=2016,请求 2025 → 不合并
	if got := len(ms.GetSchema("2025")); got != len(schema.V2025Groups()) {
		t.Fatalf("2025 组数 = %d, want %d", got, len(schema.V2025Groups()))
	}
}

func TestGetSchemaNoPack(t *testing.T) {
	rt := newExtRT(t, false)
	ms := NewMessageService(rt)
	if got := len(ms.GetSchema("2016")); got != len(schema.V2016Groups()) {
		t.Fatalf("未绑包组数 = %d, want %d", got, len(schema.V2016Groups()))
	}
}

func TestDefaultGroupsExtDefaults(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	g, ok := m["telemetry"]
	if !ok || !g.Enabled || len(g.Rows) != 1 {
		t.Fatalf("telemetry 默认 = %+v", g)
	}
	if g.Rows[0]["soc2"] != 0.0 || g.Rows[0]["packVolt"] != 0.0 {
		t.Fatalf("默认行值 = %+v", g.Rows[0])
	}
}

func TestGetGroupsFiltersStaleKeys(t *testing.T) {
	rt := newExtRT(t, false) // 已解绑
	// 注意顺序:NewMessageService 构造会读用户目录 message.json 并 SetGroups,
	// 测试注入必须在其后覆盖(全计划测试统一遵守"先构造、后注入")。
	ms := NewMessageService(rt)
	rows := []map[string]any{{"soc2": 80}}
	rt.SetGroups(map[string]schema.GroupConfig{
		"vehicle":   {Enabled: true, Rows: []map[string]any{{}}},
		"telemetry": {Enabled: true, Rows: rows}, // 旧包残留键
	})
	payload, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	m := payload.ToMap()
	if _, exists := m["telemetry"]; exists {
		t.Fatal("残留扩展键应被过滤")
	}
	if _, exists := m["vehicle"]; !exists {
		t.Fatal("标准组不应丢失")
	}
}

func TestAssembleBodyAppendsTLV(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	b, err := body.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// 标准体(空组=仅 6B 十进制时间 1a0102030405)+ TLV(80 0003 50 0021)
	const want = "1a0102030405800003500021"
	if got := hex.EncodeToString(b); got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestAssembleBodyNoPackIdentical(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	groups := map[string]schema.GroupConfig{
		"telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	}
	rt.SetConnCfg(&ConnectionConfig{Version: "2016"}) // 解绑
	rt.SetGroups(groups)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	got, err := body.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	wantBody, err := schema.Assemble(api.V2016, groups, at)
	if err != nil {
		t.Fatal(err)
	}
	want, err := wantBody.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("未绑包须逐字节一致: got %x want %x", got, want)
	}
}

func TestAssembleBodyDisabledUnitSkipped(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"telemetry": {Enabled: false, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := body.Bytes()
	if got := hex.EncodeToString(b); got != "1a0102030405" {
		t.Fatalf("禁用单元不应追加: %s", got)
	}
}

func TestPreviewTailGolden(t *testing.T) {
	rt := newExtRT(t, true)
	ms := NewMessageService(rt)
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", VIN: "LSV00000000000001", ExtensionPack: "p2golden"})
	rt.SetGroups(map[string]schema.GroupConfig{
		"telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	r, err := ms.Preview()
	if err != nil {
		t.Fatal(err)
	}
	payload := r.Hex[:len(r.Hex)-2] // 完整帧末字节是 BCC,断言须剥离
	if !strings.HasPrefix(payload, "2323") {
		t.Fatalf("帧头 = %s", payload[:8])
	}
	if !strings.HasSuffix(payload, "800003500021") {
		t.Fatalf("载荷尾 = ...%s", payload[len(payload)-24:])
	}
}

func TestAssembleBodyV2025VersionDispatch(t *testing.T) {
	// 修复既有 bug:V2025 连接的补发/实时体必须走 2025 组装(此前硬编码 2016 形体)
	rt := newExtRT(t, false)
	rt.SetConnCfg(&ConnectionConfig{Version: "2025"})
	ms := NewMessageService(rt)
	groups := ms.DefaultGroups("2025").ToMap()
	rt.SetGroups(groups)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	body, err := ms.assembleBody(at)
	if err != nil {
		t.Fatal(err)
	}
	got, err := body.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	wantBody, err := schema.Assemble(api.V2025, groups, at)
	if err != nil {
		t.Fatal(err)
	}
	want, err := wantBody.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("V2025 分发失败: got %x want %x", got, want)
	}
}

// hexBytes hex 助手(P1-2:bridge 包内无现成助手,测试断言用)。
func hexBytes(b []byte) string { return hex.EncodeToString(b) }

func newExtCmdRT(t *testing.T, bound bool) *Runtime {
	t.Helper()
	p, err := ext.LoadFile(filepath.Join("testdata", "extcmd.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{p})
	cfg := &ConnectionConfig{Version: "2016"}
	if bound {
		cfg.ExtensionPack = "extcmd"
	}
	rt.SetConnCfg(cfg)
	return rt
}

func TestGetSchemaCommandGroups(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	merged := ms.GetSchema("2016")
	var cmdGroups []schema.GroupSchema
	for _, g := range merged {
		if g.Source == "command" {
			cmdGroups = append(cmdGroups, g)
		}
	}
	// fields 体 1 组 + realtimeLike 1 单元 1 组
	if len(cmdGroups) != 2 {
		t.Fatalf("命令组数 = %d, want 2: %+v", len(cmdGroups), cmdGroups)
	}
	byKey := map[string]schema.GroupSchema{}
	for _, g := range cmdGroups {
		byKey[g.Key] = g
	}
	if g, ok := byKey["extData09"]; !ok || g.Title != "扩展数据上报" {
		t.Fatalf("extData09 = %+v", g)
	}
	if g, ok := byKey["extReport0A:telemetry"]; !ok || g.Title != "扩展报表 · 私有遥测" {
		t.Fatalf("extReport0A:telemetry = %+v", g)
	}
}

func TestGetSchemaNoPackNoCommandGroups(t *testing.T) {
	rt := newExtCmdRT(t, false)
	ms := NewMessageService(rt)
	for _, g := range ms.GetSchema("2016") {
		if g.Source == "command" {
			t.Fatalf("未绑包不应有命令组: %+v", g)
		}
	}
}

func TestAssembleCommandBodyFieldsGolden(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"extData09": {Enabled: true, Rows: []map[string]any{{"seq": 1, "volt": 3.3}}},
	})
	cmd := rt.Pack().Commands[0]
	b, err := ms.assembleCommandBody(cmd, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// seq u16=1 → 0001;volt 3.3/0.1=33 → 0021
	if got := hexBytes(b); got != "00010021" {
		t.Fatalf("fields 体 = %s, want 00010021", got)
	}
}

func TestAssembleCommandBodyRealtimeLikeGolden(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"extReport0A:telemetry": {Enabled: true, Rows: []map[string]any{{"soc2": 80, "packVolt": 3.3}}},
	})
	cmd := rt.Pack().Commands[1]
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b, err := ms.assembleCommandBody(cmd, at)
	if err != nil {
		t.Fatal(err)
	}
	// 6B 十进制时间 + TLV(80 0003 50 0021)
	if got := hexBytes(b); got != "1a0102030405800003500021" {
		t.Fatalf("realtimeLike 体 = %s", got)
	}
}

func TestSendFrameWireCommandByte(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	rt.SetGroups(map[string]schema.GroupConfig{
		"extData09": {Enabled: true, Rows: []map[string]any{{"seq": 1, "volt": 3.3}}},
	})
	cmd := rt.Pack().Commands[0]
	payload, err := ms.assembleCommandBody(cmd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := engine.BuildFrame(api.V2016, "LSV00000000000001", byte(cmd.Code), engine.NewRawBody(api.V2016, payload))
	if err != nil {
		t.Fatal(err)
	}
	hexStr := hexBytes(raw)
	// 帧布局:2323(2B)+ 命令码(1B)+ 响应标志(1B)...;命令码即 hex[4:6]
	if hexStr[4:6] != "09" {
		t.Fatalf("wire command byte = %s, want 09(折叠回归!)", hexStr[4:6])
	}
	// 对照组:0x0A 命令码不能折叠成 0x09
	cmdA := rt.Pack().Commands[1]
	payloadA, err := ms.assembleCommandBody(cmdA, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rawA, _, err := engine.BuildFrame(api.V2016, "LSV00000000000001", byte(cmdA.Code), engine.NewRawBody(api.V2016, payloadA))
	if err != nil {
		t.Fatal(err)
	}
	if h := hexBytes(rawA); h[4:6] != "0a" {
		t.Fatalf("wire command byte = %s, want 0a(0x0A 不得折叠为 0x09)", h[4:6])
	}
}

func TestDefaultGroupsCommandDefaults(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	if g, ok := m["extData09"]; !ok || !g.Enabled || len(g.Rows) != 1 {
		t.Fatalf("extData09 默认 = %+v", g)
	}
	if g, ok := m["extReport0A:telemetry"]; !ok || !g.Enabled {
		t.Fatalf("extReport0A:telemetry 默认 = %+v", g)
	}
}

func TestExtReportTickerLifecycle(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	if err := ms.SetExtAutoReport("extData09", true, 1); err != nil {
		t.Fatal(err)
	}
	ms.extMu.Lock()
	_, registered := ms.extStops["extData09"]
	ms.extMu.Unlock()
	if !registered {
		t.Fatal("ticker 未注册")
	}
	// 解绑 → 自停(每次 tick 检查包与命令存在性)
	rt.SetConnCfg(&ConnectionConfig{Version: "2016"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		ms.extMu.Lock()
		_, ok := ms.extStops["extData09"]
		ms.extMu.Unlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("解绑后 ticker 未自停")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 重绑包后重启周期并显式停止(修复评审 Important#2:原解绑态下 start 静默失败,重启路径空转)
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	if err := ms.SetExtAutoReport("extData09", true, 50); err != nil {
		t.Fatalf("重绑后重启应成功: %v", err)
	}
	ms.stopExtReport("extData09")
	if err := ms.SetExtAutoReport("extData09", false, 0); err != nil {
		t.Fatalf("停止应成功: %v", err)
	}
}

func TestExtTickerStopsOnVersionSwitch(t *testing.T) {
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	if err := ms.SetExtAutoReport("extData09", true, 1); err != nil {
		t.Fatal(err)
	}
	// 版本切换(包仍绑 extcmd,但 baseVersion 2016 ≠ 2025)→ 下次 tick 应自停
	rt.SetConnCfg(&ConnectionConfig{Version: "2025", ExtensionPack: "extcmd"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		ms.extMu.Lock()
		_, ok := ms.extStops["extData09"]
		ms.extMu.Unlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("版本切换后 ticker 未自停")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSetExtAutoReportVersionMismatch(t *testing.T) {
	rt := newExtCmdRT(t, true)
	rt.SetConnCfg(&ConnectionConfig{Version: "2025", ExtensionPack: "extcmd"})
	ms := NewMessageService(rt)
	err := ms.SetExtAutoReport("extData09", true, 10)
	if err == nil || !strings.Contains(err.Error(), "不匹配") {
		t.Fatalf("期望版本不匹配错误,实际: %v", err)
	}
}

func TestSaveGroupsExtDryRunRejects(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 隔离真实用户 message.json
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 70000, "volt": 3.3}}}
	err := ms.SaveGroups(*schema.FromMap(m, nil))
	if err == nil || !strings.Contains(err.Error(), "扩展命令 extData09") {
		t.Fatalf("超范围值应报扩展命令错误, got %v", err)
	}
	if rt.Groups() != nil {
		t.Fatal("失败不得写入内存快照")
	}
}

func TestSaveGroupsExtDryRunPasses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 1, "volt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}
}

func TestExtGroupsPersistAcrossUnbind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 42, "volt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}
	// 解绑:前端此时只提交标准组载荷
	rt.SetConnCfg(&ConnectionConfig{Version: "2016"})
	std := schema.FromMap(map[string]schema.GroupConfig{
		"vehicle": {Enabled: true, Rows: m["vehicle"].Rows},
	}, []string{"vehicle"})
	if err := ms.SaveGroups(*std); err != nil {
		t.Fatal(err)
	}
	// 重绑:命令组配置应从 extgroups.json 恢复,而非默认值
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	got, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	gm := got.ToMap()
	g, ok := gm["extData09"]
	if !ok || g.Rows[0]["seq"] != float64(42) {
		t.Fatalf("重绑后命令组配置应恢复, got %+v", gm["extData09"])
	}
	if v, ok := gm["vehicle"]; !ok || !v.Enabled {
		t.Fatal("标准组配置应保持")
	}
}

func TestExtGroupsSurviveStandardOnlySave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtCmdRT(t, true)
	ms := NewMessageService(rt)
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m["extData09"] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"seq": 42, "volt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}
	// 实时面板保存:order 仅标准组键,payload 不含命令组——不得覆盖命令组配置
	std := schema.FromMap(map[string]schema.GroupConfig{
		"vehicle": {Enabled: true, Rows: m["vehicle"].Rows},
	}, []string{"vehicle"})
	if err := ms.SaveGroups(*std); err != nil {
		t.Fatal(err)
	}
	got, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	gm := got.ToMap()
	if g, ok := gm["extData09"]; !ok || g.Rows[0]["seq"] != float64(42) {
		t.Fatalf("标准组保存不得覆盖命令组配置, got %+v", gm["extData09"])
	}
}
