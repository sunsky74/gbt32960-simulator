package ext

import (
	"os"
	"testing"
)

// TestV16PackValid 校验客户专属 v16 示例包。
// 注意:该夹具是可选文件(../../docs/extpack/local-pack-v16.json 不随仓库分发),
// 仅当本地存在时才做 scope=server + 0x8C 三命令模板校验;缺失属预期,故保留 skip。
func TestV16PackValid(t *testing.T) {
	const path = "../../docs/extpack/local-pack-v16.json"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("可选示例包未随仓库分发,跳过: %v", err)
	}
	p, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("v16 包校验失败: %v", err)
	}
}
