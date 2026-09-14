package ext

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoadText 用任意文本驱动扩展包加载入口:
// 不 panic;若成功返回非 nil 包,再调 Validate 同样不得 panic。
// 种子取自 committed 测试夹具与空/垃圾/半结构化文本。
func FuzzLoadText(f *testing.F) {
	for _, name := range []string{"demo.json", "remote-ack-fixture.json"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatalf("测试夹具缺失(%s): %v", name, err)
		}
		f.Add(string(b))
	}
	f.Add("")
	f.Add("{}")
	f.Add("not json")
	f.Add(`{"meta":{}}`)

	f.Fuzz(func(t *testing.T, text string) {
		p, err := LoadText(text, "fuzz")
		if err == nil && p != nil {
			_ = Validate(p) // 只要求不 panic;成功解析的包二次校验必须同样安全
		}
	})
}
