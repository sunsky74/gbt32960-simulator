package ext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileOK(t *testing.T) {
	p, err := LoadFile(filepath.Join("testdata", "demo.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Meta.ID != "demo" {
		t.Fatalf("id = %q", p.Meta.ID)
	}
}

func TestLoadFileFailures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("语法错误", func(t *testing.T) {
		_, err := LoadFile(write("bad.json", "{not json"))
		if err == nil || !strings.Contains(err.Error(), "JSON 语法错误") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("语义错误带路径", func(t *testing.T) {
		src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
		broken := strings.Replace(string(src), `"unitCode": 128`, `"unitCode": 9`, 1)
		_, err := LoadFile(write("sem.json", broken))
		if err == nil || !strings.Contains(err.Error(), "realtime.appendUnits[0].unitCode") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("干跑失败", func(t *testing.T) {
		src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
		broken := strings.Replace(string(src), `"type": "u8"`, `"type": "u999"`, 1)
		_, err := LoadFile(write("dry.json", broken))
		if err == nil {
			t.Fatal("未知类型应在静态校验拦截;若未来放宽静态校验,干跑必须兜底")
		}
	})
	t.Run("未知字段被拒绝", func(t *testing.T) {
		src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
		// 顶层未知字段(声明式校验器必须拒绝,防止字段名拼写错误被静默忽略)
		top := strings.Replace(string(src), `"meta":`, `"unknownTop": 1, "meta":`, 1)
		if _, err := LoadFile(write("unknown-top.json", top)); err == nil || !strings.Contains(err.Error(), "JSON 语法错误") {
			t.Fatalf("顶层未知字段应被拒绝, err = %v", err)
		}
		// 嵌套未知字段(meta 内,如 scopes 误拼)
		nested := strings.Replace(string(src), `"baseVersion": "2016"`, `"baseVersion": "2016", "unknownKey": true`, 1)
		if _, err := LoadFile(write("unknown-nested.json", nested)); err == nil || !strings.Contains(err.Error(), "JSON 语法错误") {
			t.Fatalf("嵌套未知字段应被拒绝, err = %v", err)
		}
	})
	t.Run("顶层多余内容被拒绝", func(t *testing.T) {
		src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
		if _, err := LoadFile(write("trailing.json", string(src)+`{"extra":1}`)); err == nil || !strings.Contains(err.Error(), "JSON 语法错误") {
			t.Fatalf("顶层多余内容应被拒绝, err = %v", err)
		}
	})
	t.Run("文件不存在", func(t *testing.T) {
		if _, err := LoadFile(filepath.Join(dir, "nope.json")); err == nil {
			t.Fatal("应报错")
		}
	})
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
	_ = os.WriteFile(filepath.Join(dir, "good.json"), src, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("skip"), 0o644)

	packs, err := LoadDir(dir)
	if len(packs) != 1 || packs[0].Meta.ID != "demo" {
		t.Fatalf("packs = %d, want 1", len(packs))
	}
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("err = %v, want 提及 broken.json", err)
	}

	if packs, err := LoadDir(filepath.Join(dir, "不存在")); packs != nil || err != nil {
		t.Fatalf("目录不存在应返回 (nil, nil), 实际 (%v, %v)", packs, err)
	}
}
