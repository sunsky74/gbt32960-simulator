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
