package servermode

import (
	"fmt"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/utils"
)

// encryptByteOffset 加密方式字节在帧内的偏移:起始符2 + 命令1 + 应答1 + VIN17。
const encryptByteOffset = 21

// encryptionNone 协议规定"不加密"的线上值是 0x01(types.EncryptionNone),不是 0。
const encryptionNone = 0x01

// knownCmds2016 服务端已知(会正常处理/应答)的 2016 命令白名单。
// 注意不能用 frame.PayloadType==nil 判未知:0x07/0x08 载荷类型即 nil(评审 B3)。
var knownCmds2016 = map[byte]bool{
	0x01: true, 0x02: true, 0x03: true, 0x04: true, 0x07: true, 0x08: true,
}

// Decoded 是单帧的解码与分类结果。
type Decoded struct {
	Raw       []byte
	PM        *frame.ProtocolMessage
	Version   api.GBTVersion
	Cmd       byte
	VIN       string
	Kind      FrameKind
	Encrypted bool
	Err       error
}

// decodeFrame 解码一帧并分类。加密判定在进入 ProtocolCodec.Decode 之前完成
// (库对加密标志≠0x01 的帧直接报 ErrEncryptionNotSupported,事后分支不可达,评审 B2):
// raw[21] ≠ 0x01 → KindEncrypted,不解析。BCC/结构错误返回 Err(PM=nil),
// 调用方丢弃该帧并告警(重同步由 FrameReader 保证)。
func decodeFrame(raw []byte) Decoded {
	d := Decoded{Raw: raw}
	if len(raw) > encryptByteOffset && raw[encryptByteOffset] != encryptionNone {
		d.Encrypted = true
		d.Kind = KindEncrypted
		d.VIN = string(raw[4 : 4+17]) // 头部字段直接截取,足够展示用
		d.Cmd = raw[2]
		return d
	}
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		d.Err = fmt.Errorf("帧解码失败: %w", err)
		return d
	}
	pm := msg.(*frame.ProtocolMessage)
	_ = pm.DecodePayload() // 尽力解码载荷,失败不影响帧级分类
	d.PM = pm
	d.Version = pm.Version
	d.VIN = pm.VIN
	// 命令码取帧头字节(与加密分支同源):库对私有命令码(0x80~0xFE 表外)
	// 解出的 RequestType 无法经 CommandCode 还原,零值会污染白名单判定。
	d.Cmd = raw[2]
	if pm.Version == api.V2016 && !knownCmds2016[d.Cmd] {
		if _, ok := ExtCmd(d.Cmd); !ok {
			d.Kind = KindUnknown
			return d
		}
	}
	d.Kind = KindNormal // 2025 全部只读展示,不再细分 unknown
	return d
}
