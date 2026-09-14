package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzParse 用任意文本驱动解析器:任何输入都不允许 panic(返回错误是正常路径,不做断言)。
// 种子取自 committed 金标准帧 hex 文本与若干手写畸形输入。
func FuzzParse(f *testing.F) {
	for _, name := range []string{
		"prod_login_v2016_01.hex",
		"prod_logout_v2016_01.hex",
		"prod_realtime_v2016_01.hex",
		"prod_realtime_v2025_01.hex",
		"prod_reissue_v2025_01.hex",
	} {
		path := filepath.Join("..", "..", "internal", "schema", "testdata", name)
		b, err := os.ReadFile(path)
		if err != nil {
			f.Fatalf("金标准帧缺失(%s): %v", path, err)
		}
		f.Add(strings.TrimSpace(string(b)))
	}
	f.Add("")
	f.Add("2323")
	f.Add("232301fe")
	f.Add("zz not hex at all")
	f.Add(strings.Repeat("23", 300))

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = Parse(input) // 只要求不 panic;错误结果是解析器的合法输出
	})
}
