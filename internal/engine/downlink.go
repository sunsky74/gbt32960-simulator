package engine

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

// ---------------------------------------------------------------- 下行解析(控制台展示用)

// DownlinkParam 0x81 设置命令中的单个参数项。
type DownlinkParam struct {
	ID     int    `json:"id"`
	Length int    `json:"length"`
	Hex    string `json:"hex"`
}

// DownlinkInfo 平台下行命令的解析结果。
// 标准 0x80/0x81/0x82 与 私有远控 私有 0x8A(两层应答远控)。
type DownlinkInfo struct {
	Cmd  byte   `json:"cmd"`
	Kind string `json:"kind"` // query|setup|control|remote

	ParamIDs   []int           `json:"paramIds,omitempty"`  // 0x80: 查询的参数 ID 列表
	Params     []DownlinkParam `json:"params,omitempty"`    // 0x81: 设置的参数项
	ControlHex string          `json:"controlHex,omitempty"` // 0x82: 控制命令字节

	// 私有远控 0x8A 表21 头(10B)+ 信息体
	CommandTime  string `json:"commandTime,omitempty"`
	SerialNumber int    `json:"serialNumber,omitempty"`
	CommandCount int    `json:"commandCount,omitempty"`
	InfoTypeFlag int    `json:"infoTypeFlag,omitempty"`
	HeaderHex    string `json:"headerHex,omitempty"`
	BodyHex      string `json:"bodyHex,omitempty"`
}

// 私有远控 表21 头长度: 命令时间(6B) + 流水号(2B) + 命令总数(1B) + 信息类型标志(1B)。
const remoteHeaderLen = 10

// ParseDownlink 解析平台下行的 payload。cmd 不在已知集合时返回 nil。
func ParseDownlink(cmd byte, payload []byte) *DownlinkInfo {
	switch cmd {
	case 0x80:
		return parseParamQuery(payload)
	case 0x81:
		return parseParamSetup(payload)
	case 0x82:
		return &DownlinkInfo{Cmd: cmd, Kind: "control", ControlHex: utils.BytesToHex(payload)}
	case 0x8A:
		return parseRemoteControl(payload)
	}
	return nil
}

func parseParamQuery(payload []byte) *DownlinkInfo {
	info := &DownlinkInfo{Cmd: 0x80, Kind: "query"}
	for _, b := range payload {
		info.ParamIDs = append(info.ParamIDs, int(b))
	}
	return info
}

func parseParamSetup(payload []byte) *DownlinkInfo {
	info := &DownlinkInfo{Cmd: 0x81, Kind: "setup"}
	if len(payload) < 1 {
		return info
	}
	count := int(payload[0])
	pos := 1
	for i := 0; i < count && pos+1 < len(payload); i++ {
		id := int(payload[pos])
		vlen := int(payload[pos+1])<<8 | int(payload[pos+2])
		pos += 3
		if pos+vlen > len(payload) {
			break
		}
		info.Params = append(info.Params, DownlinkParam{
			ID:     id,
			Length: vlen,
			Hex:    utils.BytesToHex(payload[pos : pos+vlen]),
		})
		pos += vlen
	}
	return info
}

func parseRemoteControl(payload []byte) *DownlinkInfo {
	info := &DownlinkInfo{Cmd: 0x8A, Kind: "remote"}
	if len(payload) < remoteHeaderLen {
		info.BodyHex = utils.BytesToHex(payload)
		return info
	}
	header := payload[:remoteHeaderLen]
	body := payload[remoteHeaderLen:]

	info.CommandTime = fmt.Sprintf("20%02d-%02d-%02d %02d:%02d:%02d",
		header[0], header[1], header[2], header[3], header[4], header[5])
	info.SerialNumber = int(header[6])<<8 | int(header[7])
	info.CommandCount = int(header[8])
	info.InfoTypeFlag = int(header[9])
	info.HeaderHex = utils.BytesToHex(header)
	info.BodyHex = utils.BytesToHex(body)
	return info
}

// ---------------------------------------------------------------- 应答组装

// ParamResponseRow 0x80 参数查询应答的一行:参数 ID + 值的原始字节(hex)。
type ParamResponseRow struct {
	ID  int    `json:"id"`
	Hex string `json:"hex"` // 值字节(不含 ID),如 u16 的 3E8 → "0ee8"? 由前端按类型生成 hex
}

// commandFor 取发送用命令对象。优先查扩展命令注册表(构造 Min=Max=code 精确副本),
// 未注册则回退库枚举区间查找(标准命令)。
func commandFor(v api.GBTVersion, cmd byte) any {
	if name, ok := extCommandName(v, cmd); ok {
		switch v {
		case api.V2025:
			return &types.CommandV2025{Code: cmd, Name: name, Min: cmd, Max: cmd}
		default:
			return &types.CommandV2016{Code: cmd, Name: name, Min: cmd, Max: cmd}
		}
	}
	switch v {
	case api.V2025:
		return types.CommandV2025ByCode(cmd)
	default:
		return types.CommandV2016ByCode(cmd)
	}
}

// rawBody 以原始字节实现 MessageBody(应答/私有心跳等场景)。
type rawBody struct {
	v api.GBTVersion
	b []byte
}

func (r rawBody) Version() api.GBTVersion { return r.v }
func (r rawBody) Bytes() ([]byte, error)  { return r.b, nil }

// NewRawBody 构造原始字节报文体。
func NewRawBody(v api.GBTVersion, b []byte) model.MessageBody { return rawBody{v: v, b: b} }

// BuildParamQueryResponse 组装 0x80 查询应答的数据单元:
// 参数个数 u8 + (参数ID u8 + 参数值字节)×N。值长度由各行 hex 决定。
func BuildParamQueryResponse(rows []ParamResponseRow) ([]byte, error) {
	if len(rows) > 255 {
		return nil, fmt.Errorf("参数行数超限: %d", len(rows))
	}
	w := utils.NewByteWriter()
	w.WriteUint8(byte(len(rows)))
	for _, r := range rows {
		val, err := hex.DecodeString(strings.TrimSpace(r.Hex))
		if err != nil {
			return nil, fmt.Errorf("参数 0x%02X 值不是合法 hex: %w", r.ID, err)
		}
		if len(val) > 255 {
			return nil, fmt.Errorf("参数 0x%02X 值过长: %d 字节", r.ID, len(val))
		}
		w.WriteUint8(byte(r.ID))
		w.WriteBytes(val)
	}
	return w.Bytes(), nil
}

// BuildRemoteSecondLayer 组装 私有远控 0x8A 第二层业务应答载荷:
// 回显表21头(10B,来自下行) + 用户编辑的信息体 hex。
func BuildRemoteSecondLayer(headerHex, bodyHex string) ([]byte, error) {
	header, err := hex.DecodeString(strings.TrimSpace(headerHex))
	if err != nil || len(header) != remoteHeaderLen {
		return nil, fmt.Errorf("表21头须为 %d 字节 hex(10字节=20个hex字符)", remoteHeaderLen)
	}
	body, err := hex.DecodeString(strings.TrimSpace(bodyHex))
	if err != nil {
		return nil, fmt.Errorf("信息体不是合法 hex: %w", err)
	}
	out := make([]byte, 0, len(header)+len(body))
	out = append(out, header...)
	out = append(out, body...)
	return out, nil
}

// downlinkInfo 命令帧(RS=0xFE)若属于 0x80/0x81/0x82/0x8A,解析出下行信息供 UI 应答。
// 注意用线上原始命令字节判定:库解码会把 0x83~0xBF 预留区间折叠到 0x83 条目。
func downlinkInfo(wireCmd byte, pm *frame.ProtocolMessage) *DownlinkInfo {
	if pm.ResponseType != types.ResponseCommand {
		return nil
	}
	switch wireCmd {
	case 0x80, 0x81, 0x82, 0x8A:
		return ParseDownlink(wireCmd, pm.RawBytes)
	}
	return nil
}

// ---------------------------------------------------------------- Client 应答入口

// RespondAck 发送指定命令的应答帧(空载荷),用于 0x81/0x82 标准应答
// 与 私有远控 0x8A 第一层 ACK。
func (c *Client) RespondAck(cmd byte, respType types.ResponseType) error {
	return c.writeFrameWithResponse(cmd, respType, nil)
}

// RespondParamQuery 发送 0x80 参数查询应答。
func (c *Client) RespondParamQuery(rows []ParamResponseRow, respType types.ResponseType) error {
	payload, err := BuildParamQueryResponse(rows)
	if err != nil {
		return err
	}
	return c.writeFrameWithResponse(0x80, respType, payload)
}

// RespondRemoteSecondLayer 发送 私有远控 0x8A 第二层业务应答(命令包形式,载荷=回显头+信息体)。
func (c *Client) RespondRemoteSecondLayer(headerHex, bodyHex string) error {
	payload, err := BuildRemoteSecondLayer(headerHex, bodyHex)
	if err != nil {
		return err
	}
	return c.writeFrameWithResponse(0x8A, types.ResponseCommand, payload)
}

// writeFrameWithResponse 发送任意应答类型的帧;payload 为 nil 时为空载荷。
func (c *Client) writeFrameWithResponse(cmd byte, respType types.ResponseType, payload []byte) error {
	c.mu.Lock()
	conn := c.conn
	version := c.opts.Version
	vin := c.opts.VIN
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("gbt32960-sim: not connected")
	}

	body := rawBody{v: version, b: payload}
	rt := commandFor(version, cmd)
	if rt == nil {
		return fmt.Errorf("gbt32960-sim: unknown command 0x%02X", cmd)
	}
	msg := frameMessage(version, vin, cmd, rt, respType, body)
	raw, err := msg.Bytes()
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, werr := conn.Write(raw)
	c.writeMu.Unlock()
	if werr != nil {
		return fmt.Errorf("gbt32960-sim: write: %w", werr)
	}

	c.bus.Emit(Event{
		Kind:    EventTx,
		Cmd:     cmdName(rt) + " [应答:" + responseText(respType) + "]",
		Hex:     utils.BytesToHex(raw),
		Bytes:   len(raw),
		Decoded: map[string]any{"payloadHex": utils.BytesToHex(payload)},
	})
	return nil
}
