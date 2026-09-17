//go:build linux

package updater

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ErrProgramDirNotWritable Linux 平台预检:程序所在目录不可写(不尝试提权;§5.4 平台替换表)。
var ErrProgramDirNotWritable = errors.New("程序所在目录不可写,无法自动更新;请检查目录权限后重试")

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

// PreflightTarget 目标存在性检查 + 所在目录可写探针;不可写 → 精确指引文案(不尝试提权)。
func PreflightTarget(target string) error {
	if _, err := os.Stat(target); err != nil {
		return ErrTargetNotFound
	}
	if err := writableProbe(filepath.Dir(target)); err != nil {
		return ErrProgramDirNotWritable
	}
	return nil
}

// writableProbe 目录可写探针(创建即删;不为此新增共享文件,darwin/windows 同款自实现)。
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

// stagedPath 同卷暂存路径:<目标同目录>/<目标基名>.new-<pid>(D16:同卷才可原子 rename)。
func stagedPath(target string) string {
	return filepath.Join(filepath.Dir(target), filepath.Base(target)+".new-"+strconv.Itoa(os.Getpid()))
}

// stageLinuxExe 复制产物到同卷暂存并显式 Chmod 0755(复制不保留权限位、且受 umask 影响);
// cleanup 回收暂存文件;失败一律 ErrStageFailed(细节由 helper 日志承载)。
func stageLinuxExe(artifact, target string) (string, func(), error) {
	staged := stagedPath(target)
	if err := copyFile(artifact, staged, 0o755); err != nil {
		return "", nil, ErrStageFailed
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		_ = os.Remove(staged)
		return "", nil, ErrStageFailed
	}
	return staged, func() { _ = os.Remove(staged) }, nil
}

// swapLinuxExe 两步 rename(unix 语义允许原子覆盖运行中二进制,无退避):
// target → <target>.old(AC-8:保留至新实例启动成功,由下次启动 CleanupStaleBackups 清理)→ staged → target;
// 第二步失败返回已产生的 backup(""=未产生),不自行回滚(§5.4 回滚唯一路径)。
func swapLinuxExe(staged, target string) (string, error) {
	backup := target + ".old"
	if err := RenameWithRetry(target, backup, nil); err != nil {
		return "", err
	}
	if err := RenameWithRetry(staged, target, nil); err != nil {
		return backup, err
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return backup, ErrSwapFailed
	}
	return backup, nil
}

// rollbackLinux 备份回位 + 显式 0755;backup == "" → nil(幂等)。
func rollbackLinux(backup, target string) error {
	if backup == "" {
		return nil
	}
	if err := os.Rename(backup, target); err != nil {
		return ErrRollbackFailed
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return ErrRollbackFailed
	}
	return nil
}

// DefaultDeps Linux 生产依赖:等待父退出 / 预检 / 同卷复制暂存 / rename 覆盖交换 / rename 回滚 / detached 拉起(§5.4)。
func DefaultDeps() HelperDeps {
	return HelperDeps{
		WaitParent: func(pid int, poll, timeout time.Duration) error {
			return WaitParentExit(pid, poll, timeout, processAlive)
		},
		Preflight: PreflightTarget,
		Stage:     stageLinuxExe,
		Swap:      swapLinuxExe,
		Rollback:  rollbackLinux,
		Relaunch: func(target string) error {
			_, err := spawnDetached(target, nil)
			return err
		},
	}
}

// CleanupStaleBackups 启动清理(尽力而为):<target>.old 与 <base>.new-*(仅程序主体,不触碰用户数据)。
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
	return nil
}

// copyFile 通用文件复制(io.Copy + 显式权限);失败清理半成品。
// 与 apply_windows.go 同名同签名:平台各持一份小实现,不为此新增共享文件(计划 Task 2 决策)。
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
