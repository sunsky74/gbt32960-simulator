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
	forwarder *bridge.Forwarder
}

// NewApp 创建应用装配(在 wails.Run 之前,保证 Bind 可用)。
func NewApp() *App {
	rt := bridge.NewRuntime()
	fwd := bridge.NewForwarder(rt)
	return &App{
		rt:        rt,
		conn:      bridge.NewConnectionService(rt),
		msg:       bridge.NewMessageService(rt),
		console:   bridge.NewConsoleService(fwd),
		parser:    bridge.NewParserService(rt),
		extsvc:    bridge.NewExtService(rt),
		sys:       bridge.NewSystemService(),
		server:    bridge.NewServerService(),
		forwarder: fwd,
	}
}

// startup wails 启动回调:注入上下文并启动事件转发。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.console.SetContext(ctx)
	a.extsvc.SetContext(ctx)
	a.sys.SetContext(ctx)
	a.server.SetContext(ctx)
	go a.forwarder.Start(ctx)
}

// shutdown wails 退出回调:优雅登出并断开。
func (a *App) shutdown(ctx context.Context) {
	if c := a.rt.CurrentClient(); c != nil {
		c.Disconnect()
	}
	_ = a.server.Stop()
}
