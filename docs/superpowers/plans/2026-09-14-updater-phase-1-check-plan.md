# 应用内更新 · Phase 1「版本与检查」实施计划

> **For agentic workers:** Recommended execution: use superpowers:ltdd for Quality-LTDD(推荐)。Alternatives: superpowers:subagent-driven-development(轻量)或 superpowers:executing-plans(内联检查点)。Steps use checkbox (`- [ ]`) syntax for tracking。

**Goal:** 交付"版本与检查"只读闭环:应用内显示版本(ldflags 接线)、手动检查 GitHub Releases(含发布说明/资产大小/跳过版本)、启动自动检查(延迟 3s/24h 冷却/静默失败/toast 可跳转关于)。零下载、零替换。

**Architecture:** 新增 `internal/updater` 纯逻辑包(semver 比较 / GitHub Releases 客户端 / 平台资产匹配,全部标准库、HTTP 可注入);新增 `bridge.UpdaterService` 绑定面(仅 2 个无参方法);前端新增"关于"设置分类面板 + 启动自动检查接线 + 轻量跨页跳转。`go.mod` 零变化。

**Tech Stack:** Go 1.25 / Wails v2.15 / Vue 3 + Ant Design Vue / vitest + @vue/test-utils / Golang httptest。

> Phase 引用:`docs/superpowers/plans/2026-09-14-updater-master-plan.md`;设计文档 §5.1/§5.2/§5.6(契约不得偏离)。

## Global Constraints

- `go.mod` 零变化;不新增第三方依赖(标准库 + 现有前端依赖)
- 不改变既有服务与页面行为;后端 `go vet ./...` + `go test ./... -count=1` + `go test -race ./...`;前端 `npm run lint` + `npm run typecheck` + `npm test -- --run` + `npm run build` 全绿
- 仅 HTTPS 访问 `api.github.com`(本期唯一出网域名);User-Agent 必填
- Git:所有操作前缀 `GIT_MASTER=1`;Conventional Commits(中文摘要);逐任务提交
- 注释/提交信息/文档全中文;错误文案直接使用中文分类文案("暂无发布版本"/"接口限流,请稍后再试"/"无法访问 GitHub,请检查网络"/"当前平台暂不支持自动更新")

## Phase Final Acceptance Checklist (Refined from Spec) - MUST

- [ ] [PAC-1] (Source: Overall Business Flow;← Global AC-1) 版本接线与显示:注入版本后 `CurrentVersion()` 返回注入值,关于面板显示 `vX.Y.Z`;未注入显示"开发构建"(检查按钮禁用)。
  Refinement: 单测 `TestUpdaterServiceCurrentVersion`(注入 `v1.2.3`);本地端到端 `wails build -ldflags "-X main.version=v9.9.9"` 运行 build/bin 应用 → 关于面板显示 `v9.9.9`;对照 `wails build`(默认)→ 显示"开发构建";边界:空串与 `dev` 两态均命中开发构建分支。
- [ ] [PAC-2] (Source: Current Requirement Flow;← Global AC-2) 手动检查与错误分类:200(有新/最新)/404/限流/网络错误分类正确;发布说明与资产大小随结果返回;dev 不发网络请求。
  Refinement: `go test ./internal/updater/ -v`(semver/release/asset 全过);`go test ./bridge/ -run TestUpdaterService -v` 全过——httptest 断言:404→`暂无发布版本`、403/429→`接口限流,请稍后再试`、连接失败→`无法访问 GitHub,请检查网络`、dev 时 handler 未被调用;手工:关于面板点"检查更新"(真实网络;若本机出口 IP 触发匿名限流则记录后跳过,网络逻辑已由 httptest 覆盖)。
- [ ] [PAC-3] (Source: Current Requirement Flow;← Global AC-3) 自动检查策略:开关默认开、首启到期、24h 冷却边界、失败静默、跳过版本(自动静默/手动可见)、toast 可点击跳转"设置→关于"。
  Refinement: `npm test -- --run src/composables/useUpdater.test.ts` 全过(边界:恰 24h 到期、`lastUpdateCheckAt=0` 首启、时钟回拨抑制、跳过命中/未命中);组件测试 `SettingsAboutPanel.test.ts`(检查→展示新版本与说明→跳过写入 `skippedVersion`);手工:`wails build -ldflags "-X main.version=v0.0.1"` 运行 → 约 3s 后 toast"发现新版本";点击 → 设置页打开且"关于"分类激活;点"跳过此版本"→ 重启不再提示;更高版本发布后恢复提示(与 PAC 终验在 P3/P4 演练)。
- [ ] [PAC-4] (Source: Development Architecture;← Global AC-9) 绑定面契约:`UpdaterService` 仅暴露 `CurrentVersion`/`CheckUpdate`(均无参数,不暴露 URL/路径);ctx 经 `WireContexts` 注入;绑定再生成产物含 `updater` 模型。
  Refinement: `grep -n "func (s \*UpdaterService)" bridge/updater_service.go` 恰 2 条且无参数;`grep -n "namespace updater" frontend/wailsjs/go/models.ts` 非空;`grep -n "updater \*UpdaterService" bridge/wiring.go` 非空。
- [ ] [PAC-5] (Source: Existing Architecture Fit;← Global AC-10/11) 回归与纪律:后端三件套 + 前端四件套全绿;`go.mod` 零变化。
  Refinement: `go vet ./... && go test ./... -count=1 && go test -race ./...` 全 exit 0;`cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build` 全 exit 0;`git diff --stat $(git merge-base HEAD origin/main)..HEAD -- go.mod go.sum` 输出为空。

---

### Task 1: `internal/updater` 版本比较纯函数

**Level:** L2
**Level Rationale:** 新包纯函数(无副作用、未接线),局部行为;其解析规则是后续检查链路的比较基准。
**Linked Acceptance Items:** PAC-2
**Task Gate:** task reviewer + focused checks(本任务单测)

**Files:**
- Create: `internal/updater/semver.go`
- Create: `internal/updater/semver_test.go`

**Interfaces:**
- Consumes: 无(标准库 `strconv`/`strings`)。
- Produces: `CompareVersions(a, b string) (int, bool)`;`IsNewer(latest, current string) bool`(包级,后续 Task 2/3 依赖)。

- [ ] **Step 1: 写失败测试**

创建 `internal/updater/semver_test.go`:

```go
package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v0.1.0", "v0.1.1", -1, true},
		{"v0.2.0", "v0.1.9", 1, true},
		{"1.0.0", "v1.0.0", 0, true},
		{"v1.2.10", "v1.2.9", 1, true}, // 数值比较而非字典序
		{" v0.1.0 ", "v0.1.0", 0, true}, // 容忍首尾空白
		{"v0.1.0", "dev", 0, false},     // 不可解析
		{"", "v0.1.0", 0, false},
		{"v1.2", "v1.2.0", 0, false},         // 段数不足
		{"v0.2.0-rc1", "v0.2.0", 0, false},   // 预发布后缀严格拒绝(fail-closed)
		{"va.b.c", "v1.0.0", 0, false},       // 非数字段
		{"v-1.0.0", "v1.0.0", 0, false},      // 负数段
	}
	for _, c := range cases {
		got, ok := CompareVersions(c.a, c.b)
		if ok != c.ok {
			t.Fatalf("CompareVersions(%q,%q) ok=%v, want %v", c.a, c.b, ok, c.ok)
		}
		if ok && got != c.want {
			t.Fatalf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.2.0", "dev", false}, // dev 不提示更新
		{"v0.2.0", "", false},
		{"bad-tag", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Fatalf("IsNewer(%q,%q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/updater/ -run "TestCompareVersions|TestIsNewer" -v`
Expected: FAIL —— `undefined: CompareVersions`(包尚无源码,或编译失败)。

- [ ] **Step 3: 最小实现**

创建 `internal/updater/semver.go`:

```go
// Package updater 应用内更新的纯逻辑:版本比较、GitHub Releases 查询、平台资产匹配。
// 本包不 import wails;网络客户端可注入以支持测试。
package updater

import (
	"strconv"
	"strings"
)

// 版本比较契约:仅接受 "vX.Y.Z"(X/Y/Z 为非负整数,v 前缀可省,首尾空白容忍)。
// 不可解析输入一律 ok=false —— 检查链路对不可解析输入 fail-closed(不提示更新)。

// CompareVersions 比较 a、b:-1(a<b)/ 0(相等)/ 1(a>b);任一不可解析时 ok=false。
func CompareVersions(a, b string) (int, bool) {
	am, ai, ap, okA := parseVersion(a)
	bm, bi, bp, okB := parseVersion(b)
	if !okA || !okB {
		return 0, false
	}
	switch {
	case am != bm:
		return cmpInt(am, bm), true
	case ai != bi:
		return cmpInt(ai, bi), true
	default:
		return cmpInt(ap, bp), true
	}
}

// IsNewer 判断 latest 是否严格新于 current;不可解析时返回 false(不提示更新)。
func IsNewer(latest, current string) bool {
	c, ok := CompareVersions(latest, current)
	return ok && c > 0
}

// parseVersion 解析 "vX.Y.Z";段数不为 3 或存在非数字/负数段时 ok=false。
func parseVersion(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/updater/ -run "TestCompareVersions|TestIsNewer" -v`
Expected: PASS(两个测试全过)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater
GIT_MASTER=1 git commit -m "feat(updater): 版本比较纯函数(semver,严格解析 fail-closed)"
```

---

### Task 2: GitHub Releases 查询客户端与平台资产匹配

**Level:** L2
**Level Rationale:** 新包内新增网络客户端(可注入 baseURL)与纯匹配函数;局部行为,完全由 focused 测试覆盖。
**Linked Acceptance Items:** PAC-2
**Task Gate:** task reviewer + focused checks(httptest 与表驱动测试)

**Files:**
- Create: `internal/updater/release.go`
- Create: `internal/updater/asset.go`
- Create: `internal/updater/release_test.go`
- Create: `internal/updater/asset_test.go`

**Interfaces:**
- Consumes: `CompareVersions`/`IsNewer`(Task 1,本任务暂不直接调用)。
- Produces: `NewClient(version string) *Client`;`(*Client).LatestRelease(ctx) (*Release, error)`;`UpdateInfo`;`Asset`/`Release` 结构;`MatchAsset(goos, goarch string, assets []Asset) (Asset, error)`;哨兵错误 `ErrNoRelease`/`ErrRateLimit`/`ErrNetwork`/`ErrUnsupportedPlatform`(后续 Task 3 依赖)。

- [ ] **Step 1: 写失败测试**

创建 `internal/updater/release_test.go`:

```go
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
```

创建 `internal/updater/asset_test.go`:

```go
package updater

import (
	"errors"
	"testing"
)

var sampleAssets = []Asset{
	{Name: "gbt32960-simulator", Size: 14155776},
	{Name: "gbt32960-simulator-amd64-installer.exe", Size: 7920000},
	{Name: "gbt32960-simulator.app.zip", Size: 11430000},
	{Name: "gbt32960-simulator.exe", Size: 15940000},
}

func TestMatchAsset(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
	}{
		{"darwin", "arm64", "gbt32960-simulator.app.zip"},
		{"darwin", "amd64", "gbt32960-simulator.app.zip"},
		{"windows", "amd64", "gbt32960-simulator.exe"}, // 精确匹配天然排除 installer
		{"linux", "amd64", "gbt32960-simulator"},
	}
	for _, c := range cases {
		got, err := MatchAsset(c.goos, c.goarch, sampleAssets)
		if err != nil || got.Name != c.want {
			t.Fatalf("MatchAsset(%s,%s) = %q,%v want %q", c.goos, c.goarch, got.Name, err, c.want)
		}
	}
}

func TestMatchAssetUnsupported(t *testing.T) {
	for _, c := range [][2]string{{"windows", "arm64"}, {"linux", "arm64"}, {"freebsd", "amd64"}} {
		if _, err := MatchAsset(c[0], c[1], sampleAssets); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Fatalf("MatchAsset(%s,%s) err = %v", c[0], c[1], err)
		}
	}
	if _, err := MatchAsset("darwin", "arm64", nil); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal("空资产列表应返回 ErrUnsupportedPlatform")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/updater/ -run "TestLatestRelease|TestMatchAsset" -v`
Expected: FAIL —— `undefined: NewClient` / `undefined: MatchAsset`。

- [ ] **Step 3: 最小实现(release.go)**

创建 `internal/updater/release.go`:

```go
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
```

- [ ] **Step 4: 最小实现(asset.go)**

创建 `internal/updater/asset.go`:

```go
package updater

import "errors"

// ErrUnsupportedPlatform 当前平台无匹配的更新产物。
var ErrUnsupportedPlatform = errors.New("当前平台暂不支持自动更新")

// expectedAssetName 发布资产命名契约(设计文档 §5.2;精确匹配,fail-closed):
//   darwin(universal) → gbt32960-simulator.app.zip
//   windows/amd64     → gbt32960-simulator.exe(精确匹配天然排除 *-installer.exe)
//   linux/amd64       → gbt32960-simulator(无扩展名)
func expectedAssetName(goos, goarch string) (string, bool) {
	switch {
	case goos == "darwin":
		return "gbt32960-simulator.app.zip", true
	case goos == "windows" && goarch == "amd64":
		return "gbt32960-simulator.exe", true
	case goos == "linux" && goarch == "amd64":
		return "gbt32960-simulator", true
	default:
		return "", false
	}
}

// MatchAsset 从发布资产中选出当前平台的更新产物;无匹配时返回 ErrUnsupportedPlatform。
func MatchAsset(goos, goarch string, assets []Asset) (Asset, error) {
	want, ok := expectedAssetName(goos, goarch)
	if !ok {
		return Asset{}, ErrUnsupportedPlatform
	}
	for _, a := range assets {
		if a.Name == want {
			return a, nil
		}
	}
	return Asset{}, ErrUnsupportedPlatform
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/updater/ -v`
Expected: PASS(semver + release + asset 全部测试)。

- [ ] **Step 6: 提交**

```bash
GIT_MASTER=1 git add internal/updater
GIT_MASTER=1 git commit -m "feat(updater): GitHub Releases 查询客户端与平台资产匹配(错误分类/httptest 覆盖)"
```

---

### Task 3: `bridge.UpdaterService` 与装配接线

**Level:** L3
**Level Rationale:** 跨模块接线(main.go 版本变量 → app.go 装配 → wiring ctx 注入 → bridge 绑定面),且绑定面即用户可见"检查更新"能力的端点;须绑定 PAC。
**Linked Acceptance Items:** PAC-1, PAC-2, PAC-4
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `bridge/updater_service.go`
- Create: `bridge/updater_service_test.go`
- Modify: `main.go`(新增 `var version`、Bind 追加)
- Modify: `app.go`(字段 + NewApp + startup WireContexts)
- Modify: `bridge/wiring.go`(WireContexts 增加 updater 形参)

**Interfaces:**
- Consumes: Task 1/2 的 `updater.NewClient` / `updater.UpdateInfo` / `updater.MatchAsset` / `updater.IsNewer`。
- Produces: `bridge.NewUpdaterService(version string) *UpdaterService`;`(*UpdaterService).CurrentVersion() string`;`(*UpdaterService).CheckUpdate() (updater.UpdateInfo, error)`(前端 Task 5/6 依赖;绑定生成物 `frontend/wailsjs/go/bridge/UpdaterService`)。

- [ ] **Step 1: 写失败测试**

创建 `bridge/updater_service_test.go`:

```go
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
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./bridge/ -run TestUpdaterService -v`
Expected: FAIL —— `undefined: NewUpdaterService`。

- [ ] **Step 3: 最小实现(updater_service.go)**

创建 `bridge/updater_service.go`:

```go
package bridge

import (
	"context"
	"runtime"
	"time"

	"gbt32960-simulator/internal/updater"
)

// UpdaterService 应用内更新服务:查询 GitHub Releases 最新版本。
// Phase 1 只读:CurrentVersion / CheckUpdate;下载与替换在后续 Phase 接入。
type UpdaterService struct {
	ctx     context.Context
	version string
	client  *updater.Client
}

// NewUpdaterService 创建更新服务(version 为 ldflags 注入版本,dev 表示本地构建)。
func NewUpdaterService(version string) *UpdaterService {
	return &UpdaterService{version: version, client: updater.NewClient(version)}
}

// CurrentVersion 当前应用版本(未注入时为 "dev")。
func (s *UpdaterService) CurrentVersion() string {
	return s.version
}

// CheckUpdate 查询最新版本并与当前版本比较;dev/空版本不发请求(直接返回 DevBuild)。
func (s *UpdaterService) CheckUpdate() (updater.UpdateInfo, error) {
	if s.version == "" || s.version == "dev" {
		return updater.UpdateInfo{Current: s.version, DevBuild: true}, nil
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rel, err := s.client.LatestRelease(ctx)
	if err != nil {
		return updater.UpdateInfo{}, err
	}
	info := updater.UpdateInfo{
		Current:     s.version,
		Latest:      rel.TagName,
		HasUpdate:   updater.IsNewer(rel.TagName, s.version),
		Notes:       rel.Body,
		PublishedAt: rel.PublishedAt,
	}
	// 资产匹配失败不阻塞检查结果(前端据 AssetName 空值提示"暂不支持自动更新")
	if asset, aerr := updater.MatchAsset(runtime.GOOS, runtime.GOARCH, rel.Assets); aerr == nil {
		info.AssetName = asset.Name
		info.AssetSize = asset.Size
	}
	return info, nil
}
```

- [ ] **Step 4: 装配接线(main.go / app.go / wiring.go)**

修改 `main.go`:在 `var assets embed.FS` 之后新增:

```go
// version 由发布流程经 -ldflags "-X main.version=${GITHUB_REF_NAME}" 注入;未注入(本地开发)为 dev。
var version = "dev"
```

并在 `Bind` 列表末尾追加 `app.updater,`(位于 `app.settings,` 之后)。

修改 `app.go`:
1. 结构体在 `settings  *bridge.SettingsService` 之后新增字段:

```go
	updater   *bridge.UpdaterService
```

2. `NewApp()` 返回值在 `settings:  bridge.NewSettingsService(fwd),` 之后新增:

```go
		updater:   bridge.NewUpdaterService(version),
```

3. `startup` 中 WireContexts 调用末尾追加 `a.updater`:

```go
	bridge.WireContexts(ctx, a.console, a.extsvc, a.sys, a.server, a.track, a.updater)
```

修改 `bridge/wiring.go`:WireContexts 增加形参与注入:

```go
// WireContexts 把 wails 上下文注入各服务(原生对话框等能力需要)。
func WireContexts(ctx context.Context, console *ConsoleService, ext *ExtService, sys *SystemService, server *ServerService, track *TrackService, updater *UpdaterService) {
	console.ctx = ctx
	ext.ctx = ctx
	sys.ctx = ctx
	server.ctx = ctx
	track.ctx = ctx
	updater.ctx = ctx
}
```

- [ ] **Step 5: 运行确认通过 + 生成前端绑定**

Run: `go vet ./... && go test ./bridge/ -run TestUpdaterService -v`
Expected: PASS(4 个测试全过)。

Run: `wails generate module`
Expected: 无报错;生成 `frontend/wailsjs/go/bridge/UpdaterService.d.ts`、`UpdaterService.js`;`frontend/wailsjs/go/models.ts` 出现 `updater` 命名空间。

验证: `grep -n "namespace updater" frontend/wailsjs/go/models.ts` 输出非空。

- [ ] **Step 6: 提交**

```bash
GIT_MASTER=1 git add bridge/updater_service.go bridge/updater_service_test.go bridge/wiring.go main.go app.go frontend/wailsjs
GIT_MASTER=1 git commit -m "feat(updater): bridge 更新服务与装配接线(CurrentVersion/CheckUpdate,ldflags 版本接线)"
```

---

### Task 4: 前端设置字段与更新共享逻辑

**Level:** L2
**Level Rationale:** 前端本地行为(设置模型字段 + 纯函数策略),由 vitest 直接覆盖;不触 UI。
**Linked Acceptance Items:** PAC-3
**Task Gate:** task reviewer + focused checks(useUpdater 单测)

**Files:**
- Modify: `frontend/src/composables/useAppSettings.ts`(接口 + DEFAULTS + persist watcher)
- Create: `frontend/src/composables/useUpdater.ts`
- Create: `frontend/src/composables/useUpdater.test.ts`

**Interfaces:**
- Consumes: `appSettings`(useAppSettings)。
- Produces: `UPDATE_CHECK_COOLDOWN_MS`/`UPDATE_CHECK_DELAY_MS` 常量;`isDevVersion`/`shouldAutoCheck`/`shouldPrompt`/`formatBytes`/`markChecked`/`skipVersion`(Task 5/6 依赖)。

- [ ] **Step 1: 写失败测试**

创建 `frontend/src/composables/useUpdater.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { appSettings } from './useAppSettings'
import {
  UPDATE_CHECK_COOLDOWN_MS,
  formatBytes,
  isDevVersion,
  markChecked,
  shouldAutoCheck,
  shouldPrompt,
  skipVersion,
} from './useUpdater'

describe('isDevVersion', () => {
  it('dev/空串视为开发构建', () => {
    expect(isDevVersion('dev')).toBe(true)
    expect(isDevVersion('')).toBe(true)
    expect(isDevVersion('v0.1.0')).toBe(false)
  })
})

describe('shouldAutoCheck', () => {
  const base = { checkUpdateOnStartup: true, lastUpdateCheckAt: 0 }
  it('开关关闭时不检查', () => {
    expect(shouldAutoCheck({ ...base, checkUpdateOnStartup: false }, 1e12)).toBe(false)
  })
  it('首启(从未检查)到期', () => {
    expect(shouldAutoCheck(base, 1e12)).toBe(true)
  })
  it('24h 内不重复检查;恰满 24h 到期;时钟回拨保持抑制', () => {
    const now = 1e12
    expect(shouldAutoCheck({ ...base, lastUpdateCheckAt: now - UPDATE_CHECK_COOLDOWN_MS + 1 }, now)).toBe(false)
    expect(shouldAutoCheck({ ...base, lastUpdateCheckAt: now - UPDATE_CHECK_COOLDOWN_MS }, now)).toBe(true)
    expect(shouldAutoCheck({ ...base, lastUpdateCheckAt: now + 60000 }, now)).toBe(false)
  })
})

describe('shouldPrompt', () => {
  it('被跳过的版本不提示,更高版本恢复提示', () => {
    expect(shouldPrompt({ skippedVersion: 'v0.2.0' }, 'v0.2.0')).toBe(false)
    expect(shouldPrompt({ skippedVersion: 'v0.2.0' }, 'v0.2.1')).toBe(true)
    expect(shouldPrompt({ skippedVersion: '' }, 'v0.2.0')).toBe(true)
  })
})

describe('formatBytes', () => {
  it('B/KB/MB 分档', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(11430000)).toBe('10.9 MB')
  })
})

describe('markChecked / skipVersion', () => {
  it('写入设置字段', () => {
    markChecked(123)
    expect(appSettings.lastUpdateCheckAt).toBe(123)
    skipVersion('v9.9.9')
    expect(appSettings.skippedVersion).toBe('v9.9.9')
  })
})
```

- [ ] **Step 2: 运行确认失败**

Run: `npm test -- --run src/composables/useUpdater.test.ts`
Expected: FAIL —— 无法解析 `./useUpdater`。

- [ ] **Step 3: 扩展 useAppSettings(接口 / DEFAULTS / 持久化)**

修改 `frontend/src/composables/useAppSettings.ts`:

1. `AppSettings` 接口追加字段:

```ts
  // 更新:启动自动检查开关(默认开)
  checkUpdateOnStartup: boolean
  // 更新:上次检查完成时间戳 ms(0=从未;自动检查 24h 冷却用;成功失败统一记录)
  lastUpdateCheckAt: number
  // 更新:用户跳过的版本号(空串=未跳过;更高版本发布后自动失效)
  skippedVersion: string
```

2. `DEFAULTS` 追加:

```ts
  checkUpdateOnStartup: true,
  lastUpdateCheckAt: 0,
  skippedVersion: '',
```

3. 在现有 watcher 区末尾追加:

```ts
// 更新相关字段:变更即持久化(与上方各设置项同一模式)
watch(
  () => [appSettings.checkUpdateOnStartup, appSettings.lastUpdateCheckAt, appSettings.skippedVersion] as const,
  () => persist(),
)
```

- [ ] **Step 4: 最小实现(useUpdater.ts)**

创建 `frontend/src/composables/useUpdater.ts`:

```ts
// 更新领域的共享常量与纯函数(设置面板与启动检查共用;不做任何网络调用)。
import { appSettings, type AppSettings } from './useAppSettings'

// 自动检查冷却:24h(与设计文档 §5.6 一致)
export const UPDATE_CHECK_COOLDOWN_MS = 24 * 60 * 60 * 1000
// 启动自动检查延迟:避开启动高峰
export const UPDATE_CHECK_DELAY_MS = 3000

// 是否为开发构建(不参与更新检查)
export function isDevVersion(v: string): boolean {
  return v === '' || v === 'dev'
}

// 自动检查是否应触发:开关开启 且 距上次检查 ≥ 24h(首启 last=0 视为到期;时钟回拨保持抑制)
export function shouldAutoCheck(
  settings: Pick<AppSettings, 'checkUpdateOnStartup' | 'lastUpdateCheckAt'>,
  now: number,
): boolean {
  if (!settings.checkUpdateOnStartup) return false
  return now - settings.lastUpdateCheckAt >= UPDATE_CHECK_COOLDOWN_MS
}

// 发现新版本时是否提示:未被用户跳过
export function shouldPrompt(settings: Pick<AppSettings, 'skippedVersion'>, latest: string): boolean {
  return settings.skippedVersion !== latest
}

// 字节数展示(资产大小)
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

// 记录一次检查完成时间戳(成功与失败统一口径:自动检查 24h 冷却)
export function markChecked(now: number) {
  appSettings.lastUpdateCheckAt = now
}

// 跳过指定版本(自动检查对其静默;更高版本发布后自动恢复提示)
export function skipVersion(latest: string) {
  appSettings.skippedVersion = latest
}
```

- [ ] **Step 5: 运行确认通过**

Run: `npm test -- --run src/composables/useUpdater.test.ts`
Expected: PASS(6 个 describe 全过);同时 `npm run typecheck` 通过。

- [ ] **Step 6: 提交**

```bash
GIT_MASTER=1 git add frontend/src/composables/useAppSettings.ts frontend/src/composables/useUpdater.ts frontend/src/composables/useUpdater.test.ts
GIT_MASTER=1 git commit -m "feat(frontend): 更新设置字段与共享逻辑(24h 冷却/跳过版本/字节格式化)"
```

---

### Task 5: 设置「关于」分类与更新检查面板

**Level:** L3
**Level Rationale:** 用户可见 UI 与交互流(版本展示/检查/跳过),是 PAC-1/2/3 的直接验收面。
**Linked Acceptance Items:** PAC-1, PAC-2, PAC-3
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `frontend/src/components/settings/SettingsAboutPanel.vue`
- Create: `frontend/src/components/settings/SettingsAboutPanel.test.ts`
- Modify: `frontend/src/pages/SettingsPage.vue`(分类注册 + 面板分支)

**Interfaces:**
- Consumes: Task 3 绑定 `UpdaterService.CurrentVersion/CheckUpdate`;Task 4 的 `isDevVersion/formatBytes/skipVersion`;`appSettings`。
- Produces: 设置第 9 分类 `about`("关于");面板组件(后续 Task 6 的 toast 跳转以此为落点)。

- [ ] **Step 1: 写失败测试**

创建 `frontend/src/components/settings/SettingsAboutPanel.test.ts`:

```ts
// SettingsAboutPanel:版本展示 / 检查更新 / 跳过版本(UpdaterService 全 mock)
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const checkUpdate = vi.fn()
let currentVersion = 'v0.1.0'
vi.mock('../../../wailsjs/go/bridge/UpdaterService', () => ({
  CurrentVersion: vi.fn(async () => currentVersion),
  CheckUpdate: (...args: unknown[]) => checkUpdate(...args),
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  BrowserOpenURL: vi.fn(),
}))

import SettingsAboutPanel from './SettingsAboutPanel.vue'
import { appSettings } from '../../composables/useAppSettings'

// a-button 渲染为真实按钮,便于点击与文本断言
const stubs = {
  'a-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
}

function findBtn(wrapper: ReturnType<typeof mount>, text: string) {
  const btn = wrapper.findAll('button').find((b) => b.text().includes(text))
  if (!btn) throw new Error(`未找到按钮: ${text}`)
  return btn
}

describe('SettingsAboutPanel', () => {
  beforeEach(() => {
    currentVersion = 'v0.1.0'
    checkUpdate.mockReset()
    appSettings.skippedVersion = ''
    checkUpdate.mockResolvedValue({
      current: 'v0.1.0',
      latest: 'v0.2.0',
      hasUpdate: true,
      devBuild: false,
      notes: '本次更新说明',
      assetName: 'gbt32960-simulator.app.zip',
      assetSize: 11430000,
    })
  })

  it('展示当前版本;检查后展示新版本/说明;可跳过版本', async () => {
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('v0.1.0')

    await findBtn(wrapper, '检查更新').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('v0.2.0')
    expect(wrapper.text()).toContain('本次更新说明')
    expect(wrapper.text()).toContain('10.9 MB')

    await findBtn(wrapper, '跳过此版本').trigger('click')
    expect(appSettings.skippedVersion).toBe('v0.2.0')
  })

  it('开发构建:显示标签且检查按钮禁用', async () => {
    currentVersion = 'dev'
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('开发构建')
    expect(findBtn(wrapper, '检查更新').attributes('disabled')).toBeDefined()
  })
})
```

- [ ] **Step 2: 运行确认失败**

Run: `npm test -- --run src/components/settings/SettingsAboutPanel.test.ts`
Expected: FAIL —— 无法解析 `./SettingsAboutPanel.vue`。

- [ ] **Step 3: 实现面板组件**

创建 `frontend/src/components/settings/SettingsAboutPanel.vue`:

```vue
<script setup lang="ts">
// 关于:版本展示与更新检查入口(Phase 1 仅检查;下载/安装后续 Phase 接入)。
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { GithubOutlined } from '@ant-design/icons-vue'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import * as UpdaterService from '../../../wailsjs/go/bridge/UpdaterService'
import { updater } from '../../../wailsjs/go/models'
import { formatBytes, isDevVersion, skipVersion } from '../../composables/useUpdater'
import SettingRow from './SettingRow.vue'

const REPO_URL = 'https://github.com/sunsky74/gbt32960-simulator'

const current = ref('')
const devBuild = ref(false)
const checking = ref(false)
const info = ref<updater.UpdateInfo | null>(null)
const notesExpanded = ref(false)

onMounted(async () => {
  try {
    current.value = await UpdaterService.CurrentVersion()
  } catch {
    current.value = ''
  }
  devBuild.value = isDevVersion(current.value)
})

async function check() {
  checking.value = true
  notesExpanded.value = false
  try {
    info.value = await UpdaterService.CheckUpdate()
  } catch (e) {
    message.error(String(e))
  } finally {
    checking.value = false
  }
}

function skip() {
  const latest = info.value?.latest
  if (!latest) return
  skipVersion(latest)
  message.success(`已跳过 ${latest},更高版本发布后将再次提示`)
}

function openRepo() {
  BrowserOpenURL(REPO_URL)
}

const hasUpdate = computed(() => info.value?.hasUpdate === true)
const upToDate = computed(() => info.value !== null && !info.value.hasUpdate && !info.value.devBuild)
const platformUnsupported = computed(() => hasUpdate.value && !info.value?.assetName)
</script>

<template>
  <div class="row-group">
    <SettingRow title="当前版本" description="版本由发布流程注入;本地开发构建显示为 dev">
      <template #action>
        <span class="about-version">{{ devBuild ? '开发构建' : current || '未知' }}</span>
      </template>
    </SettingRow>

    <SettingRow title="检查更新">
      <template #description>
        <div class="about-result">
          <span v-if="devBuild" class="about-hint">开发构建不参与更新检查(发布版本以 v 开头的版本号显示)</span>
          <span v-else-if="checking" class="about-hint">正在检查…</span>
          <span v-else-if="!info" class="about-hint">从 GitHub Releases 查询最新版本</span>
          <template v-else-if="hasUpdate">
            <span class="about-new">
              发现新版本 <b>{{ info.latest }}</b>
              <template v-if="info.assetSize">({{ formatBytes(info.assetSize) }})</template>
            </span>
            <span v-if="platformUnsupported" class="about-hint">当前平台暂不支持自动更新</span>
            <div v-if="info.notes" class="about-notes" :class="{ expanded: notesExpanded }">
              <pre>{{ info.notes }}</pre>
            </div>
            <div class="about-actions">
              <a-button v-if="info.notes" size="small" type="link" @click="notesExpanded = !notesExpanded">
                {{ notesExpanded ? '收起说明' : '展开说明' }}
              </a-button>
              <a-button size="small" @click="skip">跳过此版本</a-button>
            </div>
          </template>
          <span v-else-if="upToDate" class="about-hint">已是最新版本</span>
        </div>
      </template>
      <template #action>
        <a-button size="small" :loading="checking" :disabled="devBuild" @click="check">检查更新</a-button>
      </template>
    </SettingRow>

    <SettingRow title="GitHub 仓库" description="查看发布记录与源码">
      <template #action>
        <a-button size="small" @click="openRepo">
          <template #icon><GithubOutlined /></template>
          打开仓库
        </a-button>
      </template>
    </SettingRow>
  </div>
</template>

<style scoped>
.about-version {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-primary);
}

.about-result {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.about-hint {
  color: var(--text-tertiary);
}

.about-new {
  color: var(--text-primary);
}

.about-notes {
  max-height: 132px;
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 6px 10px;
  background: var(--bg-elevated);
}

.about-notes.expanded {
  max-height: none;
}

.about-notes pre {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
}

.about-actions {
  display: flex;
  gap: 8px;
}
</style>
```

- [ ] **Step 4: 注册「关于」分类**

修改 `frontend/src/pages/SettingsPage.vue`:

1. 图标导入追加 `InfoCircleOutlined`(保持既有导入排序风格,按现有顺序并入):

```ts
import {
  ApiOutlined,
  CodeOutlined,
  DatabaseOutlined,
  ExperimentOutlined,
  GlobalOutlined,
  InfoCircleOutlined,
  SettingOutlined,
  SearchOutlined,
  ToolOutlined,
} from '@ant-design/icons-vue'
```

2. 面板导入追加:

```ts
import SettingsAboutPanel from '../components/settings/SettingsAboutPanel.vue'
```

3. `categories` 数组末尾追加:

```ts
  { key: 'about', title: '关于', icon: markRaw(InfoCircleOutlined) },
```

4. 面板渲染链末尾(`advanced` 分支之后)追加:

```vue
          <!-- ================ 关于 ================ -->
          <SettingsAboutPanel v-else-if="activeCategory === 'about'" />
```

- [ ] **Step 5: 运行确认通过**

Run: `npm test -- --run src/components/settings/SettingsAboutPanel.test.ts && npm run typecheck`
Expected: PASS(2 个用例);typecheck 无错误。

- [ ] **Step 6: 提交**

```bash
GIT_MASTER=1 git add frontend/src/components/settings/SettingsAboutPanel.vue frontend/src/components/settings/SettingsAboutPanel.test.ts frontend/src/pages/SettingsPage.vue
GIT_MASTER=1 git commit -m "feat(frontend): 设置「关于」分类与更新检查面板(版本/说明折叠/跳过版本)"
```

---

### Task 6: 启动自动检查、toast 跳转与「启动时检查更新」转正

**Level:** L3
**Level Rationale:** 用户可见的启动行为与跨页导航流(PAC-3 的直接验收面),且改动 App 壳层与两个设置面板。
**Linked Acceptance Items:** PAC-3
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `frontend/src/composables/settingsFocus.ts`
- Create: `frontend/src/composables/settingsFocus.test.ts`
- Modify: `frontend/src/App.vue`(自动检查 + focus 监听)
- Modify: `frontend/src/components/settings/SettingsCommonPanel.vue`(占位转正)
- Modify: `frontend/src/pages/SettingsPage.vue`(消费 focus 目标)

**Interfaces:**
- Consumes: Task 4 的 `shouldAutoCheck/shouldPrompt/markChecked/UPDATE_CHECK_DELAY_MS`;Task 3 的 `UpdaterService.CheckUpdate`;Task 5 的 `about` 分类。
- Produces: `settingsFocusCategory`/`openSettingsCategory`(toast 跳转机制)。

- [ ] **Step 1: 写失败测试(跳转机制)**

创建 `frontend/src/composables/settingsFocus.test.ts`:

```ts
import { beforeEach, describe, expect, it } from 'vitest'
import { openSettingsCategory, settingsFocusCategory } from './settingsFocus'

describe('settingsFocus', () => {
  beforeEach(() => {
    settingsFocusCategory.value = null
  })

  it('openSettingsCategory 写入目标分类', () => {
    openSettingsCategory('about')
    expect(settingsFocusCategory.value).toBe('about')
  })
})
```

创建 `frontend/src/composables/settingsFocus.ts`:

```ts
import { ref } from 'vue'

// 跨页跳转目标:设置页需聚焦的分类 key。
// 写入方:openSettingsCategory(toast「发现新版本」点击等);消费方:SettingsPage(消费后置空)。
export const settingsFocusCategory = ref<string | null>(null)

// 请求打开 设置 → 指定分类(实际导航切换由 App.vue 监听该 ref 完成)。
export function openSettingsCategory(key: string) {
  settingsFocusCategory.value = key
}
```

- [ ] **Step 2: 运行确认跳转机制测试通过**

Run: `npm test -- --run src/composables/settingsFocus.test.ts`
Expected: PASS。若先跑实现文件缺失则先 FAIL 再建文件(此步骤内完成)。

- [ ] **Step 3: App.vue 接线(自动检查 + focus 监听)**

修改 `frontend/src/App.vue` script:

1. 追加导入:

```ts
import { message } from 'ant-design-vue'
import * as UpdaterService from '../wailsjs/go/bridge/UpdaterService'
import { appSettings, rememberNavPage, restoreLastNavPage } from './composables/useAppSettings'
import { UPDATE_CHECK_DELAY_MS, markChecked, shouldAutoCheck, shouldPrompt } from './composables/useUpdater'
import { openSettingsCategory, settingsFocusCategory } from './composables/settingsFocus'
```

(注意:`appSettings` 并入既有 `useAppSettings` 导入行,不重复 import 语句。)

2. 在既有"启动恢复上次工作区"块之后追加:

```ts
// 跨页跳转:settingsFocusCategory 被写入时切到设置页(具体分类由 SettingsPage 消费)
watch(settingsFocusCategory, (k) => {
  if (k) activeNavKey.value = 'settings'
})

// 启动自动检查:延迟 ~3s、24h 冷却、失败静默(设置「常用 → 启动时检查更新」控制)
onMounted(() => {
  window.setTimeout(async () => {
    const now = Date.now()
    if (!shouldAutoCheck(appSettings, now)) return
    try {
      const info = await UpdaterService.CheckUpdate()
      if (!info.devBuild && info.hasUpdate && shouldPrompt(appSettings, info.latest)) {
        message.info({
          content: `发现新版本 ${info.latest},点击查看`,
          onClick: () => openSettingsCategory('about'),
        })
      }
    } catch {
      // 静默失败:不打扰;冷却期内不重试(手动检查始终可用)
    } finally {
      markChecked(now)
    }
  }, UPDATE_CHECK_DELAY_MS)
})
```

- [ ] **Step 4: SettingsCommonPanel 占位行转正**

修改 `frontend/src/components/settings/SettingsCommonPanel.vue`:

1. `plannedRows` 数组移除 `启动时检查更新` 一行(仅保留其余 3 条占位)。
2. 模板中在"启动时恢复上次工作区"行之后插入真实设置行:

```vue
    <SettingRow title="启动时检查更新" description="启动后后台检查新版本并提示(失败不打扰;24h 冷却;可跳过指定版本)">
      <template #action>
        <a-switch v-model:checked="appSettings.checkUpdateOnStartup" size="small" />
      </template>
    </SettingRow>
```

- [ ] **Step 5: SettingsPage 消费跳转目标**

修改 `frontend/src/pages/SettingsPage.vue`:

1. 导入追加:

```ts
import { settingsFocusCategory } from '../composables/settingsFocus'
```

2. `const activeCategory = ref('appearance')` 之后追加:

```ts
// 跨页跳转:消费 focus 目标(toast「发现新版本」→ 关于);先设目标后挂载的两序均可
function consumeFocus() {
  const key = settingsFocusCategory.value
  if (!key) return
  if (categories.some((c) => c.key === key)) activeCategory.value = key
  settingsFocusCategory.value = null
}
consumeFocus()
watch(settingsFocusCategory, consumeFocus)
```

(同时把 `watch` 并入 vue 导入。)

- [ ] **Step 6: 门禁 + 手工验收**

Run: `cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build`
Expected: 全绿。

手工验收(真机):
1. `wails build`(默认 dev 版本)→ 运行 `build/bin/gbt32960-simulator.app` → 启动 3s 后**无** toast(开发构建不参与);关于面板显示"开发构建"、检查按钮禁用。
2. `wails build -ldflags "-X main.version=v0.0.1"` → 运行 → 约 3s 后 toast「发现新版本 v0.1.0,点击查看」;点击 → 设置页打开并落在"关于"分类;面板显示 当前版本 v0.0.1 / 发现新版本 / 发布说明可展开。
3. 在"关于"点"跳过此版本"→ 退出重启 → 不再出现 toast;（手动检查仍可看到）。
4. "常用"面板:关闭"启动时检查更新"→ 重启 → 无 toast;开关重开后恢复。
   记录:若本机出口 IP 触发 GitHub 匿名限流(403)导致无法发现新版本,记录截图/时间后跳过 2/3,并在记录中注明(网络逻辑已由 httptest 覆盖;最终将随 P3/P4 真机演练复验)。

- [ ] **Step 7: 提交**

```bash
GIT_MASTER=1 git add frontend/src/composables/settingsFocus.ts frontend/src/composables/settingsFocus.test.ts frontend/src/App.vue frontend/src/components/settings/SettingsCommonPanel.vue frontend/src/pages/SettingsPage.vue
GIT_MASTER=1 git commit -m "feat(frontend): 启动自动检查、toast 跳转关于与「启动时检查更新」转正"
```

---

## Self-Review Record

- **Spec coverage:** 设计文档 §5.1(检查契约)→ T2/T3;§5.2(资产匹配)→ T2;§5.6(前端契约的版本/检查/跳过/自动检查/跳转部分)→ T4/T5/T6;§5.7(全局约束)→ Global Constraints + PAC-5;§5.8(验证策略的单元/桥接部分)→ T1/T2/T3/T4 测试。P2/P3 内容(SHA256SUMS/下载/替换)不在本计划,已由 Master Plan 映射。
- **Placeholder scan:** 无 TBD/TODO;所有代码步骤给出完整代码;所有命令给出预期结果。
- **Type/interface consistency:** `UpdateInfo` 字段(current/latest/hasUpdate/devBuild/notes/publishedAt/assetName/assetSize)在 Go 与前端 mock 一致;`ShouldAutoCheck` 入参 `Pick<AppSettings,'checkUpdateOnStartup'|'lastUpdateCheckAt'>` 在 T4/T6 一致;`openSettingsCategory`/`settingsFocusCategory` 命名在 T5/T6 一致;错误文案四处一致(暂无发布版本/接口限流,请稍后再试/无法访问 GitHub,请检查网络/当前平台暂不支持自动更新)。
- **Decomposition decision:** 本 Phase 为已批准的 4 阶段拆分之 P1;自身 6 个任务(≤8-10),不再二次拆分。
- **Master/Phase completeness:** Master Plan 含 Progress Ledger 与 Global AC 覆盖矩阵;本计划 PAC-1..PAC-5 已映射回 Global AC(见各项标注)。
- **Level completeness:** 6 个任务均含 Level 与 Level Rationale(T1/T2/T4 = L2,T3/T5/T6 = L3)。
- **Task Gate completeness:** L2 = task reviewer + focused checks;L3 = task reviewer + linked AC,均与 Level 匹配。
- **L3 AC binding:** T3(PAC-1/2/4)、T5(PAC-1/2/3)、T6(PAC-3)均为非空绑定且 AC 存在于本清单。
- **Final acceptance coverage:** PAC-1~5 均绑定到非 L1 任务(T1~T6);跨模块项(绑定面/门禁)由 L3 任务与 PAC-5 覆盖。
- **Executable final acceptance:** 每条 PAC refinement 均含具体命令、数据、边界值(如恰 24h、`last=0`、非数字版本段、dev 双态)。
- **Source consistency:** PAC 的 Source 名称与设计文档一致(Overall Business Flow / Current Requirement Flow / Development Architecture / Existing Architecture Fit)。

## Execution Handoff

计划已保存至 `docs/superpowers/plans/2026-09-14-updater-phase-1-check-plan.md`。

三种执行方式:

**1. Quality-LTDD(推荐)** - Subagent-Driven + 验收门 + Final Intent Guard。每个任务先过实现者自检与 Task Reviewer;L3 任务额外跑 linked AC;全部完成后 Final Reviewer 与 plan 级 AC 终验。适合含明确 Final Intent、跨模块接线与生产质量要求的场景。

**2. Subagent-Driven** - 每任务新 subagent + 两阶段评审(spec 合规 + 代码质量)。验收门较轻。

**3. Inline Execution** - 本会话内批量执行 + 检查点人工确认。适合逐任务在场审阅。

Quality-LTDD 与 Subagent-Driven 的关系:前者在后者之上增加 Final Intent、任务级 AC、Final Reviewer、plan 级 AC 与失败裁决。Inline 为手动检查点路径。

Which approach?
