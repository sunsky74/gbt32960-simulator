# 更新功能采用自研薄层与统一 helper 替换

状态:accepted(2026-09-14)

Wails v2 没有官方更新器(官方明确 v3 才提供,而 v3 仍是 beta),社区也没有成熟可依赖的 Wails 更新库;因此更新链路(检查 GitHub Releases → 下载 → SHA256SUMS 校验 → 替换 → 重启)由项目自研:`internal/updater` 仅用标准库,`go.mod` 零变化。三平台统一采用 helper 模式——应用退出后由短驻进程等待父进程退出、执行替换,成功后拉起新版本,失败则回滚并拉起旧版本;macOS 以整体替换 `.app`(而非只换内部二进制)保留 bundle 完整性,为未来签名/公证留路。校验止于 Releases 上的 SHA256SUMS(fail-closed),Ed25519 签名暂缓、列入预留演进。

## Considered Options

- **go-selfupdate / minio/selfupdate**:前者的资产命名规则与现有 Release 资产不兼容,且不处理 `.app` 整体替换;后者只提供替换原语——算法已借鉴(rename-aside + 回滚),但不为此引入依赖。
- **迁移 Wails v3 使用内建 `app.Updater`**:能力目标一致,但 v3 仍处 beta(2026-09 为 beta.21),迁移构建链与后端 bootstrap 的成本远超收益;当前方案与之同构,未来迁移不返工。
- **其他社区 Wails 更新项目**:均为 0~15 star 的实验或停更项目,无可依赖者。

## Consequences

- 更新逻辑(含三平台替换、回滚)由项目自行维护与测试——这是零依赖与完全可控的代价。
- macOS 整体替换 `.app` 使未来接入开发者签名/公证时无需重做替换机制。
- Ed25519 校验可后续追加,不影响现有发布链与客户端结构。
