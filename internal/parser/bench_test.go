package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkParse 以 668B 生产实时帧(prod_realtime_v2016_01.hex)为输入,
// 基准测试完整解析路径;SetBytes 按解码后的帧字节数计,而非 hex 文本长度。
func BenchmarkParse(b *testing.B) {
	path := filepath.Join("..", "..", "internal", "schema", "testdata", "prod_realtime_v2016_01.hex")
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("金标准帧缺失(%s): %v", path, err)
	}
	hexStr := strings.TrimSpace(string(raw))
	b.SetBytes(int64(len(hexStr) / 2))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Parse(hexStr); err != nil {
			b.Fatalf("解析失败: %v", err)
		}
	}
}
