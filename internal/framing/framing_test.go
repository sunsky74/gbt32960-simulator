package framing

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// makeFrame 构造一个最小合法帧用于拆帧测试。
func makeFrame(cmd byte, payloadLen int) []byte {
	head := []byte{0x23, 0x23, cmd, 0xFE}
	head = append(head, []byte("LSV00000000000001")...)
	head = append(head, 0x01, byte(payloadLen>>8), byte(payloadLen))
	head = append(head, make([]byte, payloadLen)...)
	bcc := byte(0)
	for _, b := range head[2:] {
		bcc ^= b
	}
	return append(head, bcc)
}

func TestFrameReaderComplete(t *testing.T) {
	frame := makeFrame(0x07, 0)
	fr := NewFrameReader(bytes.NewReader(frame))
	got, err := fr.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatalf("frame mismatch: %x", got)
	}
	if _, err := fr.Next(); err != io.EOF {
		t.Fatalf("expect EOF, got %v", err)
	}
}

func TestFrameReaderSplitAndSticky(t *testing.T) {
	f1 := makeFrame(0x01, 10)
	f2 := makeFrame(0x02, 32)
	var stream []byte
	stream = append(stream, f1[:5]...) // 半包
	stream = append(stream, f1[5:]...)
	stream = append(stream, f2...) // 粘包

	fr := NewFrameReader(bytes.NewReader(stream))
	g1, err := fr.Next()
	if err != nil || !bytes.Equal(g1, f1) {
		t.Fatalf("f1 err=%v equal=%v", err, bytes.Equal(g1, f1))
	}
	g2, err := fr.Next()
	if err != nil || !bytes.Equal(g2, f2) {
		t.Fatalf("f2 err=%v equal=%v", err, bytes.Equal(g2, f2))
	}
}

func TestFrameReaderResyncOnGarbage(t *testing.T) {
	frame := makeFrame(0x07, 0)
	stream := append([]byte{0x00, 0xFF, 0x23, 0x99}, frame...)
	fr := NewFrameReader(bytes.NewReader(stream))
	got, err := fr.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatalf("resync failed: %x", got)
	}
}

func TestFrameReaderV2025Header(t *testing.T) {
	frame := makeFrame(0x07, 0)
	frame[0], frame[1] = 0x24, 0x24
	fr := NewFrameReader(bytes.NewReader(frame))
	if _, err := fr.Next(); err != nil {
		t.Fatalf("v2025 header rejected: %v", err)
	}
}

func TestFrameReaderLimitRejectsOversize(t *testing.T) {
	hdr := append([]byte{0x23, 0x23, 0x07, 0xFE}, []byte("LVBV3J7B0LY000001")...)
	hdr = append(hdr, 0x00, 0x23, 0x28) // 声明 payload=9000
	good := makeFrame(0x07, 0)          // 后续跟一帧合法帧,验证丢弃伪头后重同步成功
	stream := append(append([]byte{}, hdr...), good...)
	fr := NewFrameReaderLimit(bytes.NewReader(stream), 8192)
	if _, err := fr.Next(); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("超长声明应报 ErrFrameTooLarge, got %v", err)
	}
	got, err := fr.Next()
	if err != nil || !bytes.Equal(got, good) {
		t.Fatalf("超限丢弃后应重同步读出合法帧: err=%v equal=%v", err, bytes.Equal(got, good))
	}
}
