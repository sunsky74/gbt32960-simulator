package bridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const updaterFixtureJSON = `{
  "tag_name": "v0.2.0",
  "body": "更新说明",
  "published_at": "2026-09-14T08:00:00Z",
  "assets": [
    {"name": "gbt32960-simulator", "size": 100, "browser_download_url": "https://example.com/l"},
    {"name": "gbt32960-simulator.app.zip", "size": 200, "browser_download_url": "https://example.com/m"},
    {"name": "gbt32960-simulator.exe", "size": 300, "browser_download_url": "https://example.com/w"}
  ]
}`

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(updaterFixtureJSON))
	}))
	defer srv.Close()

	svc := NewUpdaterService("v0.1.0")
	svc.client.BaseURL = srv.URL
	info, err := svc.CheckUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasUpdate || info.Latest != "v0.2.0" || info.Notes != "更新说明" {
		t.Fatalf("检查结果异常: %+v", info)
	}
	if info.AssetName == "" || info.AssetSize == 0 {
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
