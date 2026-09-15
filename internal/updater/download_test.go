package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
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
)

// ---- CheckDownloadURL 矩阵 ----

func TestCheckDownloadURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://api.github.com/repos/x/y", true},
		{"https://github.com/sunsky74/gbt32960-simulator/releases/download/v0.1.0/a.zip", true},
		{"https://release-assets.githubusercontent.com/github-production-release-asset/1/2", true},
		{"https://objects.githubusercontent.com/foo", true},
		{"http://github.com/foo", false},
		{"https://evil.com/foo", false},
		{"https://github.com.evil.com/foo", false},
	}
	for _, c := range cases {
		u, err := url.Parse(c.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := CheckDownloadURL(u) == nil; got != c.want {
			t.Errorf("CheckDownloadURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

// ---- Fetch:进度节流 / 停滞 / 取消 / 截断 ----

func newTestDownloader() *Downloader {
	d := NewDownloader("dev")
	// 测试放行 localhost(白名单逻辑已由 TestCheckDownloadURL 独立覆盖)
	d.CheckURL = func(*url.URL) error { return nil }
	d.HTTP = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return d.CheckURL(req.URL) },
	}
	return d
}

func TestFetchProgressThrottle(t *testing.T) {
	payload := strings.Repeat("x", 64*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		for i := 0; i < 4; i++ {
			_, _ = io.WriteString(w, payload[i*len(payload)/4:(i+1)*len(payload)/4])
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	t.Run("节流拉满时仅首末两发", func(t *testing.T) {
		d := newTestDownloader()
		d.ProgressInterval = time.Hour
		var got []Progress
		if _, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), func(p Progress) {
			got = append(got, p)
		}); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Received != 0 || got[len(got)-1].Received != int64(len(payload)) {
			t.Fatalf("节流事件异常: %+v", got)
		}
		if got[len(got)-1].Percent() != 100 {
			t.Fatalf("末次 Percent = %d, want 100", got[len(got)-1].Percent())
		}
	})

	t.Run("零间隔时每块都发", func(t *testing.T) {
		d := newTestDownloader()
		d.ProgressInterval = 0
		n := 0
		if _, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), func(Progress) {
			n++
		}); err != nil {
			t.Fatal(err)
		}
		if n <= 2 {
			t.Fatalf("事件数 = %d, want > 2", n)
		}
	})
}

func TestFetchStall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush() // 响应头先到、随后长时间无字节
		time.Sleep(800 * time.Millisecond)
	}))
	defer srv.Close()

	d := newTestDownloader()
	d.StallTimeout = 100 * time.Millisecond
	start := time.Now()
	_, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	if time.Since(start) > 600*time.Millisecond {
		t.Fatalf("停滞未及时判定: %v", time.Since(start))
	}
}

// TestFetchStallBeforeHeaders 覆盖响应头之前(建连/等待应答)的停滞窗口。
func TestFetchStallBeforeHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(800 * time.Millisecond) // 连接已接受、响应头不发
	}))
	defer srv.Close()

	d := newTestDownloader()
	d.StallTimeout = 100 * time.Millisecond
	start := time.Now()
	_, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	if time.Since(start) > 600*time.Millisecond {
		t.Fatalf("头前停滞未及时判定: %v", time.Since(start))
	}
}

func TestFetchStallTimerResetsOnBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 5; i++ {
			_, _ = io.WriteString(w, "tick")
			w.(http.Flusher).Flush()
			time.Sleep(40 * time.Millisecond) // 每次均远小于 StallTimeout,持续有进展
		}
	}))
	defer srv.Close()

	d := newTestDownloader()
	d.StallTimeout = 400 * time.Millisecond // 10× 余量,抗调度抖动
	if _, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil); err != nil {
		t.Fatalf("持续有字节仍被判停滞: %v", err)
	}
}

func TestFetchCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-block // 挂起直至测试结束
	}))
	defer func() { close(block); srv.Close() }()

	d := newTestDownloader()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := d.Fetch(ctx, srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("err = %v, want ErrCanceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消未生效")
	}
}

func TestFetchTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, strings.Repeat("y", 50)) // 实发少于声明
	}))
	defer srv.Close()

	d := newTestDownloader()
	_, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated", err)
	}
}

// TestFetchHostNotAllowedSentinel 白名单哨兵:非白名单初始 URL 直接拒绝(不发起网络)。
func TestFetchHostNotAllowedSentinel(t *testing.T) {
	d := NewDownloader("dev") // 生产默认 CheckURL(真实白名单)
	_, err := d.Fetch(context.Background(), "https://evil.com/x", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}

// TestErrorCopiesProxyHint 网络类文案含代理提示(D21)。
func TestErrorCopiesProxyHint(t *testing.T) {
	for name, e := range map[string]error{"ErrNetwork": ErrNetwork, "ErrDownload": ErrDownload} {
		msg := e.Error()
		if !strings.Contains(msg, "TUN") || !strings.Contains(msg, "HTTPS_PROXY") {
			t.Fatalf("%s 文案缺代理提示: %q", name, msg)
		}
	}
}

// redirectUserCache 把 os.UserCacheDir 重定向到测试临时目录(跨包并发测试隔离)。
func redirectUserCache(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "localappdata"))
}

// ---- DownloadReleaseArtifact:编排(验签→解析→下载→哈希) ----

// releaseFixture 构造三平台资产(+ 可选校验资产)的发布信息;资产 URL 指向 srvURL。
func releaseFixture(srvURL string, contentLen int64, withSums bool) *Release {
	rel := &Release{TagName: "v9.9.9"}
	rel.Assets = append(rel.Assets,
		Asset{Name: "gbt32960-simulator", Size: contentLen, URL: srvURL + "/linux"},
		Asset{Name: "gbt32960-simulator.app.zip", Size: contentLen, URL: srvURL + "/darwin"},
		Asset{Name: "gbt32960-simulator.exe", Size: contentLen, URL: srvURL + "/win"},
	)
	if withSums {
		rel.Assets = append(rel.Assets,
			Asset{Name: "SHA256SUMS", Size: 200, URL: srvURL + "/sums"},
			Asset{Name: "SHA256SUMS.sig", Size: 100, URL: srvURL + "/sig"},
		)
	}
	return rel
}

// sumsFor 生成覆盖三平台资产名的校验和文本(同一内容)。
func sumsFor(content []byte) string {
	var sb strings.Builder
	for _, n := range []string{"gbt32960-simulator", "gbt32960-simulator.app.zip", "gbt32960-simulator.exe"} {
		fmt.Fprintf(&sb, "%s  %s\n", sha256Hex(content), n)
	}
	return sb.String()
}

func TestDownloadReleaseArtifactHappyPath(t *testing.T) {
	content := []byte("artifact-bytes-for-test")
	pub, priv := newTestKey(t)
	sums := sumsFor(content)
	sig := signLine(t, priv, []byte(sums))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sums":
			_, _ = io.WriteString(w, sums)
		case "/sig":
			_, _ = w.Write(sig)
		default:
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	d := newTestDownloader()
	rel := releaseFixture(srv.URL, int64(len(content)), true)
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}
	var phases []string
	art, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, func(p Progress) {
		phases = append(phases, p.Phase)
	})
	if err != nil {
		t.Fatal(err)
	}
	if art.SHA256 != sha256Hex(content) {
		t.Fatalf("SHA256 = %s", art.SHA256)
	}
	if _, err := os.Stat(art.Path); err != nil {
		t.Fatalf("落定文件不存在: %v", err)
	}
	hasVerifying := false
	for _, p := range phases {
		if p == "verifying" {
			hasVerifying = true
		}
	}
	if !hasVerifying {
		t.Fatalf("缺 verifying 相位: %v", phases)
	}
}

// TestDownloadReleaseArtifactTotalFallback 进度事件 total 回退:响应无 Content-Length 时用 asset.Size。
func TestDownloadReleaseArtifactTotalFallback(t *testing.T) {
	content := []byte("fallback-bytes")
	pub, priv := newTestKey(t)
	sums := sumsFor(content)
	sig := signLine(t, priv, []byte(sums))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sums":
			_, _ = io.WriteString(w, sums)
		case "/sig":
			_, _ = w.Write(sig)
		default:
			w.(http.Flusher).Flush() // 分块响应:无 Content-Length
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	d := newTestDownloader()
	rel := releaseFixture(srv.URL, int64(len(content)), true)
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}
	lastTotal := int64(-1)
	_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, func(p Progress) {
		if p.Phase == "downloading" {
			lastTotal = p.Total
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if lastTotal != int64(len(content)) {
		t.Fatalf("下载进度 total = %d, want 回退 asset.Size = %d", lastTotal, len(content))
	}
}

func TestDownloadReleaseArtifactRejections(t *testing.T) {
	content := []byte("artifact-bytes")
	pub, priv := newTestKey(t)
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}
	goodSums := sumsFor(content)

	newSrv := func(sums, sig []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/sums":
				_, _ = w.Write(sums)
			case "/sig":
				_, _ = w.Write(sig)
			default:
				_, _ = w.Write(content)
			}
		}))
	}

	cases := []struct {
		name    string
		sums    []byte
		sig     []byte
		wantErr error
	}{
		{
			"签名被篡改",
			[]byte(goodSums),
			func() []byte { // 解码→翻位→重编码:确定性篡改签名内容
				line := strings.TrimSpace(string(signLine(t, priv, []byte(goodSums))))
				parts := strings.SplitN(line, " ", 2)
				raw, _ := base64.StdEncoding.DecodeString(parts[1])
				raw[0] ^= 0xFF
				return []byte(parts[0] + " " + base64.StdEncoding.EncodeToString(raw) + "\n")
			}(),
			ErrSigVerify,
		},
		{
			"未知 keyid",
			[]byte(goodSums),
			[]byte("deadbeef " + strings.SplitN(strings.TrimSpace(string(signLine(t, priv, []byte(goodSums)))), " ", 2)[1] + "\n"),
			ErrSigVerify,
		},
		{
			"哈希不匹配",
			[]byte(strings.ReplaceAll(goodSums, sha256Hex(content), strings.Repeat("0", 64))),
			nil, // 下面用篡改后的 sums 重新签名
			ErrHashMismatch,
		},
		{
			"缺签名文件",
			[]byte(goodSums),
			[]byte(""),
			ErrSigVerify,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sig := c.sig
			if sig == nil { // 哈希不匹配:用篡改后的 sums 正确签名(签名有效但哈希对不上)
				sig = signLine(t, priv, c.sums)
			}
			srv := newSrv(c.sums, sig)
			defer srv.Close()
			d := newTestDownloader()
			rel := releaseFixture(srv.URL, int64(len(content)), true)
			dir := t.TempDir()
			_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", dir, keys, nil)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if matches, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(matches) != 0 {
				t.Fatalf("拒绝路径 .part 未清理: %v", matches)
			}
		})
	}

	t.Run("缺失全部校验资产", func(t *testing.T) {
		srv := newSrv([]byte(goodSums), signLine(t, priv, []byte(goodSums)))
		defer srv.Close()
		d := newTestDownloader()
		rel := releaseFixture(srv.URL, int64(len(content)), false) // 无 SHA256SUMS/sig
		_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, nil)
		if !errors.Is(err, ErrChecksumsMissing) {
			t.Fatalf("err = %v, want ErrChecksumsMissing", err)
		}
	})

	t.Run("SHA256SUMS 返回 HTML 页(签名对 HTML 有效)", func(t *testing.T) {
		html := []byte("<!DOCTYPE html><html><body>rate limit page</body></html>")
		srv := newSrv(html, signLine(t, priv, html)) // 签名对 HTML 内容有效——形态校验必须先行拦截
		defer srv.Close()
		d := newTestDownloader()
		rel := releaseFixture(srv.URL, int64(len(content)), true)
		_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, nil)
		if !errors.Is(err, ErrChecksumsMissing) {
			t.Fatalf("err = %v, want ErrChecksumsMissing", err)
		}
	})

	t.Run("仅 SHA256SUMS 在列表(sig 资产缺失)", func(t *testing.T) {
		srv := newSrv([]byte(goodSums), signLine(t, priv, []byte(goodSums)))
		defer srv.Close()
		d := newTestDownloader()
		rel := releaseFixture(srv.URL, int64(len(content)), true)
		rel.Assets = rel.Assets[:len(rel.Assets)-1] // 去掉 SHA256SUMS.sig
		_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, nil)
		if !errors.Is(err, ErrChecksumsMissing) {
			t.Fatalf("err = %v, want ErrChecksumsMissing", err)
		}
	})
}

// ---- 目录与清理 ----

func TestReleaseDirAndCleanup(t *testing.T) {
	redirectUserCache(t)
	dir, err := ReleaseDir("release/v1.2.3") // tag 含斜杠 → 清洗
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(dir); strings.ContainsAny(base, `/\`) {
		t.Fatalf("tag 未被清洗: %s", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(filepath.Dir(dir), "last-result.json") // 根级文件应保留(P3 消费)
	if err := os.WriteFile(keep, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CleanupCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("tag 子目录未被清空")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("根级 last-result.json 应保留: %v", err)
	}
}

// ---- 评审 I-1 跟进:重定向拒绝方向的生产接线证据 ----

// TestFetchRejectsRedirectToNonWhitelistedHost 重定向跳转到非白名单主机:
// 经 NewDownloader 的生产 CheckRedirect 接线拒绝,哨兵透出(不被吞为 ErrDownload)。
func TestFetchRejectsRedirectToNonWhitelistedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.com/x", http.StatusFound)
	}))
	defer srv.Close()

	d := NewDownloader("dev")
	// 初始 URL 放行 localhost;跳转目标仍按真实白名单校验
	d.CheckURL = func(u *url.URL) error {
		if u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" {
			return nil
		}
		return CheckDownloadURL(u)
	}
	_, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}

// staticRoundTripper 返回预置响应(测试缝:伪造最终 URL)。
type staticRoundTripper struct{ resp *http.Response }

func (s staticRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return s.resp, nil }

// TestFetchRechecksFinalURL 注入无 CheckRedirect 的客户端时,最终 URL 复检仍拒绝非白名单地址。
func TestFetchRechecksFinalURL(t *testing.T) {
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "evil.com", Path: "/x"}}
	d := NewDownloader("dev")
	d.HTTP = &http.Client{Transport: staticRoundTripper{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("x")),
		Request:    req,
	}}}
	_, err := d.Fetch(context.Background(), "https://github.com/any", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}
