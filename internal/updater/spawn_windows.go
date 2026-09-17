//go:build windows

package updater

import (
	"os"
	"os/exec"
	"syscall"
)

// stillActive GetExitCodeProcess 的 STILL_ACTIVE 取值(syscall 未导出该常量)。
const stillActive = 259

// processAlive 探测 PID 是否存活:OpenProcess(PROCESS_QUERY_INFORMATION)+GetExitCodeProcess==STILL_ACTIVE;
// 每次调用关闭句柄(轮询不泄漏)。
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// spawnDetached 独立进程启动(DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP):
// 父进程退出后 helper / 新实例继续运行;Start 成功后 Release(不留句柄、不阻塞 Wait)。
func spawnDetached(exe string, args []string) (*os.Process, error) {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()
	return cmd.Process, nil
}

// SpawnHelper 先自复制到 %TEMP%(避免占用目标文件),再以 helper 参数独立拉起副本。
func SpawnHelper(args HelperArgs) error {
	copy := helperCopyPath(os.Getpid())
	if err := copySelf(copy); err != nil {
		return err
	}
	_, err := spawnDetached(copy, args.Encode())
	return err
}
