package servermode

import (
	"time"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/frame"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/types"
)

// rawBody 以原始字节实现 model.MessageBody(应答场景:结果空体/校时时间)。
type rawBody struct {
	v api.GBTVersion
	b []byte
}

func (r rawBody) Version() api.GBTVersion { return r.v }
func (r rawBody) Bytes() ([]byte, error)  { return r.b, nil }

// buildReply 构造平台应答帧:同命令码 + 应答标志(0x01 成功/0x02 失败)+ body。
// 加密方式必须显式置 EncryptionNone(0x01)——零值 0x00 会被协议库 Decode 拒收(评审 B1)。
func buildReply(v api.GBTVersion, vin string, cmd byte, resp types.ResponseType, body []byte) ([]byte, error) {
	msg := frame.ProtocolMessage{
		Version:      v,
		RequestType:  &types.CommandV2016{Code: cmd},
		ResponseType: resp,
		VIN:          vin,
		Encryption:   types.EncryptionNone,
		Payload:      rawBody{v: v, b: body},
	}
	return msg.Bytes()
}

// clockBody 校时应答体:BeanTime 6 字节十进制(年-2000/月/日/时/分/秒)。
func clockBody(now time.Time) []byte {
	b := ext.EncodeBeanTime(now)
	return b[:]
}

var _ model.MessageBody = rawBody{} // 接口守卫
