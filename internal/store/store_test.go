package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// tempHome 把配置目录隔离到临时目录:os.UserConfigDir 在 macOS 读 $HOME,
// 在 Linux 读 $XDG_CONFIG_HOME(未设时 $HOME/.config),两者都设置以保证可移植。
func tempHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

type sampleConfig struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Inner struct {
		Flag bool     `json:"flag"`
		Note string   `json:"note,omitempty"`
		Vals []string `json:"vals,omitempty"`
	} `json:"inner"`
}

func sample() sampleConfig {
	var v sampleConfig
	v.Name = "示例"
	v.Count = 7
	v.Inner.Flag = true
	v.Inner.Note = "嵌套"
	v.Inner.Vals = []string{"a", "b"}
	return v
}

func TestSaveLoadRoundtrip(t *testing.T) {
	tempHome(t)
	in := sample()
	if err := Save("rt.json", &in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var out sampleConfig
	if err := Load("rt.json", &out); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("roundtrip 不一致:\n in = %+v\nout = %+v", in, out)
	}
}

func TestLoadMissingFile(t *testing.T) {
	tempHome(t)
	var v sampleConfig
	err := Load("does-not-exist.json", &v)
	if err != nil {
		t.Fatalf("缺失文件应返回 nil 错误, got %v", err)
	}
	if !reflect.DeepEqual(v, sampleConfig{}) {
		t.Fatalf("缺失文件时 v 应保持零值, got %+v", v)
	}
}

func TestLoadCorruptedJSON(t *testing.T) {
	tempHome(t)
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"name": "未闭合`), 0o644); err != nil {
		t.Fatal(err)
	}
	var v sampleConfig
	if err := Load("bad.json", &v); err == nil {
		t.Fatal("损坏 JSON 应返回非 nil 错误")
	}
}

func TestSaveOverwriteWins(t *testing.T) {
	tempHome(t)
	first := sample()
	first.Name = "第一版"
	if err := Save("ow.json", &first); err != nil {
		t.Fatal(err)
	}
	second := sample()
	second.Name = "第二版"
	second.Count = 42
	if err := Save("ow.json", &second); err != nil {
		t.Fatal(err)
	}
	var out sampleConfig
	if err := Load("ow.json", &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second, out) {
		t.Fatalf("第二次保存应获胜:\nwant %+v\ngot  %+v", second, out)
	}
}

// TestSaveLeavesNoTempFile 原子写必须清理临时文件:保存完成后配置目录
// 不应残留任何 *.tmp* 文件(CreateTemp 的中间态)。
func TestSaveLeavesNoTempFile(t *testing.T) {
	tempHome(t)
	in := sample()
	if err := Save("tmp.json", &in); err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("保存后残留临时文件: %s (目录 %v)", e.Name(), entries)
		}
	}
}
