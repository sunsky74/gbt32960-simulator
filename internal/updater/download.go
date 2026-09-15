package updater

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// 下载类错误(fail-closed;文案即前端展示文案)。
var (
	// ErrCanceled 用户取消或应用退出导致的下载中止(部分文件清理由编排层完成)。
	ErrCanceled = errors.New("已取消下载")
	// ErrStalled 持续 StallTimeout 无字节进展。
	ErrStalled = errors.New("下载超时(长时间无进展),请重试")
	// ErrHostNotAllowed 下载源(含重定向跳转)不在白名单内。
	ErrHostNotAllowed = errors.New("下载源不在白名单内,已拒绝")
	// ErrDownload 网络不可达/HTTP 状态异常(附代理提示,D21)。
	ErrDownload = errors.New("下载失败,请检查网络(如使用代理,请确认 TUN 模式或 HTTPS_PROXY 生效)")
	// ErrTruncated 下载字节数与声明大小不符。
	ErrTruncated = errors.New("下载不完整,请重试")
	// ErrDisk 本地写入失败(空间/权限)。
	ErrDisk = errors.New("写入失败,请检查磁盘空间与权限")
)

// AllowedHosts 下载白名单(设计文档 §5.7;release-assets 为实测 302 目标,objects 为历史兼容)。
var AllowedHosts = map[string]bool{
	"api.github.com":                       true,
	"github.com":                           true,
	"release-assets.githubusercontent.com": true,
	"objects.githubusercontent.com":        true,
}

// CheckDownloadURL 校验单个跳转 URL:必须 HTTPS 且 host 在白名单内(逐跳调用)。
func CheckDownloadURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || !AllowedHosts[u.Hostname()] {
		return ErrHostNotAllowed
	}
	return nil
}

// Progress 下载进度契约(设计文档 §5.5;phase: downloading|verifying)。
type Progress struct {
	Phase    string
	Received int64
	Total    int64
}

// Percent 百分比(total 未知时返回 0)。
func (p Progress) Percent() int {
	if p.Total <= 0 {
		return 0
	}
	n := int(p.Received * 100 / p.Total)
	if n > 100 {
		n = 100
	}
	return n
}

// Artifact 校验通过的更新产物。
type Artifact struct {
	Path   string
	Name   string
	Tag    string
	SHA256 string
}

// DownloadResult 下载并校验通过的更新产物(服务层返回契约;bridge 直接透出前端)。
type DownloadResult struct {
	Tag       string `json:"tag"`
	AssetName string `json:"assetName"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// Downloader 流式下载器:进度节流、停滞检测、逐跳白名单;全部可注入(测试缝)。
type Downloader struct {
	HTTP             *http.Client
	ProgressInterval time.Duration        // 进度最小间隔(默认 100ms)
	StallTimeout     time.Duration        // 无字节进展超时(默认 120s)
	CheckURL         func(*url.URL) error // 逐跳 URL 校验(默认 CheckDownloadURL)
	ua               string
}

// NewDownloader 生产默认值;HTTP 客户端不设整体超时(由停滞检测兜底)。
func NewDownloader(version string) *Downloader {
	d := &Downloader{
		ProgressInterval: 100 * time.Millisecond,
		StallTimeout:     120 * time.Second,
		CheckURL:         CheckDownloadURL,
		ua:               userAgent(version),
	}
	d.HTTP = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := d.CheckURL(req.URL); err != nil {
				return err
			}
			if len(via) >= 10 {
				return errors.New("重定向次数过多")
			}
			return nil
		},
	}
	return d
}

// Fetch 流式下载 rawURL → dest;onProgress 首次与末次必发,中间按 ProgressInterval 节流。
// 停滞/取消/截断分别返回 ErrStalled/ErrCanceled/ErrTruncated;失败时保留部分文件,由调用方清理。
func (d *Downloader) Fetch(ctx context.Context, rawURL, dest string, onProgress func(Progress)) (int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, ErrDownload
	}
	check := d.CheckURL
	if check == nil {
		check = CheckDownloadURL // 零值兜底:默认白名单策略(fail-closed)
	}
	if err := check(u); err != nil {
		return 0, err // 白名单错误原样透出(安全分类,勿并入 ErrDownload)
	}

	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()

	// 停滞计时器覆盖「建连/响应头等待 + 响应体读取」全程;每次收到数据块重置
	var stalled atomic.Bool
	timer := time.AfterFunc(d.StallTimeout, func() {
		stalled.Store(true)
		cancel2() // 解除 Do/Read 阻塞
	})
	defer timer.Stop()

	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, ErrDownload
	}
	req.Header.Set("User-Agent", d.ua)
	resp, err := d.HTTP.Do(req)
	if err != nil {
		if stalled.Load() {
			return 0, ErrStalled
		}
		if ctx.Err() != nil {
			return 0, ErrCanceled
		}
		if errors.Is(err, ErrHostNotAllowed) { // 重定向跳转被白名单拒绝:原样透出
			return 0, ErrHostNotAllowed
		}
		return 0, ErrDownload
	}
	defer resp.Body.Close()
	// 兜底:最终 URL 亦须在白名单内(防注入客户端绕过 CheckRedirect)
	if err := check(resp.Request.URL); err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, ErrDownload
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}

	f, err := os.Create(dest)
	if err != nil {
		return 0, ErrDisk
	}
	closeFile := func() error { return f.Close() }

	var received int64
	var lastEmit time.Time
	emit := func(force bool) {
		if onProgress == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastEmit) < d.ProgressInterval {
			return
		}
		lastEmit = now
		onProgress(Progress{Phase: "downloading", Received: received, Total: total})
	}
	emit(true)

	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				_ = closeFile()
				return received, ErrDisk
			}
			received += int64(n)
			timer.Reset(d.StallTimeout)
			emit(false)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = closeFile()
			if stalled.Load() {
				return received, ErrStalled
			}
			if ctx.Err() != nil {
				return received, ErrCanceled
			}
			if total > 0 && received < total {
				return received, ErrTruncated
			}
			return received, ErrDownload
		}
	}
	if err := closeFile(); err != nil {
		return received, ErrDisk
	}
	if stalled.Load() {
		return received, ErrStalled
	}
	emit(true)
	if total > 0 && received != total {
		return received, ErrTruncated
	}
	return received, nil
}

// DownloadReleaseArtifact 编排:校验信息先行(下载 sums/sig → 验签 → 解析)→ 下载产物 → 哈希比对 → .part 落定。
// 任一步失败:删除 .part;已下载的 sums/sig 保留(启动清理兜底)。
func (d *Downloader) DownloadReleaseArtifact(ctx context.Context, rel *Release, goos, goarch, dir string, keys map[string]ed25519.PublicKey, onProgress func(Progress)) (Artifact, error) {
	asset, err := MatchAsset(goos, goarch, rel.Assets)
	if err != nil {
		return Artifact{}, err
	}
	sumsAsset, ok := findAsset(rel.Assets, "SHA256SUMS")
	if !ok {
		return Artifact{}, ErrChecksumsMissing
	}
	sigAsset, ok := findAsset(rel.Assets, "SHA256SUMS.sig")
	if !ok {
		return Artifact{}, ErrChecksumsMissing
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Artifact{}, ErrDisk
	}
	sumsPath := filepath.Join(dir, "SHA256SUMS")
	sigPath := filepath.Join(dir, "SHA256SUMS.sig")
	if _, err := d.Fetch(ctx, sumsAsset.URL, sumsPath, nil); err != nil {
		return Artifact{}, err
	}
	if _, err := d.Fetch(ctx, sigAsset.URL, sigPath, nil); err != nil {
		return Artifact{}, err
	}
	sumsBytes, err := os.ReadFile(sumsPath)
	if err != nil {
		return Artifact{}, ErrDisk
	}
	sigBytes, err := os.ReadFile(sigPath)
	if err != nil {
		return Artifact{}, ErrDisk
	}
	// 先验签后解析(fail-closed 顺序契约)
	if err := VerifySumSignature(sumsBytes, sigBytes, keys); err != nil {
		return Artifact{}, err
	}
	checksums, err := ParseChecksums(strings.NewReader(string(sumsBytes)))
	if err != nil {
		return Artifact{}, ErrChecksumsMissing
	}
	want, ok := checksums[asset.Name]
	if !ok {
		return Artifact{}, ErrChecksumsMissing
	}

	part := filepath.Join(dir, asset.Name+".part")
	// total 回退:响应无 Content-Length 时,用 API 声明的资产大小填充事件载荷(截断判定仍依据 Content-Length)
	if _, err := d.Fetch(ctx, asset.URL, part, func(p Progress) {
		if p.Total == 0 && asset.Size > 0 {
			p.Total = asset.Size
		}
		if onProgress != nil {
			onProgress(p)
		}
	}); err != nil {
		_ = os.Remove(part)
		return Artifact{}, err
	}
	if onProgress != nil {
		onProgress(Progress{Phase: "verifying", Received: asset.Size, Total: asset.Size})
	}
	got, err := HashFile(part)
	if err != nil {
		_ = os.Remove(part)
		return Artifact{}, ErrDisk
	}
	if got != want {
		_ = os.Remove(part)
		return Artifact{}, ErrHashMismatch
	}
	final := filepath.Join(dir, asset.Name)
	if err := os.Rename(part, final); err != nil {
		_ = os.Remove(part)
		return Artifact{}, ErrDisk
	}
	return Artifact{Path: final, Name: asset.Name, Tag: rel.TagName, SHA256: got}, nil
}

func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// ReleaseDir 更新缓存目录(UserCacheDir/gbt32960-simulator/updates/<safeTag>);tag 做字符白名单清洗。
func ReleaseDir(tag string) (string, error) {
	cd, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cd, "gbt32960-simulator", "updates", safeTag(tag)), nil
}

var tagSanitize = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func safeTag(tag string) string {
	s := tagSanitize.ReplaceAllString(tag, "_")
	if s == "" {
		s = "unknown"
	}
	return s
}

// CleanupCache 清空更新缓存:删除各 tag 子目录(下载产物不跨会话复用);
// 根级 last-result.json / helper.log 保留(P3 启动消费失败结果与排障所需)。
func CleanupCache() error {
	cd, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	root := filepath.Join(cd, "gbt32960-simulator", "updates")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && (e.Name() == "last-result.json" || e.Name() == "helper.log") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
