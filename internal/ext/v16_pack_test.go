package ext

import "testing"

// 真实 私有远控 v16 示例包必须始终通过校验(scope=server + 0x8C 三命令模板)。
func TestV16PackValid(t *testing.T) {
	p, err := LoadFile("../../docs/extpack/local-pack-v16.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("v16 包校验失败: %v", err)
	}
}
