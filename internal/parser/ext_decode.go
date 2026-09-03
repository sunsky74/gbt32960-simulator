package parser

import (
	"fmt"
	"math"
	"strings"

	"gbt32960-simulator/internal/ext"
	"github.com/sunsky74/gb32960/utils"
)

// 自定义 TLV 数据单元区(与 ext.EncodeUnit / 协议库自定义区一致):
// unitCode(1B, 0x80~0xFE) + 长度(2B 大端) + 数据。
const customUnitCodeMin = 0x80
const customUnitCodeMax = 0xFE

func isCustomUnitCode(b byte) bool {
	return b >= customUnitCodeMin && b <= customUnitCodeMax
}

// packUnitByCode 在包内按 unitCode 查找单元定义(实时追加单元 + realtimeLike 命令体单元共享命名空间)。
func packUnitByCode(p *ext.Pack, code int) *ext.AppendUnit {
	if p == nil {
		return nil
	}
	for i := range p.Realtime.AppendUnits {
		if p.Realtime.AppendUnits[i].UnitCode == code {
			return &p.Realtime.AppendUnits[i]
		}
	}
	for i := range p.Commands {
		for j := range p.Commands[i].Body.Units {
			if p.Commands[i].Body.Units[j].UnitCode == code {
				return &p.Commands[i].Body.Units[j]
			}
		}
	}
	return nil
}

func customUnitName(code byte, unit *ext.AppendUnit) string {
	if unit != nil {
		if strings.Contains(unit.Title, fmt.Sprintf("0x%02X", code)) {
			return "自定义·" + unit.Title // title 已含单元码时不重复拼接
		}
		return fmt.Sprintf("自定义·%s (0x%02X)", unit.Title, code)
	}
	return fmt.Sprintf("自定义数据单元 0x%02X", code)
}

// parseCustomUnitBody 解析自定义单元体(u16 长度 + 数据):
// unit 非 nil 时在单元区间内用子 walker 按字段 DSL 解码,否则整体按原始字节展示。
// 单元数据与协议定义长度不符时:每单元仅产出一条汇总告警(不逐字段刷屏),
// 并经 addIssue 记录异常区间供字节视图高亮。
func parseCustomUnitBody(w *walker, code byte, unit *ext.AppendUnit, addIssue func(ByteIssue)) {
	name := customUnitName(code, unit)
	lenB := w.take(2)
	if lenB == nil {
		return
	}
	unitStart := 24 + w.base + w.pos - 3 // flag(1) + len(2) 的起始
	n := int(lenB[0])<<8 | int(lenB[1])
	if n > w.remain() {
		w.warn(fmt.Sprintf("%s 长度字段(%d)超出剩余字节(%d),按剩余解析", name, n, w.remain()))
		n = w.remain()
	}
	data := w.take(n)
	if data == nil {
		return
	}
	if unit == nil {
		w.emit(name+" 数据", "bytes", data, utils.BytesToHex(data), "-", "-", "")
		return
	}
	// 子 walker 逐字段截断告警静默丢弃,由下方按单元汇总
	sub := &walker{p: data, base: w.base + w.pos - n, out: w.out, warn: func(string) {}}
	var missingFields []string
	for _, f := range unit.Fields {
		if !extField(sub, f) {
			missingFields = append(missingFields, f.Label)
		}
	}
	w.out = sub.out

	defW := extUnitWidth(unit)
	if len(missingFields) > 0 {
		note := fmt.Sprintf("%s 数据 %d 字节,比协议定义(%d 字节)少 %d 字节,%d 个字段未解析:%s",
			name, n, defW, defW-n, len(missingFields), strings.Join(missingFields, "、"))
		w.warn(note)
		addIssue(ByteIssue{Start: unitStart, End: unitStart + 3 + n, Note: note})
	} else if n > defW && !unitHasTail(unit) {
		note := fmt.Sprintf("%s 数据 %d 字节,比协议定义(%d 字节)多 %d 字节,多余部分未翻译", name, n, defW, n-defW)
		w.warn(note)
		addIssue(ByteIssue{Start: unitStart, End: unitStart + 3 + n, Note: note})
	}
}

// unitHasTail 单元是否含变长 tail 字段(此类单元数据宽度天然可变,不做"多余"对账)。
func unitHasTail(u *ext.AppendUnit) bool {
	for _, f := range u.Fields {
		if f.Type == "tail" {
			return true
		}
	}
	return false
}

// extUnitWidth 单元固定字段宽度合计(tail 变长不计)。
func extUnitWidth(u *ext.AppendUnit) int {
	total := 0
	for _, f := range u.Fields {
		switch f.Type {
		case "u8", "i8":
			total++
		case "u16", "i16":
			total += 2
		case "u32", "i32", "f32":
			total += 4
		case "bytes":
			total += f.Length
		case "bits":
			maxIdx := 0
			for _, b := range f.Bits {
				if b.Index > maxIdx {
					maxIdx = b.Index
				}
			}
			switch {
			case maxIdx >= 16:
				total += 4
			case maxIdx >= 8:
				total += 2
			default:
				total++
			}
		}
	}
	return total
}

// extField 按扩展包字段 DSL 解码一个字段,返回是否成功(失败 = 单元数据不足或类型未知)。
// 物理值 = 线值×scale + offset(与 ext.EncodeFields 互逆)。
func extField(w *walker, f ext.FieldSpec) bool {
	pendingName = f.Label
	switch f.Type {
	case "u8", "u16", "u32", "i8", "i16", "i32":
		return extNumeric(w, f)
	case "f32":
		return extF32(w, f)
	case "bits":
		return extBits(w, f)
	case "bytes":
		b := w.take(f.Length)
		if b == nil {
			return false
		}
		w.emit(f.Label, "bytes", b, utils.BytesToHex(b), "-", "-", f.Unit)
	case "tail":
		b := w.take(w.remain())
		if b == nil {
			return false
		}
		w.emit(f.Label, "bytes", b, utils.BytesToHex(b), "-", "-", f.Unit)
	default:
		w.warn(fmt.Sprintf("扩展字段 %s: 未知类型 %q,跳过 1 字节", f.Label, f.Type))
		w.take(1)
		return false
	}
	return true
}

func extNumeric(w *walker, f ext.FieldSpec) bool {
	size := map[string]int{"u8": 1, "i8": 1, "u16": 2, "i16": 2, "u32": 4, "i32": 4}[f.Type]
	b := w.take(size)
	if b == nil {
		return false
	}
	var v int64
	switch size {
	case 1:
		if f.Type[0] == 'i' {
			v = int64(int8(b[0]))
		} else {
			v = int64(b[0])
		}
	case 2:
		raw := int64(b[0])<<8 | int64(b[1])
		if f.Type[0] == 'i' {
			v = int64(int16(raw))
		} else {
			v = raw
		}
	case 4:
		raw := int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3])
		if f.Type[0] == 'i' {
			v = int64(int32(raw))
		} else {
			v = raw
		}
	}
	scale, offset := extScaleOf(f)
	phys := trimFloat(float64(v)*scale + offset)
	w.emit(f.Label, f.Type, b, fmt.Sprint(v), phys, "-", f.Unit)
	return true
}

func extF32(w *walker, f ext.FieldSpec) bool {
	b := w.take(4)
	if b == nil {
		return false
	}
	bits := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	w.emit(f.Label, "f32", b, fmt.Sprintf("0x%08X", bits), trimFloat(float64(math.Float32frombits(bits))), "-", f.Unit)
	return true
}

func extBits(w *walker, f ext.FieldSpec) bool {
	maxIdx := 0
	for _, bit := range f.Bits {
		if bit.Index > maxIdx {
			maxIdx = bit.Index
		}
	}
	size := 1
	if maxIdx >= 16 {
		size = 4
	} else if maxIdx >= 8 {
		size = 2
	}
	b := w.take(size)
	if b == nil {
		return false
	}
	var on, off []string
	for _, bit := range f.Bits {
		set := b[bit.Index/8]&(1<<(bit.Index%8)) != 0
		if set {
			on = append(on, bit.Label)
		} else {
			off = append(off, bit.Label)
		}
	}
	trans := "-"
	if len(on) > 0 {
		trans = "开: " + strings.Join(on, " / ")
	} else if len(f.Bits) > 0 {
		trans = "全部关闭"
	}
	w.emit(f.Label, "bits", b, fmt.Sprintf("0x%0*X", size*2, b), "-", trans, f.Unit)
	return true
}

func extScaleOf(f ext.FieldSpec) (scale, offset float64) {
	scale, offset = 1, 0
	if f.Scale != nil {
		scale = *f.Scale
	}
	if f.Offset != nil {
		offset = *f.Offset
	}
	return
}
