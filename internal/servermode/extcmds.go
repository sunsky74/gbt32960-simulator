package servermode

import (
	"sync"

	"github.com/sunsky74/gb32960/types"
)

// ExtCmdRule 扩展命令的服务端处理规则(由 bridge 从扩展包声明注入,引擎不感知具体协议)。
type ExtCmdRule struct {
	Label    string             // 命令显示名(报文流/遥测)
	Reply    bool               // true=对请求帧(0xFE)按规则自动回应答;false=仅显示名
	RespType types.ResponseType // 自动应答标志(Reply=true 时有效)
	Echo     bool               // true=应答体回显请求体;false=空体
}

// extCmds 扩展命令注册表:命令码 → 应答规则。bridge 在扩展包导入/移除时同步。
var extCmds = struct {
	mu   sync.RWMutex
	rule map[byte]ExtCmdRule
}{rule: map[byte]ExtCmdRule{}}

// RegisterExtCmd 注册(覆盖)扩展命令规则;同码多命令时由调用方决定首个生效。
func RegisterExtCmd(code byte, rule ExtCmdRule) {
	extCmds.mu.Lock()
	defer extCmds.mu.Unlock()
	extCmds.rule[code] = rule
}

// ResetExtCmds 清空全部扩展命令规则(换包/卸载时调用)。
func ResetExtCmds() {
	extCmds.mu.Lock()
	defer extCmds.mu.Unlock()
	extCmds.rule = map[byte]ExtCmdRule{}
}

// ExtCmd 查询扩展命令规则。
func ExtCmd(code byte) (ExtCmdRule, bool) {
	extCmds.mu.RLock()
	defer extCmds.mu.RUnlock()
	r, ok := extCmds.rule[code]
	return r, ok
}
