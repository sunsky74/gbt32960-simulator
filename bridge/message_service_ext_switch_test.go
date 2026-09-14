package bridge

import (
	"path/filepath"
	"testing"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/store"
)

// newExtSwitchRT 加载两个共享扩展组键 "telemetry" 的包(默认绑定 A=p2golden)。
// A 的字段布局 soc2/packVolt,B 的布局 soc2/tempC(默认偏移 10),用于跨包串值回归。
func newExtSwitchRT(t *testing.T) *Runtime {
	t.Helper()
	pA, err := ext.LoadFile(filepath.Join("testdata", "extpack.json"))
	if err != nil {
		t.Fatal(err)
	}
	pB, err := ext.LoadFile(filepath.Join("testdata", "extpack_alt.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{pA, pB})
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "p2golden"})
	return rt
}

const extGroupKey = "telemetry"

// TestSaveGroupsPersistsStandardKeysOnly 扩展组值必须按包隔离存 extgroups.json,
// 不得混写进 message.json(否则切到同键包时旧值串入新包)。
func TestSaveGroupsPersistsStandardKeysOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtSwitchRT(t)
	ms := NewMessageService(rt)

	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m[extGroupKey] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"soc2": 77, "packVolt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}

	var onDisk map[string]schema.GroupConfig
	if err := store.Load(groupsFile, &onDisk); err != nil {
		t.Fatal(err)
	}
	if len(onDisk) == 0 {
		t.Fatal("message.json 不应为空(标准组必须落盘)")
	}
	for k := range onDisk {
		if !schema.IsStandardGroupKey(k) {
			t.Fatalf("message.json 不应持久化扩展组键 %q", k)
		}
	}
}

// TestExtGroupValuesDoNotBleedAcrossPacks A→B→A:同键扩展组的值不串包。
func TestExtGroupValuesDoNotBleedAcrossPacks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := newExtSwitchRT(t)
	ms := NewMessageService(rt)

	// A 包保存 telemetry 行值
	payload := ms.DefaultGroups("2016")
	m := payload.ToMap()
	m[extGroupKey] = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{"soc2": 77, "packVolt": 3.3}}}
	if err := ms.SaveGroups(*schema.FromMap(m, nil)); err != nil {
		t.Fatal(err)
	}

	// 切到 B(同键、未保存):不得暴露 A 的值
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "p2alt"})
	got, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := got.ToMap()[extGroupKey]; ok {
		if v := g.Rows[0]["soc2"]; v == float64(77) {
			t.Fatalf("跨包串值:B 包暴露了 A 包 telemetry 行值 soc2=%v", v)
		}
	}

	// 切回 A:值应从 extgroups.json 按包恢复
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "p2golden"})
	back, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	g, ok := back.ToMap()[extGroupKey]
	if !ok || len(g.Rows) == 0 {
		t.Fatalf("切回 A 后扩展组配置应恢复, got %+v", back.ToMap())
	}
	if g.Rows[0]["soc2"] != float64(77) || g.Rows[0]["packVolt"] != 3.3 {
		t.Fatalf("切回 A 后扩展组值 = %+v, want soc2=77 packVolt=3.3", g.Rows[0])
	}
}

// TestLoadAllGroupsDropsLegacyPollution message.json 中历史遗留的扩展键
// 必须在加载时剔除,不能仅靠 GetGroups 的 order 过滤(绑同键包会进入 order)。
func TestLoadAllGroupsDropsLegacyPollution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := store.Save(groupsFile, &map[string]schema.GroupConfig{
		"vehicle":   {Enabled: true, Rows: []map[string]any{{}}},
		extGroupKey: {Enabled: true, Rows: []map[string]any{{"soc2": 88, "tempC": 10}}},
	}); err != nil {
		t.Fatal(err)
	}
	rt := newExtSwitchRT(t)
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "p2alt"})
	ms := NewMessageService(rt)

	got, err := ms.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := got.ToMap()[extGroupKey]; ok {
		if g.Rows[0]["soc2"] == float64(88) {
			t.Fatalf("message.json 中的遗留扩展键未被剔除: %+v", g)
		}
	}
}
