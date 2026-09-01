package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
