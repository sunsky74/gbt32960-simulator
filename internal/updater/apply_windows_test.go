//go:build windows

package updater

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// writeFixture 造测试文件;失败即终止用例。
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

// TestWindowsStageCopy Stage 复制产物到 <目录>/<base>.new-<pid> 且字节一致;
// cleanup 可回收暂存;产物缺失 → ErrStageFailed。
func TestWindowsStageCopy(t *testing.T) {
	deps := DefaultDeps()
	if deps.WaitParent == nil || deps.Preflight == nil || deps.Stage == nil ||
		deps.Swap == nil || deps.Rollback == nil || deps.Relaunch == nil {
		t.Fatal("DefaultDeps 存在未接线字段")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "app.exe")
	writeFixture(t, target, []byte("旧版本"))
	artifact := filepath.Join(dir, "gbt32960-simulator.exe")
	content := []byte("新版本字节 new-bytes")
	writeFixture(t, artifact, content)

	staged, cleanup, err := deps.Stage(artifact, target)
	if err != nil {
		t.Fatalf("Stage 失败: %v", err)
	}
	if cleanup == nil {
		t.Fatal("Stage cleanup 不应为 nil")
	}
	want := filepath.Join(dir, "app.exe.new-"+strconv.Itoa(os.Getpid()))
	if staged != want {
		t.Fatalf("staged = %q,want %q", staged, want)
	}
	if got := readFixture(t, staged); !bytes.Equal(got, content) {
		t.Fatalf("staged 内容不一致: %q", got)
	}
	cleanup()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("cleanup 后 staged 仍存在: %v", err)
	}

	if _, _, err := deps.Stage(filepath.Join(dir, "missing.exe"), target); !errors.Is(err, ErrStageFailed) {
		t.Fatalf("产物缺失 err = %v,want ErrStageFailed", err)
	}
}

// TestWindowsSwapAndRollback Swap:目标 → .old(先清陈旧 .old)→ 暂存 → 目标,失败返回已产生 backup;
// Rollback 回位且 backup == "" 幂等;staged 缺失 → ErrSwapFailed 且 backup 语义正确。
func TestWindowsSwapAndRollback(t *testing.T) {
	deps := DefaultDeps()

	dir := t.TempDir()
	target := filepath.Join(dir, "app.exe")
	orig := []byte("原始版本")
	writeFixture(t, target, orig)
	staged := filepath.Join(dir, "app.exe.new-"+strconv.Itoa(os.Getpid()))
	newBytes := []byte("替换后的新版本")
	writeFixture(t, staged, newBytes)
	writeFixture(t, target+".old", []byte("陈旧残留")) // Swap 前必须被清理

	backup, err := deps.Swap(staged, target)
	if err != nil {
		t.Fatalf("Swap 失败: %v", err)
	}
	if backup != target+".old" {
		t.Fatalf("backup = %q,want %q", backup, target+".old")
	}
	if got := readFixture(t, backup); !bytes.Equal(got, orig) {
		t.Fatalf("backup 内容 = %q,want 原 target 内容", got)
	}
	if got := readFixture(t, target); !bytes.Equal(got, newBytes) {
		t.Fatalf("Swap 后 target 内容 = %q,want staged 内容", got)
	}

	if err := deps.Rollback(backup, target); err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	if got := readFixture(t, target); !bytes.Equal(got, orig) {
		t.Fatalf("Rollback 后 target 内容 = %q,want 原内容", got)
	}
	if err := deps.Rollback("", target); err != nil {
		t.Fatalf("Rollback(\"\") = %v,want nil", err)
	}

	// staged 缺失:第一步 rename 已产生 .old,第二步重试耗尽 → ErrSwapFailed
	dir2 := t.TempDir()
	target2 := filepath.Join(dir2, "app2.exe")
	orig2 := []byte("v1 原始")
	writeFixture(t, target2, orig2)
	backup2, err := deps.Swap(filepath.Join(dir2, "missing.exe.new-1"), target2)
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

// TestWindowsRenameRetryDefaults 锁定退避阶梯规格:200ms/500ms/1s(防实现漂移)。
func TestWindowsRenameRetryDefaults(t *testing.T) {
	want := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second}
	if len(renameRetries) != len(want) {
		t.Fatalf("len(renameRetries) = %d,want %d", len(renameRetries), len(want))
	}
	for i, d := range want {
		if renameRetries[i] != d {
			t.Fatalf("renameRetries[%d] = %v,want %v", i, renameRetries[i], d)
		}
	}
}

// TestWindowsProcessAlive 自进程存活;cmd /c exit 退出后轮询转 false(≤5s)。
func TestWindowsProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive(os.Getpid()) = false,want true")
	}

	cmd := exec.Command("cmd", "/c", "exit")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 cmd 失败: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(cmd.Process.Pid) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("pid %d 在 5s 内未转为退出", cmd.Process.Pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("cmd /c exit 等待失败: %v", err)
	}
}

// TestWindowsCleanupStaleBackups 仅清 <target>.old、<base>.new-* 与
// %TEMP%/gbt32960-helper-*.exe;无关文件保留(尽力而为)。
func TestWindowsCleanupStaleBackups(t *testing.T) {
	exe, err := RunningTarget()
	if err != nil {
		t.Fatalf("RunningTarget 失败: %v", err)
	}
	dir := filepath.Dir(exe)
	base := filepath.Base(exe)

	old := filepath.Join(dir, base+".old")
	staged := filepath.Join(dir, base+".new-999")
	keep := filepath.Join(dir, "keep.txt")
	tmpHelper := filepath.Join(os.TempDir(), "gbt32960-helper-123.exe")
	for _, p := range []string{old, staged, keep, tmpHelper} {
		writeFixture(t, p, []byte("x"))
	}
	defer os.Remove(keep)
	defer os.Remove(tmpHelper)

	if err := CleanupStaleBackups(); err != nil {
		t.Fatalf("CleanupStaleBackups 失败: %v", err)
	}
	for _, p := range []string{old, staged, tmpHelper} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s 应被清理,stat err = %v", p, err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("keep.txt 不应被清理: %v", err)
	}
}

// TestWindowsHelperCopyPath helper 副本路径纯函数契约。
func TestWindowsHelperCopyPath(t *testing.T) {
	want := filepath.Join(os.TempDir(), "gbt32960-helper-42.exe")
	if got := helperCopyPath(42); got != want {
		t.Fatalf("helperCopyPath(42) = %q,want %q", got, want)
	}
}

// TestWindowsSelfCopy 自复制字节与 os.Executable 一致且文件存在。
func TestWindowsSelfCopy(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable 失败: %v", err)
	}
	want := readFixture(t, exe)
	dst := filepath.Join(t.TempDir(), "x.exe")
	if err := copySelf(dst); err != nil {
		t.Fatalf("copySelf 失败: %v", err)
	}
	if got := readFixture(t, dst); !bytes.Equal(got, want) {
		t.Fatal("副本与自身字节不一致")
	}
}

// TestWindowsSpawnHelper SpawnHelper 会真实拉起 helper 进程,本用例不做真实拉起:
// 仅锁定其组合契约 copySelf(helperCopyPath(os.Getpid()))(含覆盖陈旧副本);
// 真实拉起由 T9 第 3 步真机走查覆盖(计划 Task 3 Step 1 显式声明该取舍)。
func TestWindowsSpawnHelper(t *testing.T) {
	dst := helperCopyPath(os.Getpid())
	writeFixture(t, dst, []byte("陈旧副本"))
	defer os.Remove(dst)

	if err := copySelf(dst); err != nil {
		t.Fatalf("copySelf(%s) 失败: %v", dst, err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable 失败: %v", err)
	}
	if got := readFixture(t, dst); !bytes.Equal(got, readFixture(t, exe)) {
		t.Fatal("helper 副本与自身字节不一致(陈旧副本未被覆盖?)")
	}
}
