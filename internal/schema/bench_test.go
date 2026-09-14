package schema

import (
	"testing"
	"time"
)

// BenchmarkAssembleRealtime 以 V2016 默认启用组配置(assemble_test.go 的 fullGroups,
// 即 V2016Groups 中 Enabled=true 的 7 组)为输入,基准测试配置 → RealTimeData 组装路径。
func BenchmarkAssembleRealtime(b *testing.B) {
	cfg := fullGroups()
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.Local)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := AssembleRealtime(cfg, at); err != nil {
			b.Fatalf("组装失败: %v", err)
		}
	}
}
