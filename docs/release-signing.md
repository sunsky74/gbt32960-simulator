# 发布签名与校验(更新包完整性)

> 适用:推送 `v*` 标签发布新版后,为 Release 补签 `SHA256SUMS`;更新客户端以内嵌公钥验签后,才信任校验和。
> 相关:设计文档 §5.3/§5.7;工具 `tools/sign-release`;公钥 `internal/updater/signing_pubkey.go`;ADR-0001。

## 1. 密钥托管(一次性,已完成)

- 密钥对由 `tools/sign-release -gen` 生成;**私钥离线保管**(密码管理器),绝不入库、绝不进 CI、绝不打印。
- 记录:keyid `91fe6136`;生成时间 2026-09-15;保管方式:用户密码管理器。
- **丢失后果**:更新链永久断裂(在用版本拒绝一切更新)——必须多处备份。
- **轮换**:新钥随新版本发布前,`signing_pubkey.go` 同时保留旧钥与新钥条目;待旧版用户升级充分后再移除旧钥。

## 2. 每次发布流程(CI 创建 Release 之后)

1. 在 GitHub Release 页下载 `SHA256SUMS`(CI 生成,覆盖全部资产、排除自身)与全部资产文件到临时目录。
2. 核对(签名前置步骤,防“盲签”):本地重算 `go run ./tools/sign-release -sums -dir <临时目录> -out /tmp/SHA256SUMS.check`,与第 1 步的 `SHA256SUMS` 逐行比对——不一致必须停止签名并排查(历史版本无 CI 版 sums 时,以本地重算结果为准并记录)。
3. 本地签名:`go run ./tools/sign-release -sign -key <私钥路径> -in SHA256SUMS -out SHA256SUMS.sig`
4. 将 `SHA256SUMS.sig` 上传到该 Release(网页拖拽,或 `gh release upload <tag> SHA256SUMS.sig`)。
5. 自检:文件内容为单行 `<keyid> <base64>`;keyid 与 `signing_pubkey.go` 公钥一致。
   (P4 启用不可变发布后,流程改为 draft → attach → publish,签名在上传阶段完成)

## 3. 本地演练(可选)

```bash
go run ./tools/sign-release -sums -dir ./build/bin -out SHA256SUMS
go run ./tools/sign-release -sign -key <私钥路径> -in SHA256SUMS -out SHA256SUMS.sig
```

## 4. 撤回速记

坏版本默认**向前修复**;确需撤回:改回 prerelease(不可变发布前)或删除 Release(不可变后 tag 名不可复用),随后发布修复版。
