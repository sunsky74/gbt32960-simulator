// client.go 客户端核心:状态机、运行参数与连接/断开生命周期。
package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

// State 客户端状态机取值。
type State string

const (
	StateIdle       State = "idle"       // 未连接
	StateConnecting State = "connecting" // TCP/TLS 建链中
	StateLoggingIn  State = "loggingIn"  // 已发 0x01 等应答
	StateOnline     State = "online"     // 登录成功,可上报
)

// Options 单客户端运行参数。
type Options struct {
	Host           string
	Port           int
	Version        api.GBTVersion
	VIN            string
	ICCID          string
	SubsystemCodes []string // 可充电储能子系统编码(登录报文)

	// 企业平台级联模式:TCP 建链后先发 0x05 平台登入(帧头 VIN = PlatformVIN,
	// 体含账号/密码),车辆数据照常转发(车辆 VIN),断开时补发 0x06 平台登出。
	PlatformMode bool
	PlatformVIN  string
	PlatformUser string
	PlatformPass string

	HeartbeatInterval time.Duration // 0x07 心跳间隔,0=不发
	LoginTimeout      time.Duration // 登录应答超时
	LoginRetries      int           // 登录重试次数
	AutoClockSync     bool          // 登录成功后自动 0x08 校时
	AutoReconnect     bool          // 断线自动重连
	ReconnectDelay    time.Duration // 自动重连间隔,0=3s
	TLS               *tls.Config
}

func (o *Options) normalize() {
	if o.LoginTimeout <= 0 {
		o.LoginTimeout = 5 * time.Second
	}
	if o.LoginRetries <= 0 {
		o.LoginRetries = 3
	}
	if o.ReconnectDelay <= 0 {
		o.ReconnectDelay = 3 * time.Second
	}
}

// ackMsg 平台应答(响应标志 != 0xFE 的帧)。
type ackMsg struct {
	cmd  byte
	resp types.ResponseType
	raw  []byte // payload 原始字节
}

// emptyBody 心跳/校时等无数据单元的报文体。
type emptyBody struct{ v api.GBTVersion }

func (e emptyBody) Version() api.GBTVersion { return e.v }
func (e emptyBody) Bytes() ([]byte, error)  { return []byte{}, nil }

// Client 是模拟器的唯一协议客户端:自动登录/心跳/登出/周期上报。
// 并发安全;所有状态迁移与收发均向 Bus 广播事件。
type Client struct {
	opts Options
	bus  *Bus

	mu    sync.Mutex
	state State
	conn  net.Conn
	// serialByVIN 各 VIN 的登入流水号计数器:同一链接内按 VIN 独立递增,从 1 起
	// (表6/表29「从 1 开始循环累加」)。
	serialByVIN map[string]uint16
	// loginSerialByVIN 各 VIN 当次登入使用的流水号:登出必须复用同一值
	// (表20/表28/表22/表30「登出流水号与当次登入流水号一致」)。
	loginSerialByVIN map[string]uint16

	writeMu sync.Mutex
	cancel  context.CancelFunc
	lifeCtx context.Context

	// 会话级生命周期:每次 doConnect 递增 sessID 并派生独立 ctx;
	// 重连/断开时旧会话的读循环与心跳循环随会话 ctx 取消而退出,避免叠加。
	sessID       uint64
	sessCancel   context.CancelFunc
	reconnecting bool // 自动重连单飞守卫

	acks chan ackMsg

	reportMu     sync.Mutex
	reportTicker *time.Ticker
	reportStop   chan struct{}
	reportFn     func() error
}

// NewClient 创建客户端(未连接)。
func NewClient(opts Options, bus *Bus) *Client {
	opts.normalize()
	if bus == nil {
		bus = NewBus()
	}
	return &Client{opts: opts, bus: bus, state: StateIdle, acks: make(chan ackMsg, 32)}
}

// State 返回当前状态。
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Version 当前客户端实际使用的协议版本(连接建立时的快照)。
func (c *Client) Version() api.GBTVersion { return c.opts.Version }

func (c *Client) setState(s State) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
}

// Bus 返回事件总线。
func (c *Client) Bus() *Bus { return c.bus }

// resetLoginSerials 新链接建立时清空流水号账本:同一链接内按 VIN 从 1 起递增。
func (c *Client) resetLoginSerials() {
	c.mu.Lock()
	c.serialByVIN = make(map[string]uint16)
	c.loginSerialByVIN = make(map[string]uint16)
	c.mu.Unlock()
}

// nextLoginSerial 分配指定 VIN 的下一次登入流水号并记为「当次登入流水号」。
// 同一链接内同一 VIN 每次登入 +1(含重试尝试);登出复用该值。
func (c *Client) nextLoginSerial(vin string) uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureSerialMaps()
	c.serialByVIN[vin]++
	c.loginSerialByVIN[vin] = c.serialByVIN[vin]
	return c.serialByVIN[vin]
}

// currentLoginSerial 返回指定 VIN 当次登入流水号供登出使用;该 VIN 从未登入过时
// 分配一个,保证登出报文仍带可用流水号。
func (c *Client) currentLoginSerial(vin string) uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureSerialMaps()
	if c.loginSerialByVIN[vin] == 0 {
		c.serialByVIN[vin]++
		c.loginSerialByVIN[vin] = c.serialByVIN[vin]
	}
	return c.loginSerialByVIN[vin]
}

// ensureSerialMaps 惰性初始化流水号账本(NewClient 后未连接即直接调用亦可用)。
func (c *Client) ensureSerialMaps() {
	if c.serialByVIN == nil {
		c.serialByVIN = make(map[string]uint16)
	}
	if c.loginSerialByVIN == nil {
		c.loginSerialByVIN = make(map[string]uint16)
	}
}

// ---------------------------------------------------------------- 连接生命周期

// Connect 建链并自动登录。阻塞直到登录成功或失败。
// ctx 控制整个连接生命期(拨号/会话派生/自动重连资格),取消将中止本次连接;
// 持续运行的模拟器通常传入 context.Background()。
func (c *Client) Connect(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.State() != StateIdle {
		return fmt.Errorf("gbt32960-sim: current state is %s, disconnect first", c.State())
	}
	cctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.lifeCtx = cctx
	c.mu.Unlock()

	if err := c.doConnect(cctx); err != nil {
		cancel()
		c.mu.Lock()
		c.cancel = nil
		c.lifeCtx = nil
		c.mu.Unlock()
		c.closeConn()
		c.setState(StateIdle)
		return err
	}
	return nil
}

// Disconnect 优雅断开:先停上报 → 取消会话(不关连接) → 登出 → 关闭连接 → 取消生命期。
func (c *Client) Disconnect() {
	c.SetAutoReport(0, nil) // 1) 先停周期上报

	c.mu.Lock()
	cancel := c.cancel
	sid := c.sessID
	conn := c.conn
	c.cancel = nil
	c.mu.Unlock()
	if cancel == nil && sid == 0 && conn == nil {
		return // 完全空闲,幂等空操作
	}

	// 2) 尽早取消生命期与会话 ctx:确定性中止进行中的 doConnect/waitAck、
	//    停止心跳与自动重连循环;连接保持打开,供登出报文使用。
	//    先取消生命期,保证重连循环不再发起新拨号(否则旧连接被覆盖后无人关闭)。
	if cancel != nil {
		cancel()
	}
	c.cancelSession(sid)

	c.sendLogout(context.Background()) // 3) 尽力而为
	if c.opts.PlatformMode {
		c.sendPlatformLogout(context.Background())
	}

	c.closeConn() // 4) 显式关闭连接
	c.setState(StateIdle)
	c.bus.Emit(Event{Kind: EventConn, Message: "已断开连接, 状态: IDLE"})
}

func (c *Client) closeConn() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}
