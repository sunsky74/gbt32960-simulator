package parser

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

/* helpers */

// treeKeys 按插入顺序取出 Node 的键序列。
func treeKeys(n Node) []string {
	keys := make([]string, 0, len(n))
	for _, kv := range n {
		keys = append(keys, kv.Key)
	}
	return keys
}

// treeGet 按键查找 Node 中的值。
func treeGet(n Node, key string) (any, bool) {
	for _, kv := range n {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return nil, false
}

// parseTree 解析一帧并返回其树(Tree 必须是 Node)。
func parseTree(t *testing.T, hexStr string) Node {
	t.Helper()
	r, err := Parse(hexStr)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := r.Tree.(Node)
	if !ok {
		t.Fatalf("Tree 类型 = %T,期望 parser.Node", r.Tree)
	}
	return n
}

// buildEmptyFrame 构造 24 字节头 + BCC 的最小 V2016 帧(数据单元长度 = 0)。
func buildEmptyFrame(cmd byte) string {
	body := []byte{0x23, 0x23, cmd, 0xFE}
	body = append(body, []byte("PARSERTESTUNIT001")...)
	body = append(body, 0x01, 0x00, 0x00)
	body = append(body, calcBCC(body[2:]))
	return fmt.Sprintf("%X", body)
}

/* (a) 登入金标准:根键顺序 / 命令单元 / JSON 数字 / 平铺载荷键 */

func TestTreeGoldenLogin(t *testing.T) {
	r, err := Parse(loadHex(t, "prod_login_v2016_01.hex"))
	if err != nil {
		t.Fatal(err)
	}
	root, ok := r.Tree.(Node)
	if !ok {
		t.Fatalf("Tree 类型 = %T,期望 parser.Node", r.Tree)
	}

	// 根节点键必须按固定顺序出现
	wantKeys := []string{"起始符", "命令单元", "唯一识别码", "数据单元加密方式", "数据单元长度", "数据单元内容", "校验码"}
	if got := treeKeys(root); strings.Join(got, "|") != strings.Join(wantKeys, "|") {
		t.Fatalf("根键顺序 = %v,期望 %v", got, wantKeys)
	}

	// 起始符取 RawValue("##"),不是 OffsetVal("V2016")
	if v, _ := treeGet(root, "起始符"); v != "##" {
		t.Errorf("起始符 = %#v,期望 ##", v)
	}

	// 命令单元恰好两键,顺序固定
	cmdAny, ok := treeGet(root, "命令单元")
	if !ok {
		t.Fatal("缺少 命令单元")
	}
	cmdNode, ok := cmdAny.(Node)
	if !ok {
		t.Fatalf("命令单元类型 = %T,期望 parser.Node", cmdAny)
	}
	if got := treeKeys(cmdNode); len(got) != 2 || got[0] != "命令标识" || got[1] != "应答标志" {
		t.Errorf("命令单元键 = %v,期望 [命令标识 应答标志]", got)
	}

	// marshal 后 数据单元长度 必须是 JSON number;登入金标准 payloadLen=30(整帧 55 字节)
	b, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	lenV, ok := m["数据单元长度"].(float64)
	if !ok {
		t.Fatalf("数据单元长度 JSON 类型 = %T,期望 number(float64)", m["数据单元长度"])
	}
	if lenV != float64(r.PayloadLen) || lenV != 30 {
		t.Errorf("数据单元长度 = %v,期望 %d(payloadLen)", lenV, r.PayloadLen)
	}
	if r.TotalBytes != 55 {
		t.Errorf("TotalBytes = %d,期望 55", r.TotalBytes)
	}

	// 登入载荷为平铺键(无 TLV 标记)
	contentAny, ok := m["数据单元内容"]
	if !ok {
		t.Fatal("缺少 数据单元内容")
	}
	content, ok := contentAny.(map[string]any)
	if !ok {
		t.Fatalf("数据单元内容 JSON 类型 = %T,期望 object", contentAny)
	}
	for _, key := range []string{"数据采集时间", "登入流水号", "ICCID"} {
		if _, ok := content[key]; !ok {
			t.Errorf("数据单元内容缺少 %s", key)
		}
	}
}

/* (b) 实时金标准:平铺时间 + TLV 分组顺序与标记行一致 */

func TestTreeGoldenRealtime(t *testing.T) {
	r, err := Parse(loadHex(t, "prod_realtime_v2016_01.hex"))
	if err != nil {
		t.Fatal(err)
	}
	root, ok := r.Tree.(Node)
	if !ok {
		t.Fatalf("Tree 类型 = %T,期望 parser.Node", r.Tree)
	}
	contentAny, _ := treeGet(root, "数据单元内容")
	content, ok := contentAny.(Node)
	if !ok || len(content) == 0 {
		t.Fatalf("数据单元内容 = %#v,期望非空 Node", contentAny)
	}

	// 平铺字段 数据采集时间 在最前
	if content[0].Key != "数据采集时间" {
		t.Errorf("数据单元内容首键 = %s,期望 数据采集时间", content[0].Key)
	}

	// 组节点(值为 Node)的键顺序必须与 Fields 中 TLV 标记行顺序一致
	var wantGroups []string
	for i := range r.Fields {
		if r.Fields[i].Name == "数据类型标志 (TLV)" {
			wantGroups = append(wantGroups, r.Fields[i].Translate)
		}
	}
	var gotGroups []string
	for _, kv := range content {
		if _, isNode := kv.Value.(Node); isNode {
			gotGroups = append(gotGroups, kv.Key)
		}
	}
	if len(wantGroups) == 0 {
		t.Fatal("Fields 中没有 TLV 标记行")
	}
	if len(gotGroups) != len(wantGroups) {
		t.Fatalf("树组数 = %d %v,Fields 标记行 = %d %v", len(gotGroups), gotGroups, len(wantGroups), wantGroups)
	}
	for i := range wantGroups {
		if !strings.HasPrefix(gotGroups[i], wantGroups[i]) {
			t.Errorf("第 %d 组 = %s,期望以 %s 开头", i, gotGroups[i], wantGroups[i])
		}
	}

	// 整车数据组存在,车辆状态是可直接展示的非空标签
	var vehicle Node
	for _, kv := range content {
		if kv.Key == "整车数据" {
			vehicle, _ = kv.Value.(Node)
		}
	}
	if vehicle == nil {
		t.Fatal("缺少 整车数据 组")
	}
	stateAny, ok := treeGet(vehicle, "车辆状态")
	if !ok {
		t.Fatal("整车数据组缺少 车辆状态")
	}
	state, ok := stateAny.(string)
	if !ok || state == "" || state == "-" {
		t.Errorf("车辆状态 = %#v,期望非空可读标签", stateAny)
	}
}

/* (c) BCC 不符:翻转末字节,解析仍成功且校验码给出计算值 */

func TestTreeBCCMismatch(t *testing.T) {
	raw := mustHex(t, loadHex(t, "prod_login_v2016_01.hex"))
	raw[len(raw)-1] ^= 0xFF
	r, err := Parse(fmt.Sprintf("%X", raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) == 0 {
		t.Fatal("BCC 不符时应产生告警")
	}
	root, ok := r.Tree.(Node)
	if !ok {
		t.Fatalf("Tree 类型 = %T,期望 parser.Node", r.Tree)
	}
	v, _ := treeGet(root, "校验码")
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, "不符(计算值") {
		t.Errorf("校验码 = %#v,期望以 不符(计算值 开头", v)
	}
}

/* (d) 空载荷:数据单元内容序列化为 {} */

func TestTreeEmptyPayload(t *testing.T) {
	r, err := Parse(buildEmptyFrame(0x07))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"数据单元内容":{}`) {
		t.Errorf("空载荷树 = %s,期望包含 \"数据单元内容\":{}", b)
	}
}

/* (e) V2025 占位行:说明 + 原始数据(HEX) */

func TestTreeV2025Placeholder(t *testing.T) {
	root := parseTree(t, loadHex(t, "prod_realtime_v2025_01.hex"))
	contentAny, ok := treeGet(root, "数据单元内容")
	if !ok {
		t.Fatal("缺少 数据单元内容")
	}
	content, ok := contentAny.(Node)
	if !ok {
		t.Fatalf("数据单元内容类型 = %T,期望 parser.Node", contentAny)
	}
	if got := treeKeys(content); len(got) != 2 || got[0] != "说明" || got[1] != "原始数据(HEX)" {
		t.Fatalf("V2025 数据单元内容键 = %v,期望 [说明 原始数据(HEX)]", got)
	}
	note, _ := treeGet(content, "说明")
	if s, ok := note.(string); !ok || !strings.Contains(s, "V2025") {
		t.Errorf("说明 = %#v,期望包含 V2025", note)
	}
	rawHex, _ := treeGet(content, "原始数据(HEX)")
	if s, ok := rawHex.(string); !ok || s == "" {
		t.Errorf("原始数据(HEX) = %#v,期望非空 hex", rawHex)
	}
}

/* (f) 同层重复键:组名与叶子名都追加 (2)、(3) 序号 */

func TestTreeDuplicateKeys(t *testing.T) {
	fields := []Field{
		{Offset: 24, Name: "数据类型标志 (TLV)", Translate: "整车数据"},
		{Offset: 25, Name: "车辆状态", RawValue: "1", Translate: "启动", OffsetVal: "-"},
		{Offset: 26, Name: "数据类型标志 (TLV)", Translate: "整车数据"},
		{Offset: 27, Name: "车辆状态", RawValue: "2", Translate: "熄火", OffsetVal: "-"},
	}
	n := buildPayloadTree(fields)
	if got := treeKeys(n); len(got) != 2 || got[0] != "整车数据" || got[1] != "整车数据 (2)" {
		t.Fatalf("组键 = %v,期望 [整车数据 整车数据 (2)]", got)
	}
	second, _ := n[1].Value.(Node)
	if got := treeKeys(second); len(got) != 1 || got[0] != "车辆状态" {
		t.Errorf("第二组键 = %v,期望 [车辆状态](不同组内同名叶子互不影响)", got)
	}

	// 同一组内重复叶子名同样追加序号
	dup := buildPayloadTree([]Field{
		{Offset: 24, Name: "数据类型标志 (TLV)", Translate: "整车数据"},
		{Offset: 25, Name: "未知类型数据", RawValue: "aa"},
		{Offset: 26, Name: "未知类型数据", RawValue: "bb"},
	})
	if len(dup) != 1 {
		t.Fatalf("组数 = %d,期望 1", len(dup))
	}
	group, _ := dup[0].Value.(Node)
	if got := treeKeys(group); len(got) != 2 || got[0] != "未知类型数据" || got[1] != "未知类型数据 (2)" {
		t.Errorf("组内叶子键 = %v,期望 [未知类型数据 未知类型数据 (2)]", got)
	}
}

/* (g) Node 序列化:保序 + 空节点为 {} */

func TestNodeMarshalOrderAndEmpty(t *testing.T) {
	b, err := json.Marshal(Node{{Key: "b", Value: 2}, {Key: "a", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"b":2,"a":1}` {
		t.Errorf("marshal = %s,期望 {\"b\":2,\"a\":1}", b)
	}

	for _, n := range []Node{nil, {}} {
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "{}" {
			t.Errorf("空 Node marshal = %s,期望 {}", b)
		}
	}
}
