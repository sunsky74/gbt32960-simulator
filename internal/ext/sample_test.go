package ext

import (
	"path/filepath"
	"testing"
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
