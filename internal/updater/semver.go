// Package updater 应用内更新的纯逻辑:版本比较、GitHub Releases 查询、平台资产匹配。
// 本包不 import wails;网络客户端可注入以支持测试。
package updater

import (
	"strconv"
	"strings"
)

// 版本比较契约:仅接受 "vX.Y.Z"(X/Y/Z 为非负整数,v 前缀可省,首尾空白容忍)。
// 不可解析输入一律 ok=false —— 检查链路对不可解析输入 fail-closed(不提示更新)。

// CompareVersions 比较 a、b:-1(a<b)/ 0(相等)/ 1(a>b);任一不可解析时 ok=false。
func CompareVersions(a, b string) (int, bool) {
	am, ai, ap, okA := parseVersion(a)
	bm, bi, bp, okB := parseVersion(b)
	if !okA || !okB {
		return 0, false
	}
	switch {
	case am != bm:
		return cmpInt(am, bm), true
	case ai != bi:
		return cmpInt(ai, bi), true
	default:
		return cmpInt(ap, bp), true
	}
}

// IsNewer 判断 latest 是否严格新于 current;不可解析时返回 false(不提示更新)。
func IsNewer(latest, current string) bool {
	c, ok := CompareVersions(latest, current)
	return ok && c > 0
}

// parseVersion 解析 "vX.Y.Z";段数不为 3 或存在非数字/负数段时 ok=false。
func parseVersion(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
