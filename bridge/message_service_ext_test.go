package bridge

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"

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
