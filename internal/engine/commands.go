package engine

import (
	"sync"

	"github.com/sunsky74/gb32960/api"
)

// extCommandNames 扩展命令注册表:code → 显示名。
// 注册的命令在 commandFor 中构造 Min=Max=code 的精确副本,
// 规避库枚举按区间折叠改写线上命令字节(如 0x0A 折叠为 0x09)。
var extCommands = struct {
	mu   sync.Mutex
	cmds map[api.GBTVersion]map[byte]string
}{cmds: map[api.GBTVersion]map[byte]string{}}

// remoteBuiltin 私有远控 0x8A 命令(迁移自原硬编码,AC-3:行为不变)。
const remoteCmdCode = 0x8A

const remoteCmdName = "REMOTE_CONTROL"

func init() {
	RegisterCommand(api.V2016, remoteCmdCode, remoteCmdName)
	RegisterCommand(api.V2025, remoteCmdCode, remoteCmdName)
}

// RegisterCommand 注册精确命令码(包激活时由 bridge 调用;幂等覆盖同名)。
func RegisterCommand(v api.GBTVersion, code byte, name string) {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	if extCommands.cmds[v] == nil {
		extCommands.cmds[v] = map[byte]string{}
	}
	extCommands.cmds[v][code] = name
}

// RegisterCommandIfAbsent 同码已注册时保留首个(同码多命令场景:首个 label 作为显示名)。
func RegisterCommandIfAbsent(v api.GBTVersion, code byte, name string) {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	if extCommands.cmds[v] == nil {
		extCommands.cmds[v] = map[byte]string{}
	}
	if _, ok := extCommands.cmds[v][code]; !ok {
		extCommands.cmds[v][code] = name
	}
}

// ResetExtCommands 清除全部扩展命令并恢复私有远控内置条目(换绑/解绑时调用)。
func ResetExtCommands() {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	extCommands.cmds = map[api.GBTVersion]map[byte]string{
		api.V2016: {remoteCmdCode: remoteCmdName},
		api.V2025: {remoteCmdCode: remoteCmdName},
	}
}

func extCommandName(v api.GBTVersion, code byte) (string, bool) {
	extCommands.mu.Lock()
	defer extCommands.mu.Unlock()
	if m := extCommands.cmds[v]; m != nil {
		n, ok := m[code]
		return n, ok
	}
	return "", false
}
