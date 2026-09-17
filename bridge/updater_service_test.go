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
