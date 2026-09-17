package updater

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// HelperSentinel helper 模式命令行哨兵(设计文档 §5.4/§5.7):
// main() 最早期以 IsHelperInvocation 判定,命中即执行替换流程并 os.Exit,绝不初始化 Wails。
const HelperSentinel = "--updater-helper"

// 替换链共享错误值(文案即用户可见文案 / 结果文件 reason;§5.4)。
var (
	// ErrTargetNotFound 三平台共用:可执行文件/bundle 定位失败。
	ErrTargetNotFound = errors.New("无法定位应用位置,请手动更新")
	// ErrParentWaitTimeout helper 等待父进程退出超时(父进程仍活:不拉起、不触碰文件)。
	ErrParentWaitTimeout = errors.New("等待应用退出超时")
	// ErrArtifactMissing 待替换产物不存在(缓存已被清理或路径失效)。
	ErrArtifactMissing = errors.New("更新包不存在")
	// ErrStageFailed 同卷暂存失败(解压/复制/结构校验)。
	ErrStageFailed = errors.New("暂存失败")
	// ErrSwapFailed 原名/新名任一 rename 失败(重试耗尽后)。
	ErrSwapFailed = errors.New("替换失败")
	// ErrRelaunchFailed 新版本拉起失败(触发回滚并拉起旧版)。
	ErrRelaunchFailed = errors.New("无法启动新版本")
	// ErrRollbackFailed 回滚失败,需用户手动重装。
	ErrRollbackFailed = errors.New("回滚失败,请手动重新安装")
)

// HelperArgs helper 进程参数(唯一进程级契约;设计文档 §5.4 参数格式)。
type HelperArgs struct {
	ParentPID int
	Artifact  string
	Target    string
	Tag       string
	Result    string
	Log       string
}

// IsHelperInvocation 仅认 args[1] == HelperSentinel(§5.7:不初始化 Wails 的最早拦截判定)。
func IsHelperInvocation(args []string) bool {
	return len(args) > 1 && args[1] == HelperSentinel
}

// Encode 生成除 argv[0] 外的完整参数表(首元素为 sentinel)。
func (a HelperArgs) Encode() []string {
	return []string{
		HelperSentinel,
		"--parent-pid", strconv.Itoa(a.ParentPID),
		"--artifact", a.Artifact,
		"--target", a.Target,
		"--tag", a.Tag,
		"--result", a.Result,
		"--log", a.Log,
	}
}

// ParseHelperArgs 解析 args[2:] 起的 helper 参数;
// 缺 sentinel/必填缺失(含 ParentPID<=0)/非数字/未知 flag/多余位置参数一律 error(fail-closed)。
func ParseHelperArgs(args []string) (HelperArgs, error) {
	if !IsHelperInvocation(args) {
		return HelperArgs{}, fmt.Errorf("非 helper 调用:缺少 %s", HelperSentinel)
	}
	fs := flag.NewFlagSet(HelperSentinel, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // 解析错误经返回值透出,避免噪声输出
	var a HelperArgs
	fs.IntVar(&a.ParentPID, "parent-pid", 0, "父进程 PID")
	fs.StringVar(&a.Artifact, "artifact", "", "更新产物路径")
	fs.StringVar(&a.Target, "target", "", "替换目标路径")
	fs.StringVar(&a.Tag, "tag", "", "目标版本")
	fs.StringVar(&a.Result, "result", "", "结果文件路径")
	fs.StringVar(&a.Log, "log", "", "日志文件路径")
	if err := fs.Parse(args[2:]); err != nil {
		return HelperArgs{}, err
	}
	if fs.NArg() > 0 {
		return HelperArgs{}, fmt.Errorf("helper 参数非法: %v", fs.Args())
	}
	for _, req := range []struct {
		name, value string
	}{
		{"--artifact", a.Artifact},
		{"--target", a.Target},
		{"--tag", a.Tag},
		{"--result", a.Result},
		{"--log", a.Log},
	} {
		if req.value == "" {
			return HelperArgs{}, fmt.Errorf("helper 参数缺失: %s", req.name)
		}
	}
	if a.ParentPID <= 0 {
		return HelperArgs{}, errors.New("helper 参数缺失或非法: --parent-pid")
	}
	return a, nil
}

// HelperDeps helper 协议依赖缝(生产 = 各平台 DefaultDeps();测试注入 fake)。
// Stage 的 cleanup 可为 nil;Swap 失败返回已产生的 backup(""=未产生),不自行回滚(§5.4 回滚唯一路径)。
type HelperDeps struct {
	WaitParent func(pid int, poll, timeout time.Duration) error
	Preflight  func(target string) error
	Stage      func(artifact, target string) (staged string, cleanup func(), err error)
	Swap       func(staged, target string) (backup string, err error)
	Rollback   func(backup, target string) error
	Relaunch   func(target string) error
}

// WaitParentExit 以 poll 间隔轮询 alive,直至父进程退出或 timeout 到期(§5.4:100ms 轮询 / 60s 超时)。
func WaitParentExit(pid int, poll, timeout time.Duration, alive func(int) bool) error {
	deadline := time.Now().Add(timeout)
	for {
		if !alive(pid) {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrParentWaitTimeout
		}
		time.Sleep(poll)
	}
}

// RenameWithRetry 首次失败后逐次退避重试;len(retries) 为重试次数(总尝试 = 1+len(retries))。
// 全部失败返回 ErrSwapFailed。Windows 杀软临时锁场景由平台层传入 200ms/500ms/1s(§5.4)。
func RenameWithRetry(old, new string, retries []time.Duration) error {
	return renameWithRetry(os.Rename, old, new, retries)
}

// renameWithRetry 携带可注入 rename 缝(测试用,不导出)。
func renameWithRetry(rename func(old, new string) error, old, new string, retries []time.Duration) error {
	if err := rename(old, new); err == nil {
		return nil
	}
	for _, d := range retries {
		time.Sleep(d)
		if err := rename(old, new); err == nil {
			return nil
		}
	}
	return ErrSwapFailed
}
