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
}

// ServerService 服务端模式的前端绑定面。
type ServerService struct {
	ctx context.Context
	mu  sync.Mutex
	srv *servermode.Server
	// lastIP/lastPort 最近一次启动的地址(UpdateIdle 持久化时回填)
	lastIP   string
	lastPort int
}

func NewServerService() *ServerService { return &ServerService{} }

func (s *ServerService) SetContext(ctx context.Context) { s.ctx = ctx }

func (s *ServerService) emit(name string, data any) {
	if s.ctx == nil {
		return // 测试环境无 wails 上下文
	}
	wailsRuntime.EventsEmit(s.ctx, name, data)
}

func (s *ServerService) LoadConfig() ServerConfig {
	cfg := ServerConfig{IP: "127.0.0.1", Port: 32960, IdleEnabled: true, IdleSeconds: 60}
	_ = store.Load(serverCfgFile, &cfg) // 文件缺失/损坏用默认值
	if cfg.IdleSeconds < 5 || cfg.IdleSeconds > 3600 {
		cfg.IdleSeconds = 60
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil && s.srv.Status().Running {
		return s.srv.Status(), nil // 幂等
	}
	// 未运行时无条件按本次配置重建 Server:Stop 后换端口重启、首次绑定失败后
	// 改端口重试都必须生效(旧实例的 Addr 已固化,复用会永久绑错地址)。
	// 停机即清会话语义合理——Sessions/ExportLog 均要求运行中。
	s.srv = servermode.New(servermode.DefaultConfig(fmt.Sprintf("%s:%d", cfg.IP, cfg.Port)), servermode.Hooks{
		OnStatus:  func(st servermode.Status) { s.emit("server:status", st) },
		OnSession: func(e servermode.SessionEvent) { s.emit("server:session", e) },
		OnFrame:   func(e servermode.FrameEvent) { s.emit("server:frame", e) },
		OnWarn:    func(e servermode.WarnEvent) { s.emit("server:warn", e) },
	})
	if err := s.srv.Start(context.Background()); err != nil {
		return servermode.Status{}, err
	}
	s.lastIP, s.lastPort = cfg.IP, cfg.Port
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
	_ = store.Save(serverCfgFile, ServerConfig{IP: s.lastIP, Port: s.lastPort, IdleEnabled: enabled, IdleSeconds: idleSeconds})
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
