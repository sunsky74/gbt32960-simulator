package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- 测试台 ----

// fakeHelper 注入式 fake 依赖缝:记录调用序列 / 拉起与回滚实参,各环节可单独注入失败(不启动真实进程)。
type fakeHelper struct {
	calls       []string      // 到访环节(按调用顺序)
	rollbackArg [][2]string   // Rollback 实参序列 (backup, target)
	relaunchArg []string      // Relaunch 实参序列
	waitPID     int           // WaitParent 实参:pID
	waitPoll    time.Duration // WaitParent 实参:轮询间隔
	waitTMO     time.Duration // WaitParent 实参:超时

	waitFn      func(pid int, poll, timeout time.Duration) error
	preflightFn func(target string) error
	stageFn     func(artifact, target string) (string, func(), error)
	swapFn      func(staged, target string) (string, error)
	rollbackFn  func(backup, target string) error
	relaunchFn  func(target string) error
}

// newFakeHelper 全成功默认实现:各环节仅记录调用并成功返回。
func newFakeHelper() *fakeHelper {
	f := &fakeHelper{}
	f.waitFn = func(int, time.Duration, time.Duration) error { return nil }
	f.preflightFn = func(string) error { return nil }
	f.stageFn = func(artifact, _ string) (string, func(), error) { return artifact + ".staged", nil, nil }
	f.swapFn = func(_, target string) (string, error) { return target + ".bak", nil }
	f.rollbackFn = func(string, string) error { return nil }
	f.relaunchFn = func(string) error { return nil }
	return f
}

// deps 组装记录型依赖缝:每次调用先记录再委托给可注入实现。
func (f *fakeHelper) deps() HelperDeps {
	return HelperDeps{
		WaitParent: func(pid int, poll, timeout time.Duration) error {
			f.calls = append(f.calls, "wait")
			f.waitPID, f.waitPoll, f.waitTMO = pid, poll, timeout
			return f.waitFn(pid, poll, timeout)
		},
		Preflight: func(target string) error {
			f.calls = append(f.calls, "preflight")
			return f.preflightFn(target)
		},
		Stage: func(artifact, target string) (string, func(), error) {
			f.calls = append(f.calls, "stage")
			return f.stageFn(artifact, target)
		},
		Swap: func(staged, target string) (string, error) {
			f.calls = append(f.calls, "swap")
			return f.swapFn(staged, target)
		},
		Rollback: func(backup, target string) error {
			f.calls = append(f.calls, "rollback")
			f.rollbackArg = append(f.rollbackArg, [2]string{backup, target})
			return f.rollbackFn(backup, target)
		},
		Relaunch: func(target string) error {
			f.calls = append(f.calls, "relaunch")
			f.relaunchArg = append(f.relaunchArg, target)
			return f.relaunchFn(target)
		},
	}
}

// wantCalls 断言调用序列逐项一致(顺序即 §5.4 执行链契约)。
func (f *fakeHelper) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	if len(f.calls) != len(want) {
		t.Fatalf("调用序列 = %v, want %v", f.calls, want)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Fatalf("调用序列 = %v, want %v", f.calls, want)
		}
	}
}

// wantRelaunch 断言拉起实参序列逐项一致(一律 target 路径,严禁 helper 自身路径)。
func (f *fakeHelper) wantRelaunch(t *testing.T, want ...string) {
	t.Helper()
	if len(f.relaunchArg) != len(want) {
		t.Fatalf("Relaunch 实参 = %v, want %v", f.relaunchArg, want)
	}
	for i := range want {
		if f.relaunchArg[i] != want[i] {
			t.Fatalf("Relaunch 实参 = %v, want %v", f.relaunchArg, want)
		}
	}
}

// relaunchSnapshot 一次拉起调用时点的结果文件快照:「写结果先于拉起」(§5.4)顺序契约的断言依据。
type relaunchSnapshot struct {
	exists bool       // 拉起时点结果文件是否已落盘
	result LastResult // exists 为真时的内容
}

// captureRelaunchSnapshots 在既有 relaunchFn 外再包一层(保留原实现语义):
// 每次拉起调用时点读取结果文件入 sink——调用前须 redirectUserCache。
func (f *fakeHelper) captureRelaunchSnapshots(t *testing.T, sink *[]relaunchSnapshot) {
	t.Helper()
	inner := f.relaunchFn
	f.relaunchFn = func(target string) error {
		got, err := ReadLastResult()
		if err != nil {
			t.Fatalf("拉起时点读取结果文件失败: %v", err)
		}
		snap := relaunchSnapshot{}
		if got != nil {
			snap.exists, snap.result = true, *got
		}
		*sink = append(*sink, snap)
		return inner(target)
	}
}

// wantRelaunchSnapshot 断言第 i 次(0 基)拉起调用时点结果文件已落盘且为 {ok, reason}。
func wantRelaunchSnapshot(t *testing.T, snaps []relaunchSnapshot, i int, wantOK bool, wantReason string) {
	t.Helper()
	if i >= len(snaps) {
		t.Fatalf("拉起时点快照数 = %d, want > %d", len(snaps), i)
	}
	snap := snaps[i]
	if !snap.exists {
		t.Fatalf("第 %d 次拉起时点结果文件未落盘:写结果必须先于拉起", i+1)
	}
	if snap.result.OK != wantOK || snap.result.Reason != wantReason {
		t.Fatalf("第 %d 次拉起时点结果 = %+v, want {ok:%v reason:%q}", i+1, snap.result, wantOK, wantReason)
	}
}

// newHelperFixture 构造临时布局与完整 helper 参数:appDir 内为替换目标,artifact 独立成文件。
// Result/Log 仅作参数透传(实际落盘位置由缓存根推导,测试以 redirectUserCache 隔离)。
func newHelperFixture(t *testing.T, tag string) (HelperArgs, string) {
	t.Helper()
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(appDir, "gbt32960-simulator")
	if err := os.WriteFile(target, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, "artifact.zip")
	if err := os.WriteFile(artifact, []byte("new-package"), 0o644); err != nil {
		t.Fatal(err)
	}
	return HelperArgs{
		ParentPID: 4242,
		Artifact:  artifact,
		Target:    target,
		Tag:       tag,
		Result:    filepath.Join(root, "updates", "last-result.json"),
		Log:       filepath.Join(root, "updates", "helper.log"),
	}, appDir
}

// helperArgv 以 exec 角度补齐 argv[0]。
func helperArgv(a HelperArgs) []string {
	return append([]string{"exe"}, a.Encode()...)
}

// readResult 读取结果文件并断言存在(调用前须 redirectUserCache)。
func readResult(t *testing.T) LastResult {
	t.Helper()
	got, err := ReadLastResult()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("结果文件未写入")
	}
	return *got
}

// readHelperLog 读取 helper.log 文本(调用前须 redirectUserCache)。
func readHelperLog(t *testing.T) string {
	t.Helper()
	return string(readRaw(t, mustPath(t, HelperLogPath)))
}

// wantFailResult 断言失败记录:{ok:false, targetVersion:tag, reason:want}。
func wantFailResult(t *testing.T, a HelperArgs, want string) {
	t.Helper()
	got := readResult(t)
	if got.OK || got.TargetVersion != a.Tag || got.Reason != want {
		t.Fatalf("结果记录 = %+v, want {ok:false targetVersion:%s reason:%s}", got, a.Tag, want)
	}
	if got.LogPath != a.Log {
		t.Fatalf("logPath = %q, want %q", got.LogPath, a.Log)
	}
}

// ---- Step 1 用例 ----

// TestHelperEntrySentinel 分发契约:helper 调用被拦截(handled=true);普通启动 (0, false) 且不触碰 deps。
func TestHelperEntrySentinel(t *testing.T) {
	t.Run("helper 调用被拦截", func(t *testing.T) {
		redirectUserCache(t)
		f := newFakeHelper()
		args, _ := newHelperFixture(t, "v1.2.3")
		code, handled := helperEntry(helperArgv(args), f.deps())
		if !handled || code != 0 {
			t.Fatalf("helperEntry(helper) = (%d, %v), want (0, true)", code, handled)
		}
		if len(f.calls) == 0 {
			t.Fatal("helper 调用应进入替换流程,实际未触碰 deps")
		}
	})

	t.Run("普通启动不拦截且不触碰依赖", func(t *testing.T) {
		f := newFakeHelper()
		code, handled := helperEntry([]string{"exe", "--other"}, f.deps())
		if handled || code != 0 {
			t.Fatalf("helperEntry(普通) = (%d, %v), want (0, false)", code, handled)
		}
		if len(f.calls) != 0 {
			t.Fatalf("普通启动触碰了 deps: %v", f.calls)
		}
	})
}

// TestHelperMainSuccess 成功链:等待→预检→暂存→交换→写结果→拉起;拉起恰 1 次且实参为 target,
// 且拉起时点结果文件已落盘为 {ok:true, reason 空}(§5.4 写结果先于拉起)。
func TestHelperMainSuccess(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	var snaps []relaunchSnapshot
	f.captureRelaunchSnapshots(t, &snaps)
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 0 {
		t.Fatalf("HelperMain = %d, want 0", code)
	}
	f.wantCalls(t, "wait", "preflight", "stage", "swap", "relaunch")
	f.wantRelaunch(t, args.Target)
	wantRelaunchSnapshot(t, snaps, 0, true, "") // 拉起时点:成功结果已落盘
	if f.waitPID != args.ParentPID || f.waitPoll != helperPollInterval || f.waitTMO != helperParentTimeout {
		t.Fatalf("WaitParent 实参 = (%d, %v, %v), want (%d, %v, %v)",
			f.waitPID, f.waitPoll, f.waitTMO, args.ParentPID, helperPollInterval, helperParentTimeout)
	}

	got := readResult(t)
	if !got.OK || got.TargetVersion != args.Tag || got.Reason != "" {
		t.Fatalf("结果记录 = %+v, want {ok:true targetVersion:%s reason 空}", got, args.Tag)
	}
	if got.LogPath != args.Log {
		t.Fatalf("logPath = %q, want %q", got.LogPath, args.Log)
	}

	logText := readHelperLog(t)
	for _, want := range []string{"helper 启动", "等待父进程退出", "父进程已退出", "暂存完成", "交换完成", "结果已写入", "拉起新版本"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("helper.log 缺阶段行 %q:\n%s", want, logText)
		}
	}
}

// TestHelperMainStageFailure 暂存失败:原因「暂存失败」、不产生备份故 Rollback 零调用、拉起旧版 1 次、码 1。
func TestHelperMainStageFailure(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	f.stageFn = func(string, string) (string, func(), error) { return "", nil, ErrStageFailed }
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait", "preflight", "stage", "relaunch")
	f.wantRelaunch(t, args.Target)
	wantFailResult(t, args, ErrStageFailed.Error())
}

// TestHelperMainSwapFailure 交换失败:Rollback(backup, target) 恰 1 次、原因「替换失败」、拉起旧版、码 1,
// 且拉起旧版前失败结果已落盘(§5.4)。
func TestHelperMainSwapFailure(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	var snaps []relaunchSnapshot
	f.captureRelaunchSnapshots(t, &snaps)
	f.swapFn = func(_, target string) (string, error) { return target + ".bak", ErrSwapFailed }
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait", "preflight", "stage", "swap", "rollback", "relaunch")
	if len(f.rollbackArg) != 1 || f.rollbackArg[0] != [2]string{args.Target + ".bak", args.Target} {
		t.Fatalf("Rollback 实参 = %v, want [[%s %s]]", f.rollbackArg, args.Target+".bak", args.Target)
	}
	f.wantRelaunch(t, args.Target)
	wantRelaunchSnapshot(t, snaps, 0, false, ErrSwapFailed.Error()) // 拉起旧版时点:失败结果已落盘
	wantFailResult(t, args, ErrSwapFailed.Error())
}

// TestHelperMainRelaunchFailure 新版本拉起失败:回滚 1 次后二次拉起旧版、原因「无法启动新版本」、码 1,
// 且首次拉起(新版本)前成功结果已落盘、二次拉起(旧版本)前失败结果已落盘(§5.4)。
func TestHelperMainRelaunchFailure(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	f.relaunchFn = func(string) error {
		if len(f.relaunchArg) == 1 {
			return ErrRelaunchFailed
		}
		return nil
	}
	var snaps []relaunchSnapshot
	f.captureRelaunchSnapshots(t, &snaps) // 包装注入实现:快照读取先于注入失败判定
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait", "preflight", "stage", "swap", "relaunch", "rollback", "relaunch")
	if len(f.rollbackArg) != 1 || f.rollbackArg[0] != [2]string{args.Target + ".bak", args.Target} {
		t.Fatalf("Rollback 实参 = %v, want [[%s %s]]", f.rollbackArg, args.Target+".bak", args.Target)
	}
	f.wantRelaunch(t, args.Target, args.Target)
	wantRelaunchSnapshot(t, snaps, 0, true, "")                         // 首次拉起(新版本)时点:成功结果已落盘
	wantRelaunchSnapshot(t, snaps, 1, false, ErrRelaunchFailed.Error()) // 二次拉起(旧版本)时点:失败结果已落盘
	wantFailResult(t, args, ErrRelaunchFailed.Error())
}

// TestHelperMainRollbackFailure 回滚失败:原因「回滚失败,请手动重新安装」且不拉起(目标不可信)、码 1。
func TestHelperMainRollbackFailure(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	f.swapFn = func(_, target string) (string, error) { return target + ".bak", ErrSwapFailed }
	f.rollbackFn = func(string, string) error { return ErrRollbackFailed }
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait", "preflight", "stage", "swap", "rollback")
	f.wantRelaunch(t)
	wantFailResult(t, args, ErrRollbackFailed.Error())
}

// TestHelperMainParentTimeout 父进程等待超时:不触碰文件(预检/暂存/回滚/拉起全零调用)、码 1。
func TestHelperMainParentTimeout(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	f.waitFn = func(int, time.Duration, time.Duration) error { return ErrParentWaitTimeout }
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait")
	f.wantRelaunch(t)
	wantFailResult(t, args, ErrParentWaitTimeout.Error())
}

// TestHelperMainArtifactMissing 产物缺失:原因「更新包不存在」、Stage 零调用、拉起旧版 1 次、码 1。
func TestHelperMainArtifactMissing(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	args, _ := newHelperFixture(t, "v1.2.3")
	args.Artifact = filepath.Join(t.TempDir(), "missing.zip")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait", "relaunch")
	f.wantRelaunch(t, args.Target)
	wantFailResult(t, args, ErrArtifactMissing.Error())
}

// TestHelperMainPreflightFailure 预检失败(防御纵深;应用内已先预检):原因「替换失败」+ 细节在日志、拉起旧版、码 1。
func TestHelperMainPreflightFailure(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	f.preflightFn = func(string) error { return errors.New("目录不可写") }
	args, _ := newHelperFixture(t, "v1.2.3")

	if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
		t.Fatalf("HelperMain = %d, want 1", code)
	}
	f.wantCalls(t, "wait", "preflight", "relaunch")
	f.wantRelaunch(t, args.Target)
	wantFailResult(t, args, ErrSwapFailed.Error())
	if logText := readHelperLog(t); !strings.Contains(logText, "目录不可写") {
		t.Fatalf("helper.log 应含预检失败细节:\n%s", logText)
	}
}

// TestHelperMainFaultInjection 仅构建期故障注入语义:stage/swap/relaunch 命中;未知取值与空串等同正常路径。
// 注入是"替代"该环节的依赖调用(绝不先真实暂存/交换再报失败),故被注入的那一步不经过 fake 记录器。
func TestHelperMainFaultInjection(t *testing.T) {
	cases := []struct {
		name       string
		fault      string
		wantCode   int
		wantReason string // 码 0 时忽略
		wantCalls  []string
		wantLaunch int // 到达 fake 的 Relaunch 次数(注入的那次被替代,不计入)
	}{
		{"swap 注入", "swap", 1, ErrSwapFailed.Error(),
			[]string{"wait", "preflight", "stage", "rollback", "relaunch"}, 1},
		{"stage 注入", "stage", 1, ErrStageFailed.Error(),
			[]string{"wait", "preflight", "relaunch"}, 1},
		{"relaunch 注入仅首次失败", "relaunch", 1, ErrRelaunchFailed.Error(),
			[]string{"wait", "preflight", "stage", "swap", "rollback", "relaunch"}, 1},
		{"未知取值等同正常路径", "bogus", 0, "",
			[]string{"wait", "preflight", "stage", "swap", "relaunch"}, 1},
		{"空串无影响", "", 0, "",
			[]string{"wait", "preflight", "stage", "swap", "relaunch"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			redirectUserCache(t)
			helperFault = c.fault
			t.Cleanup(func() { helperFault = "" })

			f := newFakeHelper()
			args, _ := newHelperFixture(t, "v1.2.3")
			if code := HelperMain(helperArgv(args), f.deps()); code != c.wantCode {
				t.Fatalf("HelperMain = %d, want %d", code, c.wantCode)
			}
			f.wantCalls(t, c.wantCalls...)
			if len(f.relaunchArg) != c.wantLaunch {
				t.Fatalf("Relaunch 次数 = %d, want %d", len(f.relaunchArg), c.wantLaunch)
			}
			got := readResult(t)
			if c.wantCode == 0 {
				if !got.OK || got.Reason != "" {
					t.Fatalf("注入未命中时应为成功记录: %+v", got)
				}
				return
			}
			if got.OK || got.Reason != c.wantReason {
				t.Fatalf("结果记录 = %+v, want reason=%s", got, c.wantReason)
			}
		})
	}

	t.Run("swap 注入不产生备份故 Rollback 收到空 backup", func(t *testing.T) {
		redirectUserCache(t)
		helperFault = "swap"
		t.Cleanup(func() { helperFault = "" })

		f := newFakeHelper()
		args, _ := newHelperFixture(t, "v1.2.3")
		if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
			t.Fatalf("HelperMain = %d, want 1", code)
		}
		if len(f.rollbackArg) != 1 || f.rollbackArg[0][0] != "" || f.rollbackArg[0][1] != args.Target {
			t.Fatalf("Rollback 实参 = %v, want [[] %s]", f.rollbackArg, args.Target)
		}
	})
}

// TestHelperMainDataBoundary 数据边界(AC-15):真实文件级暂存/交换/回滚走全流程,同目录用户数据逐字节不变。
func TestHelperMainDataBoundary(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	f.stageFn = func(artifact, target string) (string, func(), error) {
		staged := target + ".staged"
		b, err := os.ReadFile(artifact)
		if err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(staged, b, 0o644); err != nil {
			return "", nil, err
		}
		return staged, func() { _ = os.Remove(staged) }, nil
	}
	f.swapFn = func(staged, target string) (string, error) {
		backup := target + ".bak"
		if err := os.Rename(target, backup); err != nil {
			return "", err
		}
		if err := os.Rename(staged, target); err != nil {
			return backup, err
		}
		return backup, nil
	}

	args, appDir := newHelperFixture(t, "v2.0.0")
	settings := filepath.Join(appDir, "settings.json")
	packFile := filepath.Join(appDir, "packs", "x.json")
	if err := os.MkdirAll(filepath.Dir(packFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"theme":"深色","lang":"zh-CN"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packFile, []byte(`{"packs":[1,2,3]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeSettings, beforePack := readRaw(t, settings), readRaw(t, packFile)

	if code := HelperMain(helperArgv(args), f.deps()); code != 0 {
		t.Fatalf("HelperMain = %d, want 0", code)
	}

	if got := readRaw(t, args.Target); !bytes.Equal(got, readRaw(t, args.Artifact)) {
		t.Fatalf("程序主体未被替换: %q", got)
	}
	if got := readRaw(t, settings); !bytes.Equal(got, beforeSettings) {
		t.Fatalf("settings.json 被改动:\nbefore=%q\nafter =%q", beforeSettings, got)
	}
	if got := readRaw(t, packFile); !bytes.Equal(got, beforePack) {
		t.Fatalf("packs/x.json 被改动:\nbefore=%q\nafter =%q", beforePack, got)
	}
}

// TestHelperMainResultSchema 结果文件 schema:成功恰 3 键(reason omitempty);失败 4 键且 reason 为固定文案。
func TestHelperMainResultSchema(t *testing.T) {
	keysOf := func(t *testing.T, raw []byte) map[string]json.RawMessage {
		t.Helper()
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatalf("结果文件非法 JSON: %v\n%s", err, raw)
		}
		return keys
	}

	t.Run("成功:reason 被 omitempty 省略", func(t *testing.T) {
		redirectUserCache(t)
		f := newFakeHelper()
		args, _ := newHelperFixture(t, "v1.2.3")
		if code := HelperMain(helperArgv(args), f.deps()); code != 0 {
			t.Fatalf("HelperMain = %d, want 0", code)
		}
		keys := keysOf(t, readRaw(t, mustPath(t, LastResultPath)))
		if len(keys) != 3 {
			t.Fatalf("JSON 键 = %v, want 恰 ok/targetVersion/logPath", keys)
		}
		for _, k := range []string{"ok", "targetVersion", "logPath"} {
			if _, ok := keys[k]; !ok {
				t.Fatalf("缺字段 %q", k)
			}
		}
		if _, ok := keys["reason"]; ok {
			t.Fatal("成功记录不应含 reason 键")
		}
	})

	t.Run("失败:reason 为固定文案", func(t *testing.T) {
		redirectUserCache(t)
		f := newFakeHelper()
		f.stageFn = func(string, string) (string, func(), error) { return "", nil, ErrStageFailed }
		args, _ := newHelperFixture(t, "v1.2.3")
		if code := HelperMain(helperArgv(args), f.deps()); code != 1 {
			t.Fatalf("HelperMain = %d, want 1", code)
		}
		keys := keysOf(t, readRaw(t, mustPath(t, LastResultPath)))
		if len(keys) != 4 {
			t.Fatalf("JSON 键 = %v, want 恰 ok/targetVersion/reason/logPath", keys)
		}
		var reason string
		if err := json.Unmarshal(keys["reason"], &reason); err != nil || reason != ErrStageFailed.Error() {
			t.Fatalf("reason = %q (err=%v), want %s", reason, err, ErrStageFailed.Error())
		}
	})
}

// TestHelperMainArgFailure 参数不可信(缺 --target):码 2、不触碰 deps、不写结果文件(此时无可靠路径)。
func TestHelperMainArgFailure(t *testing.T) {
	redirectUserCache(t)
	f := newFakeHelper()
	args, _ := newHelperFixture(t, "v1.2.3")
	args.Target = ""

	if code := HelperMain(helperArgv(args), f.deps()); code != 2 {
		t.Fatalf("HelperMain = %d, want 2", code)
	}
	if len(f.calls) != 0 {
		t.Fatalf("参数失败不应触碰 deps: %v", f.calls)
	}
	if _, err := os.Stat(mustPath(t, LastResultPath)); !os.IsNotExist(err) {
		t.Fatalf("参数失败不应写结果文件: %v", err)
	}
}

// TestRunHelperNotRequested 生产入口普通启动不拦截:非 helper argv 调用 RunHelperIfRequested 直接返回(命中 sentinel 才会 os.Exit)。
func TestRunHelperNotRequested(t *testing.T) {
	if IsHelperInvocation(os.Args) {
		t.Skip("测试进程 argv 恰为 helper 调用,跳过")
	}
	RunHelperIfRequested() // 不退出即通过
}
