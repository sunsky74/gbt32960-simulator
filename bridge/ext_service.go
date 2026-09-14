package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/store"
)

// PackInfo 扩展包摘要(前端下拉与包管理用)。
type PackInfo struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Vendor       string   `json:"vendor,omitempty"`
	BaseVersion  string   `json:"baseVersion"`
	UnitCount    int      `json:"unitCount"`
	CommandCount int      `json:"commandCount"`
	Enabled      bool     `json:"enabled"`
	Scope        []string `json:"scope,omitempty"` // client / parser;空=仅 client(向后兼容)
}

// extGroupsFile 扩展组配置按包独立存储:map[packID]map[groupKey]GroupConfig。
// 本常量在 Task 1 定义(ImportPack 覆盖导入清配置需要),Task 4 直接复用。
const extGroupsFile = "extgroups.json"

// packStatesFile 包级启用/停用开关:map[packID]bool(true=停用)。缺省(无条目)=启用。
const packStatesFile = "packstates.json"

// ExtService 扩展包管理:扫描 packs 目录并提供列表/重载。
type ExtService struct {
	rt  *Runtime
	ctx context.Context
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
// 同时把包级停用状态同步进 Runtime(决定激活解析)。
func (s *ExtService) reloadDir(dir string) error {
	packs, err := ext.LoadDir(dir)
	disabled := loadPackStates()
	s.rt.SetPacks(packs)
	s.rt.SetPackStates(disabled)
	return err
}

// loadPackStates 读取包级停用状态;文件缺失/损坏一律视为全部启用(开关不会阻塞包加载)。
func loadPackStates() map[string]bool {
	var m map[string]bool
	if err := store.Load(packStatesFile, &m); err != nil || m == nil {
		return map[string]bool{}
	}
	return m
}

func savePackStates(m map[string]bool) error {
	return store.Save(packStatesFile, &m)
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
	err := s.reloadDir(dir)
	_ = err
	disabled := loadPackStates()
	return packInfosOf(s.rt.Packs(), disabled)
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

func packInfoOf(p *ext.Pack, disabled map[string]bool) PackInfo {
	scopes := p.Meta.Scope
	if len(scopes) == 0 {
		scopes = []string{ext.ScopeClient}
	}
	return PackInfo{
		ID: p.Meta.ID, Label: p.Meta.Label, Vendor: p.Meta.Vendor,
		BaseVersion: p.Meta.BaseVersion, UnitCount: len(p.Realtime.AppendUnits),
		CommandCount: len(p.Commands), Enabled: !disabled[p.Meta.ID],
		Scope: scopes,
	}
}

func packInfosOf(packs []*ext.Pack, disabled map[string]bool, filter ...func(*ext.Pack) bool) []PackInfo {
	var keep func(*ext.Pack) bool
	if len(filter) > 0 {
		keep = filter[0]
	}
	out := make([]PackInfo, 0, len(packs))
	for _, p := range packs {
		if keep != nil && !keep(p) {
			continue
		}
		out = append(out, packInfoOf(p, disabled))
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

// PickPackFile 打开文件选择对话框,返回所选 JSON 扩展包路径(取消返回空串)。
// 前端 wailsjs runtime 无对话框导出(v2.15 注入层不含 dialog),故由 bridge 侧提供。
func (s *ExtService) PickPackFile() (string, error) {
	return runtime.OpenFileDialog(s.ctx, runtime.OpenDialogOptions{
		Title: "选择扩展包",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
}

// installPack 落盘并重载一个已通过完整校验的包(文件导入与粘贴导入共用)。
// 同 id 覆盖导入:先清扩展组配置(新旧布局行值不兼容),移除旧文件(含热放置的
// 任意文件名副本,保证同 id 唯一),再清停用标记(导入即启用)。
func (s *ExtService) installPack(p *ext.Pack, data []byte) (PackInfo, error) {
	dir, err := packsDir()
	if err != nil {
		return PackInfo{}, err
	}
	dest := filepath.Join(dir, p.Meta.ID+".json")
	if old, err := findPackFileByID(dir, p.Meta.ID); err == nil && old != dest {
		if err := os.Remove(old); err != nil {
			return PackInfo{}, fmt.Errorf("导入失败: 移除旧副本 %s: %w", old, err)
		}
	}
	if _, err := os.Stat(dest); err == nil {
		if err := clearExtGroupsFor(p.Meta.ID); err != nil {
			return PackInfo{}, err
		}
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: 写入 %s: %w", dest, err)
	}
	disabled := loadPackStates()
	if disabled[p.Meta.ID] {
		delete(disabled, p.Meta.ID)
		if err := savePackStates(disabled); err != nil {
			return PackInfo{}, err
		}
	}
	if err := s.ReloadPacks(); err != nil {
		return PackInfo{}, err
	}
	for _, pk := range s.rt.Packs() {
		if pk.Meta.ID == p.Meta.ID {
			return packInfoOf(pk, loadPackStates()), nil
		}
	}
	return packInfoOf(p, loadPackStates()), nil
}

// ImportPack 校验并导入一个扩展包文件到 packs 目录(同 id 覆盖,绑定自动跟随新内容)。
// 复用 ext.LoadFile 做 反序列化→静态校验→干跑,任一步失败不落盘(AC-4)。
func (s *ExtService) ImportPack(path string) (PackInfo, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return PackInfo{}, fmt.Errorf("导入失败: 仅支持 .json 文件: %s", filepath.Base(path))
	}
	p, err := ext.LoadFile(path)
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: 读取 %s: %w", path, err)
	}
	return s.installPack(p, data)
}

// ImportPackJSON 校验并导入一段 JSON 文本(粘贴导入入口,校验流程与文件导入完全一致)。
func (s *ExtService) ImportPackJSON(text string) (PackInfo, error) {
	if strings.TrimSpace(text) == "" {
		return PackInfo{}, fmt.Errorf("导入失败: JSON 内容不能为空")
	}
	p, err := ext.LoadText(text, "粘贴的 JSON")
	if err != nil {
		return PackInfo{}, fmt.Errorf("导入失败: %w", err)
	}
	return s.installPack(p, []byte(text))
}

// SetPackEnabled 启用/停用一个扩展包。停用立即生效:绑定该包的连接档案运行时失效
// (扩展组不合并、命令注销),绑定关系保留,重新启用自动恢复。
func (s *ExtService) SetPackEnabled(id string, enabled bool) error {
	dir, err := packsDir()
	if err != nil {
		return err
	}
	if _, err := findPackFileByID(dir, id); err != nil {
		return fmt.Errorf("设置失败: %w", err)
	}
	disabled := loadPackStates()
	if enabled {
		delete(disabled, id)
	} else {
		disabled[id] = true
	}
	if err := savePackStates(disabled); err != nil {
		return err
	}
	return s.ReloadPacks()
}

// findPackFileByID 在 packs 目录按 meta.id 定位包文件。文件名可能与 id 不一致
// (ListPacks 支持热放置任意文件名的 .json,按内容识别);启停/删除/预览须同样
// 按内容查找,否则热放置的包"列表可见却无法操作"。
func findPackFileByID(dir, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("包 id 不能为空")
	}
	if filepath.Base(id) != id {
		return "", fmt.Errorf("非法包 id: %s", id)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		p, err := ext.LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // 坏文件不阻塞查找(ListPacks 同口径)
		}
		if p.Meta.ID == id {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("扩展包不存在: %s", id)
}

// GetPackJSON 返回指定扩展包的原始 JSON 文本(详情抽屉的代码预览用)。
func (s *ExtService) GetPackJSON(id string) (string, error) {
	dir, err := packsDir()
	if err != nil {
		return "", err
	}
	path, err := findPackFileByID(dir, id)
	if err != nil {
		return "", fmt.Errorf("读取失败: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取失败: %w", err)
	}
	return string(data), nil
}

// DeletePack 删除指定扩展包文件并重扫。若删除的是当前绑定包,重扫后
// resolvePack 落空,激活自动解绑(SetPacks → syncExtCommands 重置注册表)。
func (s *ExtService) DeletePack(id string) error {
	dir, err := packsDir()
	if err != nil {
		return err
	}
	path, err := findPackFileByID(dir, id)
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	// 删除包时一并清理其扩展组配置:同 id 重新导入时目标文件已不存在,
	// installPack 不会走同 id 清理分支,残留值会随重导入复活。
	_ = clearExtGroupsFor(id)
	disabled := loadPackStates()
	if disabled[id] {
		delete(disabled, id)
		_ = savePackStates(disabled)
	}
	return s.ReloadPacks()
}
