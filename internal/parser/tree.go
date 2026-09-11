package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// KV 树节点中的一个键值对,保持插入顺序。
type KV struct {
	Key   string
	Value any
}

// Node 有序键值对序列,JSON 序列化为保序对象;nil/空节点序列化为 {}。
// 用于把一帧报文渲染为嵌套 JSON 时保留字段的自然阅读顺序。
type Node []KV

// MarshalJSON 手动拼接 JSON 对象(encoding/json 的 map 不保序),
// 值统一交由 encoding/json 序列化与转义。
func (n Node) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range n {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// buildTree 把解析结果组装为有序中文键树(控制台嵌套 JSON 渲染用)。
// 帧头取已解析的展示值;数据单元内容按 TLV 分组;校验码取 BCC 行的翻译值。
func buildTree(r *Result) Node {
	start := ""
	if f := fieldNamed(r.Fields, "起始符"); f != nil {
		start = f.RawValue // 取原始文本("##"/"$$"),而非版本标记 OffsetVal("V2016")
	}
	bcc := ""
	if f := fieldNamed(r.Fields, "校验码 BCC"); f != nil {
		bcc = f.Translate
	}
	return Node{
		{Key: "起始符", Value: start},
		{Key: "命令单元", Value: Node{
			{Key: "命令标识", Value: r.Command},
			{Key: "应答标志", Value: r.ResponseType},
		}},
		{Key: "唯一识别码", Value: r.VIN},
		{Key: "数据单元加密方式", Value: r.Encryption},
		{Key: "数据单元长度", Value: r.PayloadLen},
		{Key: "数据单元内容", Value: buildPayloadTree(r.Fields)},
		{Key: "校验码", Value: bcc},
	}
}

// buildPayloadTree 把 Offset>=24 的载荷字段组装为分组树:
// TLV 标记行开启一个组(组键为标记的翻译值),其后字段成为组内叶子,直到下一个标记;
// 标记前的字段平铺;应答帧/未细分命令等整行原始数据作为平铺叶子;
// V2025 占位行转为 说明 + 原始数据(HEX) 两键。
func buildPayloadTree(fields []Field) Node {
	payload := make([]Field, 0, len(fields))
	for _, f := range fields {
		if f.Offset >= 24 && f.Name != "校验码 BCC" {
			payload = append(payload, f)
		}
	}
	for _, f := range payload {
		if f.Name == "数据单元 (V2025)" {
			return Node{
				{Key: "说明", Value: f.Translate},
				{Key: "原始数据(HEX)", Value: f.RawHex},
			}
		}
	}

	type item struct {
		key    string
		value  string // 平铺叶子值
		leaf   bool
		leaves Node // 组内叶子
	}
	items := make([]item, 0, len(payload))
	cur := -1 // 当前组在 items 中的下标,-1 表示未处于任何组
	for _, f := range payload {
		switch f.Name {
		case "数据类型标志 (TLV)":
			items = append(items, item{key: f.Translate})
			cur = len(items) - 1
		case "数据单元(应答帧)", "数据单元(未细分的命令)", "数据单元(未分组的命令)":
			items = append(items, item{key: f.Name, value: f.RawValue, leaf: true})
		default:
			if cur >= 0 {
				items[cur].leaves = appendKV(items[cur].leaves, f.Name, leafValue(f))
			} else {
				items = append(items, item{key: f.Name, value: leafValue(f), leaf: true})
			}
		}
	}

	out := Node{}
	for _, it := range items {
		if it.leaf {
			out = appendKV(out, it.key, it.value)
		} else {
			out = appendKV(out, it.key, it.leaves)
		}
	}
	return out
}

// leafValue 载荷叶子取值规则:优先翻译值,其次物理偏移值,最后原始值;
// 选中值非空且字段带单位时追加 " " + 单位。
func leafValue(f Field) string {
	v := f.Translate
	if v == "" || v == "-" {
		v = f.OffsetVal
		if v == "" || v == "-" {
			v = f.RawValue
		}
	}
	if v != "" && f.Unit != "" {
		v += " " + f.Unit
	}
	return v
}

// appendKV 追加键值对;同层键冲突时按 " (2)"、" (3)"… 追加序号,绝不静默丢数据。
func appendKV(n Node, key string, value any) Node {
	final := key
	for i := 2; nodeHasKey(n, final); i++ {
		final = fmt.Sprintf("%s (%d)", key, i)
	}
	return append(n, KV{Key: final, Value: value})
}

func nodeHasKey(n Node, key string) bool {
	for _, kv := range n {
		if kv.Key == key {
			return true
		}
	}
	return false
}

// fieldNamed 按字段名取第一行(树组装只需要帧头/校验码这类唯一行)。
func fieldNamed(fields []Field, name string) *Field {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}
