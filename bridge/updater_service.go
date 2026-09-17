package bridge

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"runtime"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/updater"
)

// UpdaterService 应用内更新服务:查询最新版本 + 下载校验(Phase 2)+ 应用重启(Phase 3)。
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
	ready       *updater.Artifact // 下载校验成功的产物(仅内存,不跨会话;D9)
	applying    bool              // 应用单飞标志:apply 期间检查/下载/再应用一律拒绝

	// Phase 3 应用链注入缝(构造时给默认值,测试注入;§5.4)。
	quitDelay     time.Duration // 拉起 helper 后延迟退出时长(留 UI 收尾)
	spawn         func(updater.HelperArgs) error
	quit          func(context.Context)
	resolveTarget func() (string, error)
	preflight     func(string) error
}

// NewUpdaterService 创建更新服务(version 为 ldflags 注入版本,dev 表示本地构建)。
func NewUpdaterService(version string) *UpdaterService {
	return &UpdaterService{
		version:       version,
		client:        updater.NewClient(version),
		downloader:    updater.NewDownloader(version),
		keys:          updater.EmbeddedKeys(),
		emit:          wruntime.EventsEmit,
		quitDelay:     time.Second,
		spawn:         updater.SpawnHelper,
		quit:          wruntime.Quit,
		resolveTarget: updater.RunningTarget,
		preflight:     updater.PreflightTarget,
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
	if s.isApplying() {
		return updater.UpdateInfo{}, errors.New("更新已在进行中")
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
		Current:   s.version,
		Latest:    rel.TagName,
		HasUpdate: updater.IsNewer(rel.TagName, s.version),
	}
	// 资产匹配失败不阻塞检查结果(前端据 AssetName 空值提示"暂不支持自动更新")
	if asset, aerr := updater.MatchAsset(runtime.GOOS, runtime.GOARCH, rel.Assets); aerr == nil {
		info.AssetName = asset.Name
	}
	s.mu.Lock()
	s.lastRelease = rel
	s.ready = nil // 新检查结果使旧就绪产物复位(与前端 check() 一致)
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
	if s.applying {
		s.mu.Unlock()
		return updater.DownloadResult{}, errors.New("更新已在进行中")
	}
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
	if s.applying { // 双重检查:apply 已在预检/拉起阶段则拒绝(单飞,§5.7)
		s.mu.Unlock()
		return updater.DownloadResult{}, errors.New("更新已在进行中")
	}
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
	s.mu.Lock()
	s.ready = &art // 校验通过才置就绪:ApplyUpdate 只消费该内存状态(D9)
	s.mu.Unlock()
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

// ApplyUpdate 应用已下载并校验通过的产物:守卫 → 定位/预检 → 拉起 helper → 延迟退出应用。
// 绑定面无参数:产物/目标/结果路径全部取自服务内部状态(防绑定面路径注入,§5.6)。
// spawn 成功后立即返回 nil(UI 有 ~1s 绘制 applying 态);任何失败均不退出应用。
func (s *UpdaterService) ApplyUpdate() error {
	if s.version == "" || s.version == "dev" {
		return errors.New("开发构建不参与更新")
	}
	s.mu.Lock()
	if s.downloading {
		s.mu.Unlock()
		return errors.New("更新正在下载中,请稍候")
	}
	if s.applying {
		s.mu.Unlock()
		return errors.New("更新已在进行中")
	}
	ready := s.ready
	rel := s.lastRelease
	s.mu.Unlock()

	if ready == nil {
		return errors.New("更新包未就绪,请先下载")
	}
	if _, err := os.Stat(ready.Path); err != nil {
		return errors.New("更新包未就绪,请先下载")
	}
	if rel == nil || !updater.IsNewer(rel.TagName, s.version) {
		return errors.New("已是最新版本,无需安装")
	}
	target, err := s.resolveTarget()
	if err != nil {
		return err
	}
	if err := s.preflight(target); err != nil {
		return err // 指引类文案原样透出,不退出应用
	}
	resultPath, err := updater.LastResultPath()
	if err != nil {
		return err
	}
	logPath, err := updater.HelperLogPath()
	if err != nil {
		return err
	}
	_ = updater.ClearLastResult() // 清陈旧结果,防上次残留造成启动误报

	args := updater.HelperArgs{
		ParentPID: os.Getpid(),
		Artifact:  ready.Path,
		Target:    target,
		Tag:       ready.Tag,
		Result:    resultPath,
		Log:       logPath,
	}
	if err := s.spawn(args); err != nil {
		return errors.New("无法启动更新进程,请手动更新")
	}
	s.mu.Lock()
	s.applying = true
	s.mu.Unlock()
	// ~1s 留 UI 收尾后退出;退出经注入缝(测试绝不真的退出进程)。
	go func(ctx context.Context, delay time.Duration, quit func(context.Context)) {
		time.Sleep(delay)
		quit(ctx)
	}(s.ctx, s.quitDelay, s.quit)
	return nil
}

// ConsumeLastResult 读取并尽力清除上次替换结果(新实例启动时调用):
// 无记录 → {Present:false};有记录 → 投影为 ApplyOutcome 并清除(清除失败不报错,避免二次 toast)。
func (s *UpdaterService) ConsumeLastResult() (updater.ApplyOutcome, error) {
	r, err := updater.ReadLastResult()
	if err != nil {
		return updater.ApplyOutcome{}, err
	}
	if r == nil {
		return updater.ApplyOutcome{}, nil
	}
	_ = updater.ClearLastResult() // 尽力清除:失败仅静默,不阻断本次告知
	return updater.AsOutcome(r), nil
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

// isApplying 供检查/下载链路判断互斥(设计文档 §5.7:检查/下载/应用单飞)。
func (s *UpdaterService) isApplying() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applying
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
