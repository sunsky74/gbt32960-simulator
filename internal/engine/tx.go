// tx.go 发送路径:帧构建与各命令报文写出。
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	mdl25 "github.com/sunsky74/gb32960/model/gbt2025"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

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

// Send 发送任意命令的报文体(实时 0x02 / 补发 0x03 / 其他)。
func (c *Client) Send(ctx context.Context, cmd byte, body model.MessageBody) error {
	return c.writeFrame(ctx, cmd, body)
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
