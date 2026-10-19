package ext

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"gbt32960-simulator/internal/schema"
)

// TestDocsSamplePackValid 锁死内置示例包与校验器的一致性:指南示例永不过期。
func TestDocsSamplePackValid(t *testing.T) {
	p, err := LoadFile(filepath.Join("..", "..", "docs", "extpack", "sample-pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.ID != "sample-private" || len(p.Commands) != 2 || len(p.Realtime.AppendUnits) != 1 {
		t.Fatalf("示例包结构 = %+v", p.Meta)
	}
}

// yiweiPack 加载本地易微 TSP-TBox V0.2 扩展包。
// 注意:该夹具是可选文件(docs/extpack/yiwei-tsp-v02-pack.json 不入库,已加入 .gitignore),
// 缺失时跳过——与 TestV16PackValid 同一口径。
func yiweiPack(t *testing.T) *Pack {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "extpack", "yiwei-tsp-v02-pack.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("本地扩展包未随仓库分发,跳过: %v", err)
	}
	p, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestDocsYiweiPackValid 易微 V0.2 包结构:12 个实时自定义单元(0x87~0x99),无私有命令,client+parser。
func TestDocsYiweiPackValid(t *testing.T) {
	p := yiweiPack(t)
	if p.Meta.ID != "yiwei-tsp-v02" || p.Meta.BaseVersion != "2016" {
		t.Fatalf("包元信息 = %+v", p.Meta)
	}
	if !ScopeHas(p, ScopeClient) || !ScopeHas(p, ScopeParser) || ScopeHas(p, ScopeServer) {
		t.Fatalf("scope = %v", p.Meta.Scope)
	}
	if len(p.Realtime.AppendUnits) != 12 || len(p.Commands) != 0 {
		t.Fatalf("结构 = %d 单元 / %d 命令", len(p.Realtime.AppendUnits), len(p.Commands))
	}
}

// TestDocsYiweiPackWidths 每个单元的 DSL 固定宽度必须等于协议表的数据体长度:
// 线上 TLV = unitCode + 数据长度(2B) + 数据,而"数据长度"就是协议表里的数据长度字段,
// 因此不另建 u16 字段;TLV 总长 - 3 即数据体长度。
func TestDocsYiweiPackWidths(t *testing.T) {
	p := yiweiPack(t)
	want := map[int]int{
		0x87: 51, // 电池(含 4B CLTB)
		0x89: 23, // 空调
		0x8A: 28, // 多合一
		0x8B: 27, // 底盘
		0x8C: 34, // 整车
		0x8D: 24, // 辅助驾驶
		0x94: 0,  // 循环块,tail 变长(默认空)
		0x95: 32, // 软件版本 16×u16
		0x96: 32, // 硬件版本 16×u16
		0x97: 352,
		0x98: 22,
		0x99: 8,
	}
	if len(want) != len(p.Realtime.AppendUnits) {
		t.Fatalf("单元数 = %d, 期望 %d", len(p.Realtime.AppendUnits), len(want))
	}
	for _, u := range p.Realtime.AppendUnits {
		expected, ok := want[u.UnitCode]
		if !ok {
			t.Fatalf("未预期单元码 0x%02X", u.UnitCode)
		}
		tlv, err := EncodeUnit(u, DefaultsFor(u))
		if err != nil {
			t.Fatalf("0x%02X 编码失败: %v", u.UnitCode, err)
		}
		if got := len(tlv) - 3; got != expected {
			t.Fatalf("0x%02X 数据体 = %d 字节, 期望 %d", u.UnitCode, got, expected)
		}
		if int(tlv[0]) != u.UnitCode || int(tlv[1])<<8|int(tlv[2]) != expected {
			t.Fatalf("0x%02X TLV 头 = %x", u.UnitCode, tlv[:3])
		}
	}
}

// TestDocsYiweiPackWire 锁死 0x99 的线上字节(协议类型/主副版本/5B 车型名)。
func TestDocsYiweiPackWire(t *testing.T) {
	p := yiweiPack(t)
	var unit *AppendUnit
	for i := range p.Realtime.AppendUnits {
		if p.Realtime.AppendUnits[i].UnitCode == 0x99 {
			unit = &p.Realtime.AppendUnits[i]
		}
	}
	if unit == nil {
		t.Fatal("缺少 0x99 单元")
	}
	row := schema.RowValue{
		"protocolType": 1.0, "majorVersion": 1.0, "minorVersion": 5.0,
		"vehicleName": "4356303120", // "CV01 "
	}
	got, err := EncodeUnit(*unit, row)
	if err != nil {
		t.Fatal(err)
	}
	const want = "9900080101054356303120"
	if hex.EncodeToString(got) != want {
		t.Fatalf("线上字节 = %x, 期望 %s", got, want)
	}
}
