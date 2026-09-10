package bridge

import (
	"fmt"
	"sync"

	"gbt32960-simulator/internal/store"
)

const (
	appSettingsFile         = "settings.json"
	defaultConsoleExportCap = 50000
	minConsoleExportCap     = 1000
	maxConsoleExportCap     = 500000
)

// AppSettings 应用级设置(持久化于 settings.json)。
type AppSettings struct {
	ConsoleExportCap int `json:"consoleExportCap"`
}

// SettingsService 应用设置的前端绑定面。
type SettingsService struct {
	fwd *Forwarder
	mu  sync.Mutex
	cfg AppSettings
}

// NewSettingsService 加载已存设置(尽力而为,非法/零值回落默认)并应用到转发器。
func NewSettingsService(fwd *Forwarder) *SettingsService {
	cfg := AppSettings{ConsoleExportCap: defaultConsoleExportCap}
	_ = store.Load(appSettingsFile, &cfg)
	if cfg.ConsoleExportCap < minConsoleExportCap || cfg.ConsoleExportCap > maxConsoleExportCap {
		cfg.ConsoleExportCap = defaultConsoleExportCap
	}
	if fwd != nil {
		fwd.SetCap(cfg.ConsoleExportCap)
	}
	return &SettingsService{fwd: fwd, cfg: cfg}
}

// GetAppSettings 当前设置(已裁剪到合法范围)。
func (s *SettingsService) GetAppSettings() AppSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetAppSettings 校验范围后持久化,并即时应用到导出缓冲容量。
func (s *SettingsService) SetAppSettings(cfg AppSettings) error {
	if cfg.ConsoleExportCap < minConsoleExportCap || cfg.ConsoleExportCap > maxConsoleExportCap {
		return fmt.Errorf("导出缓冲条数须在 1000~500000")
	}
	if err := store.Save(appSettingsFile, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	if s.fwd != nil {
		s.fwd.SetCap(cfg.ConsoleExportCap)
	}
	return nil
}
