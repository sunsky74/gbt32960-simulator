# GB/T 32960 客户端模拟器

GB/T 32960 客户端模拟器是一个基于 [Wails v2](https://wails.io) 的桌面工具（Go + Vue 3），用于模拟 GB/T 32960 国标协议的**车辆客户端**，面向平台接入联调与协议开发场景。内置客户端模拟、报文解析、服务端模式、扩展包与轨迹回放五大能力：无需真实车辆 / T-Box，即可完成注册、登录、实时上报、补发、平台下行应答等全流程测试。

> 协议层由独立库 [github.com/sunsky74/gb32960](https://github.com/sunsky74/gb32960) 提供（帧编解码与数据模型，V2016 / V2025 双版本）；本地联调该库时，可在 `go.mod` 中用 `replace` 临时指向本地克隆。

## 目录

1. [特性概览](#特性概览)
2. [环境要求](#环境要求)
3. [快速开始](#快速开始)
4. [发布](#发布)
5. [项目结构](#项目结构)
6. [技术栈](#技术栈)
7. [文档](#文档)

## 特性概览

- ✅ **客户端模拟**——TCP / TCP+TLS 连接，多连接档案（Profile）管理，连接前可达性测试；状态机自动完成注册 → 登录 → 心跳 → 登出
- ✅ **双协议版本**——**2016 国标版** / **2025 新版** 报文结构一键切换
- ✅ **数据上报**——实时上报（0x02）按组配置内容、hex 预览；补发（0x03）自定义条数 / 起始偏移 / 间隔；周期上报可开关、可调间隔
- ✅ **企业平台级联**——0x05 平台登入 → 车辆数据转发 → 0x06 平台登出，心跳 / 校时使用平台标识
- ✅ **轨迹回放**——导入 GPX / Excel / CSV 轨迹，搭载周期上报逐点注入位置数据
- ✅ **下行应答**——0x80 参数查询、0x81/0x82 标准应答，以及私有协议 0x8A 双层应答
- ✅ **报文解析**——hex 帧逐字节 + 字段级解读（2016 版字段表）；所有解析结果均可展示为**可折叠的中文 JSON 树**
- ✅ **服务端模式**——本机 TCP Server 接收车端 / 平台连接：会话管理、自动应答、报文监控（着色 / 过滤 / 导出）、空闲检测、平台下发
- ✅ **扩展包**——JSON 声明式私有协议扩展（私有数据单元 / 私有命令 / 下行模板），导入即生效、零代码，含内置示例与编写指南
- ✅ **控制台与导出**——收发事件实时展示，支持 CSV / LOG / JSON 导出（有序中文 JSON）
- ✅ **深色 / 浅色双主题**，全中文界面

## 环境要求

| 组件 | 说明 |
| --- | --- |
| Go | 1.25+（与协议库、Wails v2.15 对齐） |
| Node.js | 20.19+（推荐 22 LTS） |
| Wails CLI | v2.15.0：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0` |
| 平台依赖 | macOS：Xcode Command Line Tools；Windows：WebView2 Runtime（Win10+ 通常自带）；Linux：GTK3 + WebKitGTK（见下） |

### 安装示例

```bash
# Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0

# 环境自检（自动检查各平台依赖是否齐备）
wails doctor

# Linux 构建依赖（Ubuntu 24.04+）
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config libegl1
# Ubuntu 22.04 及更早：改用 libwebkit2gtk-4.0-dev，构建时去掉 webkit2_41 标签
```

## 快速开始

```bash
# 克隆并安装前端依赖
git clone https://github.com/sunsky74/gbt32960-simulator.git
cd gbt32960-simulator
cd frontend && npm install && cd ..

# 开发模式（热重载，浏览器调试入口 http://localhost:34115）
wails dev

# 运行测试
go test ./...              # 后端
cd frontend && npm test    # 前端（Vitest）

# 打包生产应用（输出 build/bin/）
wails build
```

## 发布

推送 `v*` 标签即触发 GitHub Actions 三平台构建并自动创建 Release（见 [.github/workflows/release.yml](.github/workflows/release.yml)）：

```bash
git tag v0.1.0
git push origin v0.1.0
```

产物：macOS universal `.app.zip` · Windows `.exe` + NSIS 安装包 · Linux 可执行文件。

## 项目结构

| 路径 | 说明 |
| --- | --- |
| main.go / app.go | Wails 入口与服务装配 |
| bridge/ | 前端绑定层：连接、报文、控制台、解析、服务端、扩展包、轨迹 + 事件转发 |
| internal/engine/ | 协议客户端引擎：状态机、心跳、周期上报、下行命令处理 |
| internal/servermode/ | 平台侧接收服务：accept 循环、会话注册表、自动应答、环形缓冲 |
| internal/schema/ | 2016 / 2025 双版本报文组定义与组装（golden 测试覆盖） |
| internal/parser/ | hex 帧 → 字段级解析与有序中文树 |
| internal/ext/ | 扩展包：加载 / 校验 / 编码 / 编译 |
| internal/track/ | 轨迹文件解析（GPX / Excel / CSV） |
| internal/framing/ | 帧读取器（收发共用，含超长防护） |
| internal/store/ | 配置 JSON 持久化 |
| internal/tlsconf/ | TLS 配置 |
| frontend/src/ | Vue 前端（客户端模拟 / 报文解析 / 服务端模式 / 扩展包 / 设置） |
| .github/workflows/ | release：推送 tag 触发三平台构建与发布 |

## 技术栈

| 端 | 技术 |
| --- | --- |
| 后端 | Go 1.25 · Wails v2 |
| 前端 | Vue 3 · TypeScript · Ant Design Vue · Vite |
| 协议库 | [github.com/sunsky74/gb32960](https://github.com/sunsky74/gb32960)（帧编解码 / 数据模型） |
| 测试 | Go testing · Vitest + @vue/test-utils |

## 文档

- [扩展包编写指南](docs/extpack-guide.md)——面向实施人员的 JSON 扩展包编写说明
- [待办与设计草案](docs/backlog.md)——未排期功能设计与未来工作
