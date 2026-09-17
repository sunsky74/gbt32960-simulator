package engine

import (
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

// ResponseCode 应答标志取值(表4)。
type ResponseCode struct {
	Code  int    `json:"code"`
	Label string `json:"label"`
}

// ResponseCodes 返回指定协议版本的应答码列表:
// 2016 表4 = 0x01 成功 / 0x02 错误(设置未成功) / 0x03 VIN重复;
// 2025 表4 = 0x01 成功 / 0x02 其他错误 / 0x03 VIN重复 / 0x04 VIN不存在 /
// 0x05 验签错误 / 0x06 数据结构错误 / 0x07 解密错误。
// 不含 0xFE(那是「命令包」标志,不是应答码)。
func ResponseCodes(v api.GBTVersion) []ResponseCode {
	codes := []ResponseCode{
		{int(types.ResponseSuccess), "成功"},
		{int(types.ResponseFailed), "错误(设置未成功)"},
		{int(types.ResponseVINDup), "VIN重复"},
	}
	if v == api.V2025 {
		codes[1].Label = "其他错误"
		codes = append(codes,
			ResponseCode{int(types.ResponseVINNotExist), "VIN不存在"},
			ResponseCode{int(types.ResponseSignErr), "验签错误"},
			ResponseCode{int(types.ResponseStructureErr), "数据结构错误"},
			ResponseCode{int(types.ResponseDecodeErr), "解密错误"},
		)
	}
	return codes
}
