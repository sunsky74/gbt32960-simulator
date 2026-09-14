package framing

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// BenchmarkFrameReaderNext 单次迭代拆完一整段 N 帧粘包缓冲,反映流式拆帧吞吐。
// SetBytes 报告的是缓冲区原始帧字节数。
func BenchmarkFrameReaderNext(b *testing.B) {
	const frames = 256
	frame := goldenFrame(b, "prod_realtime_v2016_01.hex")
	buf := bytes.Repeat(frame, frames)
	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fr := NewFrameReader(bytes.NewReader(buf))
		got := 0
		for {
			f, err := fr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				b.Fatalf("拆帧失败: %v", err)
			}
			if len(f) == 0 {
				b.Fatal("返回空帧")
			}
			got++
		}
		if got != frames {
			b.Fatalf("帧数 = %d, want %d", got, frames)
		}
	}
}
