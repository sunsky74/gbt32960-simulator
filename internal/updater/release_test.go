package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestClient 注入 httptest 端点(检查链路的网络缝)。
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("v0.1.0")
	c.BaseURL = srv.URL
	return c
}

// TestLatestReleaseFollowsWebRoute 正常链路:302 不跟随,Location 的 tag 段成为版本号,资产按命名契约确定性构造。
func TestLatestReleaseFollowsWebRoute(t *testing.T) {
	var requests atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/"+Repo+"/releases/latest" {
			t.Errorf("请求路径 = %s", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != "gbt32960-simulator/v0.1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "" {
			t.Errorf("不应再携带 API Accept 头: %q", got)
		}
		http.Redirect(w, r, srv.URL+"/"+Repo+"/releases/tag/v0.2.0", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("v0.1.0")
	c.BaseURL = srv.URL

	rel, err := c.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v0.2.0" {
		t.Fatalf("TagName = %q, want v0.2.0", rel.TagName)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("请求次数 = %d, want 1(不得跟随重定向)", n)
	}

	want := []string{
		"gbt32960-simulator.app.zip",
		"gbt32960-simulator.exe",
		"gbt32960-simulator",
		"SHA256SUMS",
		"SHA256SUMS.sig",
	}
	if len(rel.Assets) != len(want) {
		t.Fatalf("资产数 = %d, want %d: %+v", len(rel.Assets), len(want), rel.Assets)
	}
	prefix := srv.URL + "/" + Repo + "/releases/download/v0.2.0/"
	for i, name := range want {
		a := rel.Assets[i]
		if a.Name != name || a.URL != prefix+name || a.Size != 0 {
			t.Fatalf("资产[%d] = %+v, want {Name:%s URL:%s%s Size:0}", i, a, name, prefix, name)
		}
	}
}

// TestLatestReleaseStatusClassification 状态码分类:404 → 暂无发布版本;403/429 → 限流。
func TestLatestReleaseStatusClassification(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNoRelease},
		{http.StatusForbidden, ErrRateLimit},
		{http.StatusTooManyRequests, ErrRateLimit},
	}
	for _, cse := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(cse.status)
		})
		if _, err := c.LatestRelease(context.Background()); !errors.Is(err, cse.want) {
			t.Fatalf("status=%d err=%v, want %v", cse.status, err, cse.want)
		}
	}
}

// TestLatestReleaseUnexpectedStatus 非 3xx 分类的状态(含 200)一律异常文案。
func TestLatestReleaseUnexpectedStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_, err := c.LatestRelease(context.Background())
	if err == nil || !strings.Contains(err.Error(), "GitHub 返回异常状态(200)") {
		t.Fatalf("err = %v, want 含 GitHub 返回异常状态(200)", err)
	}
}

// TestLatestReleaseNoTagRedirect 302 指向无 tag 段的 releases 列表页 → 暂无发布版本。
func TestLatestReleaseNoTagRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/"+Repo+"/releases", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("v0.1.0")
	c.BaseURL = srv.URL

	if _, err := c.LatestRelease(context.Background()); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestLatestReleaseRedirectRejections Location 形状异常(缺失/异站/非 tag 路径)fail-closed → ErrNetwork。
func TestLatestReleaseRedirectRejections(t *testing.T) {
	cases := []struct {
		name string
		loc  string
	}{
		{"缺 Location", ""},
		{"异站 Location", "https://evil.com/" + Repo + "/releases/tag/v9.9.9"},
		{"非 tag 路径", "/" + Repo + "/releases/download/v9.9.9"},
	}
	for _, cse := range cases {
		t.Run(cse.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if cse.loc != "" {
					w.Header().Set("Location", cse.loc)
				}
				w.WriteHeader(http.StatusFound)
			})
			if _, err := c.LatestRelease(context.Background()); !errors.Is(err, ErrNetwork) {
				t.Fatalf("err = %v, want ErrNetwork", err)
			}
		})
	}
}

// TestLatestReleaseNetworkError 连接失败 → ErrNetwork。
func TestLatestReleaseNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // 立即关闭 → 连接失败
	c := NewClient("v0.1.0")
	c.BaseURL = srv.URL
	if _, err := c.LatestRelease(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err=%v, want ErrNetwork", err)
	}
}
