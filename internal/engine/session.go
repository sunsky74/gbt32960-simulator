// session.go 会话生命周期:会话派生/取消/结束与自动重连调度。
package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/sunsky74/gb32960/types"
)

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

// doConnect 执行 建链→登录→启动后台循环 的完整序列。
// lifeCtx 为整个连接生命期(跨自动重连复用),会话级取消由 beginSession 派生。
func (c *Client) doConnect(lifeCtx context.Context) error {
	// 防御性清理上一次会话残留(无活动会话时为空操作)
	c.endSession(c.currentSessID())
	// 新链接:登入流水号账本清零(同一链接内按 VIN 从 1 递增;表6/表29)
	c.resetLoginSerials()

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
