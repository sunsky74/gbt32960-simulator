package ext

import (
	"os"
	"path/filepath"
	"testing"

	"gbt32960-simulator/internal/schema"
)

// BenchmarkEncodeUnit 以 testdata/demo.json 的首个追加单元为样本,
// 用与 TestEncodeUnitTLV 相同的代表行值基准测试单行 TLV 编码路径。
func BenchmarkEncodeUnit(b *testing.B) {
	raw, err := os.ReadFile(filepath.Join("testdata", "demo.json"))
	if err != nil {
		b.Fatalf("测试夹具缺失: %v", err)
	}
	p, err := LoadText(string(raw), "demo.json")
	if err != nil {
		b.Fatalf("加载 demo 包: %v", err)
	}
	if len(p.Realtime.AppendUnits) == 0 {
		b.Fatal("demo 包无追加单元")
	}
	u := p.Realtime.AppendUnits[0]
	row := schema.RowValue{
		"soc2": 80, "packVolt": 3.3, "temp": 25,
		"flags": map[string]any{"bit0": true}, "sn": "a1b2",
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := EncodeUnit(u, row); err != nil {
			b.Fatalf("编码单元: %v", err)
		}
	}
}
