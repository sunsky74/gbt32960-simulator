package ext

import (
	"testing"
	"time"
)

func TestEncodeBeanTimeGolden(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b := EncodeBeanTime(at)
	// 十进制(非 BCD):年-2000=26=0x1a,月1 日2 时3 分4 秒5
	const want = "1a0102030405"
	if got := hexBytes(b[:]); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestEncodeBeanTimeClamp(t *testing.T) {
	at := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	b := EncodeBeanTime(at)
	if b[0] != 0 {
		t.Fatalf("2000 年前应截断为 0,got %d", b[0])
	}
}

func hexBytes(b []byte) string {
	const d = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, d[x>>4], d[x&0x0F])
	}
	return string(out)
}
