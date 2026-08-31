// Package engine 实现模拟器的运行时:TCP/TLS 客户端、自动登录登出、
// 心跳、周期上报与控制台事件总线。本包为纯 Go,不依赖 wails。
package engine

import (
	"errors"
	"io"
)

// GB/T 32960 帧头布局:Header(2B) + Cmd(1B) + Response(1B) + VIN(17B)
// + Encryption(1B) + PayloadLen(2B) = 24B 定长头,随后 Payload(NB) + BCC(1B)。
const (
	frameHeaderLen = 24
	bccLen         = 1
	maxFrameLen    = 65535 + frameHeaderLen + bccLen
)

var validHeaders = map[[2]byte]bool{
	{0x23, 0x23}: true, // "##" V2016
	{0x24, 0x24}: true, // "$$" V2025
}

// ErrFrameTooLarge 表示长度字段超出合理上限,通常是对端发来的脏数据。
var ErrFrameTooLarge = errors.New("gbt32960-sim: frame length exceeds 64KB limit")

// FrameReader 从 io.Reader 中流式拆出完整的 GB/T 32960 帧。
// 半包时阻塞读满;粘包时逐帧返回;遇到非法起始符逐字节重同步。
type FrameReader struct {
	r   io.Reader
	buf []byte
}

// NewFrameReader 创建帧读取器。
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: r, buf: make([]byte, 0, 1024)}
}

// readMore 从底层连接读一段数据追加到缓冲区。EOF 时若仍有残包返回 io.ErrUnexpectedEOF。
func (fr *FrameReader) readMore() error {
	var tmp [2048]byte
	n, err := fr.r.Read(tmp[:])
	if n > 0 {
		fr.buf = append(fr.buf, tmp[:n]...)
	}
	if err != nil {
		if n > 0 && errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

// tryParse 尝试从缓冲区头部解析出一个完整帧。
// 返回 帧字节切片(引用 buf 内部,调用方不得修改) 与 帧总长;不够时 total>0 且 frame 为 nil。
func (fr *FrameReader) tryParse() (frame []byte, total int, err error) {
	for {
		// 1. 头部不足:继续读
		if len(fr.buf) < frameHeaderLen {
			return nil, 0, nil
		}
		// 2. 起始符校验,失败则丢弃 1 字节重同步
		var hdr [2]byte
		copy(hdr[:], fr.buf[:2])
		if !validHeaders[hdr] {
			fr.buf = fr.buf[1:]
			continue
		}
		// 3. 计算整帧长度:payloadLen 位于偏移 22,大端
		payloadLen := int(fr.buf[22])<<8 | int(fr.buf[23])
		total := frameHeaderLen + payloadLen + bccLen
		if total > maxFrameLen {
			return nil, 0, ErrFrameTooLarge
		}
		if len(fr.buf) < total {
			return nil, total, nil
		}
		frame = fr.buf[:total:total]
		return frame, total, nil
	}
}

// Next 阻塞读取下一帧。连接关闭且无残包时返回 io.EOF。
func (fr *FrameReader) Next() ([]byte, error) {
	for {
		frame, _, err := fr.tryParse()
		if err != nil {
			return nil, err
		}
		if frame != nil {
			// 弹出已消费字节(复制一份交给调用方,避免被后续读覆盖)
			out := make([]byte, len(frame))
			copy(out, frame)
			fr.buf = fr.buf[len(frame):]
			return out, nil
		}
		if err := fr.readMore(); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) && len(fr.buf) == 0 {
				return nil, io.EOF
			}
			return nil, err
		}
	}
}
