// Package parser 把 GB/T 32960 原始 HEX 报文解析为逐字段的结构化结果。
// 字段值计算复用 gb32960-go 的协议定义(codec.ValueConverter 转换器、types 枚举),
// 不在此处重新定义换算规则;布局知识与 codec 一致,并由金标准报文测试交叉验证。
package parser

import (
	"fmt"
	"strings"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
	"github.com/sunsky74/gb32960/utils"
)

// Field 一个解析出的字段行。
type Field struct {
	Offset    int    `json:"offset"`
	Length    int    `json:"length"`
	Name      string `json:"name"`
	Type      string `json:"type"`      // u8/u16/u32/bytes/bits/bcd/ascii
	RawHex    string `json:"rawHex"`    // 原始字节 hex(如 "012c")
	RawValue  string `json:"rawValue"`  // 原始数值展示(如 300 / 0x012C)
	OffsetVal string `json:"offsetVal"` // 偏移值(应用协议换算后的物理值;枚举类为 "-")
	Translate string `json:"translate"` // 翻译值(枚举/位域的可读文字;数值类为 "-")
	Unit      string `json:"unit"`
	Note      string `json:"note,omitempty"`
}

// ByteIssue 报文中与协议定义不符的异常字节区间(前端字节视图微红高亮用)。
type ByteIssue struct {
	Start int    `json:"start"` // 区间起始 offset(含)
	End   int    `json:"end"`   // 区间结束 offset(不含)
	Note  string `json:"note"`
}

// Result 解析结果。
type Result struct {
	TotalBytes   int         `json:"totalBytes"`
	Version      string      `json:"version"`      // V2016 / V2025
	VersionByte  string      `json:"versionByte"`  // "##" / "$$"
	Command      string      `json:"command"`      // 如 "0x01 车辆登入"
	ResponseType string      `json:"responseType"` // 如 "0xFE 命令" / "0x01 成功"
	VIN          string      `json:"vin"`
	Encryption   string      `json:"encryption"`
	PayloadLen   int         `json:"payloadLen"` // 数据单元长度字段
	Fields       []Field     `json:"fields"`
	Warnings     []string    `json:"warnings"`
	Issues       []ByteIssue `json:"issues,omitempty"`
	Tree         any         `json:"tree,omitempty"` // 有序中文键树(控制台嵌套 JSON 渲染用)
}

// NormalizeHex 清洗输入:去掉空格/换行/逗号/冒号/0x 前缀,校验 hex 合法性与偶数长度。
func NormalizeHex(input string) ([]byte, error) {
	s := strings.TrimSpace(input)
	s = strings.ReplaceAll(s, "0x", "")
	s = strings.ReplaceAll(s, "0X", "")
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', ',', ':', '-', '_':
			return -1
		}
		return r
	}, s)
	if clean == "" {
		return nil, fmt.Errorf("HEX 输入为空")
	}
	if len(clean)%2 != 0 {
		return nil, fmt.Errorf("HEX 字符数为奇数(%d),请检查是否漏抄字符", len(clean))
	}
	for _, r := range clean {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return nil, fmt.Errorf("非法字符 %q:HEX 只允许 0-9 a-f", string(r))
		}
	}
	return utils.HexToBytes(strings.ToLower(clean))
}

// Parse 解析一帧完整报文(不使用扩展包)。
func Parse(input string) (*Result, error) {
	return parse(input, nil)
}

// ParseWithPack 解析一帧完整报文,并按扩展包的自定义单元定义解码私有 TLV(0x80~0xFE)。
// 包基准版本与报文版本不一致时告警并退化为通用展示。
func ParseWithPack(input string, pack *ext.Pack) (*Result, error) {
	return parse(input, pack)
}

func parse(input string, pack *ext.Pack) (*Result, error) {
	raw, err := NormalizeHex(input)
	if err != nil {
		return nil, err
	}
	r := &Result{TotalBytes: len(raw), Fields: []Field{}}

	if len(raw) < 25 {
		return nil, fmt.Errorf("报文长度不足:共 %d 字节,合法帧至少 25 字节(24 头 + 1 BCC)", len(raw))
	}

	// 起始符/版本
	version := api.V2016
	versionName := "V2016"
	if raw[0] == 0x24 && raw[1] == 0x24 {
		version = api.V2025
		versionName = "V2025"
	} else if raw[0] != 0x23 || raw[1] != 0x23 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("起始符异常: %02X%02X(合法为 2323 或 2424),已按 V2016 继续", raw[0], raw[1]))
	}
	r.Version = versionName
	r.VersionByte = string(raw[0:2])
	r.addField(0, 2, "起始符", "bytes", raw[0:2], r.VersionByte, versionName, "", "")

	// 命令标识
	cmd := raw[2]
	cmdLabel := commandLabel(version, cmd)
	r.Command = fmt.Sprintf("0x%02X %s", cmd, cmdLabel)
	r.addField(2, 1, "命令标识", "u8", raw[2:3], fmt.Sprintf("0x%02X", cmd), "-", cmdLabel, "")

	// 应答标志
	resp := raw[3]
	respLabel := responseLabel(types.ResponseByCode(resp))
	r.ResponseType = fmt.Sprintf("0x%02X %s", resp, respLabel)
	r.addField(3, 1, "应答标志", "u8", raw[3:4], fmt.Sprintf("0x%02X", resp), "-", respLabel, "")

	// VIN
	vin := printable(raw[4:21])
	r.VIN = vin
	r.addField(4, 17, "VIN 车辆识别码", "ascii", raw[4:21], vin, "-", "", "")

	// 加密方式
	encLabel := encryptionLabel(raw[21])
	r.Encryption = encLabel
	r.addField(21, 1, "数据单元加密方式", "u8", raw[21:22], fmt.Sprintf("0x%02X", raw[21]), "-", encLabel, "")

	// 数据单元长度
	payloadLen := int(raw[22])<<8 | int(raw[23])
	r.PayloadLen = payloadLen
	r.addField(22, 2, "数据单元长度", "u16", raw[22:24], fmt.Sprint(payloadLen), "-", fmt.Sprintf("%d 字节", payloadLen), "")

	// 长度一致性
	if payloadLen != len(raw)-25 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("长度字段(%d)与实际数据单元(%d 字节)不一致", payloadLen, len(raw)-25))
	}
	payloadEnd := 24 + payloadLen
	if payloadEnd > len(raw)-1 {
		payloadEnd = len(raw) - 1
		r.Warnings = append(r.Warnings, "报文被截断:按实际剩余字节解析")
	}

	// BCC
	bcc := raw[len(raw)-1]
	calc := calcBCC(raw[2 : len(raw)-1])
	bccNote := ""
	if bcc != calc {
		bccNote = fmt.Sprintf("不符(计算值 %02X)", calc)
		r.Warnings = append(r.Warnings, fmt.Sprintf("BCC 校验不符:报文 %02X,计算 %02X", bcc, calc))
	} else {
		bccNote = "校验通过"
	}
	r.addField(len(raw)-1, 1, "校验码 BCC", "u8", raw[len(raw)-1:], fmt.Sprintf("0x%02X", bcc), "-", bccNote, "")

	// 数据单元
	payload := raw[24:payloadEnd]
	if len(payload) > 0 {
		if resp != types.ResponseCommand.Code() {
			// 应答帧:数据单元多为空或时间,先整体展示
			r.addField(24, len(payload), "数据单元(应答帧)", "bytes", payload, utils.BytesToHex(payload), "-", "", "")
		} else {
			if pack != nil && "V"+pack.Meta.BaseVersion != versionName {
				r.Warnings = append(r.Warnings, fmt.Sprintf("扩展包「%s」基准版本 %s 与报文版本 %s 不一致,自定义单元按通用展示", pack.Meta.Label, pack.Meta.BaseVersion, versionName))
				pack = nil
			}
			frames := parsePayload(version, cmd, payload, pack,
				func(w string) { r.Warnings = append(r.Warnings, w) },
				func(i ByteIssue) { r.Issues = append(r.Issues, i) },
			)
			r.Fields = append(r.Fields, frames...)
		}
	}

	// 按 Offset 稳定排序(BCC 行在数据单元后追加过)
	sortFields(r.Fields)
	r.Tree = buildTree(r)
	return r, nil
}

// ---------------------------------------------------------------- 辅助

type warnFn func(string)

func (r *Result) addField(off, ln int, name, typ string, raw []byte, rawVal, offsetVal, translate, unit string) {
	r.Fields = append(r.Fields, Field{
		Offset: off, Length: ln, Name: name, Type: typ,
		RawHex: utils.BytesToHex(raw), RawValue: rawVal,
		OffsetVal: offsetVal, Translate: translate, Unit: unit,
	})
}

func sortFields(fs []Field) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Offset < fs[j-1].Offset; j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}

func calcBCC(b []byte) byte {
	var v byte
	for _, x := range b {
		v ^= x
	}
	return v
}

func printable(b []byte) string {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 0x20 && c < 0x7f {
			out[i] = c
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func commandLabel(v api.GBTVersion, cmd byte) string {
	if v == api.V2025 {
		if c := types.CommandV2025ByCode(cmd); c != nil {
			return c.Name
		}
		return "未知命令"
	}
	if c := types.CommandV2016ByCode(cmd); c != nil {
		return c.Name
	}
	return "未知命令"
}

func responseLabel(r types.ResponseType) string {
	switch r {
	case types.ResponseCommand:
		return "命令"
	case types.ResponseSuccess:
		return "成功"
	case types.ResponseFailed:
		return "错误"
	case types.ResponseVINDup:
		return "VIN 重复"
	case types.ResponseVINNotExist:
		return "VIN 不存在"
	case types.ResponseSignErr:
		return "验签错误"
	case types.ResponseStructureErr:
		return "数据结构错误"
	case types.ResponseDecodeErr:
		return "解密错误"
	}
	return "未知"
}

func encryptionLabel(b byte) string {
	switch types.EncryptionType(b) {
	case types.EncryptionNone:
		return "不加密"
	case types.EncryptionRSA:
		return "RSA"
	case types.EncryptionAES128:
		return "AES128"
	case types.EncryptionException:
		return "加密异常"
	case types.EncryptionInvalid:
		return "无效"
	}
	return "未知"
}
