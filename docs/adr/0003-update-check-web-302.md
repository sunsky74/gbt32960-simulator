# 更新检查通道切换为 Releases 网页 302 路由

状态:accepted(2026-09-17)

更新检查原走 `GET https://api.github.com/repos/sunsky74/gbt32960-simulator/releases/latest`:未认证 REST API,限额 60 次/时/IP,而模拟器用户集中于中国大陆,出口多为 NAT / 代理共享,额度易被他人打满。实测同一出口 IP:API 配额全量耗尽(60/60,`releases/latest` 返回 403)时,网页路由 `GET https://github.com/sunsky74/gbt32960-simulator/releases/latest` 的 302 响应(`Location: .../releases/tag/<tag>`)、`releases.atom` 与 `/releases/latest/download/<asset>` 均正常;网页 `latest` 语义经抽样(kubernetes/home-assistant)验证与 API 一致——解析到最新的非 prerelease 版本。据此检查端点切换为网页 302 路由:直接读 `Location` 解析 tag,不再消耗 API 配额(该路由无 60 次/时限额,403/429 仅来自滥用限流);资产下载 URL 也不再取 API 的 `browser_download_url`,改按 tag 与既有命名契约确定性拼接 `https://github.com/{repo}/releases/download/{tag}/{name}`。契约随之收窄:`UpdateInfo` 移除 `Notes` / `PublishedAt` / `AssetSize`,关于面板以“查看发布说明”外链替代应用内展示,下载白名单移除 `api.github.com`。安全兜底不变:`IsNewer` 对任何非 `vX.Y.Z` tag fail-closed(网页路由即使解析到 prerelease tag 也不会触发更新提示);下载仍由 SHA256SUMS + Ed25519 签名链保护。

## Considered Options

- **静态 `version.json`(raw.githubusercontent.com / jsDelivr)**:版本事实需在发布流程之外多维护一份,存在漂移风险(漏同步即静默失更);且 raw.githubusercontent.com 对中国大陆用户不可达,jsDelivr 稳定性不可控。弃。
- **Serverless 代理中转 API 请求**:引入新基础设施、新域名与新白名单 host,自增单点故障;本项目量级不足以摊销这些成本。弃。

## Consequences

- 应用内不再展示发布说明、发布时间与资产大小——信息未丢失,但查看发布说明需跳出应用到 GitHub 发布页。
- 资产存在性不再由清单预检:缺失资产表现为下载 404;发布流程 `fail_on_unmatched_files: true` 已阻止半成品 Release,风险受限。
- 检查链路解析不到 `vX.Y.Z` tag(含 GitHub 页面结构调整)时 fail-closed,只会漏报不会误报;必要时可回退 API 或另选通道。
- 下载白名单移除 `api.github.com`(已无任何链路使用 API),白名单面收窄。
- SHA256SUMS + Ed25519 验签、helper 替换与回滚、跳过版本等既有机制不变。
