// Package signature 承载 2025 版车端签名(GB/T 32960.3-2025 表8)的外部验证职责。
//
// 签名段本身的编解码由协议库完成(TLV 0xFF,model/gbt2025 VehicleSignature:
// 签名类型 + R 长度/值 + S 长度/值;SignData 覆盖数据采集时间首字节至签名段前一个字节)。
// 本包只做两件事:
//   - 把「验证」以 Verifier 接口(或 VerifierFunc 回调)开放给外部注入;
//   - 提供从已解码载荷中提取签名、生成监控摘要的辅助。
package signature

import (
	"fmt"

	"github.com/sunsky74/gb32960/model"
	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	sigrt "github.com/sunsky74/gb32960/model/gbt2025/realtime"
)

// Verifier 校验 2025 车端签名。由外部实现(接口或函数回调均可)并注入:
//   - sigType:        签名类型(表8:0x01 SM2;0x02 RSA;0x03 ECC)
//   - data:           签名覆盖的原始字节(数据采集时间首字节起,至签名段前一个字节)
//   - rValue, sValue: 签名 R/S 值
//
// 返回 nil 表示验签通过;未注入验证器时签名只做结构解码,不做校验。
type Verifier interface {
	Verify(sigType byte, data, rValue, sValue []byte) error
}

// VerifierFunc 让普通函数直接充当 Verifier(回调用法)。
type VerifierFunc func(sigType byte, data, rValue, sValue []byte) error

// Verify 实现 Verifier。
func (f VerifierFunc) Verify(sigType byte, data, rValue, sValue []byte) error {
	return f(sigType, data, rValue, sValue)
}

// TypeName 返回签名类型名称(表8)。
func TypeName(sigType byte) string {
	switch sigType {
	case 0x01:
		return "SM2"
	case 0x02:
		return "RSA"
	case 0x03:
		return "ECC"
	default:
		return fmt.Sprintf("其他(0x%02X)", sigType)
	}
}

// FromBody 从已解码的报文载荷中取出 2025 车端签名;非 2025 实时体或无签名段返回 nil。
func FromBody(body model.MessageBody) *sigrt.VehicleSignature {
	if m, ok := body.(*mdl.RealTimeV2025Data); ok {
		return m.VehicleSignature
	}
	return nil
}

// Describe 生成签名监控摘要:结构 + 验签结果;verifier 为 nil 时只报告结构。
func Describe(v Verifier, sig *sigrt.VehicleSignature) string {
	desc := fmt.Sprintf("车端签名 %s (R %dB/S %dB)", TypeName(sig.Type), sig.RLength, sig.SLength)
	if v == nil {
		return desc + ",未配置验证器"
	}
	if err := v.Verify(sig.Type, sig.SignData, sig.RValue, sig.SValue); err != nil {
		return desc + ",验签失败: " + err.Error()
	}
	return desc + ",验签通过"
}
