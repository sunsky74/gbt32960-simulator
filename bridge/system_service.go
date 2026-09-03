package bridge

import (
	"context"
	"fmt"
	"path/filepath"

	"gbt32960-simulator/internal/store"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SystemService 系统级只读信息与系统能力(设置中心「数据与存储」消费)。
type SystemService struct {
	ctx context.Context
}

// NewSystemService 创建服务。
func NewSystemService() *SystemService { return &SystemService{} }

// SetContext 注入 wails 上下文(打开目录等原生能力需要)。
func (s *SystemService) SetContext(ctx context.Context) { s.ctx = ctx }

// StoragePaths 返回应用数据存储路径(只读展示;修改位置属规划能力,不在本期)。
func (s *SystemService) StoragePaths() (map[string]string, error) {
	dir, err := store.Dir()
	if err != nil {
		return nil, fmt.Errorf("定位配置目录失败: %w", err)
	}
	return map[string]string{
		"config": dir,
		"packs":  filepath.Join(dir, "packs"),
	}, nil
}

// OpenDirectory 在系统文件管理器中打开目录(wails v2 无直接 API,经 file:// URL 走浏览器打开)。
func (s *SystemService) OpenDirectory(path string) error {
	if path == "" {
		return fmt.Errorf("目录路径为空")
	}
	wailsRuntime.BrowserOpenURL(s.ctx, "file://"+path)
	return nil
}
