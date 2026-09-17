//go:build unix

package updater

import (
	"os"
	"testing"
)

// TestSpawnUnixProcessAliveSelf processAlive:自身 → true;不存在/非法 PID → false(ESRCH 分支)。
func TestSpawnUnixProcessAliveSelf(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatalf("processAlive(%d) = false, want true", os.Getpid())
	}
	if processAlive(1 << 30) {
		t.Fatal("processAlive(超出 pid 上限的值) = true, want false")
	}
	if processAlive(0) || processAlive(-1) {
		t.Fatal("processAlive(<=0) = true, want false(避免误伤进程组)")
	}
}

// TestSpawnUnixDetached Setsid 脱离启动:子进程存活 → Kill 回收后 processAlive == false。
func TestSpawnUnixDetached(t *testing.T) {
	proc, err := spawnDetached("/bin/sleep", []string{"30"})
	if err != nil {
		t.Fatal(err)
	}
	pid := proc.Pid
	if pid <= 0 {
		t.Fatalf("返回的 Process.Pid = %d, want > 0", pid)
	}
	if !processAlive(pid) {
		t.Fatalf("processAlive(%d) = false, want true", pid)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if _, err := proc.Wait(); err != nil {
		t.Fatalf("回收子进程: %v", err)
	}
	if processAlive(pid) {
		t.Fatalf("Kill 后 processAlive(%d) = true, want false", pid)
	}
}
