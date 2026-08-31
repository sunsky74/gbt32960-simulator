package ext

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"gbt32960-simulator/internal/schema"
)

func ptrf(v float64) *float64 { return &v }

// toFloat 从 RowValue 取数值(JSON 数字可能解出 float64/int)。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

var numericRange = map[string][2]float64{
	"u8": {0, 255}, "u16": {0, 65535}, "u32": {0, 4294967295},
	"i8": {-128, 127}, "i16": {-32768, 32767}, "i32": {-2147483648, 2147483647},
}

var numericSize = map[string]int{"u8": 1, "i8": 1, "u16": 2, "i16": 2, "u32": 4, "i32": 4}

// EncodeFields 按声明顺序把一行配置编码为字节(不含任何头)。
func EncodeFields(fields []FieldSpec, row schema.RowValue) ([]byte, error) {
	out := make([]byte, 0, 64)
	for _, f := range fields {
		raw, err := encodeField(f, row)
		if err != nil {
			return nil, fmt.Errorf("字段 %s: %w", f.Key, err)
		}
		out = append(out, raw...)
	}
	return out, nil
}

func encodeField(f FieldSpec, row schema.RowValue) ([]byte, error) {
	switch f.Type {
	case "u8", "u16", "u32", "i8", "i16", "i32", "f32":
		return encodeNumeric(f, row)
	case "bits":
		return encodeBits(f, row)
	case "bytes":
		return encodeBytes(f, row)
	default:
		return nil, fmt.Errorf("未知字段类型 %q", f.Type)
	}
}

func scaleOf(f FieldSpec) (scale, offset float64) {
	scale, offset = 1, 0
	if f.Scale != nil {
		scale = *f.Scale
	}
	if f.Offset != nil {
		offset = *f.Offset
	}
	return
}

func encodeNumeric(f FieldSpec, row schema.RowValue) ([]byte, error) {
	v, ok := row[f.Key]
	if !ok {
		return nil, fmt.Errorf("缺少值")
	}
	phys, ok := toFloat(v)
	if !ok {
		return nil, fmt.Errorf("值不是数值: %v", v)
	}
	scale, offset := scaleOf(f)
	if f.Type == "f32" {
		if scale != 1 || offset != 0 {
			return nil, fmt.Errorf("f32 不支持 scale/offset")
		}
		bits := math.Float32bits(float32(phys))
		return []byte{byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits)}, nil
	}
	raw := (phys - offset) / scale
	wire := math.Round(raw)
	// 往返校验:用线值重建物理值再比对,免疫大数值下正向除法的浮点误差放大
	if math.Abs(wire*scale+offset-phys) > 1e-6 {
		return nil, fmt.Errorf("物理值 %v 换算后不是整数线值(最近线值 %v)", phys, wire)
	}
	iv := int64(wire)
	r := numericRange[f.Type]
	if float64(iv) < r[0] || float64(iv) > r[1] {
		return nil, fmt.Errorf("线值 %d 超出 %s 范围 [%v, %v]", iv, f.Type, r[0], r[1])
	}
	return putInt(f.Type, iv), nil
}

func putInt(t string, v int64) []byte {
	size := numericSize[t]
	out := make([]byte, size)
	for i := 0; i < size; i++ {
		out[size-1-i] = byte(v >> (8 * i))
	}
	return out
}

func encodeBits(f FieldSpec, row schema.RowValue) ([]byte, error) {
	v, ok := row[f.Key]
	if !ok {
		return nil, fmt.Errorf("缺少值")
	}
	flags, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("值不是位组对象: %v", v)
	}
	maxIdx := 0
	for _, b := range f.Bits {
		if b.Index > maxIdx {
			maxIdx = b.Index
		}
	}
	size := 1
	if maxIdx >= 16 {
		size = 4
	} else if maxIdx >= 8 {
		size = 2
	}
	out := make([]byte, size)
	for _, b := range f.Bits {
		if on, _ := flags[fmt.Sprintf("bit%d", b.Index)].(bool); on {
			out[b.Index/8] |= 1 << (b.Index % 8)
		}
	}
	return out, nil
}

func encodeBytes(f FieldSpec, row schema.RowValue) ([]byte, error) {
	v, ok := row[f.Key]
	if !ok {
		return nil, fmt.Errorf("缺少值")
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("值不是 hex 字符串: %v", v)
	}
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("不是合法 hex: %w", err)
	}
	if len(b) != f.Length {
		return nil, fmt.Errorf("长度须为 %d 字节,实际 %d", f.Length, len(b))
	}
	return b, nil
}

// EncodeUnit 编码完整 TLV 数据单元: unitCode(u8) + 长度(u16 大端) + 数据。
// multiple 语义由调用方实现:每行调用一次,得到一个独立 TLV。
func EncodeUnit(u AppendUnit, row schema.RowValue) ([]byte, error) {
	data, err := EncodeFields(u.Fields, row)
	if err != nil {
		return nil, fmt.Errorf("单元 %s: %w", u.Key, err)
	}
	if len(data) > 0xFFFF {
		return nil, fmt.Errorf("单元 %s 数据超长: %d 字节", u.Key, len(data))
	}
	out := make([]byte, 3, 3+len(data))
	out[0] = byte(u.UnitCode)
	out[1] = byte(len(data) >> 8)
	out[2] = byte(len(data))
	return append(out, data...), nil
}
