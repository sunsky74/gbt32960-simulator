package bridge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gbt32960-simulator/internal/servermode"
	"gbt32960-simulator/internal/store"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// serverCfgFile 服务端模式表单配置的持久化文件(配置目录下)。
const serverCfgFile = "server.json"

// ServerConfig 服务端模式页面表单持久化结构。
type ServerConfig struct {
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	IdleEnabled bool   `json:"idleEnabled"`
	IdleSeconds int    `json:"idleSeconds"`
	MaxConns    int    `json:"maxConns"`
	// MaxFrameBytes 单帧字节上限(512~65536)。
	MaxFrameBytes int `json:"maxFrameBytes"`
	// LogLines 导出日志保留行数(100~10000)。
	LogLines int `json:"logLines"`
	// MaxVinsPerConn 平台链路车辆 VIN 数上限(1~1024)。
	MaxVinsPerConn int `json:"maxVinsPerConn"`
}

// ServerService 服务端模式的前端绑定面。
type ServerService struct {
	ctx context.Context
	mu  sync.Mutex
	srv *servermode.Server
	rt  *Runtime
	// last 最近一次启动的表单配置(UpdateIdle 基于其副本回填持久化)
	last ServerConfig
}

func NewServerService(rt *Runtime) *ServerService { return &ServerService{rt: rt} }

func (s *ServerService) emit(name string, data any) {
	if s.ctx == nil {
		return // 测试环境无 wails 上下文
	}
	wailsRuntime.EventsEmit(s.ctx, name, data)
}

func (s *ServerService) LoadConfig() ServerConfig {
	cfg := ServerConfig{
		IP: "127.0.0.1", Port: 32960, IdleEnabled: true, IdleSeconds: 60,
		MaxConns: 64, MaxFrameBytes: 8192, LogLines: 500, MaxVinsPerConn: 128,
	}
	_ = store.Load(serverCfgFile, &cfg) // 文件缺失/损坏用默认值;旧版文件缺新键时预置默认值保留
	if cfg.IdleSeconds < 5 || cfg.IdleSeconds > 3600 {
		cfg.IdleSeconds = 60
	}
	if cfg.MaxConns < 1 || cfg.MaxConns > 512 {
		cfg.MaxConns = 64
	}
	if cfg.MaxFrameBytes < 512 || cfg.MaxFrameBytes > 65536 {
		cfg.MaxFrameBytes = 8192
	}
	if cfg.LogLines < 100 || cfg.LogLines > 10000 {
		cfg.LogLines = 500
	}
	if cfg.MaxVinsPerConn < 1 || cfg.MaxVinsPerConn > 1024 {
		cfg.MaxVinsPerConn = 128
	}
	return cfg
}

// Start 启动服务端;force=false 且 IP 非环回时拒绝(安全默认,spec §5.4)。
func (s *ServerService) Start(cfg ServerConfig, force bool) (servermode.Status, error) {
	if !force && cfg.IP != "127.0.0.1" && cfg.IP != "localhost" {
		return servermode.Status{}, fmt.Errorf("监听地址 %s 非环回,需在页面二次确认", cfg.IP)
	}
	// 端口 0 = 系统分配临时端口(集成/桥接测试用);UI 层由 a-input-number min=1 挡住(评审 M3)
	if cfg.Port < 0 || cfg.Port > 65535 {
		return servermode.Status{}, fmt.Errorf("端口非法: %d", cfg.Port)
	}
	if cfg.IdleSeconds < 5 || cfg.IdleSeconds > 3600 {
		return servermode.Status{}, fmt.Errorf("空闲时长须在 5~3600 秒")
	}
	if cfg.MaxConns < 1 || cfg.MaxConns > 512 {
		return servermode.Status{}, fmt.Errorf("最大连接数须在 1~512")
	}
	if cfg.MaxFrameBytes < 512 || cfg.MaxFrameBytes > 65536 {
		return servermode.Status{}, fmt.Errorf("帧上限须在 512~65536 字节")
	}
	if cfg.LogLines < 100 || cfg.LogLines > 10000 {
		return servermode.Status{}, fmt.Errorf("日志保留行数须在 100~10000")
	}
	if cfg.MaxVinsPerConn < 1 || cfg.MaxVinsPerConn > 1024 {
		return servermode.Status{}, fmt.Errorf("平台链路 VIN 数上限须在 1~1024")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil && s.srv.Status().Running {
		return s.srv.Status(), nil // 幂等
	}
	// 未运行时无条件按本次配置重建 Server:Stop 后换端口重启、首次绑定失败后
	// 改端口重试都必须生效(旧实例的 Addr 已固化,复用会永久绑错地址)。
	// 停机即清会话语义合理——Sessions/ExportLog 均要求运行中。
	// 空闲参数同样在此固化(修复:此前仅 UpdateIdle 能改,Start 的 IdleEnabled/IdleSeconds 被忽略)。
	base := servermode.DefaultConfig(fmt.Sprintf("%s:%d", cfg.IP, cfg.Port))
	base.IdleEnabled = cfg.IdleEnabled
	base.IdleTimeout = time.Duration(cfg.IdleSeconds) * time.Second
	base.MaxConns = cfg.MaxConns
	base.MaxFrameBytes = cfg.MaxFrameBytes
	base.LogLines = cfg.LogLines
	base.MaxVinsPerConn = cfg.MaxVinsPerConn
	s.srv = servermode.New(base, servermode.Hooks{
		OnStatus:  func(st servermode.Status) { s.emit("server:status", st) },
		OnSession: func(e servermode.SessionEvent) { s.emit("server:session", e) },
		OnFrame:   func(e servermode.FrameEvent) { s.emit("server:frame", e) },
		OnWarn:    func(e servermode.WarnEvent) { s.emit("server:warn", e) },
	})
	if err := s.srv.Start(context.Background()); err != nil {
		return servermode.Status{}, err
	}
	s.last = cfg
	_ = store.Save(serverCfgFile, cfg)
	return s.srv.Status(), nil
}

func (s *ServerService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return nil
	}
	return s.srv.Stop()
}

// UpdateIdle 运行中更新空闲检测(透传 Server.UpdateIdle,AC-10 即时生效)并持久化。
func (s *ServerService) UpdateIdle(enabled bool, idleSeconds int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil || !s.srv.Status().Running {
		return fmt.Errorf("服务未启动")
	}
	if idleSeconds < 5 || idleSeconds > 3600 {
		return fmt.Errorf("空闲时长须在 5~3600 秒")
	}
	s.srv.UpdateIdle(enabled, time.Duration(idleSeconds)*time.Second)
	cfg := s.last // 副本:保留连接/帧/日志/VIN 上限等高级参数
	cfg.IdleEnabled = enabled
	cfg.IdleSeconds = idleSeconds
	_ = store.Save(serverCfgFile, cfg)
	s.last = cfg
	return nil
}

// ClearLog 清空服务端导出缓冲(页面「清空日志」)。
func (s *ServerService) ClearLog() error {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil || !srv.Status().Running {
		return fmt.Errorf("服务未启动")
	}
	srv.ClearLog()
	return nil
}

func (s *ServerService) Status() servermode.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return servermode.Status{}
	}
	return s.srv.Status()
}

func (s *ServerService) Sessions() []servermode.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return nil
	}
	return s.srv.Sessions()
}

// ExportLog 弹出保存对话框,导出环形缓冲文本(AC-5)。
func (s *ServerService) ExportLog() (string, error) {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil || !srv.Status().Running {
		return "", fmt.Errorf("服务未启动")
	}
	path, err := wailsRuntime.SaveFileDialog(s.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: fmt.Sprintf("servermode-%s.log", strings.ReplaceAll(time.Now().Format("20060102-150405"), ":", "")),
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 用户取消
	}
	lines := srv.ExportLines()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
