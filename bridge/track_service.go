package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/track"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// TrackDeps 轨迹导入与回放依赖的最小接口(生产环境传 *MessageService,
// 测试可替换)。方法不导出:仅包内协作,不进入前端 RPC 绑定面。
type TrackDeps interface {
	// DefaultGroups 返回指定版本的默认报文配置(运行时无配置时兜底)。
	DefaultGroups(version string) *schema.GroupsPayload
	// ensureAutoReport 确保周期上报开启(回放的推进载体)。
	ensureAutoReport() error
}

// TrackService 轨迹导入与回放。回放不自发报文:周期上报每次 tick 经
// trackHook 钩子推进一个轨迹点写入位置组,组装出的 0x02 即携带该点——
// 上报间隔即回放步进间隔,其余实时数据组保持用户配置不变。
type TrackService struct {
	rt   *Runtime
	deps TrackDeps
	ctx  context.Context

	mu      sync.Mutex
	points  []track.Point
	info    TrackInfo
	active  bool
	index   int
	loop    bool
	lastErr string
	cur     track.Point
}

// TrackInfo 轨迹导入结果。
type TrackInfo struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Count   int    `json:"count"`
	Skipped int    `json:"skipped"`
}

// TrackReplayStatus 回放状态快照(前端轮询)。
type TrackReplayStatus struct {
	Running   bool    `json:"running"`
	Index     int     `json:"index"`
	Total     int     `json:"total"`
	Loop      bool    `json:"loop"`
	LastError string  `json:"lastError"`
	Lng       float64 `json:"lng"`
	Lat       float64 `json:"lat"`
}

// NewTrackService 创建服务。deps 生产环境为 *MessageService。
func NewTrackService(rt *Runtime, deps TrackDeps) *TrackService {
	return &TrackService{rt: rt, deps: deps}
}

// PickTrackFile 打开文件选择对话框(取消返回空串)。
func (s *TrackService) PickTrackFile() (string, error) {
	return runtime.OpenFileDialog(s.ctx, runtime.OpenDialogOptions{
		Title: "选择轨迹文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "轨迹文件 (GPX / Excel / CSV)", Pattern: "*.gpx;*.xlsx;*.csv"},
			{DisplayName: "GPX 轨迹", Pattern: "*.gpx"},
			{DisplayName: "Excel 工作簿", Pattern: "*.xlsx"},
			{DisplayName: "CSV 文本", Pattern: "*.csv"},
		},
	})
}

// ImportTrack 解析并装载轨迹文件(替换已有轨迹;正在回放则先停止),
// 并默认开启周期上报(离线等失败静默跳过,开始回放时会再次确保)。
func (s *TrackService) ImportTrack(path string) (*TrackInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("导入失败: 读取 %s: %w", filepath.Base(path), err)
	}
	res, err := track.ParseFile(filepath.Base(path), data)
	if err != nil {
		return nil, fmt.Errorf("导入失败: %w", err)
	}
	s.StopReplay()
	_ = s.deps.ensureAutoReport()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points = res.Points
	s.info = TrackInfo{
		Name:    filepath.Base(path),
		Format:  res.Format,
		Count:   len(res.Points),
		Skipped: res.Skipped,
	}
	s.index, s.lastErr, s.cur = 0, "", track.Point{}
	return &s.info, nil
}

// ClearTrack 清空已导入的轨迹(停止回放,位置组保留最后写入的值)。
func (s *TrackService) ClearTrack() {
	s.StopReplay()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points = nil
	s.info = TrackInfo{}
	s.index, s.lastErr, s.cur = 0, "", track.Point{}
}

// StartReplay 开始回放:确保周期上报开启,首点立即写入位置组作预览
// (首个 0x02 由下一次周期上报携带),之后每个周期上报 tick 推进一点。
// loop=true 时播完循环。
func (s *TrackService) StartReplay(loop bool) error {
	s.mu.Lock()
	if len(s.points) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("尚未导入轨迹文件")
	}
	first := s.points[0]
	s.mu.Unlock()

	if err := s.deps.ensureAutoReport(); err != nil {
		return fmt.Errorf("开启周期上报失败: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.index = 0
	s.lastErr = ""
	s.loop = loop
	s.cur = first
	s.applyLocationLocked(first)
	return nil
}

// StopReplay 停止回放(幂等)。周期上报不受影响,位置组保留最后一点。
func (s *TrackService) StopReplay() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
}

// ReplayStatus 返回当前回放状态。
func (s *TrackService) ReplayStatus() TrackReplayStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return TrackReplayStatus{
		Running:   s.active,
		Index:     s.index,
		Total:     len(s.points),
		Loop:      s.loop,
		LastError: s.lastErr,
		Lng:       s.cur.Lng,
		Lat:       s.cur.Lat,
	}
}

// advanceForReport 周期上报 tick 钩子:推进一个轨迹点写入位置组。
// 未激活时为空操作;播完且未开循环则自动停止。
func (s *TrackService) advanceForReport() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	if s.index >= len(s.points) {
		if s.loop {
			s.index = 0
		} else {
			s.active = false // 播完
			return
		}
	}
	p := s.points[s.index]
	s.cur = p
	s.index++
	s.applyLocationLocked(p)
}

// failOnSend 0x02 发送失败钩子:终止回放并记录原因(如中途掉线)。
func (s *TrackService) failOnSend(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.active = false
	s.lastErr = fmt.Sprintf("发送 0x02 失败,回放已停止: %v", err)
}

// applyLocationLocked 把经纬度写入运行时位置组(2016/2025 键一致:
// location / longitude / latitude / valid)。组缺失则创建并启用;
// rt.Groups() 已隔离 map 与 Rows 切片头,行 map 共享只读不可原地改,
// 故行值深拷贝后整体替换切片元素。调用方须持 s.mu
// 与 rt 无交叉锁序(先 s.mu 后 rt.mu)。
func (s *TrackService) applyLocationLocked(p track.Point) {
	groups := s.rt.Groups()
	if groups == nil {
		groups = s.deps.DefaultGroups(s.versionText()).ToMap()
	}
	g, ok := groups["location"]
	if !ok {
		g = schema.GroupConfig{Enabled: true, Rows: []map[string]any{{}}}
	}
	if len(g.Rows) == 0 {
		g.Rows = []map[string]any{{}}
	}
	row := make(map[string]any, len(g.Rows[0])+3)
	for k, v := range g.Rows[0] {
		row[k] = v
	}
	row["valid"] = true
	row["longitude"] = p.Lng
	row["latitude"] = p.Lat
	g.Enabled = true
	g.Rows[0] = row
	groups["location"] = g
	s.rt.SetGroups(groups)
}

// versionText 当前连接配置的协议版本(与 MessageService 同口径)。
func (s *TrackService) versionText() string {
	if cfg := s.rt.ConnCfg(); cfg != nil {
		return cfg.Version
	}
	return "2016"
}
