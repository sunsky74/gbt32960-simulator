package bridge

import (
	"context"
	"crypto/ed25519"
	"errors"
	"runtime"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/updater"
)

// UpdaterService 应用内更新服务:查询最新版本 + 下载校验(Phase 2);替换在后续 Phase 接入。
type UpdaterService struct {
	ctx        context.Context
	version    string
	client     *updater.Client
	downloader *updater.Downloader
	keys       map[string]ed25519.PublicKey

	// emit 前端推送函数,默认 wruntime.EventsEmit;测试可注入以确定性验证。
	emit func(ctx context.Context, eventName string, optionalData ...any)

	mu          sync.Mutex
	lastRelease *updater.Release // 最近一次成功的检查结果(下载依赖其资产直链)
	downloading bool
	cancel      context.CancelFunc
}

// NewUpdaterService 创建更新服务(version 为 ldflags 注入版本,dev 表示本地构建)。
func NewUpdaterService(version string) *UpdaterService {
	return &UpdaterService{
		version:    version,
		client:     updater.NewClient(version),
		downloader: updater.NewDownloader(version),
		keys:       updater.EmbeddedKeys(),
		emit:       wruntime.EventsEmit,
	}
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
	if s.isDownloading() {
		return updater.UpdateInfo{}, errors.New("下载已在进行中")
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
	s.mu.Lock()
	s.lastRelease = rel
	s.mu.Unlock()
	return info, nil
}

// DownloadUpdate 下载并校验最近一次检查到的更新产物。
// 无参数:URL/路径全部取自服务内部状态(绑定面不暴露任意 URL/路径,防注入)。
func (s *UpdaterService) DownloadUpdate() (updater.DownloadResult, error) {
	if s.version == "" || s.version == "dev" {
		return updater.DownloadResult{}, errors.New("开发构建不参与更新")
	}
	s.mu.Lock()
	if s.downloading {
		s.mu.Unlock()
		return updater.DownloadResult{}, errors.New("下载已在进行中")
	}
	rel := s.lastRelease
	s.mu.Unlock()
	if rel == nil {
		return updater.DownloadResult{}, errors.New("请先检查更新")
	}
	if !updater.IsNewer(rel.TagName, s.version) {
		return updater.DownloadResult{}, errors.New("已是最新版本,无需下载")
	}
	asset, err := updater.MatchAsset(runtime.GOOS, runtime.GOARCH, rel.Assets)
	if err != nil {
		return updater.DownloadResult{}, err
	}
	dir, err := updater.ReleaseDir(rel.TagName)
	if err != nil {
		return updater.DownloadResult{}, err
	}

	base := s.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base) // 派生自 wails ctx:shutdown 时自动取消
	defer cancel()

	s.mu.Lock()
	if s.downloading { // 双重检查:并发首触发只放行一个
		s.mu.Unlock()
		return updater.DownloadResult{}, errors.New("下载已在进行中")
	}
	s.downloading = true
	s.cancel = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.downloading = false
		s.cancel = nil
		s.mu.Unlock()
	}()

	art, err := s.downloader.DownloadReleaseArtifact(ctx, rel, runtime.GOOS, runtime.GOARCH, dir, s.keys, func(p updater.Progress) {
		if ctx.Err() == nil { // 取消后不再推送(事件契约)
			s.emitProgress(p)
		}
	})
	if err != nil {
		return updater.DownloadResult{}, err
	}
	return updater.DownloadResult{Tag: art.Tag, AssetName: art.Name, Size: asset.Size, SHA256: art.SHA256}, nil
}

// CancelDownload 取消进行中的下载(空闲时幂等无操作)。
func (s *UpdaterService) CancelDownload() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// cleanupCache 清空更新缓存(经 bridge.WireUpdaterStartup 在启动时调用;绑定面不暴露)。
func (s *UpdaterService) cleanupCache() {
	_ = updater.CleanupCache()
}

// isDownloading 供检查链路判断互斥(设计文档 §5.7:检查/下载/应用单飞)。
func (s *UpdaterService) isDownloading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.downloading
}

// emitProgress 推送 update:progress(nil-ctx 守卫 + 取消后不推送)。
func (s *UpdaterService) emitProgress(p updater.Progress) {
	ctx := s.ctx
	if ctx == nil || ctx.Err() != nil {
		return
	}
	s.emit(ctx, "update:progress", progressDTO{
		Phase:    p.Phase,
		Received: p.Received,
		Total:    p.Total,
		Percent:  p.Percent(),
	})
}

// progressDTO update:progress 事件载荷(设计文档 §5.5)。
type progressDTO struct {
	Phase    string `json:"phase"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Percent  int    `json:"percent"`
}
