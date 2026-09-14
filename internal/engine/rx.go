// rx.go 接收路径:读循环、帧解码与 BeanTime 解析。
package engine

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all" // 注册全部编解码器
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"

	"gbt32960-simulator/internal/framing"
)

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
