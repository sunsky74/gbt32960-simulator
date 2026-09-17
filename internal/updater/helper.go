package updater

import (
	"fmt"
	"os"
	"time"
)

const (
	// helperPollInterval 等待父进程退出的轮询间隔(§5.4)。
	helperPollInterval = 100 * time.Millisecond
	// helperParentTimeout 等待父进程退出的超时(§5.4);超时即失败:父进程仍活,不拉起、不触碰文件。
	helperParentTimeout = 60 * time.Second
)

// helper 退出码:0=成功;1=失败闭环已完成(结果文件已写/已尝试拉起);2=参数不可信(无可靠路径,不写结果文件)。
const (
	helperExitOK    = 0
	helperExitFail  = 1
	helperExitUsage = 2
)

// helperFault 演练专用故障注入:仅构建期 `-X gbt32960-simulator/internal/updater.helperFault=stage|swap|relaunch`
// 可注入(发布构建为空串,零效果);严禁任何运行时开启途径(环境变量/命令行参数/绑定面)。
var helperFault string

// RunHelperIfRequested main() 最早期拦截(§5.7):helper 调用 → 执行替换流程并以对应码退出(绝不初始化 Wails);
// 普通启动 → 立即返回,不触碰任何依赖。
func RunHelperIfRequested() {
	if code, handled := helperEntry(os.Args, DefaultDeps()); handled {
		os.Exit(code)
	}
}

// helperEntry 可测分发(不含 os.Exit):非 helper 调用 → (0, false) 且不触碰 deps;helper 调用 → (code, true)。
func helperEntry(args []string, deps HelperDeps) (code int, handled bool) {
	if !IsHelperInvocation(args) {
		return helperExitOK, false
	}
	return HelperMain(args, deps), true
}

// HelperMain helper 协议编排(§5.4 执行链):等待父退出 → 预检 → 同卷暂存 → 交换 → 写结果 → 拉起新版本;
// 任一环节失败 → 回滚(若已产生备份)→ 写失败结果 → 拉起旧版本(父进程等待超时分支除外:不拉起)。
// 返回码:0=成功;1=失败闭环已完成;2=参数不可信。
func HelperMain(args []string, deps HelperDeps) int {
	a, err := ParseHelperArgs(args)
	if err != nil {
		return helperExitUsage // 参数不可信:连结果文件路径都不可靠,不写
	}
	deps = faultDeps(deps) // 演练注入:仅构建期 -X 可开启

	log, _ := OpenHelperLog()
	if log != nil {
		defer log.Close()
	}
	logf := func(format string, v ...any) { // 日志不可用(打开失败)时静默,不阻塞闭环
		if log != nil {
			_, _ = fmt.Fprintf(log, format+"\n", v...)
		}
	}
	logf("helper 启动: target=%s tag=%s parent-pid=%d", a.Target, a.Tag, a.ParentPID)

	logf("等待父进程退出: pid=%d", a.ParentPID)
	if err := deps.WaitParent(a.ParentPID, helperPollInterval, helperParentTimeout); err != nil {
		logf("等待父进程退出失败(父进程仍活,不拉起): %v", err)
		writeResult(a, false, ErrParentWaitTimeout.Error(), logf)
		return helperExitFail
	}
	logf("父进程已退出")

	if _, err := os.Stat(a.Artifact); err != nil {
		logf("更新包不存在: %v", err)
		writeResult(a, false, ErrArtifactMissing.Error(), logf)
		relaunchOld(a, deps, logf)
		return helperExitFail
	}
	logf("产物存在: %s", a.Artifact)

	if err := deps.Preflight(a.Target); err != nil {
		logf("预检失败(细节见本行): %v", err)
		writeResult(a, false, ErrSwapFailed.Error(), logf)
		relaunchOld(a, deps, logf)
		return helperExitFail
	}
	logf("预检通过: %s", a.Target)

	staged, cleanup, err := deps.Stage(a.Artifact, a.Target)
	if err != nil {
		logf("暂存失败: %v", err)
		writeResult(a, false, ErrStageFailed.Error(), logf)
		relaunchOld(a, deps, logf)
		return helperExitFail
	}
	if cleanup != nil {
		defer cleanup()
	}
	logf("暂存完成: %s", staged)

	backup, err := deps.Swap(staged, a.Target)
	if err != nil {
		logf("交换失败: %v", err)
		return rollbackAndRelaunch(a, backup, ErrSwapFailed, deps, logf)
	}
	logf("交换完成: backup=%s", backup)

	writeResult(a, true, "", logf) // §5.4:写结果先于拉起;成功不落 reason(omitempty),版本由 targetVersion 承载
	logf("结果已写入: ok=true")

	if err := deps.Relaunch(a.Target); err != nil {
		logf("拉起新版本失败: %v", err)
		return rollbackAndRelaunch(a, backup, ErrRelaunchFailed, deps, logf)
	}
	logf("拉起新版本: %s", a.Target)
	return helperExitOK
}

// writeResult 写结果文件(LastResult{OK, TargetVersion: a.Tag, Reason, LogPath: a.Log} → WriteLastResult);
// 写盘失败仅记日志,不阻塞闭环——结果文件是尽力而为的跨会话告知。
func writeResult(a HelperArgs, ok bool, reason string, logf func(string, ...any)) {
	if err := WriteLastResult(LastResult{OK: ok, TargetVersion: a.Tag, Reason: reason, LogPath: a.Log}); err != nil {
		logf("写结果文件失败: %v", err)
	}
}

// rollbackAndRelaunch 回滚唯一路径(§5.4):Rollback 失败 → 目标不可信,写 ErrRollbackFailed 并直接返回(不拉旧版);
// 成功 → 写对应 reason → 拉起旧版本(拉起失败仅记日志,用户可手动启动)。
func rollbackAndRelaunch(a HelperArgs, backup string, reason error, deps HelperDeps, logf func(string, ...any)) int {
	if err := deps.Rollback(backup, a.Target); err != nil {
		logf("回滚失败(需手动重装): %v", err)
		writeResult(a, false, ErrRollbackFailed.Error(), logf)
		return helperExitFail
	}
	logf("回滚成功: backup=%s", backup)
	writeResult(a, false, reason.Error(), logf)
	relaunchOld(a, deps, logf)
	return helperExitFail
}

// relaunchOld 失败后拉起旧版本:重启一律使用 target 路径(严禁 helper 自身路径——替换后自身已指向新包);
// 拉起失败仅记日志(结果文件已告知,可手动启动)。
func relaunchOld(a HelperArgs, deps HelperDeps, logf func(string, ...any)) {
	if err := deps.Relaunch(a.Target); err != nil {
		logf("拉起旧版本失败(可手动启动): %v", err)
	}
}

// faultDeps 演练故障注入(仅构建期 -X;运行期无开关):
// stage/swap → 对应环节直接失败;relaunch → 仅首次拉起失败(交换成功、拉起新版本失败 → 真实回滚并拉起旧版本)。
func faultDeps(deps HelperDeps) HelperDeps {
	switch helperFault {
	case "stage":
		deps.Stage = func(string, string) (string, func(), error) { return "", nil, ErrStageFailed }
	case "swap":
		deps.Swap = func(string, string) (string, error) { return "", ErrSwapFailed }
	case "relaunch":
		relaunch, first := deps.Relaunch, true
		deps.Relaunch = func(target string) error {
			if first {
				first = false
				return ErrRelaunchFailed
			}
			return relaunch(target)
		}
	}
	return deps
}
