// message_service_extreport.go 扩展命令周期上报的启停与自清理。
package bridge

import (
	"fmt"
	"strings"
	"time"

	"gbt32960-simulator/internal/engine"
)

// SetExtAutoReport 开关扩展命令的周期发送(每命令独立 ticker)。
func (s *MessageService) SetExtAutoReport(key string, enabled bool, intervalSec int) error {
	if !enabled {
		s.stopExtReport(key)
		return nil
	}
	if intervalSec <= 0 {
		intervalSec = 10
	}
	p := s.rt.Pack()
	if p == nil {
		return fmt.Errorf("未绑定扩展包")
	}
	cmd := packCommand(p, key)
	if cmd == nil {
		return fmt.Errorf("扩展命令不存在: %s", key)
	}
	if p.Meta.BaseVersion != s.versionText() {
		return fmt.Errorf("扩展包基准版本 %s 与当前协议版本 %s 不匹配", p.Meta.BaseVersion, s.versionText())
	}
	if cmd.Trigger == "manual" {
		return fmt.Errorf("该命令不支持周期上报 (trigger=manual)") // 评审 P2-4:消费 trigger 语义
	}
	s.startExtReport(key, time.Duration(intervalSec)*time.Second)
	return nil
}

func (s *MessageService) stopExtReport(key string) {
	s.extMu.Lock()
	stop, ok := s.extStops[key]
	if ok {
		delete(s.extStops, key)
		close(stop)
	}
	s.extMu.Unlock()
}

// stopExtReportIfOwn 仅当注册表中 key 仍指向 own 时才删除并关闭它,
// 防止在途 ticker 的自停路径误杀解绑→重绑后新注册的 ticker(TOCTOU)。
func (s *MessageService) stopExtReportIfOwn(key string, own chan struct{}) {
	s.extMu.Lock()
	defer s.extMu.Unlock()
	if s.extStops[key] == own {
		delete(s.extStops, key)
		close(own)
	}
}

func (s *MessageService) startExtReport(key string, interval time.Duration) {
	s.extMu.Lock()
	if old, ok := s.extStops[key]; ok {
		close(old)
		delete(s.extStops, key)
	}
	stop := make(chan struct{})
	s.extStops[key] = stop
	s.extMu.Unlock()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				// 包被解绑/命令消失/版本切换 → 自停清理
				p := s.rt.Pack()
				if p == nil || packCommand(p, key) == nil || p.Meta.BaseVersion != s.versionText() {
					s.stopExtReportIfOwn(key, stop)
					return
				}
				if err := s.SendExtension(key); err != nil {
					// 未连接属常态,静默跳过;其余错误进事件总线
					if !strings.Contains(err.Error(), "未连接") {
						s.rt.Bus().Emit(engine.Event{Kind: engine.EventError, Message: "扩展命令周期上报失败: " + err.Error()})
					}
				}
			}
		}
	}()
}
