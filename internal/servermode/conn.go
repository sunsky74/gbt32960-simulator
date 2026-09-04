package servermode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

// conn 单个车端连接:帧循环 + 协议状态(authed)。
// serve/nextFrame 见下方(空闲判定走 cfgSnapshot 支持运行中更新)。
type conn struct {
	nc     net.Conn
	fr     *framing.FrameReader
	srv    *Server
	authed bool
	vin    string
}

func (c *conn) handleRaw(ctx context.Context, raw []byte) {
	// 超长防护在 framing 层完成(NewFrameReaderLimit + ErrFrameTooLarge,见 serve);
	// 此处不再重复判长。
	d := decodeFrame(raw)
	if d.Err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: d.Err.Error(), Hex: fmt.Sprintf("%x", raw)})
		return // 重同步由 FrameReader 保证,不断连
	}
	now := c.srv.hooks.Now()

	// 帧事件(kind 三态;2025 只读标注)
	cmdName := fmt.Sprintf("0x%02X", d.Cmd)
	if d.PM != nil && d.PM.Payload != nil {
		cmdName = fmt.Sprintf("0x%02X", d.Cmd) // 摘要后续可扩展为解码要点
	}
	sum := ""
	if d.Version == api.V2025 {
		sum = "2025 只读,应答未支持"
	}
	c.srv.hooks.OnFrame(FrameEvent{
		Time: now, VIN: d.VIN, Cmd: cmdName, Hex: fmt.Sprintf("%x", raw), Summary: sum, Kind: d.Kind,
		Unauthed: d.Version == api.V2016 && !c.authed && d.Cmd != 0x01,
		Dir:      DirRX,
	})
	if c.vin == "" && d.VIN != "" {
		c.vin = d.VIN
	}
	// 环形行落地(AC-5):每帧一行 [时间] [VIN] [命令] [hex]
	c.srv.buf.add(fmt.Sprintf("[%s] [%s] [%s] [%s]", now.Format("2006-01-02 15:04:05"), d.VIN, cmdName, fmt.Sprintf("%x", raw)))

	// 加密帧:不解析不应答
	if d.Encrypted {
		c.srv.hooks.OnWarn(WarnEvent{Note: "加密帧不支持解密,仅展示", Hex: ""})
		return
	}
	// 2025:只读,不应答
	if d.Version == api.V2025 {
		return
	}
	// 2016 处理器矩阵
	switch d.Cmd {
	case 0x01:
		c.handleLogin(d, now)
	case 0x02, 0x03:
		c.handleData(d, now)
	case 0x04:
		c.handleLogout(d, now)
	case 0x07:
		c.handleHeartbeat(d, now)
	case 0x08:
		c.handleClock(d, now)
	default: // 0x05/0x06/未知
		c.srv.hooks.OnWarn(WarnEvent{Note: fmt.Sprintf("命令 0x%02X 不在服务端支持范围(平台链路/未知命令)", d.Cmd)})
	}
	// RX 计数放在处理器之后:登入帧须等 handleLogin 注册会话后才能计数
	c.srv.registry.Count(d.VIN, 1, 0)
}

func (c *conn) reply(cmd byte, resp types.ResponseType, body []byte) {
	raw, err := buildReply(api.V2016, c.vin, cmd, resp, body)
	if err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: "应答构造失败: " + err.Error()})
		return
	}
	if _, err := c.nc.Write(raw); err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: "应答发送失败: " + err.Error()})
		return
	}
	// TX 方向遥测:应答帧进入报文流(不进导出环形缓冲——AC-5 冻结为接收侧)
	c.srv.hooks.OnFrame(FrameEvent{
		Time: c.srv.hooks.Now(), VIN: c.vin, Cmd: fmt.Sprintf("0x%02X", cmd),
		Hex: fmt.Sprintf("%x", raw), Summary: respText(resp), Kind: KindNormal, Dir: DirTX,
	})
	c.srv.registry.Count(c.vin, 0, 1)
}

func respText(resp types.ResponseType) string {
	switch resp {
	case types.ResponseSuccess:
		return "应答 成功(0x01)"
	case types.ResponseFailed:
		return "应答 错误(0x02)"
	default:
		return fmt.Sprintf("应答 0x%02X", byte(resp))
	}
}

func (c *conn) handleLogin(d Decoded, now time.Time) {
	if ok := c.srv.registry.Register(d.VIN, c.nc.RemoteAddr().String(), now); !ok {
		c.srv.hooks.OnWarn(WarnEvent{Note: "重复登入拒绝: " + d.VIN})
		c.reply(0x01, types.ResponseFailed, nil)
		return
	}
	c.authed = true
	c.vin = d.VIN
	c.reply(0x01, types.ResponseSuccess, nil)
	c.srv.hooks.OnSession(SessionEvent{VIN: d.VIN, Peer: c.nc.RemoteAddr().String(), Online: true, LastSeen: now})
}

func (c *conn) handleData(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的数据帧被丢弃: " + d.VIN})
		return
	}
	c.srv.registry.Touch(c.vin, now)
	c.reply(d.Cmd, types.ResponseSuccess, nil)
}

func (c *conn) handleLogout(d Decoded, now time.Time) {
	// 只回应答;会话注销与 offline 事件由 removeConn 在连接真正关闭后发出
	// (车端先收到 ack、后观察到掉线——与 Task 7 集成断言一致,评审 M2)。
	c.reply(0x04, types.ResponseSuccess, nil)
	time.AfterFunc(200*time.Millisecond, func() { _ = c.nc.Close() }) // 等 ack flush(D8)
}

func (c *conn) handleHeartbeat(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的心跳被丢弃"})
		return
	}
	c.srv.registry.Touch(c.vin, now)
	c.reply(0x07, types.ResponseSuccess, nil)
}

func (c *conn) handleClock(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的校时被丢弃"})
		return
	}
	c.reply(0x08, types.ResponseSuccess, clockBody(now))
}

// serve 连接帧循环。空闲判定每轮走 cfgSnapshot(支持运行中 UpdateIdle 即时生效);
// 超长帧经 framing 层限长直接告警续读(评审 B4);读超时以 net.Error.Timeout()
// 识别并输出 spec 指定文案(评审 m2)。
func (c *conn) serve(ctx context.Context) {
	defer c.srv.removeConn(c)
	c.fr = framing.NewFrameReaderLimit(c.nc, c.srv.cfgSnapshot().MaxFrameBytes)
	for {
		if ctx.Err() != nil {
			return
		}
		if cfg := c.srv.cfgSnapshot(); cfg.IdleEnabled {
			_ = c.nc.SetReadDeadline(c.srv.hooks.Now().Add(cfg.IdleTimeout))
		} else {
			_ = c.nc.SetReadDeadline(time.Time{})
		}
		raw, err := c.fr.Next()
		if err != nil {
			if ctx.Err() != nil {
				return // 主动停机
			}
			if errors.Is(err, framing.ErrFrameTooLarge) {
				c.srv.hooks.OnWarn(WarnEvent{Note: "帧超长丢弃(>8KB)"})
				continue // 已重同步,继续读后续帧
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && c.srv.cfgSnapshot().IdleEnabled {
				c.srv.hooks.OnWarn(WarnEvent{Note: "空闲超时关闭: " + c.vin}) // spec §5.2 指定文案
				return
			}
			c.srv.hooks.OnWarn(WarnEvent{Note: "连接断开: " + err.Error(), Hex: c.vin})
			return
		}
		c.handleRaw(ctx, raw)
	}
}
