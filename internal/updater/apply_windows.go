//go:build windows

package updater

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ErrInstallDirNotWritable Windows 平台预检:安装目录不可写(不尝试提权,ADR-0002)。
var ErrInstallDirNotWritable = errors.New("安装目录不可写,无法自动更新;请下载安装包手动更新")

// renameRetries Windows 杀软/索引器临时锁场景的退避阶梯:1 次首试 + 3 次重试(§5.4)。
var renameRetries = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second}

// RunningTarget 当前运行的可执行文件路径(os.Executable → EvalSymlinks)。
func RunningTarget() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", ErrTargetNotFound
	}
	target, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", ErrTargetNotFound
	}
	return target, nil
}

// PreflightTarget 目标存在性检查 + 所在目录可写探针;不可写 → 精确指引文案。
func PreflightTarget(target string) error {
	if _, err := os.Stat(target); err != nil {
		return ErrTargetNotFound
	}
	if err := writableProbe(filepath.Dir(target)); err != nil {
		return ErrInstallDirNotWritable
	}
	return nil
}

// writableProbe 目录可写探针(创建即删;不为此新增共享文件,darwin/linux 同款自实现)。
func writableProbe(dir string) error {
	f, err := os.CreateTemp(dir, ".gbt32960-writecheck-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// DefaultDeps Windows 生产依赖:同卷复制暂存 → rename-aside(退避重试)→ rename 回滚 → 独立进程拉起。
func DefaultDeps() HelperDeps {
	return HelperDeps{
		WaitParent: func(pid int, poll, timeout time.Duration) error {
			return WaitParentExit(pid, poll, timeout, processAlive)
		},
		Preflight: PreflightTarget,
		Stage:     stageWindowsExe,
		Swap:      swapWindowsExe,
		Rollback:  rollbackWindows,
		Relaunch: func(target string) error {
			_, err := spawnDetached(target, nil)
			return err
		},
	}
}

// stagedPath 同卷暂存路径:<目标同目录>/<目标基名>.new-<pid>。
func stagedPath(target string) string {
	return filepath.Join(filepath.Dir(target), filepath.Base(target)+".new-"+strconv.Itoa(os.Getpid()))
}

// stageWindowsExe 复制产物到同卷暂存;cleanup 回收暂存文件;失败一律 ErrStageFailed(细节由 helper 日志承载)。
func stageWindowsExe(artifact, target string) (string, func(), error) {
	staged := stagedPath(target)
	if err := copyFile(artifact, staged, 0o755); err != nil {
		return "", nil, ErrStageFailed
	}
	return staged, func() { _ = os.Remove(staged) }, nil
}

// swapWindowsExe rename-aside:目标 → .old(先清陈旧残留,保证 rename 目标不存在)→ 暂存 → 目标;
// 各步带退避重试;第二步失败返回已产生的 backup(""=未产生),不自行回滚(§5.4 回滚唯一路径)。
func swapWindowsExe(staged, target string) (string, error) {
	backup := target + ".old"
	_ = os.Remove(backup)
	if err := RenameWithRetry(target, backup, renameRetries); err != nil {
		return "", err
	}
	if err := RenameWithRetry(staged, target, renameRetries); err != nil {
		return backup, err
	}
	return backup, nil
}

// rollbackWindows 备份回位(同样退避重试);backup 为空或已不存在 → nil(幂等)。
func rollbackWindows(backup, target string) error {
	if backup == "" {
		return nil
	}
	if _, err := os.Stat(backup); err != nil {
		return nil
	}
	return RenameWithRetry(backup, target, renameRetries)
}

// CleanupStaleBackups 启动清理(尽力而为):<target>.old、<base>.new-*、
// %TEMP%/gbt32960-helper-*.exe;无法定位自身时才报错。
func CleanupStaleBackups() error {
	target, err := RunningTarget()
	if err != nil {
		return err
	}
	dir, base := filepath.Dir(target), filepath.Base(target)
	_ = os.Remove(target + ".old")
	staged, _ := filepath.Glob(filepath.Join(dir, base+".new-*"))
	for _, p := range staged {
		_ = os.Remove(p)
	}
	helpers, _ := filepath.Glob(filepath.Join(os.TempDir(), "gbt32960-helper-*.exe"))
	for _, p := range helpers {
		_ = os.Remove(p)
	}
	return nil
}

// helperCopyPath helper 自复制路径:%TEMP%/gbt32960-helper-<pid>.exe(纯函数)。
func helperCopyPath(pid int) string {
	return filepath.Join(os.TempDir(), "gbt32960-helper-"+strconv.Itoa(pid)+".exe")
}

// copySelf 复制当前可执行文件到 dst(mode 在 Windows 下由文件系统语义决定)。
func copySelf(dst string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return copyFile(exe, dst, 0o755)
}

// copyFile 通用文件复制(io.Copy + 显式权限);失败清理半成品。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
