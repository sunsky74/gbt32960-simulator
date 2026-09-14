package servermode

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// Server 平台侧接收服务:accept loop + per-conn goroutine。
type Server struct {
	cfg      Config
	hooks    Hooks
	registry *Registry
	buf      *ring

	mu      sync.Mutex
	ln      net.Listener
	conns   map[*conn]struct{}
	running bool
	cancel  context.CancelFunc
}

// New 创建服务(未启动)。/hooks 经 normalizeHooks 填充默认值;
// 非正数配置项回落默认值(与 DefaultConfig 同值),保证零值 Config 可直接使用。
func New(cfg Config, hooks Hooks) *Server {
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 64
	}
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = 8192
	}
	if cfg.MaxVinsPerConn <= 0 {
		cfg.MaxVinsPerConn = 128
	}
	if cfg.LogLines <= 0 {
		cfg.LogLines = 500
	}
	return &Server{
		cfg:      cfg,
		hooks:    normalizeHooks(hooks),
		registry: NewRegistry(),
		buf:      newRing(cfg.LogLines),
		conns:    map[*conn]struct{}{},
	}
}

// Start 监听并进入 accept loop。幂等:已运行返回 nil。
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("监听失败: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s.ln, s.running, s.cancel = ln, true, cancel
	s.mu.Unlock()

	s.hooks.OnStatus(Status{Running: true, ListenAddr: ln.Addr().String()})
	go s.acceptLoop(ln, ctx)
	return nil
}

// acceptLoop 参数化捕获 ln:避免无锁读 s.ln 在 Stop→快速 Start 换新 listener 时
// 构成数据 race(评审 minor-1)。
func (s *Server) acceptLoop(ln net.Listener, ctx context.Context) {
	for {
		nc, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.hooks.OnWarn(WarnEvent{Note: "accept: " + err.Error()})
				select {
				case <-ctx.Done():
					return
				case <-time.After(50 * time.Millisecond):
				}
				continue
			}
		}
		s.mu.Lock()
		if len(s.conns) >= s.cfg.MaxConns {
			s.mu.Unlock()
			_ = nc.Close() // 超限立即拒绝(AC-9)
			continue
		}
		c := &conn{nc: nc, srv: s}
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		go c.serve(ctx)
	}
}

// Stop 停止监听、主动关闭全部连接(阻塞读被唤醒 → removeConn 发 offline)并等待
// 退出(≤2s)。幂等:未运行时直接返回 nil(评审 M4:去掉 stopOnce,支持 停止→再启动→再停止)。
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	ln, cancel := s.ln, s.cancel
	s.running = false
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	var err error
	if ln != nil {
		err = ln.Close()
	}
	if cancel != nil {
		cancel()
	}
	for _, c := range conns {
		_ = c.nc.Close() // 唤醒阻塞读;authed 连接经 removeConn 发 offline + 注销
	}
	deadline := time.After(2 * time.Second) // 停机上限(AC-9)
	for {
		s.mu.Lock()
		n := len(s.conns)
		s.mu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-deadline:
			s.hooks.OnStatus(Status{Running: false})
			return err
		case <-time.After(20 * time.Millisecond):
		}
	}
	s.hooks.OnStatus(Status{Running: false})
	return err
}

// Status 当前运行状态。
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.ln == nil {
		return Status{Running: false}
	}
	return Status{Running: true, ListenAddr: s.ln.Addr().String()}
}

// Sessions 会话快照(bridge 下发前端)。
func (s *Server) Sessions() []Session { return s.registry.Snapshot() }

// ExportLines 导出环形缓冲为文本行: [时间] [VIN] [命令] [hex](AC-5)。
func (s *Server) ExportLines() []string { return s.buf.snapshot() }

// ClearLog 清空导出环形缓冲(页面「清空日志」)。
func (s *Server) ClearLog() { s.buf.clear() }

// removeConn 连接退出清理 + offline 事件。
// 平台链路(0x05)可能注册多个会话(平台标识 + 各车辆),断开须逐一注销;
// 车辆直连 vins 只有一个,行为与旧版单 VIN 注销一致。
func (s *Server) removeConn(c *conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	_ = c.nc.Close()
	// 仅已登入连接才注销会话:被拒的重复登入连接 vin 已置位但未 authed,
	// 若按 vin 注销会误删原会话(评审 watch-out)
	if !c.authed {
		return
	}
	for _, vin := range c.vins {
		if _, ok := s.registry.Remove(vin); ok {
			s.hooks.OnSession(SessionEvent{
				VIN: vin, Peer: c.nc.RemoteAddr().String(), Online: false,
				LastSeen: s.hooks.Now(), Platform: c.platform,
			})
		}
	}
}

// cfgSnapshot 并发安全地读取当前配置(serve 循环每轮调用)。
func (s *Server) cfgSnapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// UpdateIdle 运行中更新空闲检测(AC-10「即时生效」,评审 M1):改配置后逐连接
// 主动重设读超时——阻塞在 Next() 的静默连接立即被唤醒:OFF→ON 时超时倒计时开始;
// ON→OFF 时 deadline 清零。serve 循环下一轮按新配置继续。
func (s *Server) UpdateIdle(enabled bool, d time.Duration) {
	s.mu.Lock()
	s.cfg.IdleEnabled = enabled
	if enabled {
		s.cfg.IdleTimeout = d
	}
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		if enabled {
			_ = c.nc.SetReadDeadline(s.hooks.Now().Add(d))
		} else {
			_ = c.nc.SetReadDeadline(time.Time{})
		}
	}
}

// WriteFrameVIN 向指定 VIN 的已登入会话写入一帧原始字节(平台下发通道,
// bridge 组帧后调用;产生 TX 遥测)。会话不在线或未登入返回错误。
// 平台链路多车复用:命中任一已注册该 VIN 的连接(hasVIN),不再只认最后登入者。
func (s *Server) WriteFrameVIN(vin string, cmd byte, raw []byte, summary string) error {
	s.mu.Lock()
	var target *conn
	for c := range s.conns {
		if c.hasVIN(vin) { // 锁序 Server.mu → idMu;hasVIN 为叶子,不回调
			target = c
			break
		}
	}
	s.mu.Unlock()
	if target == nil {
		return fmt.Errorf("车辆 %s 不在线或未登入", vin)
	}
	target.writeFrame(vin, cmd, raw, summary)
	return nil
}
