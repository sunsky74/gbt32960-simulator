// report.go 心跳与周期上报循环。
package engine

import (
	"context"
	"time"

	"github.com/sunsky74/gb32960/model"
)

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
