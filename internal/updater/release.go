package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// 检查链路的错误分类:文案即前端展示文案(bridge 层直接透出)。
var (
	// ErrNoRelease 仓库暂无 Release(404)。
	ErrNoRelease = errors.New("暂无发布版本")
	// ErrRateLimit GitHub 接口限流(403/429)。
	ErrRateLimit = errors.New("接口限流,请稍后再试")
	// ErrNetwork 网络不可达/超时/响应解析失败。
	ErrNetwork = errors.New("无法访问 GitHub,请检查网络")
)

// Repo 更新分发仓库(GitHub Releases 为唯一分发渠道)。
const Repo = "sunsky74/gbt32960-simulator"

// Client GitHub Releases 查询客户端;BaseURL/HTTP 可注入(测试用 httptest)。
type Client struct {
	BaseURL string
	HTTP    *http.Client
	ua      string
}

// NewClient 默认客户端:官方 API 端点 + 10s 超时;version 进入 User-Agent。
func NewClient(version string) *Client {
	ua := "gbt32960-simulator"
	if version != "" && version != "dev" {
		ua += "/" + version
	}
	return &Client{
		BaseURL: "https://api.github.com",
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		ua:      ua,
	}
}

// Release 检查所需的最小发布信息。
type Release struct {
	TagName     string
	Body        string
	PublishedAt string
	Assets      []Asset
}

// Asset 发布资产(更新产物)。
type Asset struct {
	Name string
	Size int64
	URL  string
}

// UpdateInfo 检查结果(前端展示契约,字段与设计文档 §5.1 对齐)。
type UpdateInfo struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	HasUpdate   bool   `json:"hasUpdate"`
	DevBuild    bool   `json:"devBuild"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"publishedAt"`
	AssetName   string `json:"assetName"`
	AssetSize   int64  `json:"assetSize"`
}

// latestReleaseDTO GitHub releases/latest 响应的子集。
type latestReleaseDTO struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// LatestRelease 查询最新版本(releases/latest 语义:排除 draft 与 prerelease)。
func (c *Client) LatestRelease(ctx context.Context) (*Release, error) {
	url := c.BaseURL + "/repos/" + Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, ErrNetwork
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrNetwork
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// 正常解析
	case http.StatusNotFound:
		return nil, ErrNoRelease
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, ErrRateLimit
	default:
		return nil, fmt.Errorf("GitHub 返回异常状态(%d)", resp.StatusCode)
	}

	var dto latestReleaseDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return nil, ErrNetwork
	}
	rel := &Release{TagName: dto.TagName, Body: dto.Body, PublishedAt: dto.PublishedAt}
	for _, a := range dto.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, Size: a.Size, URL: a.URL})
	}
	return rel, nil
}
