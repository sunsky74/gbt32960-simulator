package bridge

import (
	"path/filepath"
	"testing"

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
