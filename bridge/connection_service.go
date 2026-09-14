package bridge

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/store"
	"gbt32960-simulator/internal/tlsconf"
	"github.com/sunsky74/gb32960/api"
)

// ConnectionConfig 连接配置(持久化 + 引擎 Options 的来源)。
type ConnectionConfig struct {
	Name             string   `json:"name"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Version          string   `json:"version"` // "2016" | "2025"
	VIN              string   `json:"vin"`
	ICCID            string   `json:"iccid"`
	SubsystemCodes   []string `json:"subsystemCodes"`
	HeartbeatSec     int      `json:"heartbeatSec"`     // 0x07 间隔,0=不发,默认 30
	AutoClockSync    bool     `json:"autoClockSync"`    // 登录后自动校时
	AutoReconnect    bool     `json:"autoReconnect"`    // 断线重连
	ReportInterval   int      `json:"reportInterval"`   // 周期上报间隔秒,0=不发,默认 10
	ReissueOffsetSec int      `json:"reissueOffsetSec"` // 补发时间戳偏移(秒)

	// 企业平台级联:0x05 平台登入(平台 VIN/账号/密码)→ 车辆数据转发 → 0x06 平台登出。
	PlatformMode bool   `json:"platformMode"`
	PlatformVIN  string `json:"platformVin,omitempty"`  // 平台标识(17 位,目标平台颁发)
	PlatformUser string `json:"platformUser,omitempty"` // 平台账号(协议定长 12 字节,自动填充)
	PlatformPass string `json:"platformPass,omitempty"` // 平台密码(协议定长 20 字节,自动填充)

	ExtensionPack string         `json:"extensionPack,omitempty"` // 绑定的扩展包 id(空=不使用)
	TLS           tlsconf.Config `json:"tls"`
}

const (
	profilesFile   = "profiles.json"
	legacyConnFile = "connection.json"
)

// DefaultConnectionConfig 返回带默认值的配置。
func DefaultConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		Name:           "默认连接",
		Host:           "127.0.0.1",
		Port:           32960,
		Version:        "2016",
		VIN:            "LSV00000000000001",
		ICCID:          "89860000000000000001",
		HeartbeatSec:   30,
		AutoClockSync:  true,
		AutoReconnect:  false,
		ReportInterval: 10,
	}
}

// TestResult 连通性测试结果。
type TestResult struct {
	OK        bool   `json:"ok"`
	ElapsedMs int64  `json:"elapsedMs"`
	Message   string `json:"message"`
}

// ProfileSummary 连接档案摘要(导航栏展示:名称 + IP)。
type ProfileSummary struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Active bool   `json:"active"`
}

// profilesData 多连接档案的持久化形态。
type profilesData struct {
	Active string             `json:"active"`
	Items  []ConnectionConfig `json:"items"`
}

// ConnectionService 连接相关的前端服务。
type ConnectionService struct {
	rt *Runtime

	// connectMu 串行化 Connect:进行中时直接拒绝并发调用,避免两次连接
	// 各自取代客户端后留下孤儿长连。Disconnect 有意不加锁——它必须能在
	// 任意时刻取消进行中的 Connect(引擎侧由会话 ctx 取消完成)。
	connectMu sync.Mutex
}

// NewConnectionService 创建服务。
func NewConnectionService(rt *Runtime) *ConnectionService { return &ConnectionService{rt: rt} }

// getProfiles 读取档案;无档案时尝试迁移旧版单配置文件,再兜底空档案。
func (s *ConnectionService) getProfiles() (*profilesData, error) {
	var pd profilesData
	if err := store.Load(profilesFile, &pd); err != nil {
		return nil, err
	}
	if pd.Items != nil {
		if pd.Active == "" && len(pd.Items) > 0 {
			pd.Active = pd.Items[0].Name
		}
		return &pd, nil
	}

	// 迁移旧版 connection.json
	var legacy ConnectionConfig
	if err := store.Load(legacyConnFile, &legacy); err == nil && legacy.Name != "" {
		migrated := &profilesData{Active: legacy.Name, Items: []ConnectionConfig{legacy}}
		_ = store.Save(profilesFile, migrated)
		return migrated, nil
	}
	return &profilesData{}, nil
}

// GetProfiles 返回全部连接档案摘要(供导航栏)。
func (s *ConnectionService) GetProfiles() ([]ProfileSummary, error) {
	pd, err := s.getProfiles()
	if err != nil {
		return nil, err
	}
	out := make([]ProfileSummary, 0, len(pd.Items))
	for _, it := range pd.Items {
		out = append(out, ProfileSummary{Name: it.Name, Host: it.Host, Port: it.Port, Active: it.Name == pd.Active})
	}
	return out, nil
}

// GetConfig 读取当前激活档案;无档案时返回默认值。命中即回填 Runtime
// (启动链 loadInitialData → GetConfig 由此完成连接配置与扩展包激活)。
func (s *ConnectionService) GetConfig() (*ConnectionConfig, error) {
	pd, err := s.getProfiles()
	if err != nil {
		return nil, err
	}
	for i := range pd.Items {
		if pd.Items[i].Name == pd.Active {
			s.rt.SetConnCfg(&pd.Items[i])
			return &pd.Items[i], nil
		}
	}
	def := DefaultConnectionConfig()
	s.rt.SetConnCfg(def)
	return def, nil
}

// SaveConfig 校验并按名称 upsert 档案,保存后即设为激活。
func (s *ConnectionService) SaveConfig(cfg ConnectionConfig) error {
	if err := validateConn(&cfg); err != nil {
		return err
	}
	pd, err := s.getProfiles()
	if err != nil {
		return err
	}
	replaced := false
	for i := range pd.Items {
		if pd.Items[i].Name == cfg.Name {
			pd.Items[i] = cfg
			replaced = true
			break
		}
	}
	if !replaced {
		pd.Items = append(pd.Items, cfg)
	}
	pd.Active = cfg.Name
	s.rt.SetConnCfg(&cfg)
	return store.Save(profilesFile, pd)
}

// SwitchProfile 切换激活档案并返回其完整配置。
func (s *ConnectionService) SwitchProfile(name string) (*ConnectionConfig, error) {
	pd, err := s.getProfiles()
	if err != nil {
		return nil, err
	}
	for i := range pd.Items {
		if pd.Items[i].Name == name {
			pd.Active = name
			if err := store.Save(profilesFile, pd); err != nil {
				return nil, err
			}
			s.rt.SetConnCfg(&pd.Items[i])
			return &pd.Items[i], nil
		}
	}
	return nil, fmt.Errorf("连接档案不存在: %s", name)
}

// DeleteProfile 删除档案;若删除的是激活档案则激活第一条剩余档案。
func (s *ConnectionService) DeleteProfile(name string) error {
	pd, err := s.getProfiles()
	if err != nil {
		return err
	}
	idx := -1
	for i := range pd.Items {
		if pd.Items[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("连接档案不存在: %s", name)
	}
	pd.Items = append(pd.Items[:idx], pd.Items[idx+1:]...)
	if pd.Active == name {
		if len(pd.Items) > 0 {
			pd.Active = pd.Items[0].Name
		} else {
			pd.Active = ""
		}
	}
	return store.Save(profilesFile, pd)
}

// TestConnect 测试 TCP(或 TCP+TLS)可达性,不发送协议帧。
func (s *ConnectionService) TestConnect(cfg ConnectionConfig) TestResult {
	if err := validateConn(&cfg); err != nil {
		return TestResult{OK: false, Message: err.Error()}
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	start := time.Now()

	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return TestResult{OK: false, Message: "TCP 连接失败: " + err.Error()}
	}
	defer func() { _ = conn.Close() }()

	suffix := "TCP 可达"
	if cfg.TLS.Enabled {
		tlsCfg, terr := cfg.TLS.Build()
		if terr != nil {
			return TestResult{OK: false, Message: "TLS 配置错误: " + terr.Error()}
		}
		tconn := tls.Client(conn, tlsCfg)
		_ = tconn.SetDeadline(time.Now().Add(3 * time.Second))
		if herr := tconn.Handshake(); herr != nil {
			return TestResult{OK: false, Message: "TCP 可达, 但 TLS 握手失败: " + herr.Error()}
		}
		suffix = "TCP + TLS 握手成功"
	}
	return TestResult{OK: true, ElapsedMs: time.Since(start).Milliseconds(), Message: suffix}
}

// Connect 用给定配置建链并自动登录(阻塞至成功/失败)。
func (s *ConnectionService) Connect(cfg ConnectionConfig) error {
	if !s.connectMu.TryLock() {
		return fmt.Errorf("连接进行中,请稍候")
	}
	defer s.connectMu.Unlock()

	if err := validateConn(&cfg); err != nil {
		return err
	}
	if err := s.SaveConfig(cfg); err != nil {
		return err
	}

	// 单活动连接:先断开旧客户端
	if old := s.rt.CurrentClient(); old != nil {
		old.Disconnect()
	}

	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return fmt.Errorf("TLS 配置错误: %w", err)
	}

	client := engine.NewClient(engine.Options{
		Host:              cfg.Host,
		Port:              cfg.Port,
		Version:           parseVersion(cfg.Version),
		VIN:               cfg.VIN,
		ICCID:             cfg.ICCID,
		SubsystemCodes:    cfg.SubsystemCodes,
		PlatformMode:      cfg.PlatformMode,
		PlatformVIN:       cfg.PlatformVIN,
		PlatformUser:      cfg.PlatformUser,
		PlatformPass:      cfg.PlatformPass,
		HeartbeatInterval: time.Duration(cfg.HeartbeatSec) * time.Second,
		AutoClockSync:     cfg.AutoClockSync,
		AutoReconnect:     cfg.AutoReconnect,
		TLS:               tlsCfg,
	}, s.rt.Bus())
	s.rt.replaceClient(client)

	if err := client.Connect(context.Background()); err != nil {
		return err
	}
	return nil
}

// Disconnect 自动登出并断开。
func (s *ConnectionService) Disconnect() error {
	if c := s.rt.CurrentClient(); c != nil {
		c.Disconnect()
	}
	return nil
}

// State 返回当前状态字符串(idle/connecting/loggingIn/online)。
func (s *ConnectionService) State() string {
	if c := s.rt.CurrentClient(); c != nil {
		return string(c.State())
	}
	return string(engine.StateIdle)
}

func validateConn(cfg *ConnectionConfig) error {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.VIN = strings.TrimSpace(cfg.VIN)
	cfg.ICCID = strings.TrimSpace(cfg.ICCID)
	cfg.PlatformVIN = strings.TrimSpace(cfg.PlatformVIN)
	cfg.PlatformUser = strings.TrimSpace(cfg.PlatformUser)
	if cfg.Host == "" {
		return fmt.Errorf("IP 地址不能为空")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("端口非法: %d", cfg.Port)
	}
	if len(cfg.VIN) != 17 {
		return fmt.Errorf("VIN 必须为 17 位,当前 %d 位", len(cfg.VIN))
	}
	if len(cfg.ICCID) > 20 {
		return fmt.Errorf("ICCID 最长 20 位,当前 %d 位", len(cfg.ICCID))
	}
	if cfg.Version != "2016" && cfg.Version != "2025" {
		return fmt.Errorf("协议版本必须为 2016 或 2025")
	}
	if cfg.PlatformMode {
		if len(cfg.PlatformVIN) != 17 {
			return fmt.Errorf("平台标识 VIN 必须为 17 位(由目标平台颁发),当前 %d 位", len(cfg.PlatformVIN))
		}
		if cfg.PlatformUser == "" {
			return fmt.Errorf("平台账号不能为空")
		}
		if len(cfg.PlatformUser) > 12 {
			return fmt.Errorf("平台账号最长 12 位,当前 %d 位", len(cfg.PlatformUser))
		}
		if len(cfg.PlatformPass) > 20 {
			return fmt.Errorf("平台密码最长 20 位,当前 %d 位", len(cfg.PlatformPass))
		}
	}
	return nil
}

func parseVersion(v string) api.GBTVersion {
	if v == "2025" {
		return api.V2025
	}
	return api.V2016
}
