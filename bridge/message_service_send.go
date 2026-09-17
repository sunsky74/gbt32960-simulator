// message_service_send.go 报文组装、预览、发送与周期上报控制。
package bridge

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	v2025rt "github.com/sunsky74/gb32960/model/gbt2025/realtime"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

// PreviewResult 发送前的报文预览。
type PreviewResult struct {
	Cmd string `json:"cmd"`
	Hex string `json:"hex"`
	Len int    `json:"len"`
}

// assembleBody 组装 0x02/0x03 报文体:标准体(typed)+ 激活扩展包的追加 TLV。
// 未绑包/版本不符/无启用行 → 原样返回标准体(与既有行为逐字节一致)。
func (s *MessageService) assembleBody(at time.Time) (model.MessageBody, error) {
	groups := s.rt.Groups()
	if groups == nil {
		groups = s.DefaultGroups(s.versionText()).ToMap()
	}
	base, err := schema.Assemble(s.version(), groups, at)
	if err != nil {
		return nil, err
	}
	// 2025 车端签名(表8):配置了签名类型时,签名段位于报文体最末、涵盖其前全部字节。
	sig, err := s.v2025Signature()
	if err != nil {
		return nil, err
	}
	p := s.rt.Pack()
	if p == nil || p.Meta.BaseVersion != s.versionText() {
		return s.withSignature(base, sig), nil
	}
	tail := make([]byte, 0, 64)
	for _, u := range p.Realtime.AppendUnits {
		g, ok := groups[u.Key]
		if !ok || !g.Enabled || len(g.Rows) == 0 {
			continue
		}
		for _, row := range g.Rows { // multiple 语义:每行独立 TLV
			tlv, err := ext.EncodeUnit(u, row)
			if err != nil {
				return nil, fmt.Errorf("扩展单元 %s: %w", u.Key, err)
			}
			tail = append(tail, tlv...)
		}
	}
	if len(tail) == 0 {
		return s.withSignature(base, sig), nil
	}
	baseBytes, err := base.Bytes()
	if err != nil {
		return nil, err
	}
	raw := append(baseBytes, tail...)
	if sig != nil {
		// 原始体路径:签名必须在扩展 TLV 之后手工追加(类型化路径由库 codec 固定排最后)。
		raw = append(raw, encodeSignatureTLV(sig)...)
	}
	return engine.NewRawBody(s.version(), raw), nil
}

// v2025Signature 按连接配置构造 2025 车端签名(表8;TLV 0xFF)。
// R/S 由外部签名工具用设备私钥生成(HEX)——模拟器不持有私钥,只按规范编码;
// 未配置(签名类型为 0)或当前非 2025 版本时返回 nil。
func (s *MessageService) v2025Signature() (*v2025rt.VehicleSignature, error) {
	if s.version() != api.V2025 {
		return nil, nil
	}
	cfg := s.rt.ConnCfg()
	if cfg == nil || cfg.SignatureType <= 0 {
		return nil, nil
	}
	r, err := hex.DecodeString(cfg.SignatureR)
	if err != nil {
		return nil, fmt.Errorf("签名 R 值不是合法 HEX: %w", err)
	}
	sv, err := hex.DecodeString(cfg.SignatureS)
	if err != nil {
		return nil, fmt.Errorf("签名 S 值不是合法 HEX: %w", err)
	}
	return &v2025rt.VehicleSignature{
		Type: byte(cfg.SignatureType), RLength: len(r), RValue: r, SLength: len(sv), SValue: sv,
	}, nil
}

// withSignature 类型化路径附加签名:库 codec 固定把签名 TLV 编码在最后,
// 并按已写入字节刷新 SignData(覆盖数据采集时间起至签名段前)。
func (s *MessageService) withSignature(base model.MessageBody, sig *v2025rt.VehicleSignature) model.MessageBody {
	if sig == nil {
		return base
	}
	if rt, ok := base.(*mdl.RealTimeV2025Data); ok {
		rt.VehicleSignature = sig
	}
	return base
}

// encodeSignatureTLV 手写 0xFF 签名 TLV(原始体路径用,须置于全部 TLV 之后)。
func encodeSignatureTLV(sig *v2025rt.VehicleSignature) []byte {
	out := make([]byte, 0, 7+len(sig.RValue)+len(sig.SValue))
	out = append(out, byte(types.RealTimeV2025Signature), sig.Type,
		byte(sig.RLength>>8), byte(sig.RLength))
	out = append(out, sig.RValue...)
	out = append(out, byte(sig.SLength>>8), byte(sig.SLength))
	out = append(out, sig.SValue...)
	return out
}

// Preview 用当前配置生成 0x02 报文 hex(不发送)。
func (s *MessageService) Preview() (*PreviewResult, error) {
	cfg := s.rt.ConnCfg()
	if cfg == nil {
		cfg = DefaultConnectionConfig()
	}
	body, err := s.assembleBody(time.Now())
	if err != nil {
		return nil, err
	}
	raw, name, err := engine.BuildFrame(parseVersion(cfg.Version), cfg.VIN, 0x02, body)
	if err != nil {
		return nil, err
	}
	return &PreviewResult{Cmd: name, Hex: utils.BytesToHex(raw), Len: len(raw)}, nil
}

// SendRealtime 立即发送一次 0x02 实时信息上报。
func (s *MessageService) SendRealtime() error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录 (state=%s)", s.stateText())
	}
	if s.rt.Groups() == nil {
		return fmt.Errorf("报文配置为空,请先保存报文配置")
	}
	body, err := s.assembleBody(time.Now())
	if err != nil {
		return err
	}
	return c.Send(context.Background(), 0x02, body)
}

// SendReissue 发送 count 条 0x03 补发。时间戳从 now-offsetSec 起,
// 每条按 intervalSec 前移 —— 模拟离线时段内每 intervalSec 采集一条的补报。
func (s *MessageService) SendReissue(count int, offsetSec int, intervalSec int) error {
	if count <= 0 || count > 100 {
		return fmt.Errorf("补发条数须在 1~100 之间")
	}
	if offsetSec < 0 {
		return fmt.Errorf("起始时间偏移不能为负")
	}
	if intervalSec <= 0 || intervalSec > 86400 {
		return fmt.Errorf("补发间隔须在 1~86400 秒之间")
	}
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录 (state=%s)", s.stateText())
	}
	groups := s.rt.Groups()
	if groups == nil {
		return fmt.Errorf("报文配置为空,请先保存报文配置")
	}
	base := time.Now().Add(-time.Duration(offsetSec) * time.Second)
	for i := 0; i < count; i++ {
		at := base.Add(-time.Duration(i) * time.Duration(intervalSec) * time.Second)
		body, err := s.assembleBody(at)
		if err != nil {
			return err
		}
		if err := c.Send(context.Background(), 0x03, body); err != nil {
			return err
		}
	}
	return nil
}

// SetAutoReport 开关周期上报(默认 10s,可改)。enabled=false 时停止并联动
// 停止轨迹回放(回放搭载周期上报,停报后无推进载体)。enabled=true 时每次
// tick 先推进轨迹回放(激活时)再组装发送,实现"每条周期 0x02 携带下一轨迹点"。
func (s *MessageService) SetAutoReport(enabled bool, intervalSec int) error {
	s.reportMu.Lock()
	s.reportOn = enabled
	if intervalSec > 0 {
		s.reportInterval = intervalSec
	}
	s.reportMu.Unlock()
	if !enabled {
		if h := s.replayHook(); h != nil {
			h.StopReplay()
		}
	}
	c := s.rt.CurrentClient()
	if c == nil {
		return fmt.Errorf("未连接")
	}
	if !enabled {
		c.SetAutoReport(0, nil)
		return nil
	}
	if intervalSec <= 0 {
		intervalSec = s.effectiveReportInterval()
	}
	c.SetAutoReport(time.Duration(intervalSec)*time.Second, func() error {
		if s.rt.Groups() == nil {
			return fmt.Errorf("报文配置为空")
		}
		if h := s.replayHook(); h != nil {
			h.advanceForReport()
		}
		body, err := s.assembleBody(time.Now())
		if err != nil {
			return err
		}
		if err := c.Send(context.Background(), 0x02, body); err != nil {
			if h := s.replayHook(); h != nil {
				h.failOnSend(err)
			}
			return err
		}
		return nil
	})
	return nil
}

// ensureAutoReport 确保周期上报处于开启状态:未开启则按记忆间隔(缺省取
// 连接配置 reportInterval,再缺省 10s)开启;已开启也重装 ticker——客户端
// 重建后 ticker 丢失,重装幂等。轨迹导入与回放启动的"默认开周期上报"
// 由此保证。经 TrackDeps 接口由 TrackService 调用。
func (s *MessageService) ensureAutoReport() error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return s.SetAutoReport(true, s.effectiveReportInterval())
}

// resumeAutoReport 客户端重建(手动重连)后按记忆状态恢复周期上报;
// reportOn 为 false 时静默返回(用户已关闭的语义不被扭转)。
// 经 wiring.WireAutoReportResume 注入为连接成功回调。
func (s *MessageService) resumeAutoReport() error {
	s.reportMu.Lock()
	on := s.reportOn
	s.reportMu.Unlock()
	if !on {
		return nil
	}
	return s.SetAutoReport(true, s.effectiveReportInterval())
}

// ReportState 周期上报状态快照(前端展示与同步)。
type ReportState struct {
	On          bool `json:"on"`
	IntervalSec int  `json:"intervalSec"`
}

// AutoReportState 返回周期上报开关与生效间隔。
func (s *MessageService) AutoReportState() ReportState {
	s.reportMu.Lock()
	on, remembered := s.reportOn, s.reportInterval
	s.reportMu.Unlock()
	return ReportState{On: on, IntervalSec: effectiveSec(remembered, s.rt)}
}

// effectiveReportInterval 生效间隔:记忆值 > 连接配置 reportInterval > 10s。
func (s *MessageService) effectiveReportInterval() int {
	return effectiveSec(s.reportInterval, s.rt)
}

func effectiveSec(remembered int, rt *Runtime) int {
	if remembered > 0 {
		return remembered
	}
	if cfg := rt.ConnCfg(); cfg != nil && cfg.ReportInterval > 0 {
		return cfg.ReportInterval
	}
	return 10
}

// ParamRespondRow 0x80 参数查询应答行(前端提交)。
type ParamRespondRow = engine.ParamResponseRow

// RespondAck 对平台下行命令发送应答码(空载荷)。适用 0x81/0x82 标准应答
// 与私有远控 0x8A 第一层 ACK。respCode: 0x01 成功 / 0x02 错误 / ...
func (s *MessageService) RespondAck(cmd byte, respCode byte) error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return c.RespondAck(cmd, types.ResponseType(respCode))
}

// RespondParamQuery 对 0x80 参数查询发送参数值应答。
func (s *MessageService) RespondParamQuery(rows []ParamRespondRow, respCode byte) error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return c.RespondParamQuery(rows, types.ResponseType(respCode))
}

// RespondRemoteSecondLayer 发送私有远控 0x8A 第二层业务应答(回显表21头+信息体)。
func (s *MessageService) RespondRemoteSecondLayer(headerHex string, bodyHex string) error {
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	return c.RespondRemoteSecondLayer(headerHex, bodyHex)
}

func (s *MessageService) stateText() string {
	if c := s.rt.CurrentClient(); c != nil {
		return string(c.State())
	}
	return string(engine.StateIdle)
}

// packCommand 按 key 查找激活包内的扩展命令;未绑定/不存在返回 nil。
func packCommand(p *ext.Pack, key string) *ext.Command {
	if p == nil {
		return nil
	}
	for i := range p.Commands {
		if p.Commands[i].Key == key {
			return &p.Commands[i]
		}
	}
	return nil
}

// assembleCommandBody 组装扩展命令报文体:
// fields=平铺字段单行;realtimeLike=6B 十进制时间 + 单元 TLV×N;
// 私有远控 0x8A 应答模板=表21头(命令时间6B+流水号2B+N=1+子指令码1B)+ 应答体平铺字段。
// 0x8A 模板约定 fields 首字段 key="serialNumber"(u16),编码时提取填入表21头流水号(回显下行请求)。
func (s *MessageService) assembleCommandBody(cmd ext.Command, at time.Time) ([]byte, error) {
	groups := s.rt.Groups()
	if cmd.RemoteSub > 0 {
		grp, ok := groups[cmd.Key]
		if !ok || !grp.Enabled || len(grp.Rows) == 0 {
			return nil, fmt.Errorf("扩展命令 %s 未配置或未启用", cmd.Key)
		}
		row := grp.Rows[0]
		if len(cmd.Body.Fields) == 0 || cmd.Body.Fields[0].Key != "serialNumber" || cmd.Body.Fields[0].Type != "u16" {
			return nil, fmt.Errorf("0x8A 应答模板 %s 首字段须为 serialNumber(u16)", cmd.Key)
		}
		body, err := ext.EncodeFields(cmd.Body.Fields[1:], row)
		if err != nil {
			return nil, fmt.Errorf("扩展命令 %s: %w", cmd.Key, err)
		}
		serial, err := ext.EncodeFields([]ext.FieldSpec{{Key: "serialNumber", Type: "u16"}}, row)
		if err != nil {
			return nil, fmt.Errorf("扩展命令 %s 流水号: %w", cmd.Key, err)
		}
		t := ext.EncodeBeanTime(at)
		out := make([]byte, 0, 10+len(body))
		out = append(out, t[:]...)
		out = append(out, serial[0], serial[1], 0x01, byte(cmd.RemoteSub))
		out = append(out, body...)
		return out, nil
	}
	switch cmd.Body.Type {
	case "fields":
		grp, ok := groups[cmd.Key]
		if !ok || !grp.Enabled || len(grp.Rows) == 0 {
			return nil, fmt.Errorf("扩展命令 %s 未配置或未启用", cmd.Key)
		}
		return ext.EncodeFields(cmd.Body.Fields, grp.Rows[0])
	case "realtimeLike":
		t := ext.EncodeBeanTime(at)
		out := append([]byte{}, t[:]...)
		for _, u := range cmd.Body.Units {
			grp, ok := groups[cmd.Key+":"+u.Key]
			if !ok || !grp.Enabled || len(grp.Rows) == 0 {
				continue
			}
			for _, row := range grp.Rows {
				tlv, err := ext.EncodeUnit(u, row)
				if err != nil {
					return nil, fmt.Errorf("扩展命令 %s 单元 %s: %w", cmd.Key, u.Key, err)
				}
				out = append(out, tlv...)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("扩展命令 %s: 未知体类型 %q", cmd.Key, cmd.Body.Type)
	}
}

// SendExtension 手动发送一次扩展命令(须在线)。
func (s *MessageService) SendExtension(key string) error {
	p := s.rt.Pack()
	if p == nil {
		return fmt.Errorf("未绑定扩展包")
	}
	if p.Meta.BaseVersion != s.versionText() {
		return fmt.Errorf("扩展包基准版本 %s 与当前协议版本 %s 不匹配", p.Meta.BaseVersion, s.versionText())
	}
	cmd := packCommand(p, key)
	if cmd == nil {
		return fmt.Errorf("扩展命令不存在: %s", key)
	}
	c := s.rt.CurrentClient()
	if c == nil || c.State() != engine.StateOnline {
		return fmt.Errorf("未连接或未登录")
	}
	if s.version() != c.Version() {
		return fmt.Errorf("连接档案版本已变更,请重新连接后再发送扩展命令")
	}
	payload, err := s.assembleCommandBody(*cmd, time.Now())
	if err != nil {
		return err
	}
	if cmd.RemoteSub > 0 {
		return c.RespondRemoteAck(payload)
	}
	if cmd.RespType == ext.RespTypeSuccess {
		return c.RespondRaw(byte(cmd.Code), types.ResponseSuccess, payload)
	}
	return c.Send(context.Background(), byte(cmd.Code), engine.NewRawBody(s.version(), payload))
}
