package ext

import (
	"fmt"
	"strings"

	"gbt32960-simulator/internal/schema"
)

// CompileUnit 把 DSL 单元编译为前端表单渲染用的 GroupSchema。
func CompileUnit(u AppendUnit) schema.GroupSchema {
	fields := make([]schema.FieldSchema, len(u.Fields))
	for i, f := range u.Fields {
		fields[i] = CompileField(f)
	}
	maxRows := u.MaxRows
	if maxRows == 0 && u.Multiple {
		maxRows = 10
	}
	return schema.GroupSchema{
		Key: u.Key, Title: u.Title, Enabled: u.Enabled,
		Multiple: u.Multiple, MaxRows: maxRows, Fields: fields,
	}
}

// CompileField DSL 字段 → 前端 FieldSchema(kind 映射 + 物理量程 + 换算说明)。
func CompileField(f FieldSpec) schema.FieldSchema {
	out := schema.FieldSchema{Key: f.Key, Label: f.Label, Unit: f.Unit, ScaleNote: scaleNote(f)}
	switch f.Type {
	case "u8", "u16", "u32", "i8", "i16", "i32":
		rng := numericRange[f.Type]
		lo, hi := rng[0], rng[1]
		scale, offset := scaleOf(f)
		if scale == 1 { // 仓库约定:无 scale → int;×0.1 类 → float(如 speed/mileage)
			out.Kind = "int"
		} else {
			out.Kind = "float"
		}
		out.Min = ptrf(lo*scale + offset)
		out.Max = ptrf(hi*scale + offset)
	case "f32":
		out.Kind = "float"
	case "bits":
		out.Kind = "bitgroup"
		out.Bits = make([]schema.BitDef, len(f.Bits))
		for i, b := range f.Bits {
			out.Bits[i] = schema.BitDef{Index: b.Index, Label: b.Label}
		}
	case "bytes":
		out.Kind = "bytes"
		out.Length = f.Length
	}
	return out
}

func scaleNote(f FieldSpec) string {
	scale, offset := scaleOf(f)
	if scale == 1 && offset == 0 {
		return ""
	}
	var b strings.Builder
	if scale != 1 {
		fmt.Fprintf(&b, "×%g", scale)
	}
	if offset != 0 {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "偏移%+g", offset)
	}
	return b.String()
}

// DefaultsFor 生成单元默认行值:数值取 offset(线值 0),位全 false,bytes 为 00 填充。
func DefaultsFor(u AppendUnit) schema.RowValue {
	return defaultsForFields(u.Fields)
}

func defaultsForFields(fields []FieldSpec) schema.RowValue {
	row := schema.RowValue{}
	for _, f := range fields {
		switch f.Type {
		case "bits":
			flags := map[string]any{}
			for _, b := range f.Bits {
				flags[fmt.Sprintf("bit%d", b.Index)] = false
			}
			row[f.Key] = flags
		case "bytes":
			row[f.Key] = strings.Repeat("00", f.Length)
		default:
			offset := 0.0
			if f.Offset != nil {
				offset = *f.Offset
			}
			row[f.Key] = offset
		}
	}
	return row
}
