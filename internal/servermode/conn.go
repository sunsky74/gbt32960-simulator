package servermode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"gbt32960-simulator/internal/framing"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

// conn 单个车端连接:帧循环 + 协议状态(authed)。
// platform:本连接为平台链路(0x05 平台登入建立),vins 记录该连接注册的
// 全部会话 VIN(平台标识 + 各车辆;断开时逐一注销,避免多车会话泄漏)。
// serve/nextFrame 见下方(空闲判定走 cfgSnapshot 支持运行中更新)。
//
// 锁序:idMu 守护身份字段(vin/vins/authed/platform),连接自身 goroutine 的
// 读写与 bridge 侧读取(hasVIN/platformFlag)共用;叶子锁——持有时不得调用
// hooks/registry/Server 方法或做 I/O。仅允许 Server.mu → idMu 的嵌套
// (WriteFrameVIN 持 s.mu 选连接时调 hasVIN),反向嵌套禁止。
type conn struct {
	nc       net.Conn
	fr       *framing.FrameReader
	srv      *Server
	idMu     sync.Mutex
	authed   bool
	platform bool
	vin      string
	vins     []string
	writeMu  sync.Mutex // 帧写串行化:读循环应答与外部下发通道共用
}

// markPlatform 标记本连接为平台链路(0x05 平台登入)。
func (c *conn) markPlatform() {
	c.idMu.Lock()
	c.platform = true
	c.idMu.Unlock()
}

// bindAuth 登入成功后绑定会话身份:authed=true、当前 VIN、已注册 VIN 列表追加。
func (c *conn) bindAuth(vin string) {
	c.idMu.Lock()
	c.authed = true
	c.vin = vin
	c.vins = append(c.vins, vin)
	c.idMu.Unlock()
}

// setVINIfEmpty 首帧 VIN 回填:当前 VIN 为空且帧头带 VIN 时置位。
func (c *conn) setVINIfEmpty(v string) {
	c.idMu.Lock()
	if c.vin == "" && v != "" {
		c.vin = v
	}
	c.idMu.Unlock()
}

// hasVIN 该连接是否已登入且注册过 vin。
// 平台链路多车复用:vins 含全部会话 VIN,故早于最后登入者的车辆也可定向。
func (c *conn) hasVIN(vin string) bool {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	return c.authed && slices.Contains(c.vins, vin)
}

// platformFlag 平台链路标记快照(调用方须在 I/O/hooks 前取快照并释放 idMu)。
func (c *conn) platformFlag() bool {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	return c.platform
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
	if r, ok := ExtCmd(d.Cmd); ok && r.Label != "" {
		cmdName = r.Label
	}
	if d.PM != nil && d.PM.Payload != nil {
		cmdName = fmt.Sprintf("0x%02X", d.Cmd) // 摘要后续可扩展为解码要点
		if r, ok := ExtCmd(d.Cmd); ok && r.Label != "" {
			cmdName = r.Label
		}
	}
	sum := ""
	if d.Version == api.V2025 {
		sum = "2025 只读,应答未支持"
	}
	c.srv.hooks.OnFrame(FrameEvent{
		Time: now, VIN: d.VIN, Cmd: cmdName, Hex: fmt.Sprintf("%x", raw), Summary: sum, Kind: d.Kind,
		Unauthed: d.Version == api.V2016 && !c.authed && d.Cmd != 0x01 && d.Cmd != 0x05,
		Dir:      DirRX,
		Platform: c.platform,
	})
	c.setVINIfEmpty(d.VIN)
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
	// 0x05/0x06 平台登入/登出(企业平台级联):会话语义与车辆登入/登出
	// 同构——以帧头 VIN(此处为平台标识)注册会话/应答/关闭。
	case 0x05:
		c.handleLogin(d, now)
	case 0x06:
		c.handleLogout(d, now)
	case 0x07:
		c.handleHeartbeat(d, now)
	case 0x08:
		c.handleClock(d, now)
	default: // 未知命令
		if rule, ok := ExtCmd(d.Cmd); ok {
			c.handleExtCmd(d, now, rule)
		} else {
			c.srv.hooks.OnWarn(WarnEvent{Note: fmt.Sprintf("命令 0x%02X 不在服务端支持范围(平台链路/未知命令)", d.Cmd)})
		}
	}
	// RX 计数放在处理器之后:登入帧须等 handleLogin 注册会话后才能计数
	c.srv.registry.Count(d.VIN, 1, 0)
}

// reply 构造并发送应答帧。vin 必须回显请求帧的 VIN:平台链路多车复用时,
// 连接级 c.vin 是最后一次登入者,不回显请求 VIN 会导致 ACK 与请求对不上。
func (c *conn) reply(vin string, cmd byte, resp types.ResponseType, body []byte) {
	raw, err := buildReply(api.V2016, vin, cmd, resp, body)
	if err != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: "应答构造失败: " + err.Error()})
		return
	}
	c.writeFrame(vin, cmd, raw, respText(resp))
}

// writeFrame 串行写一帧并产生 TX 遥测(reply 与外部下发通道共用)。
func (c *conn) writeFrame(vin string, cmd byte, raw []byte, summary string) {
	// bridge goroutine 可能调用本方法:身份标记先取快照(叶子锁),再释放锁做
	// I/O 与 hooks,绝不持有 idMu 回调外部。
	platform := c.platformFlag()
	c.writeMu.Lock()
	_, werr := c.nc.Write(raw)
	c.writeMu.Unlock()
	if werr != nil {
		c.srv.hooks.OnWarn(WarnEvent{Note: "应答发送失败: " + werr.Error()})
		return
	}
	cmdName := fmt.Sprintf("0x%02X", cmd)
	if r, ok := ExtCmd(cmd); ok && r.Label != "" {
		cmdName = r.Label
	}
	// TX 方向遥测:应答帧进入报文流(不进导出环形缓冲——AC-5 冻结为接收侧)
	c.srv.hooks.OnFrame(FrameEvent{
		Time: c.srv.hooks.Now(), VIN: vin, Cmd: cmdName,
		Hex: fmt.Sprintf("%x", raw), Summary: summary, Kind: KindNormal, Dir: DirTX,
		Platform: platform,
	})
	c.srv.registry.Count(vin, 0, 1)
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
	isPlatformLogin := d.Cmd == 0x05
	// 车辆直连连接只允许一次登入(0x01);已登入后同 VIN/换 VIN/0x05 升级一律拒绝,
	// 且不改动原会话(重复登入的拒绝语义在此前置,Register 的 putIfAbsent 仅兜底)。
	if c.authed && !c.platform {
		c.srv.hooks.OnWarn(WarnEvent{Note: "连接已登入,拒绝重复登入: " + d.VIN})
		c.reply(d.VIN, d.Cmd, types.ResponseFailed, nil)
		return
	}
	// 平台链路 VIN 数上限:c.vins 含 0x05 平台标识,故以 > 比较——上限指可复用的
	// 车辆 VIN 数(平台标识不占额度),与「平台链路 VIN 数超限(上限 N)」文案一致。
	if cfg := c.srv.cfgSnapshot(); len(c.vins) > cfg.MaxVinsPerConn {
		c.srv.hooks.OnWarn(WarnEvent{Note: fmt.Sprintf("平台链路 VIN 数超限(上限 %d),拒绝登入: %s", cfg.MaxVinsPerConn, d.VIN)})
		c.reply(d.VIN, d.Cmd, types.ResponseFailed, nil)
		return
	}
	if isPlatformLogin {
		c.markPlatform() // 0x05 平台登入:本连接此后按平台链路对待
	}
	if ok := c.srv.registry.Register(d.VIN, c.nc.RemoteAddr().String(), now, c.platform); !ok {
		c.srv.hooks.OnWarn(WarnEvent{Note: "重复登入拒绝: " + d.VIN})
		c.reply(d.VIN, d.Cmd, types.ResponseFailed, nil)
		return
	}
	c.bindAuth(d.VIN)
	// 应答回显请求命令码:0x01 车辆登入 / 0x05 平台登入共用本处理器
	c.reply(d.VIN, d.Cmd, types.ResponseSuccess, nil)
	c.srv.hooks.OnSession(SessionEvent{VIN: d.VIN, Peer: c.nc.RemoteAddr().String(), Online: true, LastSeen: now, Platform: c.platform})
}

func (c *conn) handleData(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的数据帧被丢弃: " + d.VIN})
		return
	}
	// 平台链路多车复用:按数据帧自身 VIN 活跃(Touch/Count 同口径)
	c.srv.registry.Touch(d.VIN, now)
	c.reply(d.VIN, d.Cmd, types.ResponseSuccess, nil)
}

// handleExtCmd 扩展命令处理器:请求帧(标志 0xFE)按注册规则回应答;
// 应答帧(终端对下发命令的回执)只读展示不回帧,避免回环。
func (c *conn) handleExtCmd(d Decoded, now time.Time, rule ExtCmdRule) {
	if !rule.Reply {
		return // 仅注册显示名的命令(如应答模板):不自动回帧
	}
	if d.PM != nil && d.PM.ResponseType != types.ResponseCommand {
		return
	}
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的扩展命令帧被丢弃: " + d.VIN})
		return
	}
	c.srv.registry.Touch(d.VIN, now)
	var body []byte
	if rule.Echo {
		// 回显体从原始帧切(头 24B:起始2+cmd1+resp1+vin17+加密1+长度2);私有命令不经库解码
		bodyLen := int(d.Raw[22])<<8 | int(d.Raw[23])
		if len(d.Raw) >= 24+bodyLen {
			body = d.Raw[24 : 24+bodyLen]
		}
	}
	c.reply(d.VIN, d.Cmd, rule.RespType, body)
}

func (c *conn) handleLogout(d Decoded, now time.Time) {
	// 只回应答(回显请求命令码:0x04 车辆登出 / 0x06 平台登出共用);
	// 会话注销与 offline 事件由 removeConn 在连接真正关闭后发出
	// (车端先收到 ack、后观察到掉线——与 Task 7 集成断言一致,评审 M2)。
	c.reply(d.VIN, d.Cmd, types.ResponseSuccess, nil)
	time.AfterFunc(200*time.Millisecond, func() { _ = c.nc.Close() }) // 等 ack flush(D8)
}

func (c *conn) handleHeartbeat(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的心跳被丢弃"})
		return
	}
	c.srv.registry.Touch(d.VIN, now)
	c.reply(d.VIN, d.Cmd, types.ResponseSuccess, nil)
}

func (c *conn) handleClock(d Decoded, now time.Time) {
	if !c.authed {
		c.srv.hooks.OnWarn(WarnEvent{Note: "未登入连接的校时被丢弃"})
		return
	}
	c.srv.registry.Touch(d.VIN, now)
	c.reply(d.VIN, d.Cmd, types.ResponseSuccess, clockBody(now))
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
