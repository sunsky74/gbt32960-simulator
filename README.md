# GB/T 32960 客户端模拟器

基于 [Wails v2](https://wails.io) 的桌面工具(Go + Vue 3),用于模拟 GB/T 32960 国标协议的**车辆客户端**,面向平台接入联调与协议开发场景:无需真实车辆/T-Box,即可完成注册、登录、实时上报、补发、平台下行应答等全流程测试。

## 功能

- **客户端模拟**(核心页)
  - TCP / TCP+TLS 连接,多连接配置档案(Profile)管理,连接前可达性测试
  - 协议版本切换:**2016 国标版** / **2025 新版**(双版本报文结构)
  - 自动流程:车辆注册 → 登录 → 心跳 → 登出;状态机驱动,事件实时推送前端
  - 实时信息上报(0x02):按组配置报文内容,支持 hex 预览(不发送)
  - 补发上报(0x03):自定义条数、起始偏移、间隔,模拟离线补报
  - 周期上报:可开关、可调间隔
  - 平台下行命令应答:0x80 参数查询、0x81/0x82 标准应答,以及 私有远控 私有协议 0x8A 双层应答
- **报文解析**:粘贴/输入 hex 帧,逐字节 + 字段级解读(2016 版字段表)
- **服务端模式**:本机 TCP Server 接收客户端上报(当前为界面框架,功能规划中)
- 深色 / 浅色双主题,中文界面

## 技术栈

| 端 | 技术 |
|---|---|
| 后端 | Go 1.25 · Wails v2 |
| 前端 | Vue 3 · TypeScript · Ant Design Vue · Vite |
| 协议库 | `github.com/sunsky74/gb32960`(帧编解码/数据模型) |

## 目录结构

```
main.go / app.go        # Wails 入口与服务装配
bridge/                 # 前端绑定层:连接、报文、控制台、解析服务 + 事件转发
internal/engine/        # 协议客户端引擎:状态机、心跳、周期上报、下行命令处理
internal/schema/        # 2016/2025 双版本报文组定义与组装(golden 测试覆盖)
internal/parser/        # hex 帧 → 字段级解析
internal/store/         # 配置 JSON 持久化
internal/tlsconf/       # TLS 配置
frontend/src/           # Vue 前端(客户端模拟 / 报文解析 / 服务端模式 / 设置)
```

## 开发与构建

```bash
# 前端依赖
cd frontend && npm install && cd ..

# 开发模式(热重载,浏览器调试入口 http://localhost:34115)
wails dev

# 运行后端测试
go test ./...

# 打包生产应用(输出 build/bin/)
wails build
```

## 依赖说明

协议库通过本地 `replace` 引用(Java project4j 中 gateway-connect-gbt32960 的 Go 移植版,同步演进):

```
replace github.com/sunsky74/gb32960 => /Users/sunsky/code/java/template/project4j/gb32960-go
```

在其他机器构建前,需先克隆 project4j 并将 `go.mod` 中的 replace 路径指向本地位置。
