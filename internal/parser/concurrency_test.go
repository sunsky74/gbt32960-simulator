package parser

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// tb1Cases:不同帧、不同截断点 → 截断告警中应出现的字段名。
// 截断点由金标准帧尾部裁剪得到(声明长度不变,解析器按实际剩余字节截断)。
var tb1Cases = []struct {
	file  string
	cut   int
	field string
}{
	{"prod_login_v2016_01.hex", 25, "数据采集时间"},
	{"prod_login_v2016_01.hex", 24, "登入流水号"},
	{"prod_login_v2016_01.hex", 22, "ICCID"},
	{"prod_realtime_v2016_01.hex", 9, "温度1·温度探针个数"},
	{"prod_realtime_v2016_01.hex", 12, "温度数据子系统个数"},
	{"prod_realtime_v2016_01.hex", 14, "电压1·电压[104]"},
}

// truncateHex 把金标准帧按字节尾部裁剪 cut 字节(保留声明的数据单元长度)。
func truncateHex(t *testing.T, file string, cut int) string {
	t.Helper()
	b := mustHex(t, loadHex(t, file))
	if cut < 1 || cut >= len(b)-24 {
		t.Fatalf("非法截断点 cut=%d(%s 共 %d 字节)", cut, file, len(b))
	}
	return fmt.Sprintf("%X", b[:len(b)-cut])
}

func hasTruncationWarning(res *Result, field string) bool {
	want := fmt.Sprintf("字段 %q 处被截断", field)
	for _, w := range res.Warnings {
		if strings.Contains(w, want) {
			return true
		}
	}
	return false
}

// TB1:截断告警中的字段名必须与真正被截断的字段一致。
// 行为锁:per-walker pending 修复不得改变单线程下的告警文本。
func TestTB1TruncationWarningNamesField(t *testing.T) {
	type frame struct{ hex, field string }
	frames := make([]frame, 0, len(tb1Cases))
	for _, c := range tb1Cases {
		frames = append(frames, frame{hex: truncateHex(t, c.file, c.cut), field: c.field})
	}

	for i := 0; i < 100; i++ {
		for _, f := range frames {
			r, err := Parse(f.hex)
			if err != nil {
				t.Fatalf("第 %d 轮解析失败: %v", i, err)
			}
			if !hasTruncationWarning(r, f.field) {
				t.Fatalf("第 %d 轮:告警 = %v,期望包含 字段 %q 处被截断", i, r.Warnings, f.field)
			}
		}
	}
}

// TB2:并发 Parse 不同截断帧,-race 下不得报竞态
// (修复前 take() 读、各字段方法写的包级 pendingName 是真实数据竞争)。
func TestTB2ConcurrentParseTruncated(t *testing.T) {
	inputs := make([]string, 0, len(tb1Cases))
	for _, c := range tb1Cases {
		inputs = append(inputs, truncateHex(t, c.file, c.cut))
	}

	const goroutines = 8
	const iterations = 300
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				r, err := Parse(inputs[(g+i)%len(inputs)])
				if err != nil {
					t.Errorf("goroutine %d: 解析失败: %v", g, err)
					return
				}
				if len(r.Warnings) == 0 {
					t.Errorf("goroutine %d: 截断帧未产生告警", g)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
