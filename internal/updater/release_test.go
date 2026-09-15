package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const fixtureJSON = `{
  "tag_name": "v0.2.0",
  "body": "## 更新说明\n- 修复若干问题",
  "published_at": "2026-09-14T08:00:00Z",
  "assets": [
    {"name": "gbt32960-simulator", "size": 14155776, "browser_download_url": "https://example.com/linux"},
    {"name": "gbt32960-simulator-amd64-installer.exe", "size": 7920000, "browser_download_url": "https://example.com/nsis"},
    {"name": "gbt32960-simulator.app.zip", "size": 11430000, "browser_download_url": "https://example.com/mac"},
    {"name": "gbt32960-simulator.exe", "size": 15940000, "browser_download_url": "https://example.com/win"}
  ]
}`

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("v0.1.0")
	c.BaseURL = srv.URL
	return c
}

func TestLatestReleaseParsesFields(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases/latest" {
			t.Errorf("路径 = %s", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != "gbt32960-simulator/v0.1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		_, _ = w.Write([]byte(fixtureJSON))
	})
	rel, err := c.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v0.2.0" || len(rel.Assets) != 4 {
		t.Fatalf("解析结果异常: %+v", rel)
	}
	if rel.Assets[2].Name != "gbt32960-simulator.app.zip" || rel.Assets[2].Size != 11430000 {
		t.Fatalf("资产解析异常: %+v", rel.Assets[2])
	}
}

func TestLatestReleaseErrorClassification(t *testing.T) {
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
		_, err := c.LatestRelease(context.Background())
		if !errors.Is(err, cse.want) {
			t.Fatalf("status=%d err=%v, want %v", cse.status, err, cse.want)
		}
	}
}

func TestLatestReleaseNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // 立即关闭 → 连接失败
	c := NewClient("v0.1.0")
	c.BaseURL = srv.URL
	if _, err := c.LatestRelease(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err=%v, want ErrNetwork", err)
	}
}
