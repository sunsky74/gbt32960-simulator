//go:build linux

package updater

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// writeFixture 造测试文件;失败即终止用例(与 apply_windows_test.go 同名同签名,两平台构建标签互斥)。
func writeFixture(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("造文件 %s 失败: %v", path, err)
	}
}

// readFixture 读测试文件;失败即终止用例。
func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件 %s 失败: %v", path, err)
	}
	return b
}

// TestLinuxStageCopyAndMode Stage 复制产物到 <目录>/<base>.new-<pid>,内容一致且显式 0755(umask 无关);
// cleanup 可回收暂存;产物缺失 → ErrStageFailed。
func TestLinuxStageCopyAndMode(t *testing.T) {
	deps := DefaultDeps()
	if deps.WaitParent == nil || deps.Preflight == nil || deps.Stage == nil ||
		deps.Swap == nil || deps.Rollback == nil || deps.Relaunch == nil {
		t.Fatal("DefaultDeps 存在未接线字段")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "gbt32960-simulator")
	writeFixture(t, target, []byte("旧版本"))
	artifact := filepath.Join(dir, "gbt32960-simulator-linux-amd64")
	content := []byte("新版本字节 new-bytes")
	writeFixture(t, artifact, content)

	// umask 077:若实现依赖 OpenFile 创建权限而非显式 Chmod,暂存文件将是 0700 而非 0755。
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	staged, cleanup, err := deps.Stage(artifact, target)
	if err != nil {
		t.Fatalf("Stage 失败: %v", err)
	}
	if cleanup == nil {
		t.Fatal("Stage cleanup 不应为 nil")
	}
	want := filepath.Join(dir, "gbt32960-simulator.new-"+strconv.Itoa(os.Getpid()))
	if staged != want {
		t.Fatalf("staged = %q,want %q", staged, want)
	}
	if got := readFixture(t, staged); !bytes.Equal(got, content) {
		t.Fatalf("staged 内容不一致: %q", got)
	}
	fi, err := os.Stat(staged)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("staged mode = %v,want 0755(umask 无关,须显式 Chmod)", fi.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("cleanup 后 staged 仍存在: %v", err)
	}

	if _, _, err := deps.Stage(filepath.Join(dir, "missing"), target); !errors.Is(err, ErrStageFailed) {
		t.Fatalf("产物缺失 err = %v,want ErrStageFailed", err)
	}
}

// TestLinuxSwapKeepsOld Swap:target → <target>.old(AC-8:保留至新实例启动成功,由下次启动清理)→ staged → target;
// .old 内容为原 target、target 内容为 staged、权限 0755(暂存 0644 也须被换后 Chmod 兜底)。
func TestLinuxSwapKeepsOld(t *testing.T) {
	deps := DefaultDeps()

	dir := t.TempDir()
	target := filepath.Join(dir, "gbt32960-simulator")
	orig := []byte("原始版本")
	writeFixture(t, target, orig) // 0644:非可执行旧权限
	staged := filepath.Join(dir, "gbt32960-simulator.new-"+strconv.Itoa(os.Getpid()))
	newBytes := []byte("替换后的新版本")
	writeFixture(t, staged, newBytes) // 0644:验证 Swap 后显式 Chmod

	backup, err := deps.Swap(staged, target)
	if err != nil {
		t.Fatalf("Swap 失败: %v", err)
	}
	if backup != target+".old" {
		t.Fatalf("backup = %q,want %q", backup, target+".old")
	}
	if got := readFixture(t, backup); !bytes.Equal(got, orig) {
		t.Fatalf(".old 内容 = %q,want 原 target 内容", got)
	}
	if got := readFixture(t, target); !bytes.Equal(got, newBytes) {
		t.Fatalf("Swap 后 target 内容 = %q,want staged 内容", got)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("Swap 后 target mode = %v,want 0755", fi.Mode().Perm())
	}
}

// TestLinuxSwapAndRollback staged 缺失 → ErrSwapFailed 且 backup 语义正确;
// Rollback 还原内容 + 0755;Rollback("") 幂等。
func TestLinuxSwapAndRollback(t *testing.T) {
	deps := DefaultDeps()

	dir := t.TempDir()
	target := filepath.Join(dir, "gbt32960-simulator")
	orig := []byte("原始版本")
	writeFixture(t, target, orig)
	staged := filepath.Join(dir, "gbt32960-simulator.new-"+strconv.Itoa(os.Getpid()))
	writeFixture(t, staged, []byte("新版本"))

	backup, err := deps.Swap(staged, target)
	if err != nil {
		t.Fatalf("Swap 失败: %v", err)
	}
	if err := deps.Rollback(backup, target); err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	if got := readFixture(t, target); !bytes.Equal(got, orig) {
		t.Fatalf("Rollback 后 target 内容 = %q,want 原内容", got)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("Rollback 后 target mode = %v,want 0755", fi.Mode().Perm())
	}
	if err := deps.Rollback("", target); err != nil {
		t.Fatalf("Rollback(\"\") = %v,want nil(幂等)", err)
	}

	// staged 缺失:第一步 rename 已产生 .old,第二步失败 → ErrSwapFailed 且 backup 语义正确。
	dir2 := t.TempDir()
	target2 := filepath.Join(dir2, "app2")
	orig2 := []byte("v1 原始")
	writeFixture(t, target2, orig2)
	backup2, err := deps.Swap(filepath.Join(dir2, "missing.new-1"), target2)
	if !errors.Is(err, ErrSwapFailed) {
		t.Fatalf("staged 缺失 err = %v,want ErrSwapFailed", err)
	}
	if backup2 != target2+".old" {
		t.Fatalf("staged 缺失 backup = %q,want %q", backup2, target2+".old")
	}
	if got := readFixture(t, backup2); !bytes.Equal(got, orig2) {
		t.Fatalf("staged 缺失 backup 内容 = %q,want 原 target 内容", got)
	}
	if _, err := os.Stat(target2); !os.IsNotExist(err) {
		t.Fatalf("staged 缺失时 target 不应存在: %v", err)
	}
}

// TestLinuxPreflightGuidance 正常目录通过;父目录 chmod 0555 → 精确文案;目标缺失 → ErrTargetNotFound。
func TestLinuxPreflightGuidance(t *testing.T) {
	t.Run("正常目录通过", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "gbt32960-simulator")
		writeFixture(t, target, []byte("x"))
		if err := PreflightTarget(target); err != nil {
			t.Fatalf("PreflightTarget(%q) = %v,want nil", target, err)
		}
	})

	t.Run("父目录不可写", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, "gbt32960-simulator")
		writeFixture(t, target, []byte("x"))
		if err := os.Chmod(parent, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o755) }) // 先恢复权限,再让 t.TempDir 清理

		err := PreflightTarget(target)
		if !errors.Is(err, ErrProgramDirNotWritable) {
			t.Fatalf("err = %v,want ErrProgramDirNotWritable", err)
		}
		if got, want := err.Error(), "程序所在目录不可写,无法自动更新;请检查目录权限后重试"; got != want {
			t.Fatalf("文案 = %q,want %q", got, want)
		}
	})

	t.Run("目标不存在", func(t *testing.T) {
		err := PreflightTarget(filepath.Join(t.TempDir(), "missing"))
		if !errors.Is(err, ErrTargetNotFound) {
			t.Fatalf("err = %v,want ErrTargetNotFound", err)
		}
	})
}

// TestLinuxCleanupStaleBackups 仅删 <target>.old 与 <base>.new-*,无关文件保留(尽力而为)。
func TestLinuxCleanupStaleBackups(t *testing.T) {
	target, err := RunningTarget()
	if err != nil {
		t.Fatalf("RunningTarget 失败: %v", err)
	}
	dir, base := filepath.Dir(target), filepath.Base(target)

	old := target + ".old"
	staged := filepath.Join(dir, base+".new-999")
	keep := filepath.Join(dir, "keep.txt")
	for _, p := range []string{old, staged, keep} {
		writeFixture(t, p, []byte("x"))
	}
	defer os.Remove(keep)

	if err := CleanupStaleBackups(); err != nil {
		t.Fatalf("CleanupStaleBackups 失败: %v", err)
	}
	for _, p := range []string{old, staged} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s 应被清理,stat err = %v", p, err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("keep.txt 不应被清理: %v", err)
	}
}
