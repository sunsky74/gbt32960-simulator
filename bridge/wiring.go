package bridge

import (
	"context"

	"gbt32960-simulator/internal/updater"
)

// wiring.go 汇总 app 装配期的服务接线(替代原导出的 SetContext/SetXxx 方法:
// Wails v2 会把绑定结构体上所有导出方法暴露为 RPC,内部注入方法不应出现在
// 前端绑定面,故统一走包内直赋字段)。
// 这些函数只应在 app 装配与 startup 阶段调用一次。

// WireContexts 把 wails 上下文注入各服务(原生对话框等能力需要)。
func WireContexts(ctx context.Context, console *ConsoleService, ext *ExtService, sys *SystemService, server *ServerService, track *TrackService, updater *UpdaterService) {
	console.ctx = ctx
	ext.ctx = ctx
	sys.ctx = ctx
	server.ctx = ctx
	track.ctx = ctx
	updater.ctx = ctx
}

// WireTrackReplay 装配轨迹回放钩子:周期上报每次 tick 经钩子推进轨迹点。
func WireTrackReplay(msg *MessageService, track *TrackService) {
	msg.setTrackReplay(track)
}

// WireAutoReportResume 装配"登录成功"回调:手动重连会重建客户端(旧 ticker
// 已随 Disconnect 停止),登录成功后按 MessageService 的记忆状态恢复周期上报。
func WireAutoReportResume(conn *ConnectionService, msg *MessageService) {
	conn.setOnConnected(func() { _ = msg.resumeAutoReport() })
}

// WireUpdaterStartup 装配更新服务启动行为:清理更新缓存(不跨会话复用)+ 清理陈旧替换备份
// (.bak-*/.old/暂存,上一会话失败或中断的残留;尽力而为,失败忽略)。
func WireUpdaterStartup(u *UpdaterService) {
	u.cleanupCache()
	_ = updater.CleanupStaleBackups()
}
