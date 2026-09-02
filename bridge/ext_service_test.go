package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/store"
)

func copyExtFixture(t *testing.T, dir, name string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "extpack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtServiceReloadDir(t *testing.T) {
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)

	dir := t.TempDir()
	copyExtFixture(t, dir, "good.json")
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("skip"), 0o644)

	err := s.reloadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("err = %v, want 提及 broken.json", err)
	}
	packs := rt.Packs()
	if len(packs) != 1 || packs[0].Meta.ID != "p2golden" {
		t.Fatalf("packs = %+v, want 1 个 p2golden", packs)
	}

	t.Run("绑定后可激活", func(t *testing.T) {
		rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "p2golden"})
		if p := rt.Pack(); p == nil || p.Meta.ID != "p2golden" {
			t.Fatalf("激活失败: %v", p)
		}
	})
}

func TestExtServiceListPacksBadFileIgnored(t *testing.T) {
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)
	dir := t.TempDir()
	copyExtFixture(t, dir, "good.json")
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	infos := s.listPacksDir(dir)
	if len(infos) != 1 || infos[0].ID != "p2golden" {
		t.Fatalf("坏文件不应阻塞列表: %+v", infos)
	}
}

func TestExtServiceListPacksShape(t *testing.T) {
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)
	dir := t.TempDir()
	copyExtFixture(t, dir, "a.json")
	if err := s.reloadDir(dir); err != nil {
		t.Fatal(err)
	}
	infos := packInfosOf(rt.Packs())
	if len(infos) != 1 {
		t.Fatalf("infos = %+v", infos)
	}
	i := infos[0]
	if i.ID != "p2golden" || i.BaseVersion != "2016" || i.UnitCount != 1 || i.Label != "P2 黄金包" {
		t.Fatalf("info = %+v", i)
	}
}

// tempHome 让 store.Dir() 落到临时目录(macOS UserConfigDir 跟随 $HOME)。
func tempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const extcmdJSON = `{
  "meta": {"id": "extcmd", "label": "扩展命令包", "vendor": "Test", "baseVersion": "2016"},
  "realtime": {"appendUnits": []},
  "commands": [
    {"key": "extData09", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual+periodic",
     "body": {"type": "fields", "fields": [
       {"key": "seq", "label": "流水号", "type": "u16"},
       {"key": "volt", "label": "电压", "type": "u16", "scale": 0.1, "unit": "V"}
     ]}}
  ]
}`

func TestImportPackValid(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	src := writeFile(t, t.TempDir(), "vendor-pack.json", extcmdJSON)
	info, err := svc.ImportPack(src)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "extcmd" || info.CommandCount != 1 || info.BaseVersion != "2016" {
		t.Fatalf("info = %+v", info)
	}
	dir, err := packsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "extcmd.json")); err != nil {
		t.Fatalf("应落盘 packs 目录: %v", err)
	}
	packs, err := svc.ListPacks()
	if err != nil || len(packs) != 1 {
		t.Fatalf("packs = %+v err = %v", packs, err)
	}
}

func TestImportPackInvalidNotPersisted(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	src := writeFile(t, t.TempDir(), "bad.json", `{"meta": {}}`)
	if _, err := svc.ImportPack(src); err == nil {
		t.Fatal("非法包应拒绝")
	}
	dir, _ := packsDir()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("非法包不得落盘: %v", entries)
	}
}

func TestDeletePackAutoUnbind(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	src := writeFile(t, t.TempDir(), "vendor-pack.json", extcmdJSON)
	if _, err := svc.ImportPack(src); err != nil {
		t.Fatal(err)
	}
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	if rt.Pack() == nil {
		t.Fatal("导入后应可绑定")
	}
	if err := svc.DeletePack("extcmd"); err != nil {
		t.Fatal(err)
	}
	if rt.Pack() != nil {
		t.Fatal("删除绑定包后应自动解绑")
	}
	packs, _ := svc.ListPacks()
	if len(packs) != 0 {
		t.Fatalf("删除后列表应空: %+v", packs)
	}
}

func TestImportPackOverwriteClearsExtGroups(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	if _, err := svc.ImportPack(writeFile(t, t.TempDir(), "vendor-pack.json", extcmdJSON)); err != nil {
		t.Fatal(err)
	}
	// 预置旧布局下已保存的扩展组配置
	all := map[string]map[string]schema.GroupConfig{
		"extcmd": {"extData09": {Enabled: true, Rows: []map[string]any{{"seq": float64(42), "volt": 3.3}}}},
	}
	if err := store.Save(extGroupsFile, &all); err != nil {
		t.Fatal(err)
	}
	// 覆盖导入同 id 新布局(仅 seq,无 volt)
	newJSON := `{
  "meta": {"id": "extcmd", "label": "扩展命令包v2", "vendor": "Test", "baseVersion": "2016"},
  "realtime": {"appendUnits": []},
  "commands": [
    {"key": "extData09", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual",
     "body": {"type": "fields", "fields": [{"key": "seq", "label": "流水号", "type": "u16"}]}}
  ]
}`
	if _, err := svc.ImportPack(writeFile(t, t.TempDir(), "vendor-pack-v2.json", newJSON)); err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["extcmd"]; ok {
		t.Fatal("覆盖导入应清除该包扩展组配置")
	}
}
