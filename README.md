# GB/T 32960 客户端模拟器

基于 [Wails v2](https://wails.io) 的桌面工具(Go + Vue 3),用于模拟 GB/T 32960 国标协议的**车辆客户端**,面向平台接入联调与协议开发场景:无需真实车辆/T-Box,即可完成注册、登录、实时上报、补发、平台下行应答等全流程测试。

## 功能

- **客户端模拟**(核心页)
  - TCP / TCP+TLS 连接,多连接配置档案(Profile)管理,连接前可达性测试
  - 协议版本切换:**2016 国标版** / **2025 新版**(双版本报文结构)
  - 自动流程:车辆注册 → 登录 → 心跳 → 登出;状态机驱动,事件实时推送前端
  - **企业平台级联模式**:0x05 平台登入(平台 VIN/账号/密码)→ 车辆数据转发(车辆 VIN)→ 0x06 平台登出,心跳/校时使用平台标识
  - 实时信息上报(0x02):按组配置报文内容,支持 hex 预览(不发送)
  - 补发上报(0x03):自定义条数、起始偏移、间隔,模拟离线补报
  - 周期上报:可开关、可调间隔
  - 平台下行命令应答:0x80 参数查询、0x81/0x82 标准应答,以及 私有远控 私有协议 0x8A 双层应答
  - 轨迹回放:导入 GPX / Excel / CSV 轨迹,搭载周期上报逐点注入位置数据(0x02)
- **报文解析**:粘贴/输入 hex 帧,逐字节 + 字段级解读(2016 版字段表)
- **服务端模式**:本机 TCP Server 接收车端/平台连接(被动接收器)
  - 会话管理:车辆/平台登入自动应答、在线会话表(登入时长、最后活跃、RX/TX 计数)、重复登入拒绝
  - 报文监控:报文流实时着色(RX/TX/告警/未登入标记)、帧详情解码、按 VIN 过滤、会话日志导出
  - 空闲检测:可开关、可调时长(5~3600 秒),运行中即时生效
  - 平台下发:按扩展包 down 命令模板向在线车辆下发指令
- **扩展包**:JSON 声明式私有协议扩展——私有数据单元 / 私有命令 / 服务端应答规则,导入即生效、零代码;含内置示例包与[编写指南](docs/extpack-guide.md)
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
bridge/                 # 前端绑定层:连接、报文、控制台、解析、服务端、扩展包、轨迹 + 事件转发
internal/engine/        # 协议客户端引擎:状态机、心跳、周期上报、下行命令处理
internal/servermode/    # 平台侧接收服务:accept 循环、会话注册表、自动应答、环形缓冲
internal/schema/        # 2016/2025 双版本报文组定义与组装(golden 测试覆盖)
internal/parser/        # hex 帧 → 字段级解析
internal/ext/           # 扩展包:加载 / 校验 / 编码 / 编译
internal/track/         # 轨迹文件解析(GPX / Excel / CSV)
internal/framing/       # 帧读取器(收发共用,含超长防护)
internal/store/         # 配置 JSON 持久化
internal/tlsconf/       # TLS 配置
frontend/src/           # Vue 前端(客户端模拟 / 报文解析 / 服务端模式 / 扩展包 / 设置)
```

## 文档

- [扩展包编写指南](docs/extpack-guide.md) — 面向实施人员的 JSON 扩展包编写说明
- [待办与设计草案](docs/backlog.md) — 未排期功能设计与未来工作

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

# 发布:推送 v* 标签(如 v0.1.0)即触发三平台自动构建并创建 Release(见 .github/workflows/release.yml)
```

## 依赖说明

协议层由独立库 [`github.com/sunsky74/gb32960`](https://github.com/sunsky74/gb32960) 提供(GB/T 32960 帧编解码与数据模型),通过 `go.mod` 版本号引用,无需本地路径。

本地联调协议库时,可用 `replace` 临时指向本地克隆:

```
replace github.com/sunsky74/gb32960 => /path/to/gb32960
```
