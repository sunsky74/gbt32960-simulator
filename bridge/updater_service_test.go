package bridge

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gbt32960-simulator/internal/updater"
)

// latestPath 检查链路的请求路径(网页路由)。
const latestPath = "/" + updater.Repo + "/releases/latest"

// redirectLatest 以 302 指向 releases/tag/{tag}(检查链路不跟随重定向,Location 即 tag 来源)。
func redirectLatest(w http.ResponseWriter, r *http.Request, srvURL, tag string) {
	http.Redirect(w, r, srvURL+"/"+updater.Repo+"/releases/tag/"+tag, http.StatusFound)
}

func TestUpdaterServiceCurrentVersion(t *testing.T) {
	svc := NewUpdaterService("v1.2.3")
	if got := svc.CurrentVersion(); got != "v1.2.3" {
		t.Fatalf("CurrentVersion() = %q, want v1.2.3", got)
	}
}

func TestUpdaterServiceDevSkipsNetwork(t *testing.T) {
	for _, v := range []string{"", "dev"} {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		svc := NewUpdaterService(v)
		svc.client.BaseURL = srv.URL
		info, err := svc.CheckUpdate()
		srv.Close()
		if err != nil || !info.DevBuild {
			t.Fatalf("version=%q 应返回 DevBuild 结果: %+v, %v", v, info, err)
		}
		if called {
			t.Fatalf("version=%q 不应发起网络请求", v)
		}
	}
}

func TestUpdaterServiceCheckUpdate(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != latestPath {
			t.Errorf("请求路径 = %s", r.URL.Path)
		}
		redirectLatest(w, r, srv.URL, "v0.2.0")
	}))
	defer srv.Close()

	svc := NewUpdaterService("v0.1.0")
	svc.client.BaseURL = srv.URL
	info, err := svc.CheckUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasUpdate || info.Latest != "v0.2.0" {
		t.Fatalf("检查结果异常: %+v", info)
	}
	if info.AssetName == "" {
		t.Fatalf("资产应已匹配: %+v", info)
	}
	// 新检查使旧就绪产物复位(与前端 check() 复位一致)
	svc.ready = &updater.Artifact{Path: "stale", Tag: "v0.1.0"}
	if _, err := svc.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
	if svc.ready != nil {
		t.Fatalf("检查后 ready 应复位: %+v", svc.ready)
	}

	// 已是最新:tag 与当前一致
	svc2 := NewUpdaterService("v0.2.0")
	svc2.client.BaseURL = srv.URL
	info2, err := svc2.CheckUpdate()
	if err != nil || info2.HasUpdate {
		t.Fatalf("同版本不应提示更新: %+v, %v", info2, err)
	}
}

func TestUpdaterServiceErrorPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	svc := NewUpdaterService("v0.1.0")
	svc.client.BaseURL = srv.URL
	if _, err := svc.CheckUpdate(); err == nil || err.Error() != "暂无发布版本" {
		t.Fatalf("err = %v, want 暂无发布版本", err)
	}
}

// ==== Phase 2 追加:下载与校验 ====

func newTestSigning(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func hexSha256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// fixtureSums 生成覆盖三平台资产名的校验和文本(同一内容)。
func fixtureSums(content []byte) []byte {
	var sb strings.Builder
	for _, n := range []string{"gbt32960-simulator", "gbt32960-simulator.app.zip", "gbt32960-simulator.exe"} {
		fmt.Fprintf(&sb, "%s  %s\n", hexSha256(content), n)
	}
	return []byte(sb.String())
}

// sigLineFor 生成契约签名行 `<keyid> <base64(sig)>\n`。
func sigLineFor(t *testing.T, priv ed25519.PrivateKey, data []byte) []byte {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)
	return []byte(updater.KeyID(pub) + " " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
}

// redirectUserCache 把 os.UserCacheDir 重定向到测试临时目录(跨包并发隔离;三平台 env 覆盖)。
func redirectUserCache(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "localappdata"))
}

func newDownloadTestService(serverURL string) (*UpdaterService, *[]progressDTO) {
	svc := NewUpdaterService("v0.1.0")
	svc.client.BaseURL = serverURL
	svc.downloader.CheckURL = func(*url.URL) error { return nil } // 放行 localhost
	svc.ctx = context.Background()
	events := &[]progressDTO{}
	svc.emit = func(_ context.Context, name string, data ...any) {
		if name != "update:progress" {
			return
		}
		*events = append(*events, data[0].(progressDTO))
	}
	return svc, events
}

func TestUpdaterServiceDownloadAndVerify(t *testing.T) {
	redirectUserCache(t)
	content := []byte("artifact-bytes-for-bridge-test")
	pub, priv := newTestSigning(t)
	sums := fixtureSums(content)
	sig := sigLineFor(t, priv, sums)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == latestPath:
			redirectLatest(w, r, srv.URL, "v0.2.0")
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			_, _ = w.Write(sums)
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS.sig"):
			_, _ = w.Write(sig)
		default:
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()
	t.Cleanup(func() { _ = updater.CleanupCache() })

	svc, events := newDownloadTestService(srv.URL)
	svc.keys = map[string]ed25519.PublicKey{updater.KeyID(pub): pub}

	if _, err := svc.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
	res, err := svc.DownloadUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if res.Tag != "v0.2.0" || res.AssetName == "" || res.SHA256 != hexSha256(content) {
		t.Fatalf("结果异常: %+v", res)
	}
	if len(*events) < 2 {
		t.Fatalf("事件过少: %+v", *events)
	}
	// verifying 以实测字节数为进度基准:Size 恒为 0 也不再出现 0% 复位
	last := (*events)[len(*events)-1]
	if last.Phase != "verifying" || last.Percent != 100 {
		t.Fatalf("末条事件异常: %+v", last)
	}
	dir, _ := updater.ReleaseDir("v0.2.0")
	fi, err := os.Stat(filepath.Join(dir, res.AssetName))
	if err != nil {
		t.Fatalf("落定文件不存在: %v", err)
	}
	if fi.Size() != int64(len(content)) {
		t.Fatalf("落定文件大小 = %d, want %d", fi.Size(), len(content))
	}
	if svc.ready == nil || svc.ready.Path != filepath.Join(dir, res.AssetName) || svc.ready.Tag != "v0.2.0" {
		t.Fatalf("ready 未留存: %+v", svc.ready)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(matches) != 0 {
		t.Fatalf(".part 残留: %v", matches)
	}
}

func TestUpdaterServiceDownloadCancel(t *testing.T) {
	redirectUserCache(t)
	content := []byte("artifact")
	pub, priv := newTestSigning(t)
	sums := fixtureSums(content)
	sig := sigLineFor(t, priv, sums)
	block := make(chan struct{})
	assetStarted := make(chan struct{})

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == latestPath:
			redirectLatest(w, r, srv.URL, "v0.2.0")
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			_, _ = w.Write(sums)
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS.sig"):
			_, _ = w.Write(sig)
		default:
			w.Header().Set("Content-Length", "9999999")
			w.(http.Flusher).Flush()
			close(assetStarted)
			<-block // 挂起直至测试结束
		}
	}))
	defer func() { close(block); srv.Close() }()
	t.Cleanup(func() { _ = updater.CleanupCache() })

	svc, _ := newDownloadTestService(srv.URL)
	svc.keys = map[string]ed25519.PublicKey{updater.KeyID(pub): pub}
	if _, err := svc.CheckUpdate(); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.DownloadUpdate()
		errCh <- err
	}()
	<-assetStarted // 确认已进入资产下载
	svc.CancelDownload()
	select {
	case err := <-errCh:
		if err == nil || err.Error() != "已取消下载" {
			t.Fatalf("err = %v, want 已取消下载", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消未生效")
	}
	dir, _ := updater.ReleaseDir("v0.2.0")
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(matches) != 0 {
		t.Fatalf(".part 残留: %v", matches)
	}
}

func TestUpdaterServiceCleanupCache(t *testing.T) {
	redirectUserCache(t)
	dir, err := updater.ReleaseDir("v0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644)
	keep := filepath.Join(filepath.Dir(dir), "helper.log")
	_ = os.WriteFile(keep, []byte("log"), 0o644)

	svc := NewUpdaterService("v1.0.0")
	svc.cleanupCache()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("tag 子目录未清空")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("根级 helper.log 应保留: %v", err)
	}
}

func TestUpdaterServiceDownloadGuards(t *testing.T) {
	svc := NewUpdaterService("v1.0.0")
	if _, err := svc.DownloadUpdate(); err == nil || err.Error() != "请先检查更新" {
		t.Fatalf("未检查应先报错: %v", err)
	}
	dev := NewUpdaterService("dev")
	if _, err := dev.DownloadUpdate(); err == nil || err.Error() != "开发构建不参与更新" {
		t.Fatalf("dev 应拒绝: %v", err)
	}
	eq := NewUpdaterService("v1.0.0")
	eq.lastRelease = &updater.Release{TagName: "v1.0.0"} // 同版本:不得进入下载
	if _, err := eq.DownloadUpdate(); err == nil || err.Error() != "已是最新版本,无需下载" {
		t.Fatalf("同版本应拒绝: %v", err)
	}
}

// TestUpdaterServiceErrorClassification 锁定检查链路分类(spec §5.8,网页 302 语义):
// 403 限流原样透出;302 缺 Location 等形状异常 fail-closed 为网络错误;302 指向 releases 列表页为暂无发布版本。
func TestUpdaterServiceErrorClassification(t *testing.T) {
	t.Run("403 限流", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"API rate limit exceeded for 1.2.3.4"}`)
		}))
		defer srv.Close()
		svc := NewUpdaterService("v0.1.0")
		svc.client.BaseURL = srv.URL
		_, err := svc.CheckUpdate()
		if err == nil || err.Error() != "接口限流,请稍后再试" {
			t.Fatalf("err = %v, want 接口限流,请稍后再试", err)
		}
	})

	t.Run("302 缺 Location", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusFound)
		}))
		defer srv.Close()
		svc := NewUpdaterService("v0.1.0")
		svc.client.BaseURL = srv.URL
		_, err := svc.CheckUpdate()
		if !errors.Is(err, updater.ErrNetwork) {
			t.Fatalf("err = %v, want ErrNetwork", err)
		}
	})

	t.Run("302 指向 releases 列表", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, srv.URL+"/"+updater.Repo+"/releases", http.StatusFound)
		}))
		defer srv.Close()
		svc := NewUpdaterService("v0.1.0")
		svc.client.BaseURL = srv.URL
		_, err := svc.CheckUpdate()
		if err == nil || err.Error() != "暂无发布版本" {
			t.Fatalf("err = %v, want 暂无发布版本", err)
		}
	})
}

// ==== Phase 3 追加:应用服务(ApplyUpdate / ConsumeLastResult / 启动清理扩展) ====

// applyTestFixture 构造"已就绪"的应用服务:真实产物文件 / 新版本 lastRelease /
// 目标与预检注入 / spawn 与 quit 注入(测试绝不真的退出进程或拉起 helper)。
type applyTestFixture struct {
	svc       *UpdaterService
	artifact  string
	target    string
	spawnArgs []updater.HelperArgs
	spawnErr  error
	quitN     atomic.Int32
	quitCh    chan struct{}
}

func newApplyFixture(t *testing.T) *applyTestFixture {
	t.Helper()
	redirectUserCache(t)
	f := &applyTestFixture{
		svc:    NewUpdaterService("v0.1.0"),
		target: t.TempDir(),
		quitCh: make(chan struct{}),
	}
	f.artifact = filepath.Join(t.TempDir(), "gbt32960-simulator.app.zip")
	if err := os.WriteFile(f.artifact, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.svc.ready = &updater.Artifact{Path: f.artifact, Name: filepath.Base(f.artifact), Tag: "v0.2.0"}
	f.svc.lastRelease = &updater.Release{TagName: "v0.2.0"}
	f.svc.resolveTarget = func() (string, error) { return f.target, nil }
	f.svc.preflight = func(string) error { return nil }
	f.svc.quitDelay = 10 * time.Millisecond
	f.svc.spawn = func(a updater.HelperArgs) error {
		f.spawnArgs = append(f.spawnArgs, a)
		return f.spawnErr
	}
	f.svc.quit = func(context.Context) {
		if f.quitN.Add(1) == 1 {
			close(f.quitCh)
		}
	}
	return f
}

// waitQuit 等待退出信号(≤1s);waitQuiet 观察 quit 是否被误调用。
func (f *applyTestFixture) waitQuit(t *testing.T) {
	t.Helper()
	select {
	case <-f.quitCh:
	case <-time.After(time.Second):
		t.Fatal("延迟退出未触发")
	}
}

func (f *applyTestFixture) assertQuitCount(t *testing.T, want int32) {
	t.Helper()
	time.Sleep(50 * time.Millisecond) // 观察窗口:晚到的第二次 quit 也应被抓到
	if n := f.quitN.Load(); n != want {
		t.Fatalf("quit 调用 %d 次, want %d", n, want)
	}
}

func TestUpdaterServiceApplyGuards(t *testing.T) {
	// 守卫顺序即契约:dev → 下载中 → 进行中 → 未就绪(内存无产物 / 文件丢失)→ 版本。
	cases := []struct {
		name  string
		setup func(t *testing.T) *UpdaterService
		want  string
	}{
		{
			name: "dev 构建",
			setup: func(t *testing.T) *UpdaterService {
				return NewUpdaterService("dev")
			},
			want: "开发构建不参与更新",
		},
		{
			name: "下载中",
			setup: func(t *testing.T) *UpdaterService {
				svc := NewUpdaterService("v0.1.0")
				svc.downloading = true
				return svc
			},
			want: "更新正在下载中,请稍候",
		},
		{
			name: "应用进行中(二次调用)",
			setup: func(t *testing.T) *UpdaterService {
				svc := NewUpdaterService("v0.1.0")
				svc.applying = true
				return svc
			},
			want: "更新已在进行中",
		},
		{
			name: "无就绪产物",
			setup: func(t *testing.T) *UpdaterService {
				return NewUpdaterService("v0.1.0")
			},
			want: "更新包未就绪,请先下载",
		},
		{
			name: "就绪产物文件丢失",
			setup: func(t *testing.T) *UpdaterService {
				svc := NewUpdaterService("v0.1.0")
				svc.ready = &updater.Artifact{Path: filepath.Join(t.TempDir(), "gone.zip"), Tag: "v0.2.0"}
				return svc
			},
			want: "更新包未就绪,请先下载",
		},
		{
			name: "同版本",
			setup: func(t *testing.T) *UpdaterService {
				svc := NewUpdaterService("v1.0.0")
				p := filepath.Join(t.TempDir(), "same.zip")
				if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				svc.ready = &updater.Artifact{Path: p, Tag: "v1.0.0"}
				svc.lastRelease = &updater.Release{TagName: "v1.0.0"}
				return svc
			},
			want: "已是最新版本,无需安装",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			redirectUserCache(t)
			svc := tc.setup(t)
			// 守卫回归若放行:立刻暴露,绝不真的 spawn/退出
			svc.spawn = func(updater.HelperArgs) error { t.Fatal("守卫应拦截,不应到 spawn 阶段"); return nil }
			svc.quit = func(context.Context) { t.Fatal("守卫应拦截,不得退出应用") }
			if err := svc.ApplyUpdate(); err == nil || err.Error() != tc.want {
				t.Fatalf("ApplyUpdate() err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestUpdaterServiceApplySpawnsAndQuits(t *testing.T) {
	f := newApplyFixture(t)
	wantResult, err := updater.LastResultPath()
	if err != nil {
		t.Fatal(err)
	}
	wantLog, err := updater.HelperLogPath()
	if err != nil {
		t.Fatal(err)
	}
	// 预置陈旧结果:ApplyUpdate 必须在 spawn 前清除(防上次残留误报)
	if err := updater.WriteLastResult(updater.LastResult{OK: true, TargetVersion: "v0.1.0", LogPath: wantLog}); err != nil {
		t.Fatal(err)
	}
	spawnCheck := f.svc.spawn
	f.svc.spawn = func(a updater.HelperArgs) error {
		if _, err := os.Stat(wantResult); !os.IsNotExist(err) {
			t.Errorf("spawn 前结果文件应已清除: %v", err)
		}
		return spawnCheck(a)
	}

	if err := f.svc.ApplyUpdate(); err != nil {
		t.Fatalf("ApplyUpdate() = %v", err)
	}
	f.waitQuit(t)
	f.assertQuitCount(t, 1)

	if len(f.spawnArgs) != 1 {
		t.Fatalf("spawn 调用 %d 次, want 1", len(f.spawnArgs))
	}
	got := f.spawnArgs[0]
	if got.ParentPID != os.Getpid() {
		t.Errorf("ParentPID = %d, want %d", got.ParentPID, os.Getpid())
	}
	if got.Artifact != f.artifact {
		t.Errorf("Artifact = %q, want %q", got.Artifact, f.artifact)
	}
	if got.Target != f.target {
		t.Errorf("Target = %q, want %q", got.Target, f.target)
	}
	if got.Tag != "v0.2.0" {
		t.Errorf("Tag = %q, want v0.2.0", got.Tag)
	}
	if got.Result != wantResult {
		t.Errorf("Result = %q, want %q", got.Result, wantResult)
	}
	if got.Log != wantLog {
		t.Errorf("Log = %q, want %q", got.Log, wantLog)
	}
	if !f.svc.applying {
		t.Error("spawn 成功后 applying 应为 true")
	}
	if _, err := os.Stat(wantResult); !os.IsNotExist(err) {
		t.Errorf("结果文件应保持清除: %v", err)
	}
}

func TestUpdaterServiceApplySpawnFailure(t *testing.T) {
	f := newApplyFixture(t)
	f.spawnErr = errors.New("boom")
	if err := f.svc.ApplyUpdate(); err == nil || err.Error() != "无法启动更新进程,请手动更新" {
		t.Fatalf("err = %v, want 无法启动更新进程,请手动更新", err)
	}
	f.assertQuitCount(t, 0)
	if f.svc.applying {
		t.Fatal("spawn 失败 applying 必须保持 false")
	}
	// 失败不锁死:修复后可再次应用
	f.spawnErr = nil
	if err := f.svc.ApplyUpdate(); err != nil {
		t.Fatalf("重试 ApplyUpdate() = %v", err)
	}
	f.waitQuit(t)
	f.assertQuitCount(t, 1)
	if len(f.spawnArgs) != 2 {
		t.Fatalf("spawn 调用 %d 次, want 2", len(f.spawnArgs))
	}
}

func TestUpdaterServiceApplyPreflightGuidance(t *testing.T) {
	f := newApplyFixture(t)
	// 平台中性预检错误:平台专属预检哨兵仅在各自构建标签下定义,
	// 本测试须跨平台编译,故以本地错误等价验证"指引文案原文透出、不退出应用"。
	preflightErr := errors.New("预检失败(测试)")
	f.svc.preflight = func(string) error { return preflightErr }
	err := f.svc.ApplyUpdate()
	if !errors.Is(err, preflightErr) || err.Error() != preflightErr.Error() {
		t.Fatalf("err = %v, want 预检指引原文透出", err)
	}
	if len(f.spawnArgs) != 0 {
		t.Fatalf("预检失败不得 spawn: %+v", f.spawnArgs)
	}
	f.assertQuitCount(t, 0)
	if f.svc.applying {
		t.Fatal("预检失败 applying 必须保持 false")
	}
}

func TestUpdaterServiceConsumeLastResult(t *testing.T) {
	redirectUserCache(t)
	resultPath, err := updater.LastResultPath()
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUpdaterService("v1.0.0")

	t.Run("无文件", func(t *testing.T) {
		res, err := svc.ConsumeLastResult()
		if err != nil || res.Present {
			t.Fatalf("res = %+v, err = %v, want 无记录", res, err)
		}
	})

	t.Run("失败结果读取即清除", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "helper.log")
		if err := updater.WriteLastResult(updater.LastResult{OK: false, TargetVersion: "v0.2.0", Reason: "替换失败", LogPath: logPath}); err != nil {
			t.Fatal(err)
		}
		res, err := svc.ConsumeLastResult()
		if err != nil {
			t.Fatal(err)
		}
		if !res.Present || res.OK || res.TargetVersion != "v0.2.0" || res.Reason != "替换失败" || res.LogPath != logPath {
			t.Fatalf("res = %+v", res)
		}
		if _, err := os.Stat(resultPath); !os.IsNotExist(err) {
			t.Fatalf("结果文件应已清除: %v", err)
		}
		res2, err := svc.ConsumeLastResult()
		if err != nil || res2.Present {
			t.Fatalf("二次消费 res = %+v, err = %v, want Present=false", res2, err)
		}
	})

	t.Run("成功结果", func(t *testing.T) {
		if err := updater.WriteLastResult(updater.LastResult{OK: true, TargetVersion: "v0.2.0", LogPath: "log"}); err != nil {
			t.Fatal(err)
		}
		res, err := svc.ConsumeLastResult()
		if err != nil || !res.Present || !res.OK {
			t.Fatalf("res = %+v, err = %v, want OK=true", res, err)
		}
		if _, err := os.Stat(resultPath); !os.IsNotExist(err) {
			t.Fatalf("结果文件应已清除: %v", err)
		}
	})

	t.Run("损坏 JSON", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(resultPath, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(resultPath)
		if _, err := svc.ConsumeLastResult(); err == nil {
			t.Fatal("损坏 JSON 应报错")
		}
	})
}

func TestUpdaterServiceStartupCleanup(t *testing.T) {
	redirectUserCache(t)
	dir, err := updater.ReleaseDir("v0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644)

	svc := NewUpdaterService("v1.0.0")
	WireUpdaterStartup(svc) // 测试二进制非 .app:CleanupStaleBackups 报错须被吞掉(不 panic)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("tag 子目录未清空")
	}
}
