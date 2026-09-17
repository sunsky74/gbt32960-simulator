//go:build darwin

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// macOS 平台专属预检错误值(文案即用户可见文案;设计文档 §5.4)。
var (
	// ErrTranslocated App Translocation(隔离执行)路径下拒绝替换,指引用户移入 /Applications。
	ErrTranslocated = errors.New("应用正从临时位置运行,无法自动更新;请将应用移到 /Applications 后重试")
	// ErrTargetNotWritable .app 父目录不可写(非提权场景),指引用户移入 /Applications。
	ErrTargetNotWritable = errors.New("应用所在目录不可写,无法自动更新;请将应用移到 /Applications 后重试")
)

// translocationMarker App Translocation 路径标记(Gatekeeper 随机挂载执行)。
const translocationMarker = "/AppTranslocation/"

// RunningTarget 定位当前运行实例的 .app 根:os.Executable → EvalSymlinks → bundleRootFrom。
func RunningTarget() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", ErrTargetNotFound
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", ErrTargetNotFound
	}
	return bundleRootFrom(resolved)
}

// bundleRootFrom 自 exePath 逐级向上取最外层 *.app 组件;路径中无 *.app → ErrTargetNotFound(纯函数)。
func bundleRootFrom(exePath string) (string, error) {
	if exePath == "" {
		return "", ErrTargetNotFound
	}
	root := ""
	p := filepath.Clean(exePath)
	for {
		if strings.HasSuffix(filepath.Base(p), ".app") {
			root = p
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	if root == "" {
		return "", ErrTargetNotFound
	}
	return root, nil
}

// PreflightTarget helper 前预检:translocation 拒绝 → 目标存在且为目录 → 父目录可写探针(§5.4)。
func PreflightTarget(target string) error {
	if strings.Contains(target, translocationMarker) {
		return ErrTranslocated
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		return ErrTargetNotFound
	}
	if err := writableProbe(filepath.Dir(target)); err != nil {
		return ErrTargetNotWritable
	}
	return nil
}

// writableProbe 目录可写探针:创建临时文件后立即关闭并删除(3 行实现,各平台文件内各自持有)。
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

// stagingDirFor 同卷暂存目录:位于 target 父目录内,保证交换 rename 为同卷原子操作(D16/§5.4)。
func stagingDirFor(target string) string {
	return filepath.Join(filepath.Dir(target), fmt.Sprintf(".gbt32960-update-%d.staging", os.Getpid()))
}

// stageDarwinBundle ditto 解压产物到同卷暂存目录 → .app 结构校验;
// 失败一律 ErrStageFailed,并清理暂存目录(日志由 helper 侧记录;§5.4)。
func stageDarwinBundle(artifact, target string) (string, func(), error) {
	stagingDir := stagingDirFor(target)
	_ = os.RemoveAll(stagingDir) // 先清同名残留,避免 ditto 与陈旧内容合并
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", nil, ErrStageFailed
	}
	cleanup := func() { _ = os.RemoveAll(stagingDir) }

	// ditto -x -k 保留符号链接/权限位/扩展属性(§5.4 平台替换表)。
	if err := exec.Command("ditto", "-x", "-k", artifact, stagingDir).Run(); err != nil {
		cleanup()
		return "", nil, ErrStageFailed
	}
	staged := filepath.Join(stagingDir, filepath.Base(target))
	if fi, err := os.Stat(staged); err != nil || !fi.IsDir() {
		cleanup()
		return "", nil, ErrStageFailed
	}
	if err := validateBundle(staged); err != nil {
		cleanup()
		return "", nil, ErrStageFailed
	}
	return staged, cleanup, nil
}

// validateBundle 校验 .app 结构:Contents/Info.plist 为常规文件 ∧ Contents/MacOS 至少 1 个可执行常规文件。
func validateBundle(app string) error {
	if fi, err := os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil || !fi.Mode().IsRegular() {
		return errors.New("bundle 缺少 Contents/Info.plist")
	}
	entries, err := os.ReadDir(filepath.Join(app, "Contents", "MacOS"))
	if err != nil {
		return errors.New("bundle 缺少 Contents/MacOS 目录")
	}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
			continue
		}
		return nil
	}
	return errors.New("Contents/MacOS 无有效可执行文件")
}

// swapDarwinBundle 两步 rename:target → <target>.bak-<ts>,staged → target;
// 第二步失败返回已产生的 backup 与 ErrSwapFailed,不自行回滚(§5.4 回滚唯一路径)。
func swapDarwinBundle(staged, target string) (string, error) {
	backup := target + ".bak-" + time.Now().Format("20060102-150405")
	if err := os.Rename(target, backup); err != nil {
		return "", ErrSwapFailed
	}
	if err := os.Rename(staged, target); err != nil {
		return backup, ErrSwapFailed
	}
	return backup, nil
}

// rollbackRename 通用回滚:backup == "" → nil(幂等);
// 备份缺失(已被消费)时拒绝且不动 target,避免二次回滚误删已还原的新包;
// rename 不能覆盖非空目录,故先移除新包(此时备份仍完整),再还原备份;任一失败 → ErrRollbackFailed。
func rollbackRename(backup, target string) error {
	if backup == "" {
		return nil
	}
	if _, err := os.Lstat(backup); err != nil {
		return ErrRollbackFailed
	}
	if err := os.RemoveAll(target); err != nil {
		return ErrRollbackFailed
	}
	if err := os.Rename(backup, target); err != nil {
		return ErrRollbackFailed
	}
	return nil
}

// DefaultDeps macOS 生产依赖组合:等待父退出 / 预检 / ditto 暂存 / .bak-<ts> 交换 / open 重启(§5.4)。
func DefaultDeps() HelperDeps {
	return HelperDeps{
		WaitParent: func(pid int, poll, timeout time.Duration) error {
			return WaitParentExit(pid, poll, timeout, processAlive)
		},
		Preflight: PreflightTarget,
		Stage:     stageDarwinBundle,
		Swap:      swapDarwinBundle,
		Rollback:  rollbackRename,
		Relaunch: func(target string) error {
			// open 不带 -n:复用系统既有实例语义,交给 LaunchServices 前移已运行实例。
			if err := exec.Command("open", target).Run(); err != nil {
				return ErrRelaunchFailed
			}
			return nil
		},
	}
}

// CleanupStaleBackups 启动清理:删除运行目标父目录下的 *.bak-* 与陈旧 .gbt32960-update-*.staging(尽力而为)。
func CleanupStaleBackups() error {
	target, err := RunningTarget()
	if err != nil {
		return err
	}
	return cleanupStaleBackupsIn(filepath.Dir(target))
}

// cleanupStaleBackupsIn 清理指定目录下的备份与暂存残留:全部尝试,返回首个错误(尽力而为)。
func cleanupStaleBackupsIn(dir string) error {
	var firstErr error
	for _, pattern := range []string{"*.bak-*", ".gbt32960-update-*.staging"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, m := range matches {
			if err := os.RemoveAll(m); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
