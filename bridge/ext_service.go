package bridge

import (
	"fmt"
	"path/filepath"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/store"
)

// PackInfo 扩展包摘要(前端下拉与包管理用)。
type PackInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Vendor      string `json:"vendor,omitempty"`
	BaseVersion string `json:"baseVersion"`
	UnitCount   int    `json:"unitCount"`
}

// ExtService 扩展包管理:扫描 packs 目录并提供列表/重载。
type ExtService struct {
	rt *Runtime
}

// NewExtService 创建服务并预加载一次包集合。
// 绑定激活发生在 GetConfig 回填 connCfg 时(见本任务 GetConfig 修改):
// 启动链 loadInitialData → GetConfig → SetConnCfg → resolvePack。
func NewExtService(rt *Runtime) *ExtService {
	s := &ExtService{rt: rt}
	_ = s.ReloadPacks()
	return s
}

// NewExtServiceForTest 测试用构造:不触碰用户目录。
func NewExtServiceForTest(rt *Runtime) *ExtService { return &ExtService{rt: rt} }

func packsDir() (string, error) {
	dir, err := store.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "packs"), nil
}

// reloadDir 扫描指定目录:合法包推入 Runtime,坏文件聚合上报且不阻塞好包。
func (s *ExtService) reloadDir(dir string) error {
	packs, err := ext.LoadDir(dir)
	s.rt.SetPacks(packs)
	return err
}

// ReloadPacks 扫描默认 packs 目录。
func (s *ExtService) ReloadPacks() error {
	dir, err := packsDir()
	if err != nil {
		return fmt.Errorf("定位扩展包目录失败: %w", err)
	}
	if err := s.reloadDir(dir); err != nil {
		return fmt.Errorf("扩展包目录存在非法文件: %w", err)
	}
	return nil
}

// listPacksDir 扫描目录并返回列表。坏文件不阻塞列表:reloadDir 已把好包推入
// Runtime,扫描错误忽略(P4 导入 UI 再做逐文件提示)。
func (s *ExtService) listPacksDir(dir string) []PackInfo {
	_ = s.reloadDir(dir)
	return packInfosOf(s.rt.Packs())
}

// ListPacks 返回全部可绑定扩展包摘要。每次调用重新扫描,支持热放置文件;
// 存在坏文件时列表照常返回(全局约束:坏包不阻塞好包)。
func (s *ExtService) ListPacks() ([]PackInfo, error) {
	dir, err := packsDir()
	if err != nil {
		return nil, fmt.Errorf("定位扩展包目录失败: %w", err)
	}
	return s.listPacksDir(dir), nil
}

func packInfosOf(packs []*ext.Pack) []PackInfo {
	out := make([]PackInfo, 0, len(packs))
	for _, p := range packs {
		out = append(out, PackInfo{
			ID: p.Meta.ID, Label: p.Meta.Label, Vendor: p.Meta.Vendor,
			BaseVersion: p.Meta.BaseVersion, UnitCount: len(p.Realtime.AppendUnits),
		})
	}
	return out
}
