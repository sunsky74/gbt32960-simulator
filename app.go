package main

import (
	"context"

	"gbt32960-simulator/bridge"
)

// App 装配运行时、前端服务与事件转发器。
type App struct {
	ctx       context.Context
	rt        *bridge.Runtime
	conn      *bridge.ConnectionService
	msg       *bridge.MessageService
	console   *bridge.ConsoleService
	parser    *bridge.ParserService
	extsvc    *bridge.ExtService
	sys       *bridge.SystemService
	server    *bridge.ServerService
	track     *bridge.TrackService
	forwarder *bridge.Forwarder
	settings  *bridge.SettingsService
	updater   *bridge.UpdaterService

	// fwdCancel 取消事件转发 goroutine(shutdown 时先停转发,再断开连接)。
	fwdCancel context.CancelFunc
}

// NewApp 创建应用装配(在 wails.Run 之前,保证 Bind 可用)。
func NewApp() *App {
	rt := bridge.NewRuntime()
	fwd := bridge.NewForwarder(rt)
	msg := bridge.NewMessageService(rt)
	track := bridge.NewTrackService(rt, msg)
	bridge.WireTrackReplay(msg, track)
	conn := bridge.NewConnectionService(rt)
	// 手动重连会重建客户端(旧客户端 ticker 已随 Disconnect 停止),登录成功后
	// 按 MessageService 的记忆状态恢复周期上报。
	bridge.WireAutoReportResume(conn, msg)
	return &App{
		rt:        rt,
		conn:      conn,
		msg:       msg,
		console:   bridge.NewConsoleService(fwd),
		parser:    bridge.NewParserService(rt),
		extsvc:    bridge.NewExtService(rt),
		sys:       bridge.NewSystemService(),
		server:    bridge.NewServerService(rt),
		track:     track,
		forwarder: fwd,
		settings:  bridge.NewSettingsService(fwd),
		updater:   bridge.NewUpdaterService(version),
	}
}

// startup wails 启动回调:注入上下文并启动事件转发。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	bridge.WireContexts(ctx, a.console, a.extsvc, a.sys, a.server, a.track, a.updater)
	bridge.WireUpdaterStartup(a.updater) // 更新缓存不跨会话复用(设计文档 §5.3)
	fwdCtx, cancel := context.WithCancel(ctx)
	a.fwdCancel = cancel
	go a.forwarder.Start(fwdCtx)
}

// shutdown wails 退出回调:先停事件转发(此后事件不再推送/镜像),
// 再优雅登出并断开。
func (a *App) shutdown(ctx context.Context) {
	if a.fwdCancel != nil {
		a.fwdCancel()
	}
	if c := a.rt.CurrentClient(); c != nil {
		c.Disconnect()
	}
	_ = a.server.Stop()
}
