package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v0.1.0", "v0.1.1", -1, true},
		{"v0.2.0", "v0.1.9", 1, true},
		{"1.0.0", "v1.0.0", 0, true},
		{"v1.2.10", "v1.2.9", 1, true},  // 数值比较而非字典序
		{" v0.1.0 ", "v0.1.0", 0, true}, // 容忍首尾空白
		{"v0.1.0", "dev", 0, false},     // 不可解析
		{"", "v0.1.0", 0, false},
		{"v1.2", "v1.2.0", 0, false},       // 段数不足
		{"v0.2.0-rc1", "v0.2.0", 0, false}, // 预发布后缀严格拒绝(fail-closed)
		{"va.b.c", "v1.0.0", 0, false},     // 非数字段
		{"v-1.0.0", "v1.0.0", 0, false},    // 负数段
	}
	for _, c := range cases {
		got, ok := CompareVersions(c.a, c.b)
		if ok != c.ok {
			t.Fatalf("CompareVersions(%q,%q) ok=%v, want %v", c.a, c.b, ok, c.ok)
		}
		if ok && got != c.want {
			t.Fatalf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.2.0", "dev", false}, // dev 不提示更新
		{"v0.2.0", "", false},
		{"bad-tag", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Fatalf("IsNewer(%q,%q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
