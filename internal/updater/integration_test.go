package updater

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestIntegrationReleaseChain 真实链路:GET v0.1.0 资产(github.com → release-assets.githubusercontent.com),
// 逐跳白名单校验;断言 ≥2 个 host 且全部合法。默认跳过;UPDATER_INTEGRATION=1 运行(需网络)。
func TestIntegrationReleaseChain(t *testing.T) {
	if os.Getenv("UPDATER_INTEGRATION") != "1" {
		t.Skip("设置 UPDATER_INTEGRATION=1 运行真实链路集成测试")
	}
	const assetURL = "https://github.com/sunsky74/gbt32960-simulator/releases/download/v0.1.0/gbt32960-simulator.app.zip"
	u, err := url.Parse(assetURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckDownloadURL(u); err != nil {
		t.Fatalf("初始 URL 不在白名单: %v", err)
	}
	hops := []string{u.Hostname()} // 含初始站,便于断言 ≥2 host
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			hops = append(hops, req.URL.Hostname())
			return CheckDownloadURL(req.URL) // 逐跳校验:任一跳越界即失败
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-65535") // 声明读取上限;实际只需少量字节验证链路
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	n, err := io.CopyN(io.Discard, resp.Body, 4096)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if n < 4096 {
		t.Fatalf("读取字节过少: %d", n)
	}
	if len(hops) < 2 || !strings.Contains(strings.Join(hops, ","), "release-assets.githubusercontent.com") {
		t.Fatalf("302 跳转链异常(期望 ≥2 host,含 release-assets):%v", hops)
	}
}

// TestIntegrationLatestRelease 真实链路:网页 302 路由解析最新 tag,并按命名契约构造资产直链。
// 默认跳过;UPDATER_INTEGRATION=1 运行(需网络)。
func TestIntegrationLatestRelease(t *testing.T) {
	if os.Getenv("UPDATER_INTEGRATION") != "1" {
		t.Skip("设置 UPDATER_INTEGRATION=1 运行真实链路集成测试")
	}
	rel, err := NewClient("v0.0.1").LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(rel.TagName) {
		t.Fatalf("TagName = %q, want vX.Y.Z", rel.TagName)
	}
	prefix := "https://github.com/" + Repo + "/releases/download/" + rel.TagName + "/"
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
	for i, name := range want {
		a := rel.Assets[i]
		if a.Name != name || a.URL != prefix+name {
			t.Fatalf("资产[%d] = %+v, want {Name:%s URL:%s}", i, a, name, prefix+name)
		}
	}
}
