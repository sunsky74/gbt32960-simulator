package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestHelperArgsRoundtrip 参数契约往返:Encode → exec 角度 argv → IsHelperInvocation → ParseHelperArgs 逐字段一致。
func TestHelperArgsRoundtrip(t *testing.T) {
	want := HelperArgs{
		ParentPID: 4242,
		Artifact:  "/Users/张三/缓存 dir/updates/v1.2.3/gbt32960-simulator.app.zip",
		Target:    "/Applications/GB 模拟器.app",
		Tag:       "v1.2.3",
		Result:    "/Users/张三/缓存 dir/updates/last-result.json",
		Log:       "/Users/张三/缓存 dir/updates/helper.log",
	}
	enc := want.Encode()
	if len(enc) == 0 || enc[0] != HelperSentinel {
		t.Fatalf("Encode() = %v, want 首元素 %q", enc, HelperSentinel)
	}
	// exec 角度:argv[0] 为可执行文件路径,其后为 Encode 结果
	args := append([]string{"/Applications/GB 模拟器.app/Contents/MacOS/gbt32960-simulator"}, enc...)
	if !IsHelperInvocation(args) {
		t.Fatalf("IsHelperInvocation(%v) = false, want true", args)
	}
	got, err := ParseHelperArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("往返不一致:\n got = %+v\nwant = %+v", got, want)
	}
}

// TestHelperArgsRejections 参数契约拒绝面:缺 sentinel / 缺必填 / 非数字 / 未知 flag 一律 fail-closed。
func TestHelperArgsRejections(t *testing.T) {
	base := HelperArgs{ParentPID: 7, Artifact: "a", Target: "t", Tag: "v1", Result: "r", Log: "l"}

	t.Run("缺 sentinel", func(t *testing.T) {
		args := []string{"app"}
		if IsHelperInvocation(args) {
			t.Fatalf("IsHelperInvocation(%v) = true, want false", args)
		}
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(缺 sentinel) 应报错")
		}
	})

	t.Run("sentinel 形近但不等", func(t *testing.T) {
		args := []string{"app", "--updater-helper-x"}
		if IsHelperInvocation(args) {
			t.Fatalf("IsHelperInvocation(%v) = true, want false", args)
		}
	})

	t.Run("缺 --target", func(t *testing.T) {
		a := base
		a.Target = ""
		if _, err := ParseHelperArgs(append([]string{"app"}, a.Encode()...)); err == nil {
			t.Fatal("ParseHelperArgs(缺 --target) 应报错")
		}
	})

	t.Run("必填空串", func(t *testing.T) {
		for _, c := range []struct {
			name string
			mut  func(*HelperArgs)
		}{
			{"--artifact", func(a *HelperArgs) { a.Artifact = "" }},
			{"--tag", func(a *HelperArgs) { a.Tag = "" }},
			{"--result", func(a *HelperArgs) { a.Result = "" }},
			{"--log", func(a *HelperArgs) { a.Log = "" }},
		} {
			a := base
			c.mut(&a)
			if _, err := ParseHelperArgs(append([]string{"app"}, a.Encode()...)); err == nil {
				t.Fatalf("ParseHelperArgs(缺 %s) 应报错", c.name)
			}
		}
	})

	t.Run("缺 --parent-pid", func(t *testing.T) {
		args := []string{"app", HelperSentinel,
			"--artifact", "a", "--target", "t", "--tag", "v1", "--result", "r", "--log", "l"}
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(缺 --parent-pid) 应报错")
		}
	})

	t.Run("--parent-pid 非数字", func(t *testing.T) {
		args := []string{"app", HelperSentinel,
			"--parent-pid", "abc", "--artifact", "a", "--target", "t", "--tag", "v1", "--result", "r", "--log", "l"}
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(--parent-pid abc) 应报错")
		}
	})

	t.Run("--parent-pid 为 0", func(t *testing.T) {
		args := []string{"app", HelperSentinel,
			"--parent-pid", "0", "--artifact", "a", "--target", "t", "--tag", "v1", "--result", "r", "--log", "l"}
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(--parent-pid 0) 应报错")
		}
	})

	t.Run("--parent-pid 为负", func(t *testing.T) {
		args := []string{"app", HelperSentinel,
			"--parent-pid", "-1", "--artifact", "a", "--target", "t", "--tag", "v1", "--result", "r", "--log", "l"}
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(--parent-pid -1) 应报错")
		}
	})

	t.Run("未知 flag", func(t *testing.T) {
		args := append(append([]string{"app"}, base.Encode()...), "--bogus", "x")
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(未知 flag) 应报错")
		}
	})

	t.Run("多余位置参数", func(t *testing.T) {
		args := append(append([]string{"app"}, base.Encode()...), "stray")
		if _, err := ParseHelperArgs(args); err == nil {
			t.Fatal("ParseHelperArgs(多余位置参数) 应报错")
		}
	})
}

// TestRenameWithRetryRetries 退避重试语义:len(retries) = 重试次数(总尝试 = 1+len(retries));全程失败 → ErrSwapFailed。
func TestRenameWithRetryRetries(t *testing.T) {
	cases := []struct {
		name      string
		failTimes int
		retries   []time.Duration
		wantCalls int
		wantErr   bool
	}{
		{"首次成功仅 1 次尝试", 0, []time.Duration{time.Millisecond}, 1, false},
		{"1 败 1 成", 1, []time.Duration{0}, 2, false},
		{"3 败 1 成", 3, []time.Duration{0, 0, 0}, 4, false},
		{"4 败", 4, []time.Duration{0, 0, 0}, 4, true},
		{"retries 为空仅 1 次尝试", 1, nil, 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			rename := func(old, new string) error {
				calls++
				if old != "old" || new != "new" {
					t.Errorf("rename(%q, %q), want (old, new)", old, new)
				}
				if calls <= c.failTimes {
					return errors.New("目标被占用")
				}
				return nil
			}
			err := renameWithRetry(rename, "old", "new", c.retries)
			if calls != c.wantCalls {
				t.Fatalf("调用次数 = %d, want %d", calls, c.wantCalls)
			}
			if c.wantErr {
				if !errors.Is(err, ErrSwapFailed) {
					t.Fatalf("err = %v, want ErrSwapFailed", err)
				}
			} else if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}

	t.Run("导出包装走真实 os.Rename", func(t *testing.T) {
		dir := t.TempDir()
		src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
		if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RenameWithRetry(src, dst, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("rename 未生效: %v", err)
		}
		if err := RenameWithRetry(filepath.Join(dir, "缺失"), dst, []time.Duration{0}); !errors.Is(err, ErrSwapFailed) {
			t.Fatalf("err = %v, want ErrSwapFailed", err)
		}
	})
}

// TestWaitParentExit 父进程等待:alive 立即 false → nil 且不等待 poll;超时 → ErrParentWaitTimeout;第三次探测退出 → nil。
func TestWaitParentExit(t *testing.T) {
	t.Run("父进程已退出立即返回", func(t *testing.T) {
		start := time.Now()
		if err := WaitParentExit(7, 200*time.Millisecond, time.Second, func(int) bool { return false }); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d >= 100*time.Millisecond {
			t.Fatalf("耗时 %v, want < 100ms(不应等待 poll)", d)
		}
	})

	t.Run("超时返回等待应用退出超时", func(t *testing.T) {
		start := time.Now()
		err := WaitParentExit(7, 20*time.Millisecond, 120*time.Millisecond, func(int) bool { return true })
		if !errors.Is(err, ErrParentWaitTimeout) {
			t.Fatalf("err = %v, want ErrParentWaitTimeout", err)
		}
		if d := time.Since(start); d > 600*time.Millisecond {
			t.Fatalf("超时耗时 %v, want < 600ms", d)
		}
	})

	t.Run("第三次探测已退出", func(t *testing.T) {
		calls, seen := 0, 0
		err := WaitParentExit(42, time.Millisecond, time.Second, func(pid int) bool {
			calls++
			seen = pid
			return calls < 3
		})
		if err != nil {
			t.Fatal(err)
		}
		if calls != 3 || seen != 42 {
			t.Fatalf("探测次数 = %d(pid=%d), want 3 次且 pid=42", calls, seen)
		}
	})
}
