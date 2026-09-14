package framing

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenFrame 读取 schema 金标准帧 hex 并解码为原始字节。
// 金标准随仓库提交,缺失即断供,直接失败而不是跳过。
func goldenFrame(tb testing.TB, name string) []byte {
	tb.Helper()
	path := filepath.Join("..", "schema", "testdata", name)
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("金标准帧缺失(%s): %v", path, err)
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		tb.Fatalf("金标准帧 hex 解码失败(%s): %v", path, err)
	}
	return raw
}

// FuzzFrameReaderNext 用任意字节流驱动拆帧器,校验三个不变量:
// 不 panic、遍历必然终止(Next 最终返回错误)、返回的每一帧非空。
func FuzzFrameReaderNext(f *testing.F) {
	for _, name := range []string{
		"prod_login_v2016_01.hex",
		"prod_logout_v2016_01.hex",
		"prod_realtime_v2016_01.hex",
		"prod_realtime_v2025_01.hex",
		"prod_reissue_v2025_01.hex",
	} {
		f.Add(goldenFrame(f, name))
	}
	f.Add([]byte{0x23, 0x23, 0x02, 0xfe})                      // 半帧:仅起始符 + 帧头前 4 字节
	f.Add([]byte("garbage before frame ##\x01\x02 and after")) // 垃圾流:需要重同步
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// limit 取服务端模式同款 8KB(spec §5.2 超长防护),同时覆盖 ErrFrameTooLarge 重同步路径。
		fr := NewFrameReaderLimit(bytes.NewReader(data), 8192)
		// 每轮要么从缓冲区消费 ≥1 字节,要么向前读入 ≥1 字节,终止性有界;
		// 防御性上限把"疑似不终止"变成可归约的失败,而不是让 fuzz 进程挂起。
		for i := 0; i <= 4*len(data)+64; i++ {
			frame, err := fr.Next()
			if err != nil {
				return // io.EOF / io.ErrUnexpectedEOF / ErrFrameTooLarge 均为合法终点
			}
			if len(frame) == 0 {
				t.Fatalf("Next 返回空帧,data=%d 字节", len(data))
			}
		}
		t.Fatalf("Next 未在 %d 次迭代内终止,data=%d 字节", 4*len(data)+64, len(data))
	})
}
