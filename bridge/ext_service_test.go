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
	tempHome(t)
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
	tempHome(t)
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
	tempHome(t)
	rt := NewRuntime()
	s := NewExtServiceForTest(rt)
	dir := t.TempDir()
	copyExtFixture(t, dir, "a.json")
	if err := s.reloadDir(dir); err != nil {
		t.Fatal(err)
	}
	infos := packInfosOf(rt.Packs(), map[string]bool{})
	if len(infos) != 1 {
		t.Fatalf("infos = %+v", infos)
	}
	i := infos[0]
	if i.ID != "p2golden" || i.BaseVersion != "2016" || i.UnitCount != 1 || i.Label != "P2 黄金包" {
		t.Fatalf("info = %+v", i)
	}
	if !i.Enabled {
		t.Fatalf("默认应启用: %+v", i)
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

func TestImportPackJSONValid(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	info, err := svc.ImportPackJSON(extcmdJSON)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "extcmd" || !info.Enabled {
		t.Fatalf("info = %+v", info)
	}
	dir, err := packsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "extcmd.json")); err != nil {
		t.Fatalf("粘贴导入应落盘 packs 目录: %v", err)
	}
}

func TestImportPackJSONInvalidNotPersisted(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)

	if _, err := svc.ImportPackJSON("   "); err == nil {
		t.Fatal("空内容应拒绝")
	}
	if _, err := svc.ImportPackJSON("{bad json"); err == nil {
		t.Fatal("非法 JSON 应拒绝")
	}
	// 语法合法但校验失败(unitCode 占用标准 0x01)
	bad := `{"meta": {"id": "x1", "label": "坏包", "baseVersion": "2016"},
	         "realtime": {"appendUnits": [{"key": "u", "title": "单元", "unitCode": 1, "enabled": true,
	           "fields": [{"key": "f", "label": "字段", "type": "u8"}]}]}}`
	if _, err := svc.ImportPackJSON(bad); err == nil {
		t.Fatal("校验失败应拒绝")
	}
	dir, _ := packsDir()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("失败导入不得落盘: %v", entries)
	}
}

func TestSetPackEnabled(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	if _, err := svc.ImportPackJSON(extcmdJSON); err != nil {
		t.Fatal(err)
	}
	rt.SetConnCfg(&ConnectionConfig{Version: "2016", ExtensionPack: "extcmd"})
	if rt.Pack() == nil {
		t.Fatal("绑定后应激活")
	}

	if err := svc.SetPackEnabled("extcmd", false); err != nil {
		t.Fatal(err)
	}
	if rt.Pack() != nil {
		t.Fatal("停用后运行时不应激活")
	}
	if rt.ConnCfg().ExtensionPack != "extcmd" {
		t.Fatal("停用不应改变连接档案绑定关系")
	}
	infos, _ := svc.ListPacks()
	if len(infos) != 1 || infos[0].Enabled {
		t.Fatalf("列表应显示已停用: %+v", infos)
	}

	if err := svc.SetPackEnabled("extcmd", true); err != nil {
		t.Fatal(err)
	}
	if rt.Pack() == nil || rt.Pack().Meta.ID != "extcmd" {
		t.Fatal("重新启用应恢复激活")
	}
}

func TestSetPackEnabledMissing(t *testing.T) {
	tempHome(t)
	svc := NewExtServiceForTest(NewRuntime())
	if err := svc.SetPackEnabled("nope", true); err == nil {
		t.Fatal("不存在的包应拒绝")
	}
}

func TestImportPackOverwriteResetsDisabled(t *testing.T) {
	tempHome(t)
	rt := NewRuntime()
	svc := NewExtServiceForTest(rt)
	if _, err := svc.ImportPackJSON(extcmdJSON); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPackEnabled("extcmd", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportPackJSON(extcmdJSON); err != nil {
		t.Fatal(err)
	}
	infos, _ := svc.ListPacks()
	if len(infos) != 1 || !infos[0].Enabled {
		t.Fatalf("覆盖导入应重置为启用: %+v", infos)
	}
}

// TestFindPackFileByID 热放置场景:文件名与 meta.id 不一致时,启停/删除/预览
// 仍须按内容定位(修复"列表可见却无法操作"的不一致)。
func TestFindPackFileByID(t *testing.T) {
	dir := t.TempDir()
	// 复制测试夹具为任意文件名(≠ id.json)
	data, err := os.ReadFile("testdata/extpack.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "my-golden-pack.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := findPackFileByID(dir, "p2golden")
	if err != nil {
		t.Fatalf("按内容应找到热放置包: %v", err)
	}
	if filepath.Base(got) != "my-golden-pack.json" {
		t.Fatalf("定位到 %s,want my-golden-pack.json", got)
	}
	if _, err := findPackFileByID(dir, "no-such-pack"); err == nil {
		t.Fatal("不存在的 id 应报错")
	}
	if _, err := findPackFileByID(dir, "../escape"); err == nil {
		t.Fatal("路径穿越 id 应被拒绝")
	}
}
