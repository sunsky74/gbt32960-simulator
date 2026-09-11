package ext

import (
	"os"
	"testing"
)

// 客户专属 v16 示例包不随仓库分发;该文件存在时必须通过校验(scope=server + 0x8C 三命令模板)。
func TestV16PackValid(t *testing.T) {
	const path = "../../docs/extpack/local-pack-v16.json"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("示例包未随仓库分发,跳过: %v", err)
	}
	p, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("v16 包校验失败: %v", err)
	}
}
