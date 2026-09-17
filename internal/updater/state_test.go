package updater

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustPath 便捷解包无参路径构造函数(测试内失败即 t.Fatal)。
func mustPath(t *testing.T, f func() (string, error)) string {
	t.Helper()
	p, err := f()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// readRaw 读取原始字节(结果文件 schema 逐字断言用)。
func readRaw(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestUpdatesRootPaths 路径契约:UpdatesRoot() 下 LastResultPath()==<root>/last-result.json、HelperLogPath()==<root>/helper.log。
func TestUpdatesRootPaths(t *testing.T) {
	redirectUserCache(t)
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	root := mustPath(t, UpdatesRoot)
	if want := filepath.Join(cache, "gbt32960-simulator", "updates"); root != want {
		t.Fatalf("UpdatesRoot() = %q, want %q", root, want)
	}
	if got, want := mustPath(t, LastResultPath), filepath.Join(root, "last-result.json"); got != want {
		t.Fatalf("LastResultPath() = %q, want %q", got, want)
	}
	if got, want := mustPath(t, HelperLogPath), filepath.Join(root, "helper.log"); got != want {
		t.Fatalf("HelperLogPath() = %q, want %q", got, want)
	}
}

// TestLastResultCodec 结果文件编解码:往返、MkdirAll、缺文件、非法 JSON、schema 键名逐字一致。
func TestLastResultCodec(t *testing.T) {
	redirectUserCache(t)
	path := mustPath(t, LastResultPath)

	t.Run("成功记录往返且空 reason 省略", func(t *testing.T) {
		want := LastResult{OK: true, TargetVersion: "v0.1.1", LogPath: "/tmp/updates/helper.log"}
		if err := WriteLastResult(want); err != nil {
			t.Fatal(err)
		}
		raw := readRaw(t, path)
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatalf("结果文件非法 JSON: %v\n%s", err, raw)
		}
		if len(keys) != 3 {
			t.Fatalf("JSON 键 = %v, want 恰 ok/targetVersion/logPath", keys)
		}
		for _, k := range []string{"ok", "targetVersion", "logPath"} {
			if _, ok := keys[k]; !ok {
				t.Fatalf("缺字段 %q: %s", k, raw)
			}
		}
		if _, ok := keys["reason"]; ok {
			t.Fatalf("空 reason 应被 omitempty 省略: %s", raw)
		}
		var okVal bool
		if err := json.Unmarshal(keys["ok"], &okVal); err != nil || !okVal {
			t.Fatalf("ok 字段解析异常: %s", keys["ok"])
		}
		got, err := ReadLastResult()
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || *got != want {
			t.Fatalf("往返不一致: got = %+v, want = %+v", got, want)
		}
	})

	t.Run("失败记录 reason 透出且键名逐字一致", func(t *testing.T) {
		want := LastResult{OK: false, TargetVersion: "v0.1.0", Reason: "替换失败", LogPath: "/tmp/updates/helper.log"}
		if err := WriteLastResult(want); err != nil {
			t.Fatal(err)
		}
		raw := readRaw(t, path)
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatalf("结果文件非法 JSON: %v\n%s", err, raw)
		}
		if len(keys) != 4 {
			t.Fatalf("JSON 键 = %v, want 恰 ok/targetVersion/reason/logPath", keys)
		}
		for _, k := range []string{"ok", "targetVersion", "reason", "logPath"} {
			if _, ok := keys[k]; !ok {
				t.Fatalf("缺字段 %q: %s", k, raw)
			}
		}
		var reason string
		if err := json.Unmarshal(keys["reason"], &reason); err != nil || reason != "替换失败" {
			t.Fatalf("reason = %q (err=%v), want 替换失败", reason, err)
		}
		got, err := ReadLastResult()
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || *got != want {
			t.Fatalf("往返不一致: got = %+v, want = %+v", got, want)
		}
	})

	t.Run("根目录缺失时自动创建", func(t *testing.T) {
		if err := ClearLastResult(); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(mustPath(t, UpdatesRoot)); err != nil {
			t.Fatal(err)
		}
		if err := WriteLastResult(LastResult{OK: true, TargetVersion: "v0.1.2", LogPath: "l"}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("结果文件未创建: %v", err)
		}
	})

	t.Run("缺文件返回 nil,nil", func(t *testing.T) {
		if err := ClearLastResult(); err != nil {
			t.Fatal(err)
		}
		got, err := ReadLastResult()
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("ReadLastResult() = %+v, want nil", got)
		}
	})

	t.Run("非法 JSON 报错", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"ok":`), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadLastResult()
		if err == nil {
			t.Fatal("非法 JSON 应返回 error")
		}
		if got != nil {
			t.Fatalf("ReadLastResult() = %+v, want nil", got)
		}
		if err := ClearLastResult(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestLastResultClearIdempotent 读取即清除语义:写→Clear→再 Clear 均无 error;Clear 后 Read → (nil,nil);无 .tmp 残留。
func TestLastResultClearIdempotent(t *testing.T) {
	redirectUserCache(t)
	root := mustPath(t, UpdatesRoot)
	if err := WriteLastResult(LastResult{OK: false, TargetVersion: "v0.1.0", Reason: "替换失败", LogPath: "l"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearLastResult(); err != nil {
		t.Fatal(err)
	}
	if err := ClearLastResult(); err != nil {
		t.Fatalf("二次 Clear 应幂等: %v", err)
	}
	got, err := ReadLastResult()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("Clear 后 Read = %+v, want nil", got)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}

// TestOpenHelperLogTruncates 日志文件契约:>1MiB 打开即截断;追加写可读回;未超限保留;目录缺失自动创建。
func TestOpenHelperLogTruncates(t *testing.T) {
	redirectUserCache(t)
	path := mustPath(t, HelperLogPath)

	t.Run("超过 1MiB 先截断", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), (1<<20)+1), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := OpenHelperLog()
		if err != nil {
			t.Fatal(err)
		}
		fi, err := f.Stat()
		if err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if fi.Size() != 0 {
			_ = f.Close()
			t.Fatalf("截断后大小 = %d, want 0", fi.Size())
		}
		if _, err := f.WriteString("启动\n"); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if got := string(readRaw(t, path)); got != "启动\n" {
			t.Fatalf("追加写读回 = %q, want %q", got, "启动\n")
		}
	})

	t.Run("未超限时保留旧内容并追加", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("旧行\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := OpenHelperLog()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("新行\n"); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if got := string(readRaw(t, path)); got != "旧行\n新行\n" {
			t.Fatalf("追加结果 = %q, want %q", got, "旧行\n新行\n")
		}
	})

	t.Run("根目录缺失时自动创建", func(t *testing.T) {
		if err := os.RemoveAll(mustPath(t, UpdatesRoot)); err != nil {
			t.Fatal(err)
		}
		f, err := OpenHelperLog()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("日志文件未创建: %v", err)
		}
	})
}

// TestAsOutcome 绑定面投影:nil → {Present:false};记录 → 字段透出。
func TestAsOutcome(t *testing.T) {
	if got := AsOutcome(nil); got.Present || got.OK || got.TargetVersion != "" || got.Reason != "" || got.LogPath != "" {
		t.Fatalf("AsOutcome(nil) = %+v, want 零值 Present=false", got)
	}
	fail := LastResult{OK: false, TargetVersion: "v0.1.0", Reason: "替换失败", LogPath: "/tmp/updates/helper.log"}
	got := AsOutcome(&fail)
	if !got.Present || got.OK || got.TargetVersion != fail.TargetVersion || got.Reason != fail.Reason || got.LogPath != fail.LogPath {
		t.Fatalf("AsOutcome(失败记录) = %+v, want 全字段透出且 Present=true", got)
	}
	ok := LastResult{OK: true, TargetVersion: "v0.1.1", LogPath: "l"}
	if got := AsOutcome(&ok); !got.Present || !got.OK || got.TargetVersion != ok.TargetVersion {
		t.Fatalf("AsOutcome(成功记录) = %+v, want Present=true/OK=true", got)
	}
}
