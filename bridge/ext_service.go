package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/store"
)

// PackInfo 扩展包摘要(前端下拉与包管理用)。
type PackInfo struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Vendor       string `json:"vendor,omitempty"`
	BaseVersion  string `json:"baseVersion"`
	UnitCount    int    `json:"unitCount"`
	CommandCount int    `json:"commandCount"`
}

// extGroupsFile 扩展组配置按包独立存储:map[packID]map[groupKey]GroupConfig。
// 本常量在 Task 1 定义(ImportPack 覆盖导入清配置需要),Task 4 直接复用。
const extGroupsFile = "extgroups.json"

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
	dir = filepath.Join(dir, "packs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建扩展包目录失败: %w", err)
	}
	return dir, nil
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

func packInfoOf(p *ext.Pack) PackInfo {
	return PackInfo{
		ID: p.Meta.ID, Label: p.Meta.Label, Vendor: p.Meta.Vendor,
		BaseVersion: p.Meta.BaseVersion, UnitCount: len(p.Realtime.AppendUnits),
		CommandCount: len(p.Commands),
	}
}

func packInfosOf(packs []*ext.Pack) []PackInfo {
	out := make([]PackInfo, 0, len(packs))
	for _, p := range packs {
		out = append(out, packInfoOf(p))
	}
	return out
}

// clearExtGroupsFor 清除指定包的扩展组配置(覆盖导入同 id 新布局时调用,
// 防止旧布局行值残留导致新布局保存被"缺少值"拦截——评审 P0-2)。
func clearExtGroupsFor(packID string) error {
	var all map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &all); err != nil || all == nil {
		return nil // 无扩展组配置,无需清理
	}
	if _, ok := all[packID]; !ok {
		return nil
	}
	delete(all, packID)
	return store.Save(extGroupsFile, &all)
}

// ImportPack 校验并导入一个扩展包文件到 packs 目录(同 id 覆盖,绑定自动跟随新内容)。
// 复用 ext.LoadFile 做 反序列化→静态校验→干跑,任一步失败不落盘(AC-4)。
// 同 id 覆盖导入时清除该包的扩展组配置(新旧布局行值不兼容,保留只会让保存失败)。
func (s *ExtService) ImportPack(path string) (PackInfo, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return PackInfo{}, fmt.Errorf("导入失败: 仅支持 .json 文件: %s", filepath.Base(path))
	}
	p, err := ext.LoadFile(path)
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: %w", err)
	}
	dir, err := packsDir()
	if err != nil {
		return PackInfo{}, err
	}
	dest := filepath.Join(dir, p.Meta.ID+".json")
	if _, err := os.Stat(dest); err == nil {
		if err := clearExtGroupsFor(p.Meta.ID); err != nil {
			return PackInfo{}, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: 读取 %s: %w", path, err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: 写入 %s: %w", dest, err)
	}
	if err := s.ReloadPacks(); err != nil {
		return PackInfo{}, err
	}
	for _, pk := range s.rt.Packs() {
		if pk.Meta.ID == p.Meta.ID {
			return packInfoOf(pk), nil
		}
	}
	return packInfoOf(p), nil
}

// DeletePack 删除指定扩展包文件并重扫。若删除的是当前绑定包,重扫后
// resolvePack 落空,激活自动解绑(SetPacks → syncExtCommands 重置注册表)。
func (s *ExtService) DeletePack(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("删除失败: 包 id 不能为空")
	}
	dir, err := packsDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, id+".json")
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return fmt.Errorf("删除失败: 扩展包不存在: %s", id)
	}
	if err := os.Remove(dest); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return s.ReloadPacks()
}
