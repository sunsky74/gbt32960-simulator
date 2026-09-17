//go:build darwin

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// makeBundle 构造最小可校验 .app:Contents/Info.plist + 可执行常规文件 Contents/MacOS/x。
func makeBundle(t *testing.T, app, content string) {
	t.Helper()
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(macos, "x"), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// zipBundle 用系统 ditto 真实打包(与 release.yml 的产物打包方式一致,不 mock)。
func zipBundle(t *testing.T, app, zipPath string) {
	t.Helper()
	cmd := exec.Command("ditto", "-c", "-k", "--keepParent", app, zipPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ditto 打包失败: %v(%s)", err, out)
	}
}

// writeMarker 在 dir 下写 payload 文件(目录不存在则创建),用于以内容区分新旧 bundle。
func writeMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readMarker 读取 writeMarker 写入的内容。
func readMarker(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "payload"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestDarwinRunningTargetBundleRoot bundleRootFrom:向上取最外层 *.app;非 bundle → ErrTargetNotFound(精确文案)。
func TestDarwinRunningTargetBundleRoot(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"可执行文件位于 bundle 内", "/Applications/A.app/Contents/MacOS/A", "/Applications/A.app", false},
		{"bundle 根自身", "/Applications/A.app", "/Applications/A.app", false},
		{"嵌套 bundle 取最外层", "/Applications/Outer.app/Contents/Helpers/Inner.app/Contents/MacOS/x", "/Applications/Outer.app", false},
		{"裸二进制", "/tmp/plain-bin", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := bundleRootFrom(c.in)
			if c.wantErr {
				if !errors.Is(err, ErrTargetNotFound) {
					t.Fatalf("err = %v, want ErrTargetNotFound", err)
				}
				if err.Error() != "无法定位应用位置,请手动更新" {
					t.Fatalf("文案 = %q, want 精确 ErrTargetNotFound 文案", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("bundleRootFrom(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestDarwinPreflight translocation 拒绝、正常目录放行、父目录不可写精确指引、目标缺失。
func TestDarwinPreflight(t *testing.T) {
	t.Run("App Translocation 拒绝", func(t *testing.T) {
		err := PreflightTarget("/AppTranslocation/xxx/A.app")
		if !errors.Is(err, ErrTranslocated) {
			t.Fatalf("err = %v, want ErrTranslocated", err)
		}
		if got, want := err.Error(), "应用正从临时位置运行,无法自动更新;请将应用移到 /Applications 后重试"; got != want {
			t.Fatalf("文案 = %q, want %q", got, want)
		}
	})

	t.Run("正常目录通过", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "A.app")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := PreflightTarget(target); err != nil {
			t.Fatalf("PreflightTarget(%q) = %v, want nil", target, err)
		}
	})

	t.Run("父目录不可写", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "Applications")
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(parent, "A.app")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(parent, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o755) }) // 先恢复权限,再让 t.TempDir 清理

		err := PreflightTarget(target)
		if !errors.Is(err, ErrTargetNotWritable) {
			t.Fatalf("err = %v, want ErrTargetNotWritable", err)
		}
		if got, want := err.Error(), "应用所在目录不可写,无法自动更新;请将应用移到 /Applications 后重试"; got != want {
			t.Fatalf("文案 = %q, want %q", got, want)
		}
	})

	t.Run("目标不存在", func(t *testing.T) {
		err := PreflightTarget(filepath.Join(t.TempDir(), "No.app"))
		if !errors.Is(err, ErrTargetNotFound) {
			t.Fatalf("err = %v, want ErrTargetNotFound", err)
		}
	})
}

// TestDarwinStageExtracts 真实 ditto 打包 → Stage 解压到同卷暂存目录,可执行位保留,cleanup 后消失。
func TestDarwinStageExtracts(t *testing.T) {
	src := t.TempDir()
	app := filepath.Join(src, "X.app")
	makeBundle(t, app, "#!/bin/sh\necho hello\n")
	zipPath := filepath.Join(t.TempDir(), "X.app.zip")
	zipBundle(t, app, zipPath)

	deploy := t.TempDir()
	target := filepath.Join(deploy, "X.app")

	staged, cleanup, err := stageDarwinBundle(zipPath, target)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup == nil {
		t.Fatal("cleanup 不应为 nil")
	}
	defer cleanup()

	wantStaged := filepath.Join(deploy, fmt.Sprintf(".gbt32960-update-%d.staging", os.Getpid()), "X.app")
	if staged != wantStaged {
		t.Fatalf("staged = %q, want %q", staged, wantStaged)
	}
	fi, err := os.Stat(filepath.Join(staged, "Contents", "MacOS", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("解压后可执行位丢失: mode = %v", fi.Mode())
	}

	cleanup()
	if _, err := os.Stat(filepath.Dir(staged)); !os.IsNotExist(err) {
		t.Fatalf("cleanup 后暂存目录仍存在: err = %v", err)
	}
}

// TestDarwinStageRejections 非 zip 与缺 Contents/MacOS 的合法 zip 一律 ErrStageFailed,且清理暂存目录。
func TestDarwinStageRejections(t *testing.T) {
	t.Run("文本文件冒充 zip", func(t *testing.T) {
		notZip := filepath.Join(t.TempDir(), "bad.zip")
		if err := os.WriteFile(notZip, []byte("这不是一个 zip 文件"), 0o644); err != nil {
			t.Fatal(err)
		}
		deploy := t.TempDir()
		target := filepath.Join(deploy, "X.app")

		staged, cleanup, err := stageDarwinBundle(notZip, target)
		if !errors.Is(err, ErrStageFailed) {
			t.Fatalf("err = %v, want ErrStageFailed", err)
		}
		if staged != "" || cleanup != nil {
			t.Fatalf("失败应返回 (\"\", nil), got (%q, %v)", staged, cleanup != nil)
		}
		assertNoStaging(t, deploy)
	})

	t.Run("合法 zip 但缺 Contents/MacOS", func(t *testing.T) {
		src := t.TempDir()
		app := filepath.Join(src, "X.app")
		if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
			t.Fatal(err)
		}
		zipPath := filepath.Join(t.TempDir(), "X.app.zip")
		zipBundle(t, app, zipPath)

		deploy := t.TempDir()
		target := filepath.Join(deploy, "X.app")
		if _, _, err := stageDarwinBundle(zipPath, target); !errors.Is(err, ErrStageFailed) {
			t.Fatalf("err = %v, want ErrStageFailed", err)
		}
		assertNoStaging(t, deploy)
	})
}

// assertNoStaging 断言目录下无 .gbt32960-update-*.staging 残留。
func assertNoStaging(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".gbt32960-update-*.staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("暂存目录未清理: %v", matches)
	}
}

// TestDarwinSwapAndRollback 交换产生 .bak-<ts> 一代备份;回滚还原;Rollback("") 幂等;staged 缺失备份语义正确。
func TestDarwinSwapAndRollback(t *testing.T) {
	t.Run("正常交换与回滚", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "A.app")
		staged := filepath.Join(dir, ".gbt32960-update-1.staging", "A.app")
		writeMarker(t, target, "old")
		writeMarker(t, staged, "new")

		backup, err := swapDarwinBundle(staged, target)
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^A\.app\.bak-\d{8}-\d{6}$`).MatchString(filepath.Base(backup)) {
			t.Fatalf("备份名 = %q, want 形如 A.app.bak-YYYYMMDD-HHMMSS", backup)
		}
		if got := readMarker(t, target); got != "new" {
			t.Fatalf("交换后 target 内容 = %q, want new", got)
		}
		if got := readMarker(t, backup); got != "old" {
			t.Fatalf("备份内容 = %q, want old", got)
		}

		if err := rollbackRename(backup, target); err != nil {
			t.Fatal(err)
		}
		if got := readMarker(t, target); got != "old" {
			t.Fatalf("回滚后 target 内容 = %q, want old", got)
		}
		if _, err := os.Stat(backup); !os.IsNotExist(err) {
			t.Fatalf("回滚后备份应消失: err = %v", err)
		}

		if err := rollbackRename("", target); err != nil {
			t.Fatalf("Rollback(\"\") = %v, want nil(幂等)", err)
		}
	})

	t.Run("staged 缺失", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "A.app")
		writeMarker(t, target, "old")

		backup, err := swapDarwinBundle(filepath.Join(dir, "缺失.app"), target)
		if !errors.Is(err, ErrSwapFailed) {
			t.Fatalf("err = %v, want ErrSwapFailed", err)
		}
		if backup == "" {
			t.Fatal("第一步 rename 已产生备份,应返回非空 backup")
		}
		if got := readMarker(t, backup); got != "old" {
			t.Fatalf("备份内容 = %q, want old", got)
		}
	})
}

// TestDarwinCleanupStaleBackups 仅删 *.bak-* 与 .gbt32960-update-*.staging,无关文件保留。
func TestDarwinCleanupStaleBackups(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, filepath.Join(dir, "A.app.bak-20260101-000000"), "old")
	writeMarker(t, filepath.Join(dir, ".gbt32960-update-999.staging", "X.app"), "staged")
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanupStaleBackupsIn(dir); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"A.app.bak-20260101-000000", ".gbt32960-update-999.staging"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s 应被清理: err = %v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("无关文件被误删: %v", err)
	}
}
