//go:build unix

package updater

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// processAlive 以 kill(pid, 0) 探测进程存活:ESRCH → false;EPERM → true(存在但无权限,保守视为存活)。
func processAlive(pid int) bool {
	if pid <= 0 {
		return false // pid <= 0 会命中进程组语义,直接判否避免误伤
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

// spawnDetached 以 Setsid 脱离父进程会话启动 exe;Start 成功后 Release,避免父进程退出前持有子进程句柄。
// 注:Release 会把 Process.Pid 置为 -1,故以 pid 重建只读句柄返回,调用方仍可探测/终止该子进程。
func spawnDetached(exe string, args []string) (*os.Process, error) {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return nil, err
	}
	return os.FindProcess(pid)
}

// SpawnHelper 以当前可执行文件 + helper 参数表拉起 detached helper 进程(§5.4 触发链)。
func SpawnHelper(args HelperArgs) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_, err = spawnDetached(exe, args.Encode())
	return err
}
