package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all" // 注册全部编解码器
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	mdl25 "github.com/sunsky74/gb32960/model/gbt2025"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"

	"gbt32960-simulator/internal/framing"
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

	mu     sync.Mutex
	state  State
	conn   net.Conn
	serial uint16

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

func (c *Client) nextSerial() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.serial++
	return c.serial
}

// ---------------------------------------------------------------- 帧构建

// BuildFrame 组装完整协议帧(封帧+BCC),不发送。Preview 与发送共用。
func BuildFrame(v api.GBTVersion, vin string, cmd byte, body model.MessageBody) ([]byte, string, error) {
	rt := commandFor(v, cmd)
	if rt == nil {
		return nil, "", fmt.Errorf("gbt32960-sim: unknown command 0x%02X", cmd)
	}
	msg := frameMessage(v, vin, cmd, rt, types.ResponseCommand, body)
	raw, err := msg.Bytes()
	if err != nil {
		return nil, "", err
	}
	return raw, cmdName(rt), nil
}

// frameMessage 构造协议帧对象(发送/应答共用)。
func frameMessage(v api.GBTVersion, vin string, cmd byte, rt any, respType types.ResponseType, body model.MessageBody) *frame.ProtocolMessage {
	return &frame.ProtocolMessage{
		Version:      v,
		RequestType:  rt,
		ResponseType: respType,
		VIN:          vin,
		Encryption:   types.EncryptionNone,
		Payload:      body,
	}
}

func cmdName(rt any) string {
	switch c := rt.(type) {
	case *types.CommandV2016:
		return fmt.Sprintf("0x%02X %s", c.Code, c.Name)
	case *types.CommandV2025:
		return fmt.Sprintf("0x%02X %s", c.Code, c.Name)
	}
	return "unknown"
}

// connVIN 连接级帧(平台登入/登出/心跳/校时)使用的帧头 VIN:
// 企业平台级联模式下为平台标识,否则为车辆 VIN。
func (c *Client) connVIN() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opts.PlatformMode {
		return c.opts.PlatformVIN
	}
	return c.opts.VIN
}

// writeFrame 以车辆 VIN 发帧(车辆登入/登出/数据上报/下行应答)。
func (c *Client) writeFrame(ctx context.Context, cmd byte, body model.MessageBody) error {
	c.mu.Lock()
	vin := c.opts.VIN
	c.mu.Unlock()
	return c.writeFrameAs(ctx, vin, cmd, body)
}

// writeFrameAs 以指定 VIN 加锁写帧并发 TX 事件。
func (c *Client) writeFrameAs(ctx context.Context, vin string, cmd byte, body model.MessageBody) error {
	c.mu.Lock()
	conn := c.conn
	version := c.opts.Version
	c.mu.Unlock()
	if conn == nil {
		return errors.New("gbt32960-sim: not connected")
	}

	raw, name, err := BuildFrame(version, vin, cmd, body)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetWriteDeadline(deadline)
	_, werr := conn.Write(raw)
	c.writeMu.Unlock()
	if werr != nil {
		return fmt.Errorf("gbt32960-sim: write: %w", werr)
	}

	c.bus.Emit(Event{
		Kind:  EventTx,
		Cmd:   name,
		Hex:   utils.BytesToHex(raw),
		Bytes: len(raw),
	})
	return nil
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

// doConnect 执行 建链→登录→启动后台循环 的完整序列。
// lifeCtx 为整个连接生命期(跨自动重连复用),会话级取消由 beginSession 派生。
func (c *Client) doConnect(lifeCtx context.Context) error {
	// 防御性清理上一次会话残留(无活动会话时为空操作)
	c.endSession(c.currentSessID())

	c.setState(StateConnecting)
	c.bus.Emit(Event{Kind: EventConn, Message: fmt.Sprintf("正在连接 %s:%d ...", c.opts.Host, c.opts.Port)})

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(lifeCtx, "tcp", net.JoinHostPort(c.opts.Host, fmt.Sprint(c.opts.Port)))
	if err != nil {
		c.bus.Emit(Event{Kind: EventError, Message: "连接失败: " + err.Error()})
		return err
	}
	if c.opts.TLS != nil {
		tconn := tls.Client(conn, c.opts.TLS)
		tconn.SetDeadline(time.Now().Add(5 * time.Second))
		if err := tconn.HandshakeContext(lifeCtx); err != nil {
			_ = conn.Close()
			c.bus.Emit(Event{Kind: EventError, Message: "TLS 握手失败: " + err.Error()})
			return err
		}
		_ = tconn.SetDeadline(time.Time{})
		conn = tconn
		c.bus.Emit(Event{Kind: EventConn, Message: "TLS 握手完成"})
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.bus.Emit(Event{Kind: EventConn, Message: "TCP 已连接 " + conn.RemoteAddr().String()})

	// 读循环先启动,登录应答由它送入 acks;读/心跳循环绑定本会话,随会话取消而退出
	sessCtx, id := c.beginSession(lifeCtx)
	go c.readLoop(sessCtx, id)
	go c.heartbeatLoop(sessCtx, id)

	// 企业平台级联:先 0x05 平台登入(重试口径与车辆登入一致)
	if c.opts.PlatformMode {
		if err := c.platformLoginPhase(sessCtx); err != nil {
			c.endSession(id)
			return err
		}
	}

	// 登录(带重试)
	c.setState(StateLoggingIn)
	var loginErr error
	for attempt := 1; attempt <= c.opts.LoginRetries; attempt++ {
		if err := c.sendLogin(sessCtx); err != nil {
			loginErr = err
			break
		}
		ack, err := c.waitAck(sessCtx, 0x01, c.opts.LoginTimeout)
		if err == nil {
			if ack.resp != types.ResponseSuccess {
				loginErr = fmt.Errorf("平台拒绝登录: %s", responseText(ack.resp))
				c.bus.Emit(Event{Kind: EventError, Message: loginErr.Error()})
				break
			}
			loginErr = nil
			break
		}
		loginErr = err
		if errors.Is(err, context.Canceled) {
			break
		}
		c.bus.Emit(Event{Kind: EventError, Message: fmt.Sprintf("登录应答超时(第 %d/%d 次)", attempt, c.opts.LoginRetries)})
	}
	if loginErr != nil {
		c.endSession(id)
		return loginErr
	}

	// 登录成功但会话已被取消(如并发 Disconnect),不得宣告 ONLINE
	if sessCtx.Err() != nil {
		c.endSession(id)
		return sessCtx.Err()
	}

	c.setState(StateOnline)
	c.bus.Emit(Event{Kind: EventConn, Message: "车辆登录成功 (0x01), 状态: ONLINE"})

	if c.opts.AutoClockSync {
		go c.sendClockSync(sessCtx)
	}
	return nil
}

// ---------------------------------------------------------------- 会话管理

// currentSessID 返回当前会话 ID(0 表示无活动会话)。
func (c *Client) currentSessID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessID
}

// beginSession 开启新会话:派生独立 ctx、递增 sessID,并清空上一会话
// 遗留的应答(迟到 0x01 不得满足新登录)。返回会话 ctx 与 ID。
func (c *Client) beginSession(lifeCtx context.Context) (context.Context, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessID++
	ctx, cancel := context.WithCancel(lifeCtx)
	c.sessCancel = cancel
	for {
		select {
		case <-c.acks:
			continue
		default:
		}
		break
	}
	return ctx, c.sessID
}

// cancelSession 取消指定会话(不关闭连接);ID 不匹配或为 0 时为空操作。
func (c *Client) cancelSession(id uint64) {
	if id == 0 {
		return
	}
	c.mu.Lock()
	if c.sessID != id {
		c.mu.Unlock()
		return
	}
	c.sessID = 0
	cancel := c.sessCancel
	c.sessCancel = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// endSession 结束指定会话:取消会话 ctx 并在同一临界区原子清空连接,
// 锁外关闭连接。用于登录失败/读错误等自清理路径。
func (c *Client) endSession(id uint64) {
	if id == 0 {
		return
	}
	c.mu.Lock()
	if c.sessID != id {
		c.mu.Unlock()
		return
	}
	c.sessID = 0
	cancel := c.sessCancel
	c.sessCancel = nil
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

// sendLogin 发送 0x01 车辆登录(按版本构造报文体:V2016 共享码长,V2025 每码独立长度)。
func (c *Client) sendLogin(ctx context.Context) error {
	codes := c.opts.SubsystemCodes
	codeLen := 0
	for _, s := range codes {
		if len(s) > codeLen {
			codeLen = len(s)
		}
	}

	var body model.MessageBody
	if c.opts.Version == api.V2025 {
		lengths := make([]int, len(codes))
		for i, s := range codes {
			lengths[i] = len(s)
		}
		body = &mdl25.VehicleLoginV2025{
			BeanTime:  BeanTimeNow(),
			SerialNum: int(c.nextSerial()),
			ICCID:     c.opts.ICCID,
			Count:     len(codes),
			Lengths:   lengths,
			Codes:     codes,
		}
	} else {
		body = &mdl.VehicleLogin{
			BeanTime:  BeanTimeNow(),
			SerialNum: int(c.nextSerial()),
			ICCID:     c.opts.ICCID,
			Count:     len(codes),
			Length:    codeLen,
			Codes:     codes,
		}
	}
	return c.writeFrame(ctx, 0x01, body)
}

// sendLogout 发送 0x04 车辆登出(尽力而为)。
func (c *Client) sendLogout(ctx context.Context) {
	logoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body := &mdl.VehicleLogout{
		BeanTime:  BeanTimeNow(),
		SerialNum: int(c.nextSerial()),
	}
	if err := c.writeFrame(logoutCtx, 0x04, body); err != nil {
		c.bus.Emit(Event{Kind: EventError, Message: "发送登出报文失败: " + err.Error()})
		return
	}
	// 最多等 500ms 应答,不强求
	_, _ = c.waitAck(logoutCtx, 0x04, 500*time.Millisecond)
}

// platformLoginPhase 发送 0x05 平台登入并等待应答(重试口径与车辆登入一致)。
func (c *Client) platformLoginPhase(ctx context.Context) error {
	c.setState(StateLoggingIn)
	c.bus.Emit(Event{Kind: EventConn, Message: "正在平台登入 (0x05) ..."})
	var perr error
	for attempt := 1; attempt <= c.opts.LoginRetries; attempt++ {
		if err := c.sendPlatformLogin(ctx); err != nil {
			perr = err
			break
		}
		ack, err := c.waitAck(ctx, 0x05, c.opts.LoginTimeout)
		if err == nil {
			if ack.resp != types.ResponseSuccess {
				perr = fmt.Errorf("平台拒绝登入: %s", responseText(ack.resp))
				c.bus.Emit(Event{Kind: EventError, Message: perr.Error()})
				break
			}
			perr = nil
			break
		}
		perr = err
		if errors.Is(err, context.Canceled) {
			break
		}
		c.bus.Emit(Event{Kind: EventError, Message: fmt.Sprintf("平台登入应答超时(第 %d/%d 次)", attempt, c.opts.LoginRetries)})
	}
	if perr != nil {
		return perr
	}
	c.bus.Emit(Event{Kind: EventConn, Message: "平台登入成功 (0x05)"})
	return nil
}

// sendPlatformLogin 发送 0x05 平台登入(帧头 VIN = 平台标识;账号/密码为
// 协议定长字段,编解码器自动空格填充)。2025 复用同一线格式。
func (c *Client) sendPlatformLogin(ctx context.Context) error {
	bean, serial := BeanTimeNow(), int(c.nextSerial())
	user, pass := c.opts.PlatformUser, c.opts.PlatformPass
	var body model.MessageBody
	if c.opts.Version == api.V2025 {
		body = &mdl25.PlatformLoginV2025{
			BeanTime: bean, SerialNum: serial,
			Username: user, Password: pass, Cipher: byte(types.EncryptionNone),
		}
	} else {
		body = &mdl.PlatformLogin{
			BeanTime: bean, SerialNum: serial,
			Username: user, Password: pass, Cipher: byte(types.EncryptionNone),
		}
	}
	return c.writeFrameAs(ctx, c.connVIN(), 0x05, body)
}

// sendPlatformLogout 发送 0x06 平台登出(尽力而为,与车辆登出同口径)。
func (c *Client) sendPlatformLogout(ctx context.Context) {
	logoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var body model.MessageBody
	if c.opts.Version == api.V2025 {
		body = &mdl25.PlatformLogoutV2025{BeanTime: BeanTimeNow(), SerialNum: int(c.nextSerial())}
	} else {
		body = &mdl.PlatformLogout{BeanTime: BeanTimeNow(), SerialNum: int(c.nextSerial())}
	}
	if err := c.writeFrameAs(logoutCtx, c.connVIN(), 0x06, body); err != nil {
		c.bus.Emit(Event{Kind: EventError, Message: "发送平台登出失败: " + err.Error()})
		return
	}
	_, _ = c.waitAck(logoutCtx, 0x06, 500*time.Millisecond)
}

// sendClockSync 发送 0x08 校时请求。
func (c *Client) sendClockSync(ctx context.Context) {
	if err := c.writeFrameAs(ctx, c.connVIN(), 0x08, emptyBody{v: c.opts.Version}); err == nil {
		c.bus.Emit(Event{Kind: EventConn, Message: "已发送校时请求 (0x08)"})
	}
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

// waitAck 等待指定命令的应答。不匹配的应答丢弃。
func (c *Client) waitAck(ctx context.Context, cmd byte, timeout time.Duration) (ackMsg, error) {
	deadline := time.After(timeout)
	for {
		select {
		case <-ctx.Done():
			return ackMsg{}, ctx.Err()
		case <-deadline:
			return ackMsg{}, fmt.Errorf("gbt32960-sim: wait ack 0x%02X timeout", cmd)
		case a := <-c.acks:
			if a.cmd == cmd {
				return a, nil
			}
			// 其他应答忽略,继续等
		}
	}
}

// ---------------------------------------------------------------- 读循环

func (c *Client) readLoop(ctx context.Context, id uint64) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	fr := framing.NewFrameReader(conn)
	for {
		raw, err := fr.Next()
		if err != nil {
			if ctx.Err() != nil {
				return // 会话主动结束
			}
			c.handleReadError(id, err)
			return
		}
		c.handleFrame(raw)
	}
}

// handleReadError 处理指定会话的读错误;过期会话的报错直接忽略。
func (c *Client) handleReadError(id uint64, err error) {
	c.mu.Lock()
	if c.sessID != id {
		c.mu.Unlock()
		return // 过期会话,忽略
	}
	life := c.lifeCtx
	c.mu.Unlock()

	c.endSession(id)
	c.bus.Emit(Event{Kind: EventError, Message: "连接断开: " + err.Error()})

	if c.opts.AutoReconnect && life != nil && life.Err() == nil {
		c.setState(StateConnecting)
		c.startReconnectOnce(life)
	} else {
		c.setState(StateIdle)
		c.mu.Lock()
		cancel := c.cancel
		c.cancel = nil
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		c.bus.Emit(Event{Kind: EventConn, Message: "状态: IDLE"})
	}
}

// startReconnectOnce 启动单飞自动重连循环:已有重连在途时直接返回。
func (c *Client) startReconnectOnce(lifeCtx context.Context) {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			c.reconnecting = false
			c.mu.Unlock()
		}()
		for {
			c.bus.Emit(Event{Kind: EventConn, Message: "3 秒后自动重连 ..."})
			select {
			case <-lifeCtx.Done():
				return
			case <-time.After(c.reconnectDelay()):
			}
			if lifeCtx.Err() != nil {
				return
			}
			if err := c.doConnect(lifeCtx); err == nil {
				return
			}
		}
	}()
}

func (c *Client) reconnectDelay() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opts.ReconnectDelay
}

// handleFrame 解码单帧 → 发 RX 事件 → 应答送入 acks。
func (c *Client) handleFrame(raw []byte) {
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		c.bus.Emit(Event{
			Kind:    EventError,
			Message: "帧解码失败: " + err.Error(),
			Hex:     utils.BytesToHex(raw),
			Bytes:   len(raw),
		})
		return
	}
	pm := msg.(*frame.ProtocolMessage)
	_ = pm.DecodePayload() // 尽力解码 payload,失败不影响帧级展示

	name := "unknown"
	if code, ok := frame.CommandCode(pm.RequestType); ok {
		cmdLabel := ""
		switch v := pm.RequestType.(type) {
		case *types.CommandV2016:
			cmdLabel = v.Name
		case *types.CommandV2025:
			cmdLabel = v.Name
		}
		if alias, ok := extCommandName(pm.Version, raw[2]); ok {
			cmdLabel = alias
		}
		name = fmt.Sprintf("0x%02X %s", code, cmdLabel)
	}
	respSuffix := ""
	if pm.ResponseType != types.ResponseCommand {
		respSuffix = " [应答:" + responseText(pm.ResponseType) + "]"
	}

	// 校时应答:解析平台时间并给出偏差
	if code, ok := frame.CommandCode(pm.RequestType); ok && code == 0x08 && pm.ResponseType != types.ResponseCommand {
		if bt := parseBeanTime(pm.RawBytes); bt != nil {
			plat := time.Date(bt.Year+2000, time.Month(bt.Month), bt.Day, bt.Hour, bt.Minute, bt.Second, 0, time.Local)
			offset := time.Since(plat).Truncate(time.Second)
			c.bus.Emit(Event{Kind: EventConn, Message: fmt.Sprintf("平台时间 %s, 与本地偏差 %s", plat.Format("2006-01-02 15:04:05"), offset)})
		}
	}

	c.bus.Emit(Event{
		Kind:     EventRx,
		Cmd:      name + respSuffix,
		Hex:      utils.BytesToHex(raw),
		Bytes:    len(raw),
		Downlink: downlinkInfo(raw[2], pm),
	})

	if pm.ResponseType != types.ResponseCommand {
		code, _ := frame.CommandCode(pm.RequestType)
		select {
		case c.acks <- ackMsg{cmd: code, resp: pm.ResponseType, raw: pm.RawBytes}:
		default:
		}
	}
}

// parseBeanTime 用注册的 BeanTime codec 解析 6 字节十进制时间。
func parseBeanTime(raw []byte) *model.BeanTime {
	bt := reflect.TypeOf((*model.BeanTime)(nil)).Elem()
	c := api.GetCodec(api.V2016, bt)
	if c == nil || len(raw) < 6 {
		return nil
	}
	m, err := c.Decode(utils.NewByteReader(raw))
	if err != nil {
		return nil
	}
	t, ok := m.(*model.BeanTime)
	if !ok {
		return nil
	}
	return t
}

// ---------------------------------------------------------------- 心跳与周期上报

func (c *Client) heartbeatLoop(ctx context.Context, id uint64) {
	c.mu.Lock()
	interval := c.opts.HeartbeatInterval
	c.mu.Unlock()
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if c.State() != StateOnline {
				continue
			}
			if err := c.writeFrameAs(ctx, c.connVIN(), 0x07, emptyBody{v: c.opts.Version}); err != nil {
				c.bus.Emit(Event{Kind: EventError, Message: "心跳发送失败: " + err.Error()})
				c.handleReadError(id, err)
				return
			}
		}
	}
}

// SetAutoReport 设置周期上报。interval<=0 停止;fn 在每次 tick 时由引擎调用
// (通常由 bridge 组装最新配置并调用 Send)。每次重设通过 stop 通道显式
// 结束旧上报协程(Ticker.Stop 不会关闭通道,旧实现因此泄漏)。
func (c *Client) SetAutoReport(interval time.Duration, fn func() error) {
	c.reportMu.Lock()
	defer c.reportMu.Unlock()
	// 先发停止信号再停 ticker:旧协程阻塞在 select 上,可被立即唤醒退出
	if c.reportStop != nil {
		close(c.reportStop)
		c.reportStop = nil
	}
	if c.reportTicker != nil {
		c.reportTicker.Stop()
		c.reportTicker = nil
	}
	if interval <= 0 || fn == nil {
		c.reportFn = nil
		return
	}
	c.reportFn = fn
	ticker := time.NewTicker(interval)
	stop := make(chan struct{})
	c.reportTicker = ticker
	c.reportStop = stop
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				c.reportMu.Lock()
				f := c.reportFn
				c.reportMu.Unlock()
				if f == nil {
					return
				}
				if c.State() != StateOnline {
					continue
				}
				if err := f(); err != nil {
					c.bus.Emit(Event{Kind: EventError, Message: "周期上报失败: " + err.Error()})
				}
			}
		}
	}()
}

// Send 发送任意命令的报文体(实时 0x02 / 补发 0x03 / 其他)。
func (c *Client) Send(ctx context.Context, cmd byte, body model.MessageBody) error {
	return c.writeFrame(ctx, cmd, body)
}

// ---------------------------------------------------------------- 工具

// BeanTimeNow 当前时间的 BeanTime(Year 存 2000 偏移的十进制值,codec 按十进制原字节编码(非 BCD))。
func BeanTimeNow() model.BeanTime {
	n := time.Now()
	return model.BeanTime{
		Year:   n.Year() - 2000,
		Month:  int(n.Month()),
		Day:    n.Day(),
		Hour:   n.Hour(),
		Minute: n.Minute(),
		Second: n.Second(),
	}
}

func responseText(r types.ResponseType) string {
	switch r {
	case types.ResponseSuccess:
		return "成功"
	case types.ResponseFailed:
		return "错误"
	case types.ResponseVINDup:
		return "VIN重复"
	case types.ResponseVINNotExist:
		return "VIN不存在"
	case types.ResponseSignErr:
		return "验签错误"
	case types.ResponseStructureErr:
		return "数据结构错误"
	case types.ResponseDecodeErr:
		return "解密错误"
	default:
		return fmt.Sprintf("0x%02X", byte(r))
	}
}
