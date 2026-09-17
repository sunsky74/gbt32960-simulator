package updater

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 检查链路的错误分类:文案即前端展示文案(bridge 层直接透出)。
var (
	// ErrNoRelease 仓库暂无 Release(404,或 302 指向无 tag 段的 releases 列表页)。
	ErrNoRelease = errors.New("暂无发布版本")
	// ErrRateLimit GitHub 接口限流(403/429)。
	ErrRateLimit = errors.New("接口限流,请稍后再试")
	// ErrNetwork 网络不可达/超时/响应解析失败(附代理提示,D21)。
	ErrNetwork = errors.New("无法访问 GitHub,请检查网络(如使用代理,请确认 TUN 模式或 HTTPS_PROXY 生效)")
)

// Repo 更新分发仓库(GitHub Releases 为唯一分发渠道)。
const Repo = "sunsky74/gbt32960-simulator"

// Client GitHub Releases 查询客户端;BaseURL/HTTP 可注入(测试用 httptest)。
type Client struct {
	BaseURL string
	HTTP    *http.Client
	ua      string
}

// userAgent 构造 User-Agent(dev/空版本不带版本号)。
func userAgent(version string) string {
	ua := "gbt32960-simulator"
	if version != "" && version != "dev" {
		ua += "/" + version
	}
	return ua
}

// NewClient 默认客户端:网页端点 + 10s 超时;version 进入 User-Agent。
// 不跟随重定向:releases/latest 的 302 Location 即版本 tag 来源(网页路由不消耗 API 配额)。
func NewClient(version string) *Client {
	return &Client{
		BaseURL: "https://github.com",
		HTTP: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		ua: userAgent(version),
	}
}

// Release 检查所需的最小发布信息。
type Release struct {
	TagName string
	Assets  []Asset
}

// Asset 发布资产(更新产物)。
type Asset struct {
	Name string
	Size int64
	URL  string
}

// UpdateInfo 检查结果(前端展示契约,字段与设计文档 §5.1 对齐)。
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"hasUpdate"`
	DevBuild  bool   `json:"devBuild"`
	AssetName string `json:"assetName"`
}

// LatestRelease 查询最新版本:GET {BaseURL}/{Repo}/releases/latest,由 302 Location 的 tag 段解析版本号。
// 网页路由语义同 API(releases/latest 排除 draft 与 prerelease),且不消耗 API 配额。
func (c *Client) LatestRelease(ctx context.Context) (*Release, error) {
	rawURL := c.BaseURL + "/" + Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrNetwork
	}
	req.Header.Set("User-Agent", c.ua)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrNetwork
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNoRelease
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, ErrRateLimit
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		// 302:Location 指向 /{Repo}/releases/tag/{tag}
	default:
		return nil, fmt.Errorf("GitHub 返回异常状态(%d)", resp.StatusCode)
	}

	tag, err := releaseTag(resp)
	if err != nil {
		return nil, err
	}
	return &Release{TagName: tag, Assets: releaseAssets(c.BaseURL, tag)}, nil
}

// releaseTag 从 302 Location 解析版本 tag:仅信任同站地址与 tag 路径契约,形状异常一律 fail-closed。
func releaseTag(resp *http.Response) (string, error) {
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", ErrNetwork
	}
	u, err := url.Parse(loc)
	if err != nil {
		return "", ErrNetwork
	}
	if u.Host != resp.Request.URL.Host { // 仅接受同站跳转(绝对 Location)
		return "", ErrNetwork
	}
	path := u.EscapedPath()
	prefix := "/" + Repo + "/releases/tag/"
	if !strings.HasPrefix(path, prefix) {
		// 无 tag 段(如 /{Repo}/releases):仓库尚无已发布版本,等同 404
		if path == "/"+Repo+"/releases" {
			return "", ErrNoRelease
		}
		return "", ErrNetwork
	}
	tag, err := url.PathUnescape(strings.TrimPrefix(path, prefix))
	if err != nil || tag == "" {
		return "", ErrNetwork
	}
	return tag, nil
}
