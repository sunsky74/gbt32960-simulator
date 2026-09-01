package ext

import "time"

// EncodeBeanTime 按协议库 BeanTime 线格式编码时间:
// 6 字节十进制(年-2000, 月, 日, 时, 分, 秒)——非 BCD。
func EncodeBeanTime(at time.Time) [6]byte {
	y := at.Year() - 2000
	if y < 0 {
		y = 0
	}
	if y > 255 {
		y = 255
	}
	return [6]byte{
		byte(y), byte(at.Month()), byte(at.Day()),
		byte(at.Hour()), byte(at.Minute()), byte(at.Second()),
	}
}
