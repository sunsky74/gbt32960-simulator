# 应用内更新 · Phase 2「下载与校验」实施计划

> **For agentic workers:** Recommended execution: use superpowers:ltdd for Quality-LTDD(推荐)。Alternatives: superpowers:subagent-driven-development(轻量)或 superpowers:executing-plans(内联检查点)。Steps use checkbox (`- [ ]`) syntax for tracking。

**Goal:** 交付“下载与校验”闭环:发现新版本后可带进度下载产物(可取消、120s 停滞防护)、经 **Ed25519 签名(离线密钥)+ SHA256SUMS** 双重校验后落定“更新包已就绪”;发布链补齐 SHA256SUMS 生成(CI)与离线签名流程(工具 + 文档)。零安装、零替换(P3)。

**Architecture:** `internal/updater` 追加 `verify.go`(验签/校验和解析/哈希)与 `download.go`(流式下载器 + 白名单逐跳 + 停滞检测 + 下载校验编排);`bridge.UpdaterService` 扩展 `DownloadUpdate()/CancelDownload()`(无参,内部状态;`update:progress` 事件);`tools/sign-release` 离线签名工具 + `internal/updater/signing_pubkey.go` 内嵌公钥;前端“关于”面板补下载进度/取消/就绪态。`go.mod` 零变化。

**Tech Stack:** Go 1.25(标准库 `crypto/ed25519`、`crypto/sha256`、`net/http`)/ Wails v2.15 / Vue 3 + Ant Design Vue / vitest + @vue/test-utils / Golang httptest。

> Phase 引用:`docs/superpowers/plans/2026-09-14-updater-master-plan.md`;设计文档 `docs/superpowers/specs/2026-09-14-updater-design.md` §5.3/§5.5/§5.7/§5.8(2026-09-15 P2 修订后,契约不得偏离);审计依据:`.superpowers/sdd/updater-p1/oracle-audit-update-strategy-vs-industry.md` §4 P2 条。

## Global Constraints

- `go.mod` 零变化;不新增第三方依赖(下载/校验/验签全部标准库)
- 不改变既有服务与页面行为;后端 `go vet ./...` + `go test ./... -count=1` + `go test -race ./...`;前端 `npm run lint` + `npm run typecheck` + `npm test -- --run` + `npm run build` 全绿
- 下载仅允许 HTTPS 白名单域名(`api.github.com` / `github.com` / `release-assets.githubusercontent.com` / `objects.githubusercontent.com`),**重定向逐跳校验**;初始 URL 与每个跳转目标均校验
- **校验顺序 = 先验签(`SHA256SUMS.sig`,Ed25519 内嵌公钥)后解析哈希**;任何一步失败 fail-closed:拒绝 + 删除产物 + 中文分类文案
- 私钥离线保管(绝不入库、绝不进 CI、绝不打印);签名步骤为发布者本地手工动作(见 T4 密钥仪式)
- 新增错误文案(错误值即前端展示文案,禁止在 bridge/前端二次改写):
  - 校验类:`发布未附校验信息,已拒绝更新` / `更新包签名校验失败,已拒绝更新` / `更新包校验失败(哈希不匹配),已拒绝更新`
  - 下载类:`已取消下载` / `下载超时(长时间无进展),请重试` / `下载失败,请检查网络` / `下载不完整,请重试` / `写入失败,请检查磁盘空间与权限` / `下载源不在白名单内,已拒绝`
  - 状态类:`请先检查更新` / `下载已在进行中` / `开发构建不参与更新`
- Git:所有操作前缀 `GIT_MASTER=1`;Conventional Commits(中文摘要);逐任务提交;注释/提交信息/文档全中文
- `update:progress` 载荷契约:`{phase: "downloading" | "verifying", received, total, percent}`;≥100ms 节流(首次与末次必发)

## Phase Final Acceptance Checklist (Refined from Spec) - MUST

- [ ] [PAC-1] (Source: Current Requirement Flow;← Global AC-4) 下载:进度事件驱动(≥100ms 节流、首末必发),支持取消(取消后清理未完成产物),无字节进展 120s 判失败,失败可重试。
  Refinement: `go test ./internal/updater/ -run 'TestFetch' -v` 全过——节流用例(注入 `ProgressInterval=1h`、分块响应 → 事件数 == 2(首+末);注入 `ProgressInterval=0` → 事件数 > 2);停滞用例(注入 `StallTimeout=80ms`、服务端长时间无字节 → `ErrStalled` 且判定及时(<300ms);持续滴字节 → 不误杀);取消用例(ctx 取消 → `ErrCanceled`);截断用例(声明 100 字节实发 50 → `ErrTruncated`)。桥接:`go test ./bridge/ -run TestUpdaterServiceDownloadCancel -v` 通过——进行中 `CancelDownload()` → `DownloadUpdate` 返回 `已取消下载`,目录无 `*.part` 残留。手工:真机下载大资产时进度条推进、点“取消下载”立即停止且无残留(演练记录留存,见 T6)。
- [ ] [PAC-2] (Source: Current Requirement Flow;← Global AC-5) 校验 fail-closed 全矩阵:验签先于解析;缺 SHA256SUMS / 缺签名 / 未知 keyid / 签名被篡改 / 哈希不匹配 → 一律拒绝并删除产物;有效签名 + 匹配哈希 → 落定。
  Refinement: `go test ./internal/updater/ -run 'TestVerify|TestParseChecksums|TestHashFile|TestDownloadReleaseArtifact' -v` 全过(篡改 sums、错 keyid、garbage base64、长度不符、空签名全矩阵);`go test ./bridge/ -run TestUpdaterServiceDownloadAndVerify -v` 通过(测试密钥注入;落定文件存在、SHA256 等于预期、事件含 `verifying` 相位);手工 v0.1.0 真机(发布未附 SHA256SUMS)→ 文案 `发布未附校验信息,已拒绝更新`,缓存目录无产物残留(证据留存)。
- [ ] [PAC-3] (Source: New Architecture Enablement;← Global AC-11) 白名单逐跳:`https` + 4 域名放行、其余拒绝;初始 URL 与重定向每一跳均校验;真实 Release 302 链(github.com → release-assets.githubusercontent.com)通过;`go.mod` 零变化。
  Refinement: `go test ./internal/updater/ -run TestCheckDownloadURL -v`(矩阵:`https://github.com/...` 允 / `http://...` 拒 / `https://evil.com` 拒 / `https://github.com.evil.com` 拒 / `api.github.com`、`objects.githubusercontent.com`、`release-assets.githubusercontent.com` 允);`UPDATER_INTEGRATION=1 go test ./internal/updater/ -run TestIntegrationReleaseChain -v` 通过(真实 GET v0.1.0 资产前 64KiB,记录跳转 host 链:全部 ∈ 白名单 且 ≥2 个 host);`git diff --stat $(git merge-base HEAD origin/main)..HEAD -- go.mod go.sum` 输出为空。
- [ ] [PAC-4] (Source: Development Architecture;← Global AC-9/15) 绑定面与事件契约:`UpdaterService` 导出恰 4 方法(`CurrentVersion`/`CheckUpdate`/`DownloadUpdate`/`CancelDownload`,均无参数,不暴露 URL/路径);`update:progress` 登记于 `WailsEventMap`;启动清理更新缓存(不跨会话复用)。
  Refinement: `grep -n "func (s \*UpdaterService)" bridge/updater_service.go` 恰 4 条且无参数;`grep -n "'update:progress':" frontend/src/api/events.ts` 非空;`go test ./bridge/ -run TestUpdaterServiceCleanupCache -v`(造缓存文件 → `cleanupCache()` → 目录消失);`grep -n "WireUpdaterStartup" app.go bridge/wiring.go` 各 1 条;`go test ./bridge/ -run 'TestUpdaterServiceCheckUpdate|TestUpdaterServiceDevSkipsNetwork' -v` 仍全过(检查链路回归)。
- [ ] [PAC-5] (Source: Overall Business Flow;← Global AC-12, P2 部分) 发布链:release.yml 生成 SHA256SUMS(排除自身、覆盖 4 资产);离线签名工具 gen→sign→verify 往返通过;公钥已嵌入(signing_pubkey 测试断言非空);`docs/release-signing.md` 完整(仪式/备份/签名步骤/轮换/丢失预案)。
  Refinement: `grep -n "SHA256SUMS" .github/workflows/release.yml` 显示生成步骤;`go test ./tools/sign-release/ -v`(roundtrip:临时目录 `-gen` → `-sign` → `updater.ParseSignature`+`ed25519.Verify` 通过;`-sums` 排除 `SHA256SUMS` 与 `*.sig`;私钥 0600);`go test ./internal/updater/ -run TestEmbeddedKeys -v` 通过(恰 1 个公钥且可解析);本地对 `build/bin` 产物跑一遍 `-sums`(输出覆盖 4 资产、与 CI 步骤一致)。增强证据(推荐、不阻塞):对 v0.1.0 Release 补传 SHA256SUMS + SHA256SUMS.sig 后,真机 happy path 落定“更新包已就绪”。
- [ ] [PAC-6] (Source: Existing Architecture Fit;← Global AC-10) 回归与纪律:后端三件套 + 前端四件套全绿;`go.mod` 零变化;既有功能逐字节不变。
  Refinement: `go vet ./... && go test ./... -count=1 && go test -race ./...` 全 exit 0;`cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build` 全 exit 0;`git diff --stat $(git merge-base HEAD origin/main)..HEAD -- go.mod go.sum` 为空。

---

### Task 1: `internal/updater/verify.go` 校验核心(签名 + 校验和 + 哈希)

**Level:** L2
**Level Rationale:** 纯函数、无副作用、无接线;但它是 fail-closed 安全语义的基准,测试矩阵要求全。
**Linked Acceptance Items:** PAC-2
**Task Gate:** task reviewer + focused checks(本任务单测矩阵)

**Files:**
- Create: `internal/updater/verify.go`
- Create: `internal/updater/verify_test.go`
- Create: `internal/updater/signing_pubkey.go`(公钥表占位;真实公钥由 T4 密钥仪式填入)

**Interfaces:**
- Consumes: 标准库 `crypto/ed25519`、`crypto/sha256`。
- Produces: `KeyID(pub ed25519.PublicKey) string`;`ParseSignature(raw []byte) (string, []byte, error)`;`VerifySumSignature(sums, sigRaw []byte, keys map[string]ed25519.PublicKey) error`;`ParseChecksums(r io.Reader) (map[string]string, error)`;`HashFile(path string) (string, error)`;错误值 `ErrChecksumsMissing` / `ErrSigVerify` / `ErrHashMismatch`(T2/T3/T4 依赖)。

- [ ] **Step 1: 写失败测试(完整矩阵)**

创建 `internal/updater/verify_test.go`:

```go
package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- 测试助手(同包共享;download_test.go 复用,勿重复声明)----

func newTestKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// signLine 生成契约签名行:`<keyid> <base64(sig)>\n`
func signLine(t *testing.T, priv ed25519.PrivateKey, data []byte) []byte {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)
	return []byte(KeyID(pub) + " " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
}

// sha256Hex 独立实现(与 HashFile 同算法,防同源错误)。
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---- KeyID ----

func TestKeyID(t *testing.T) {
	pub, _ := newTestKey(t)
	id := KeyID(pub)
	if len(id) != 8 {
		t.Fatalf("keyid 长度 = %d, want 8", len(id))
	}
	if id != KeyID(pub) {
		t.Fatal("keyid 不稳定")
	}
	pub2, _ := newTestKey(t)
	if !bytes.Equal(pub, pub2) && KeyID(pub2) == id {
		t.Fatal("不同公钥 keyid 不应相同")
	}
}

// ---- ParseSignature + VerifySumSignature ----

func TestVerifySumSignature(t *testing.T) {
	pub, priv := newTestKey(t)
	_, otherPriv := newTestKey(t)
	sums := []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  a.bin\n")
	valid := signLine(t, priv, sums)
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}

	if err := VerifySumSignature(sums, valid, keys); err != nil {
		t.Fatalf("有效签名被拒: %v", err)
	}

	// 篡改 sums 1 字节
	tampered := append([]byte{}, sums...)
	tampered[0] = 'f'
	if !errors.Is(VerifySumSignature(tampered, valid, keys), ErrSigVerify) {
		t.Fatal("篡改后的内容应验签失败")
	}

	// 未知 keyid(空公钥表)
	if !errors.Is(VerifySumSignature(sums, valid, map[string]ed25519.PublicKey{}), ErrSigVerify) {
		t.Fatal("空公钥表应验签失败")
	}

	// 用另一个私钥签名(公钥表未收录)
	foreign := signLine(t, otherPriv, sums)
	if !errors.Is(VerifySumSignature(sums, foreign, keys), ErrSigVerify) {
		t.Fatal("未收录公钥的签名应失败")
	}

	// 各类非法签名文件
	for name, raw := range map[string][]byte{
		"garbage base64": []byte("deadbeef !!!not-base64!!!"),
		"长度不符":          []byte("deadbeef YWJj"), // "abc" = 3 字节
		"单字段":           []byte("deadbeef"),
		"空内容":           []byte(""),
		"多字段":           []byte("a b c"),
	} {
		if !errors.Is(VerifySumSignature(sums, raw, keys), ErrSigVerify) {
			t.Fatalf("%s 应验签失败", name)
		}
	}

	// 多公钥表命中(keyid 配对,轮换场景)
	keys2 := map[string]ed25519.PublicKey{
		KeyID(pub):                                    pub,
		KeyID(otherPriv.Public().(ed25519.PublicKey)): otherPriv.Public().(ed25519.PublicKey),
	}
	if err := VerifySumSignature(sums, valid, keys2); err != nil {
		t.Fatalf("多公钥表命中失败: %v", err)
	}
}

// ---- ParseChecksums ----

func TestParseChecksums(t *testing.T) {
	h1 := strings.Repeat("a", 64)
	h2 := strings.Repeat("B", 64)
	input := h1 + "  file-one.zip\n" + h2 + " *file-two.exe\n\n"
	m, err := ParseChecksums(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if m["file-one.zip"] != h1 {
		t.Fatalf("GNU 格式解析失败: %q", m["file-one.zip"])
	}
	if m["file-two.exe"] != strings.ToLower(h2) {
		t.Fatalf("二进制格式解析失败/未归一化小写: %q", m["file-two.exe"])
	}

	for name, bad := range map[string]string{
		"短 hash": "abc  name\n",
		"非 hex":  strings.Repeat("z", 64) + "  name\n",
		"空内容":    "",
	} {
		if _, err := ParseChecksums(strings.NewReader(bad)); err == nil {
			t.Fatalf("%s 应报错", name)
		}
	}
}

// ---- HashFile ----

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	content := []byte("hello")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := HashFile(p)
	if err != nil || got != sha256Hex(content) {
		t.Fatalf("HashFile = %q, %v; want %q", got, err, sha256Hex(content))
	}
	if _, err := HashFile(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("不存在文件应报错")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/updater/ -run 'TestKeyID|TestVerifySumSignature|TestParseChecksums|TestHashFile' -v
```

预期:编译失败(`KeyID`/`VerifySumSignature`/`ParseChecksums`/`HashFile` 未定义)。

- [ ] **Step 3: 实现 `internal/updater/verify.go`**

```go
package updater

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// 校验类错误(fail-closed;文案即前端展示文案)。
var (
	// ErrChecksumsMissing 发布未附 SHA256SUMS/SHA256SUMS.sig,或校验和不可解析。
	ErrChecksumsMissing = errors.New("发布未附校验信息,已拒绝更新")
	// ErrSigVerify 签名缺失/无法解析/未知 keyid/验签失败。
	ErrSigVerify = errors.New("更新包签名校验失败,已拒绝更新")
	// ErrHashMismatch 产物哈希与校验和不匹配。
	ErrHashMismatch = errors.New("更新包校验失败(哈希不匹配),已拒绝更新")
)

// KeyID 公钥标识:SHA-256(pub) 前 4 字节十六进制(8 字符);签名文件与公钥表以此配对。
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:4])
}

// ParseSignature 解析签名文件(单行契约:`<keyid> <base64(64 字节签名)>`)。
// 容忍首尾空白与结尾换行;格式/长度不符一律 error。
func ParseSignature(raw []byte) (keyID string, sig []byte, err error) {
	parts := strings.Fields(string(raw))
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("签名格式非法")
	}
	sig, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return "", nil, fmt.Errorf("签名内容非法")
	}
	return parts[0], sig, nil
}

// VerifySumSignature 校验 SHA256SUMS 全文的 Ed25519 签名(fail-closed)。
// keys 为 keyid→公钥表(轮换留缝:可同时容纳新旧公钥)。
func VerifySumSignature(sums, sigRaw []byte, keys map[string]ed25519.PublicKey) error {
	keyID, sig, err := ParseSignature(sigRaw)
	if err != nil {
		return ErrSigVerify
	}
	pub, ok := keys[keyID]
	if !ok || !ed25519.Verify(pub, sums, sig) {
		return ErrSigVerify
	}
	return nil
}

// ParseChecksums 解析 SHA256SUMS:兼容 `hash  name`(GNU)与 `hash *name`(二进制模式);
// 返回 name→小写 hash。空行跳过;非法行整体报错(fail-closed)。
func ParseChecksums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("校验和行非法: %q", line)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("校验和行非法: %q", line)
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("校验和为空")
	}
	return out, nil
}

// HashFile 流式计算文件 SHA-256(小写十六进制)。
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

同时创建 `internal/updater/signing_pubkey.go`(占位;T4 密钥仪式填入真实公钥):

```go
package updater

import (
	"crypto/ed25519"
	"encoding/base64"
)

// signingPubKeyB64 发布签名公钥(Ed25519,std base64;由 tools/sign-release -gen 生成)。
// 私钥离线保管、绝不入库;轮换:保留旧钥条目直至所有在用版本都升级到含新钥的版本。
const signingPubKeyB64 = "" // 由 T4 密钥仪式填入

// EmbeddedKeys 返回 keyid→公钥表;未配置/解析失败时返回空表(下载校验 fail-closed,拒绝一切更新)。
func EmbeddedKeys() map[string]ed25519.PublicKey {
	out := map[string]ed25519.PublicKey{}
	if signingPubKeyB64 == "" {
		return out
	}
	raw, err := base64.StdEncoding.DecodeString(signingPubKeyB64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return out
	}
	pub := ed25519.PublicKey(raw)
	out[KeyID(pub)] = pub
	return out
}
```

> 注:`signing_pubkey.go` 无独立单测(T4 以 `TestEmbeddedKeys` 作仪式门槛);`EmbeddedKeys()` 行为由 T3 生产路径与 T4 测试覆盖。

- [ ] **Step 4: 运行确认通过 + vet**

```bash
go test ./internal/updater/ -run 'TestKeyID|TestVerifySumSignature|TestParseChecksums|TestHashFile' -count=1 -v
go vet ./internal/updater/
```

预期:全部 PASS、vet 无输出。既有 P1 测试(TestCompareVersions 等)不受影响。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/verify.go internal/updater/verify_test.go
GIT_MASTER=1 git commit -m "feat(updater): 校验核心——SHA256SUMS 解析、Ed25519 验签与哈希(fail-closed)"
```

---

### Task 2: `internal/updater/download.go` 下载器(进度/停滞/白名单/编排)

**Level:** L3
**Level Rationale:** 网络 IO 核心路径;承载 AC-4(进度/取消/停滞)、AC-11(逐跳白名单)与 AC-5 编排语义,跨模块被 bridge 直接依赖。
**Linked Acceptance Items:** PAC-1、PAC-2、PAC-3
**Task Gate:** task reviewer + linked AC

**Files:**
- Create: `internal/updater/download.go`
- Create: `internal/updater/download_test.go`
- Modify: `internal/updater/release.go`(仅抽取 `userAgent` 帮助函数,行为不变)

**Interfaces:**
- Consumes: T1 的 `VerifySumSignature`/`ParseChecksums`/`HashFile`;`MatchAsset`;`Release`/`Asset`。
- Produces: `Downloader`(字段 `HTTP`/`ProgressInterval`/`StallTimeout`/`CheckURL` 可注入);`NewDownloader(version string) *Downloader`;`(d *Downloader) Fetch(ctx, rawURL, dest, onProgress) (int64, error)`;`(d *Downloader) DownloadReleaseArtifact(ctx, rel, goos, goarch, dir, keys, onProgress) (Artifact, error)`;`Artifact`;`DownloadResult`(bridge 返回值契约);`Progress{Phase,Received,Total}` + `Percent()`;`CheckDownloadURL`/`AllowedHosts`;`ReleaseDir(tag)`/`CleanupCache()`;错误值(Global Constraints 清单)。T3/T5/T6 依赖。

- [ ] **Step 1: 写失败测试(完整矩阵)**

创建 `internal/updater/download_test.go`:

```go
package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- CheckDownloadURL 矩阵 ----

func TestCheckDownloadURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://api.github.com/repos/x/y", true},
		{"https://github.com/sunsky74/gbt32960-simulator/releases/download/v0.1.0/a.zip", true},
		{"https://release-assets.githubusercontent.com/github-production-release-asset/1/2", true},
		{"https://objects.githubusercontent.com/foo", true},
		{"http://github.com/foo", false},
		{"https://evil.com/foo", false},
		{"https://github.com.evil.com/foo", false},
	}
	for _, c := range cases {
		u, err := url.Parse(c.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := CheckDownloadURL(u) == nil; got != c.want {
			t.Errorf("CheckDownloadURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

// ---- Fetch:进度节流 / 停滞 / 取消 / 截断 ----

func newTestDownloader() *Downloader {
	d := NewDownloader("dev")
	// 测试放行 localhost(白名单逻辑已由 TestCheckDownloadURL 独立覆盖)
	d.CheckURL = func(*url.URL) error { return nil }
	d.HTTP = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return d.CheckURL(req.URL) },
	}
	return d
}

func TestFetchProgressThrottle(t *testing.T) {
	payload := strings.Repeat("x", 64*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		for i := 0; i < 4; i++ {
			_, _ = io.WriteString(w, payload[i*len(payload)/4:(i+1)*len(payload)/4])
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	t.Run("节流拉满时仅首末两发", func(t *testing.T) {
		d := newTestDownloader()
		d.ProgressInterval = time.Hour
		var got []Progress
		if _, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), func(p Progress) {
			got = append(got, p)
		}); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Received != 0 || got[len(got)-1].Received != int64(len(payload)) {
			t.Fatalf("节流事件异常: %+v", got)
		}
		if got[len(got)-1].Percent() != 100 {
			t.Fatalf("末次 Percent = %d, want 100", got[len(got)-1].Percent())
		}
	})

	t.Run("零间隔时每块都发", func(t *testing.T) {
		d := newTestDownloader()
		d.ProgressInterval = 0
		n := 0
		if _, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), func(Progress) {
			n++
		}); err != nil {
			t.Fatal(err)
		}
		if n <= 2 {
			t.Fatalf("事件数 = %d, want > 2", n)
		}
	})
}

func TestFetchStall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush() // 响应头先到、随后长时间无字节
		time.Sleep(400 * time.Millisecond)
	}))
	defer srv.Close()

	d := newTestDownloader()
	d.StallTimeout = 80 * time.Millisecond
	start := time.Now()
	_, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("停滞未及时判定: %v", time.Since(start))
	}
}

func TestFetchStallTimerResetsOnBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 5; i++ {
			_, _ = io.WriteString(w, "tick")
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond) // 每次均 < StallTimeout,持续有进展
		}
	}))
	defer srv.Close()

	d := newTestDownloader()
	d.StallTimeout = 80 * time.Millisecond
	if _, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil); err != nil {
		t.Fatalf("持续有字节仍被判停滞: %v", err)
	}
}

func TestFetchCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-block // 挂起直至测试结束
	}))
	defer func() { close(block); srv.Close() }()

	d := newTestDownloader()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := d.Fetch(ctx, srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("err = %v, want ErrCanceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消未生效")
	}
}

func TestFetchTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, strings.Repeat("y", 50)) // 实发少于声明
	}))
	defer srv.Close()

	d := newTestDownloader()
	_, err := d.Fetch(context.Background(), srv.URL+"/a", filepath.Join(t.TempDir(), "a.bin"), nil)
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated", err)
	}
}

// ---- DownloadReleaseArtifact:编排(验签→解析→下载→哈希) ----

// releaseFixture 构造三平台资产(+ 可选校验资产)的发布信息;资产 URL 指向 srvURL。
func releaseFixture(srvURL string, contentLen int64, withSums bool) *Release {
	rel := &Release{TagName: "v9.9.9"}
	rel.Assets = append(rel.Assets,
		Asset{Name: "gbt32960-simulator", Size: contentLen, URL: srvURL + "/linux"},
		Asset{Name: "gbt32960-simulator.app.zip", Size: contentLen, URL: srvURL + "/darwin"},
		Asset{Name: "gbt32960-simulator.exe", Size: contentLen, URL: srvURL + "/win"},
	)
	if withSums {
		rel.Assets = append(rel.Assets,
			Asset{Name: "SHA256SUMS", Size: 200, URL: srvURL + "/sums"},
			Asset{Name: "SHA256SUMS.sig", Size: 100, URL: srvURL + "/sig"},
		)
	}
	return rel
}

// sumsFor 生成覆盖三平台资产名的校验和文本(同一内容)。
func sumsFor(content []byte) string {
	var sb strings.Builder
	for _, n := range []string{"gbt32960-simulator", "gbt32960-simulator.app.zip", "gbt32960-simulator.exe"} {
		fmt.Fprintf(&sb, "%s  %s\n", sha256Hex(content), n)
	}
	return sb.String()
}

func TestDownloadReleaseArtifactHappyPath(t *testing.T) {
	content := []byte("artifact-bytes-for-test")
	pub, priv := newTestKey(t)
	sums := sumsFor(content)
	sig := signLine(t, priv, []byte(sums))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sums":
			_, _ = io.WriteString(w, sums)
		case "/sig":
			_, _ = w.Write(sig)
		default:
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	d := newTestDownloader()
	rel := releaseFixture(srv.URL, int64(len(content)), true)
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}
	var phases []string
	art, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, func(p Progress) {
		phases = append(phases, p.Phase)
	})
	if err != nil {
		t.Fatal(err)
	}
	if art.SHA256 != sha256Hex(content) {
		t.Fatalf("SHA256 = %s", art.SHA256)
	}
	if _, err := os.Stat(art.Path); err != nil {
		t.Fatalf("落定文件不存在: %v", err)
	}
	hasVerifying := false
	for _, p := range phases {
		if p == "verifying" {
			hasVerifying = true
		}
	}
	if !hasVerifying {
		t.Fatalf("缺 verifying 相位: %v", phases)
	}
}

func TestDownloadReleaseArtifactRejections(t *testing.T) {
	content := []byte("artifact-bytes")
	pub, priv := newTestKey(t)
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}
	goodSums := sumsFor(content)

	newSrv := func(sums, sig []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/sums":
				_, _ = w.Write(sums)
			case "/sig":
				_, _ = w.Write(sig)
			default:
				_, _ = w.Write(content)
			}
		}))
	}

	cases := []struct {
		name    string
		sums    []byte
		sig     []byte
		wantErr error
	}{
		{
			"签名被篡改",
			[]byte(goodSums),
			func() []byte { // 解码→翻位→重编码:确定性篡改签名内容
				line := strings.TrimSpace(string(signLine(t, priv, []byte(goodSums))))
				parts := strings.SplitN(line, " ", 2)
				raw, _ := base64.StdEncoding.DecodeString(parts[1])
				raw[0] ^= 0xFF
				return []byte(parts[0] + " " + base64.StdEncoding.EncodeToString(raw) + "\n")
			}(),
			ErrSigVerify,
		},
		{
			"未知 keyid",
			[]byte(goodSums),
			[]byte("deadbeef " + strings.SplitN(strings.TrimSpace(string(signLine(t, priv, []byte(goodSums)))), " ", 2)[1] + "\n"),
			ErrSigVerify,
		},
		{
			"哈希不匹配",
			[]byte(strings.Replace(goodSums, sha256Hex(content), strings.Repeat("0", 64), 1)),
			nil, // 下面用篡改后的 sums 重新签名
			ErrHashMismatch,
		},
		{
			"缺签名文件",
			[]byte(goodSums),
			[]byte(""),
			ErrSigVerify,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sig := c.sig
			if sig == nil { // 哈希不匹配:用篡改后的 sums 正确签名(签名有效但哈希对不上)
				sig = signLine(t, priv, c.sums)
			}
			srv := newSrv(c.sums, sig)
			defer srv.Close()
			d := newTestDownloader()
			rel := releaseFixture(srv.URL, int64(len(content)), true)
			_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, nil)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}

	t.Run("缺失全部校验资产", func(t *testing.T) {
		srv := newSrv([]byte(goodSums), signLine(t, priv, []byte(goodSums)))
		defer srv.Close()
		d := newTestDownloader()
		rel := releaseFixture(srv.URL, int64(len(content)), false) // 无 SHA256SUMS/sig
		_, err := d.DownloadReleaseArtifact(context.Background(), rel, "darwin", "arm64", t.TempDir(), keys, nil)
		if !errors.Is(err, ErrChecksumsMissing) {
			t.Fatalf("err = %v, want ErrChecksumsMissing", err)
		}
	})
}

// ---- 目录与清理 ----

func TestReleaseDirAndCleanup(t *testing.T) {
	dir, err := ReleaseDir("release/v1.2.3") // tag 含斜杠 → 清洗
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(dir); strings.ContainsAny(base, `/\`) {
		t.Fatalf("tag 未被清洗: %s", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CleanupCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("缓存未被清空")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/updater/ -run 'TestCheckDownloadURL|TestFetch|TestDownloadReleaseArtifact|TestReleaseDir' -v
```

预期:编译失败(`Downloader`/`CheckDownloadURL`/`Fetch`/`DownloadReleaseArtifact`/`ReleaseDir`/`CleanupCache` 未定义)。

- [ ] **Step 3: 实现 `internal/updater/download.go`**

```go
package updater

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// 下载类错误(fail-closed;文案即前端展示文案)。
var (
	// ErrCanceled 用户取消或应用退出导致的下载中止(部分文件清理由编排层完成)。
	ErrCanceled = errors.New("已取消下载")
	// ErrStalled 持续 StallTimeout 无字节进展。
	ErrStalled = errors.New("下载超时(长时间无进展),请重试")
	// ErrDownload 网络不可达/HTTP 状态异常。
	ErrDownload = errors.New("下载失败,请检查网络")
	// ErrTruncated 下载字节数与声明大小不符。
	ErrTruncated = errors.New("下载不完整,请重试")
	// ErrDisk 本地写入失败(空间/权限)。
	ErrDisk = errors.New("写入失败,请检查磁盘空间与权限")
)

// AllowedHosts 下载白名单(设计文档 §5.7;release-assets 为实测 302 目标,objects 为历史兼容)。
var AllowedHosts = map[string]bool{
	"api.github.com":                        true,
	"github.com":                            true,
	"release-assets.githubusercontent.com":  true,
	"objects.githubusercontent.com":         true,
}

// CheckDownloadURL 校验单个跳转 URL:必须 HTTPS 且 host 在白名单内(逐跳调用)。
func CheckDownloadURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || !AllowedHosts[u.Hostname()] {
		return errors.New("下载源不在白名单内,已拒绝")
	}
	return nil
}

// Progress 下载进度契约(设计文档 §5.5;phase: downloading|verifying)。
type Progress struct {
	Phase    string
	Received int64
	Total    int64
}

// Percent 百分比(total 未知时返回 0)。
func (p Progress) Percent() int {
	if p.Total <= 0 {
		return 0
	}
	n := int(p.Received * 100 / p.Total)
	if n > 100 {
		n = 100
	}
	return n
}

// Artifact 校验通过的更新产物。
type Artifact struct {
	Path   string
	Name   string
	Tag    string
	SHA256 string
}

// DownloadResult 下载并校验通过的更新产物(服务层返回契约;bridge 直接透出前端)。
type DownloadResult struct {
	Tag       string `json:"tag"`
	AssetName string `json:"assetName"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// Downloader 流式下载器:进度节流、停滞检测、逐跳白名单;全部可注入(测试缝)。
type Downloader struct {
	HTTP             *http.Client
	ProgressInterval time.Duration        // 进度最小间隔(默认 100ms)
	StallTimeout     time.Duration        // 无字节进展超时(默认 120s)
	CheckURL         func(*url.URL) error // 逐跳 URL 校验(默认 CheckDownloadURL)
	ua               string
}

// NewDownloader 生产默认值;HTTP 客户端不设整体超时(由停滞检测兜底)。
func NewDownloader(version string) *Downloader {
	d := &Downloader{
		ProgressInterval: 100 * time.Millisecond,
		StallTimeout:     120 * time.Second,
		CheckURL:         CheckDownloadURL,
		ua:               userAgent(version),
	}
	d.HTTP = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := d.CheckURL(req.URL); err != nil {
				return err
			}
			if len(via) >= 10 {
				return errors.New("重定向次数过多")
			}
			return nil
		},
	}
	return d
}

// Fetch 流式下载 rawURL → dest;onProgress 首次与末次必发,中间按 ProgressInterval 节流。
// 停滞/取消/截断分别返回 ErrStalled/ErrCanceled/ErrTruncated;失败时保留部分文件,由调用方清理。
func (d *Downloader) Fetch(ctx context.Context, rawURL, dest string, onProgress func(Progress)) (int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, ErrDownload
	}
	if err := d.CheckURL(u); err != nil {
		return 0, err // 白名单错误原样透出(安全分类,勿并入 ErrDownload)
	}

	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, ErrDownload
	}
	req.Header.Set("User-Agent", d.ua)
	resp, err := d.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ErrCanceled
		}
		return 0, ErrDownload
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, ErrDownload
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}

	f, err := os.Create(dest)
	if err != nil {
		return 0, ErrDisk
	}
	closeFile := func() error { return f.Close() }

	var stalled atomic.Bool
	timer := time.AfterFunc(d.StallTimeout, func() {
		stalled.Store(true)
		cancel2() // 解除 Read 阻塞
	})
	defer timer.Stop()

	var received int64
	var lastEmit time.Time
	emit := func(force bool) {
		if onProgress == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastEmit) < d.ProgressInterval {
			return
		}
		lastEmit = now
		onProgress(Progress{Phase: "downloading", Received: received, Total: total})
	}
	emit(true)

	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				_ = closeFile()
				return received, ErrDisk
			}
			received += int64(n)
			timer.Reset(d.StallTimeout)
			emit(false)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = closeFile()
			if stalled.Load() {
				return received, ErrStalled
			}
			if ctx.Err() != nil {
				return received, ErrCanceled
			}
			if total > 0 && received < total {
				return received, ErrTruncated
			}
			return received, ErrDownload
		}
	}
	if err := closeFile(); err != nil {
		return received, ErrDisk
	}
	if stalled.Load() {
		return received, ErrStalled
	}
	emit(true)
	if total > 0 && received != total {
		return received, ErrTruncated
	}
	return received, nil
}

// DownloadReleaseArtifact 编排:校验信息先行(下载 sums/sig → 验签 → 解析)→ 下载产物 → 哈希比对 → .part 落定。
// 任一步失败:删除 .part;已下载的 sums/sig 保留(启动清理兜底)。
func (d *Downloader) DownloadReleaseArtifact(ctx context.Context, rel *Release, goos, goarch, dir string, keys map[string]ed25519.PublicKey, onProgress func(Progress)) (Artifact, error) {
	asset, err := MatchAsset(goos, goarch, rel.Assets)
	if err != nil {
		return Artifact{}, err
	}
	sumsAsset, ok := findAsset(rel.Assets, "SHA256SUMS")
	if !ok {
		return Artifact{}, ErrChecksumsMissing
	}
	sigAsset, ok := findAsset(rel.Assets, "SHA256SUMS.sig")
	if !ok {
		return Artifact{}, ErrChecksumsMissing
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Artifact{}, ErrDisk
	}
	sumsPath := filepath.Join(dir, "SHA256SUMS")
	sigPath := filepath.Join(dir, "SHA256SUMS.sig")
	if _, err := d.Fetch(ctx, sumsAsset.URL, sumsPath, nil); err != nil {
		return Artifact{}, err
	}
	if _, err := d.Fetch(ctx, sigAsset.URL, sigPath, nil); err != nil {
		return Artifact{}, err
	}
	sumsBytes, err := os.ReadFile(sumsPath)
	if err != nil {
		return Artifact{}, ErrDisk
	}
	sigBytes, err := os.ReadFile(sigPath)
	if err != nil {
		return Artifact{}, ErrDisk
	}
	// 先验签后解析(fail-closed 顺序契约)
	if err := VerifySumSignature(sumsBytes, sigBytes, keys); err != nil {
		return Artifact{}, err
	}
	checksums, err := ParseChecksums(strings.NewReader(string(sumsBytes)))
	if err != nil {
		return Artifact{}, ErrChecksumsMissing
	}
	want, ok := checksums[asset.Name]
	if !ok {
		return Artifact{}, ErrChecksumsMissing
	}

	part := filepath.Join(dir, asset.Name+".part")
	if _, err := d.Fetch(ctx, asset.URL, part, func(p Progress) {
		if onProgress != nil {
			onProgress(p)
		}
	}); err != nil {
		_ = os.Remove(part)
		return Artifact{}, err
	}
	if onProgress != nil {
		onProgress(Progress{Phase: "verifying", Received: asset.Size, Total: asset.Size})
	}
	got, err := HashFile(part)
	if err != nil {
		_ = os.Remove(part)
		return Artifact{}, ErrDisk
	}
	if got != want {
		_ = os.Remove(part)
		return Artifact{}, ErrHashMismatch
	}
	final := filepath.Join(dir, asset.Name)
	if err := os.Rename(part, final); err != nil {
		_ = os.Remove(part)
		return Artifact{}, ErrDisk
	}
	return Artifact{Path: final, Name: asset.Name, Tag: rel.TagName, SHA256: got}, nil
}

func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// ReleaseDir 更新缓存目录(UserCacheDir/gbt32960-simulator/updates/<safeTag>);tag 做字符白名单清洗。
func ReleaseDir(tag string) (string, error) {
	cd, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cd, "gbt32960-simulator", "updates", safeTag(tag)), nil
}

var tagSanitize = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func safeTag(tag string) string {
	s := tagSanitize.ReplaceAllString(tag, "_")
	if s == "" {
		s = "unknown"
	}
	return s
}

// CleanupCache 清空整个更新缓存目录(启动时调用;不跨会话复用)。
func CleanupCache() error {
	cd, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(cd, "gbt32960-simulator", "updates"))
}
```

同时修改 `internal/updater/release.go`(抽取 UA 帮助函数,`NewClient` 行为不变):

```go
// userAgent 构造 User-Agent(dev/空版本不带版本号)。
func userAgent(version string) string {
	ua := "gbt32960-simulator"
	if version != "" && version != "dev" {
		ua += "/" + version
	}
	return ua
}

// NewClient 默认客户端:官方 API 端点 + 10s 超时;version 进入 User-Agent。
func NewClient(version string) *Client {
	return &Client{
		BaseURL: "https://api.github.com",
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		ua:      userAgent(version),
	}
}
```

- [ ] **Step 4: 运行确认通过 + 全包回归 + race**

```bash
go test ./internal/updater/ -count=1
go vet ./internal/updater/
go test -race ./internal/updater/
```

预期:全部 PASS;vet 无输出;race 干净(`stalled` 使用 atomic;`timer.Reset` 与读循环同 goroutine)。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add internal/updater/download.go internal/updater/download_test.go internal/updater/release.go
GIT_MASTER=1 git commit -m "feat(updater): 下载器——流式进度/120s 停滞/逐跳白名单/SHA256SUMS+Ed25519 编排"
```

---

### Task 3: `bridge.UpdaterService` 下载服务(单飞/取消/事件/启动清理)

**Level:** L3
**Level Rationale:** 绑定面扩展(AC-9)与下载编排状态机(单飞、取消、事件出口),跨包接线(app.go 启动清理经 wiring.go 装配),直接面向前端契约。
**Linked Acceptance Items:** PAC-1、PAC-2、PAC-4
**Task Gate:** task reviewer + linked AC

**Files:**
- Modify: `bridge/updater_service.go`(整体替换为下方完整实现)
- Modify: `bridge/updater_service_test.go`(追加)
- Modify: `bridge/wiring.go`(新增 `WireUpdaterStartup`)
- Modify: `app.go`(startup 调用装配函数)
- Regenerate: `frontend/wailsjs/**`

**Interfaces:**
- Consumes: T1/T2 的 `updater.Downloader`/`DownloadReleaseArtifact`/`Progress`/`ReleaseDir`/`CleanupCache`/`EmbeddedKeys`/`DownloadResult`;wails `runtime.EventsEmit`。
- Produces: `UpdaterService.DownloadUpdate() (updater.DownloadResult, error)`;`CancelDownload()`(无参数,内部状态);事件 `update:progress`(载荷 `progressDTO`);`bridge.WireUpdaterStartup`。T5/T6 依赖。

- [ ] **Step 1: 写失败测试(追加到 `bridge/updater_service_test.go`)**

```go
// ==== Phase 2 追加:下载与校验 ====
// 新增导入(与既有 import 合并去重):context、crypto/ed25519、crypto/rand、crypto/sha256、
// encoding/base64、encoding/hex、fmt、io、net/url、os、path/filepath、strings、time、
// "gbt32960-simulator/internal/updater"

func newTestSigning(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func hexSha256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// fixtureSums 生成覆盖三平台资产名的校验和文本(同一内容)。
func fixtureSums(content []byte) []byte {
	var sb strings.Builder
	for _, n := range []string{"gbt32960-simulator", "gbt32960-simulator.app.zip", "gbt32960-simulator.exe"} {
		fmt.Fprintf(&sb, "%s  %s\n", hexSha256(content), n)
	}
	return []byte(sb.String())
}

// sigLineFor 生成契约签名行 `<keyid> <base64(sig)>\n`。
func sigLineFor(t *testing.T, priv ed25519.PrivateKey, data []byte) []byte {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)
	return []byte(updater.KeyID(pub) + " " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
}

// updaterFixtureFor 构造指向 httptest 的 releases/latest 响应(含校验资产)。
func updaterFixtureFor(srvURL string) string {
	return fmt.Sprintf(`{
  "tag_name": "v0.2.0",
  "body": "更新说明",
  "published_at": "2026-09-14T08:00:00Z",
  "assets": [
    {"name": "gbt32960-simulator", "size": 100, "browser_download_url": "%[1]s/linux"},
    {"name": "gbt32960-simulator.app.zip", "size": 200, "browser_download_url": "%[1]s/darwin"},
    {"name": "gbt32960-simulator.exe", "size": 300, "browser_download_url": "%[1]s/win"},
    {"name": "SHA256SUMS", "size": 300, "browser_download_url": "%[1]s/sums"},
    {"name": "SHA256SUMS.sig", "size": 100, "browser_download_url": "%[1]s/sig"}
  ]
}`, srvURL)
}

func newDownloadTestService(serverURL string) (*UpdaterService, *[]progressDTO) {
	svc := NewUpdaterService("v0.1.0")
	svc.client.BaseURL = serverURL
	svc.downloader.CheckURL = func(*url.URL) error { return nil } // 放行 localhost
	svc.ctx = context.Background()
	events := &[]progressDTO{}
	svc.emit = func(_ context.Context, name string, data ...any) {
		if name != "update:progress" {
			return
		}
		*events = append(*events, data[0].(progressDTO))
	}
	return svc, events
}

func TestUpdaterServiceDownloadAndVerify(t *testing.T) {
	content := []byte("artifact-bytes-for-bridge-test")
	pub, priv := newTestSigning(t)
	sums := fixtureSums(content)
	sig := sigLineFor(t, priv, sums)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			_, _ = io.WriteString(w, updaterFixtureFor(srv.URL))
		case r.URL.Path == "/sums":
			_, _ = w.Write(sums)
		case r.URL.Path == "/sig":
			_, _ = w.Write(sig)
		default:
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()
	t.Cleanup(func() { _ = updater.CleanupCache() })

	svc, events := newDownloadTestService(srv.URL)
	svc.keys = map[string]ed25519.PublicKey{updater.KeyID(pub): pub}

	if _, err := svc.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
	res, err := svc.DownloadUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if res.Tag != "v0.2.0" || res.AssetName == "" || res.SHA256 != hexSha256(content) {
		t.Fatalf("结果异常: %+v", res)
	}
	if len(*events) < 2 {
		t.Fatalf("事件过少: %+v", *events)
	}
	last := (*events)[len(*events)-1]
	if last.Phase != "verifying" || last.Percent != 100 {
		t.Fatalf("末条事件异常: %+v", last)
	}
	dir, _ := updater.ReleaseDir("v0.2.0")
	if _, err := os.Stat(filepath.Join(dir, res.AssetName)); err != nil {
		t.Fatalf("落定文件不存在: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(matches) != 0 {
		t.Fatalf(".part 残留: %v", matches)
	}
}

func TestUpdaterServiceDownloadCancel(t *testing.T) {
	content := []byte("artifact")
	pub, priv := newTestSigning(t)
	sums := fixtureSums(content)
	sig := sigLineFor(t, priv, sums)
	block := make(chan struct{})
	assetStarted := make(chan struct{})

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			_, _ = io.WriteString(w, updaterFixtureFor(srv.URL))
		case r.URL.Path == "/sums":
			_, _ = w.Write(sums)
		case r.URL.Path == "/sig":
			_, _ = w.Write(sig)
		default:
			w.Header().Set("Content-Length", "9999999")
			w.(http.Flusher).Flush()
			close(assetStarted)
			<-block // 挂起直至测试结束
		}
	}))
	defer func() { close(block); srv.Close() }()
	t.Cleanup(func() { _ = updater.CleanupCache() })

	svc, _ := newDownloadTestService(srv.URL)
	svc.keys = map[string]ed25519.PublicKey{updater.KeyID(pub): pub}
	if _, err := svc.CheckUpdate(); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.DownloadUpdate()
		errCh <- err
	}()
	<-assetStarted // 确认已进入资产下载
	svc.CancelDownload()
	select {
	case err := <-errCh:
		if err == nil || err.Error() != "已取消下载" {
			t.Fatalf("err = %v, want 已取消下载", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消未生效")
	}
	dir, _ := updater.ReleaseDir("v0.2.0")
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(matches) != 0 {
		t.Fatalf(".part 残留: %v", matches)
	}
}

func TestUpdaterServiceCleanupCache(t *testing.T) {
	dir, err := updater.ReleaseDir("v0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644)
	svc := NewUpdaterService("v1.0.0")
	svc.cleanupCache()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("缓存未清空")
	}
}

func TestUpdaterServiceDownloadGuards(t *testing.T) {
	svc := NewUpdaterService("v1.0.0")
	if _, err := svc.DownloadUpdate(); err == nil || err.Error() != "请先检查更新" {
		t.Fatalf("未检查应先报错: %v", err)
	}
	dev := NewUpdaterService("dev")
	if _, err := dev.DownloadUpdate(); err == nil || err.Error() != "开发构建不参与更新" {
		t.Fatalf("dev 应拒绝: %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./bridge/ -run 'TestUpdaterServiceDownload|TestUpdaterServiceCleanup' -v
```

预期:编译失败(`DownloadUpdate`/`CancelDownload`/`cleanupCache` 未定义)。

- [ ] **Step 3: 实现(整体替换 `bridge/updater_service.go`)**

```go
package bridge

import (
	"context"
	"crypto/ed25519"
	"errors"
	"runtime"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/updater"
)

// UpdaterService 应用内更新服务:查询最新版本 + 下载校验(Phase 2);替换在后续 Phase 接入。
type UpdaterService struct {
	ctx        context.Context
	version    string
	client     *updater.Client
	downloader *updater.Downloader
	keys       map[string]ed25519.PublicKey

	// emit 前端推送函数,默认 wruntime.EventsEmit;测试可注入以确定性验证。
	emit func(ctx context.Context, eventName string, optionalData ...any)

	mu          sync.Mutex
	lastRelease *updater.Release // 最近一次成功的检查结果(下载依赖其资产直链)
	downloading bool
	cancel      context.CancelFunc
}

// NewUpdaterService 创建更新服务(version 为 ldflags 注入版本,dev 表示本地构建)。
func NewUpdaterService(version string) *UpdaterService {
	return &UpdaterService{
		version:    version,
		client:     updater.NewClient(version),
		downloader: updater.NewDownloader(version),
		keys:       updater.EmbeddedKeys(),
		emit:       wruntime.EventsEmit,
	}
}

// CurrentVersion 当前应用版本(未注入时为 "dev")。
func (s *UpdaterService) CurrentVersion() string {
	return s.version
}

// CheckUpdate 查询最新版本并与当前版本比较;dev/空版本不发请求(直接返回 DevBuild)。
func (s *UpdaterService) CheckUpdate() (updater.UpdateInfo, error) {
	if s.version == "" || s.version == "dev" {
		return updater.UpdateInfo{Current: s.version, DevBuild: true}, nil
	}
	if s.isDownloading() {
		return updater.UpdateInfo{}, errors.New("下载已在进行中")
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rel, err := s.client.LatestRelease(ctx)
	if err != nil {
		return updater.UpdateInfo{}, err
	}
	info := updater.UpdateInfo{
		Current:     s.version,
		Latest:      rel.TagName,
		HasUpdate:   updater.IsNewer(rel.TagName, s.version),
		Notes:       rel.Body,
		PublishedAt: rel.PublishedAt,
	}
	// 资产匹配失败不阻塞检查结果(前端据 AssetName 空值提示"暂不支持自动更新")
	if asset, aerr := updater.MatchAsset(runtime.GOOS, runtime.GOARCH, rel.Assets); aerr == nil {
		info.AssetName = asset.Name
		info.AssetSize = asset.Size
	}
	s.mu.Lock()
	s.lastRelease = rel
	s.mu.Unlock()
	return info, nil
}

// DownloadUpdate 下载并校验最近一次检查到的更新产物。
// 无参数:URL/路径全部取自服务内部状态(绑定面不暴露任意 URL/路径,防注入)。
func (s *UpdaterService) DownloadUpdate() (updater.DownloadResult, error) {
	if s.version == "" || s.version == "dev" {
		return updater.DownloadResult{}, errors.New("开发构建不参与更新")
	}
	s.mu.Lock()
	if s.downloading {
		s.mu.Unlock()
		return updater.DownloadResult{}, errors.New("下载已在进行中")
	}
	rel := s.lastRelease
	s.mu.Unlock()
	if rel == nil {
		return updater.DownloadResult{}, errors.New("请先检查更新")
	}
	asset, err := updater.MatchAsset(runtime.GOOS, runtime.GOARCH, rel.Assets)
	if err != nil {
		return updater.DownloadResult{}, err
	}
	dir, err := updater.ReleaseDir(rel.TagName)
	if err != nil {
		return updater.DownloadResult{}, err
	}

	base := s.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base) // 派生自 wails ctx:shutdown 时自动取消
	defer cancel()

	s.mu.Lock()
	if s.downloading { // 双重检查:并发首触发只放行一个
		s.mu.Unlock()
		return updater.DownloadResult{}, errors.New("下载已在进行中")
	}
	s.downloading = true
	s.cancel = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.downloading = false
		s.cancel = nil
		s.mu.Unlock()
	}()

	art, err := s.downloader.DownloadReleaseArtifact(ctx, rel, runtime.GOOS, runtime.GOARCH, dir, s.keys, s.emitProgress)
	if err != nil {
		return updater.DownloadResult{}, err
	}
	return updater.DownloadResult{Tag: art.Tag, AssetName: art.Name, Size: asset.Size, SHA256: art.SHA256}, nil
}

// CancelDownload 取消进行中的下载(空闲时幂等无操作)。
func (s *UpdaterService) CancelDownload() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// cleanupCache 清空更新缓存(经 bridge.WireUpdaterStartup 在启动时调用;绑定面不暴露)。
func (s *UpdaterService) cleanupCache() {
	_ = updater.CleanupCache()
}

// isDownloading 供检查链路判断互斥(设计文档 §5.7:检查/下载/应用单飞)。
func (s *UpdaterService) isDownloading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.downloading
}

// emitProgress 推送 update:progress(nil-ctx 守卫 + 取消后不推送)。
func (s *UpdaterService) emitProgress(p updater.Progress) {
	ctx := s.ctx
	if ctx == nil || ctx.Err() != nil {
		return
	}
	s.emit(ctx, "update:progress", progressDTO{
		Phase:    p.Phase,
		Received: p.Received,
		Total:    p.Total,
		Percent:  p.Percent(),
	})
}

// progressDTO update:progress 事件载荷(设计文档 §5.5)。
type progressDTO struct {
	Phase    string `json:"phase"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Percent  int    `json:"percent"`
}
```

`bridge/wiring.go` 追加(沿用"内部行为不进绑定面"惯例):

```go
// WireUpdaterStartup 装配更新服务启动行为:清理更新缓存(不跨会话复用;尽力清理,失败忽略)。
func WireUpdaterStartup(u *UpdaterService) {
	u.cleanupCache()
}
```

`app.go` startup 追加一行:

```go
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	bridge.WireContexts(ctx, a.console, a.extsvc, a.sys, a.server, a.track, a.updater)
	bridge.WireUpdaterStartup(a.updater) // 更新缓存不跨会话复用(设计文档 §5.3)
	fwdCtx, cancel := context.WithCancel(ctx)
	a.fwdCancel = cancel
	go a.forwarder.Start(fwdCtx)
}
```

- [ ] **Step 4: 再生成绑定 + 运行确认**

```bash
wails generate module
go test ./bridge/ -run 'TestUpdaterService' -count=1 -v
go vet ./bridge/
grep -n "func (s \*UpdaterService)" bridge/updater_service.go   # 预期恰 4 条且均无参数
grep -n "DownloadUpdate\|CancelDownload" frontend/wailsjs/go/bridge/UpdaterService.d.ts
grep -n "DownloadResult" frontend/wailsjs/go/models.ts
```

预期:测试全 PASS;4 方法;绑定 d.ts 含新方法、models.ts 含 `updater.DownloadResult`。

- [ ] **Step 5: 提交**

```bash
GIT_MASTER=1 git add bridge/updater_service.go bridge/updater_service_test.go bridge/wiring.go app.go frontend/wailsjs
GIT_MASTER=1 git commit -m "feat(bridge): 下载服务——单飞/取消/update:progress 事件/启动缓存清理"
```

---

### Task 4: 发布签名基础设施(工具 + 密钥仪式 + CI + 文档)

**Level:** L3
**Level Rationale:** 供应链安全关键路径(签名格式契约、私钥离线托管);含人工密钥仪式与 CI 发布链改动,跨工具/代码/CI/文档。
**Linked Acceptance Items:** PAC-5
**Task Gate:** task reviewer + linked AC + 密钥仪式证据(公钥已嵌入、测试绿)

**Files:**
- Create: `tools/sign-release/main.go`
- Create: `tools/sign-release/main_test.go`
- Modify: `internal/updater/signing_pubkey.go`(T1 已创建占位;本任务填入真实公钥)
- Create: `internal/updater/signing_pubkey_test.go`
- Create: `docs/release-signing.md`
- Modify: `.github/workflows/release.yml`(release job 追加 SHA256SUMS 生成步骤)
- Modify: `.gitignore`(私钥防误提交)

**Interfaces:**
- Consumes: `updater.KeyID`(签名行 keyid 与嵌入公钥配对)。
- Produces: CLI `go run ./tools/sign-release -gen|-sign|-sums`;签名文件格式契约 `<keyid> <base64(sig)>\n`(与 T1 `ParseSignature` 配对);`updater.EmbeddedKeys()` 返回真实公钥(T1 占位 → 本任务填实)。

- [ ] **Step 1: 写失败测试**

创建 `tools/sign-release/main_test.go`:

```go
package main

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gbt32960-simulator/internal/updater"
)

func TestGenSignVerifyRoundtrip(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.key")
	if err := runGen(keyPath); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("私钥权限 = %o, want 600", st.Mode().Perm())
	}
	if err := runGen(keyPath); err == nil {
		t.Fatal("已存在文件应拒绝覆盖")
	}

	sumsPath := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(sumsPath, []byte("abc  x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "SHA256SUMS.sig")
	if err := runSign(keyPath, sumsPath, sigPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	keyID, sig, err := updater.ParseSignature(raw)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := loadKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if keyID != updater.KeyID(pub) {
		t.Fatalf("keyid 不匹配: %s vs %s", keyID, updater.KeyID(pub))
	}
	sums, _ := os.ReadFile(sumsPath)
	if !ed25519.Verify(pub, sums, sig) {
		t.Fatal("验签失败")
	}
	// 客户端验签路径(与生产同代码)
	keys := map[string]ed25519.PublicKey{updater.KeyID(pub): pub}
	if err := updater.VerifySumSignature(sums, raw, keys); err != nil {
		t.Fatalf("VerifySumSignature 失败: %v", err)
	}
}

func TestRunSumsExclusions(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "gbt32960-simulator.exe"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte("stale"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "SHA256SUMS.sig"), []byte("stale-sig"), 0o644)
	out := filepath.Join(dir, "SHA256SUMS")
	if err := runSums(dir, out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if strings.Contains(string(got), "SHA256SUMS") {
		t.Fatalf("SHA256SUMS 应排除自身与 .sig:\n%s", got)
	}
	if !strings.Contains(string(got), "gbt32960-simulator.exe") {
		t.Fatalf("应覆盖资产:\n%s", got)
	}
}
```

创建 `internal/updater/signing_pubkey_test.go`(密钥仪式前为红——刻意作为仪式完成门槛):

```go
package updater

import "testing"

// TestEmbeddedKeys 嵌入公钥必须恰 1 个且可解析(密钥仪式完成标志;失败=仪式未完成)。
func TestEmbeddedKeys(t *testing.T) {
	keys := EmbeddedKeys()
	if len(keys) != 1 {
		t.Fatalf("嵌入公钥数 = %d, want 1(密钥仪式未完成?)", len(keys))
	}
	for id, pub := range keys {
		if id == "" || len(pub) != 32 {
			t.Fatalf("公钥异常: id=%q len=%d", id, len(pub))
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./tools/sign-release/ -v   # 编译失败:runGen 等未定义
go test ./internal/updater/ -run TestEmbeddedKeys -v   # 失败:嵌入公钥数 = 0
```

- [ ] **Step 3: 实现 `tools/sign-release/main.go`**

```go
// sign-release:发布签名离线工具(仅标准库;不经 CI,私钥绝不入库)。
//
//	go run ./tools/sign-release -gen -out ~/gbt32960-update-signing.key
//	go run ./tools/sign-release -sign -key ~/gbt32960-update-signing.key -in SHA256SUMS -out SHA256SUMS.sig
//	go run ./tools/sign-release -sums -dir ./build/bin -out SHA256SUMS   # 本地演练/补签
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	gen := flag.Bool("gen", false, "生成 Ed25519 密钥对(PKCS#8 PEM;0600)")
	sign := flag.Bool("sign", false, "签名:SHA256SUMS → SHA256SUMS.sig(单行 <keyid> <base64>)")
	sums := flag.Bool("sums", false, "生成 SHA256SUMS(排除自身与 *.sig)")
	out := flag.String("out", "", "输出文件路径")
	key := flag.String("key", "", "私钥文件路径(PKCS#8 PEM)")
	in := flag.String("in", "", "待签名文件路径")
	dir := flag.String("dir", "", "校验和目录")
	flag.Parse()

	var err error
	switch {
	case *gen:
		err = runGen(*out)
	case *sign:
		err = runSign(*key, *in, *out)
	case *sums:
		err = runSums(*dir, *out)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func runGen(out string) error {
	if out == "" {
		return fmt.Errorf("-gen 需要 -out")
	}
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("拒绝覆盖已存在文件: %s", out)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	sum := sha256.Sum256(pub)
	fmt.Println("私钥已写入(0600;请移入密码管理器后删除明文):", out)
	fmt.Println("keyid:", hex.EncodeToString(sum[:4]))
	fmt.Println("公钥(base64,粘贴到 internal/updater/signing_pubkey.go):")
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func loadKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("密钥格式非 PKCS#8 PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("不是 Ed25519 私钥")
	}
	return priv, nil
}

func runSign(keyPath, inPath, outPath string) error {
	if keyPath == "" || inPath == "" || outPath == "" {
		return fmt.Errorf("-sign 需要 -key/-in/-out")
	}
	priv, err := loadKey(keyPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, data)
	pub := priv.Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	line := hex.EncodeToString(sum[:4]) + " " + base64.StdEncoding.EncodeToString(sig) + "\n"
	if err := os.WriteFile(outPath, []byte(line), 0o644); err != nil {
		return err
	}
	fmt.Println("已生成:", outPath)
	return nil
}

func runSums(dir, outPath string) error {
	if dir == "" || outPath == "" {
		return fmt.Errorf("-sums 需要 -dir/-out")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || n == "SHA256SUMS" || strings.HasSuffix(n, ".sig") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, n := range names {
		f, err := os.Open(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Fprintf(&sb, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), n)
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("已生成:", outPath)
	return nil
}
```

- [ ] **Step 4: CI + 防误提交 + 文档**

`.github/workflows/release.yml` 的 `release` job 中,在 `download-artifact` 之后、`softprops/action-gh-release` 之前插入:

```yaml
      - name: 生成 SHA256SUMS(排除自身;客户端契约 `hash  name`)
        run: |
          cd release
          for f in *; do
            [ "$f" = "SHA256SUMS" ] && continue
            sha256sum "$f"
          done > SHA256SUMS
          echo "—— SHA256SUMS ——"
          cat SHA256SUMS
        shell: bash

      # 说明:SHA256SUMS.sig 不在 CI 生成(私钥离线保管);发布者本地签名后上传,见 docs/release-signing.md
```

`.gitignore` 追加:

```gitignore

# 更新签名私钥(离线保管,绝不入库)
update-signing*.key
```

创建 `docs/release-signing.md`:

```markdown
# 发布签名与校验(更新包完整性)

> 适用:推送 `v*` 标签发布新版后,为 Release 补签 `SHA256SUMS`;更新客户端以内嵌公钥验签后,才信任校验和。
> 相关:设计文档 §5.3/§5.7;工具 `tools/sign-release`;公钥 `internal/updater/signing_pubkey.go`;ADR-0001。

## 1. 密钥托管(一次性,已完成)

- 密钥对由 `tools/sign-release -gen` 生成;**私钥离线保管**(密码管理器),绝不入库、绝不进 CI、绝不打印。
- 记录:keyid `________`;生成时间 2026-09-15;保管方式:用户密码管理器。
- **丢失后果**:更新链永久断裂(在用版本拒绝一切更新)——必须多处备份。
- **轮换**:新钥随新版本发布前,`signing_pubkey.go` 同时保留旧钥与新钥条目;待旧版用户升级充分后再移除旧钥。

## 2. 每次发布流程(CI 创建 Release 之后)

1. 在 GitHub Release 页下载 `SHA256SUMS`(CI 生成,覆盖全部资产、排除自身)。
2. 本地签名:`go run ./tools/sign-release -sign -key <私钥路径> -in SHA256SUMS -out SHA256SUMS.sig`
3. 将 `SHA256SUMS.sig` 上传到该 Release(网页拖拽,或 `gh release upload <tag> SHA256SUMS.sig`)。
4. 自检:文件内容为单行 `<keyid> <base64>`;keyid 与 `signing_pubkey.go` 公钥一致。
   (P4 启用不可变发布后,流程改为 draft → attach → publish,签名在上传阶段完成)

## 3. 本地演练(可选)

```bash
go run ./tools/sign-release -sums -dir ./build/bin -out SHA256SUMS
go run ./tools/sign-release -sign -key <私钥路径> -in SHA256SUMS -out SHA256SUMS.sig
```

## 4. 撤回速记

坏版本默认**向前修复**;确需撤回:改回 prerelease(不可变发布前)或删除 Release(不可变后 tag 名不可复用),随后发布修复版。
```

- [ ] **Step 5: 密钥仪式(人工步骤,主控协调)**

```bash
# 1) 生成密钥对(在用户机器、仓库之外;输出仅含「公钥+keyid」,绝不打印私钥)
go run ./tools/sign-release -gen -out ~/gbt32960-update-signing.key

# 2) 将 stdout 的「公钥 base64」粘贴进 internal/updater/signing_pubkey.go 的 signingPubKeyB64 常量
#    将 keyid 填入 docs/release-signing.md §1 记录

# 3) 验证嵌入生效
go test ./internal/updater/ -run TestEmbeddedKeys -v

# 4) 私钥移入用户密码管理器后删除明文;确认工作区无密钥文件
rm ~/gbt32960-update-signing.key
git status --short   # 不得出现任何 *.key
```

> 执行纪律:全程不得 `cat`/打印私钥内容;若用户选择自行执行 `-gen`,同样的嵌入与清理步骤照做。

- [ ] **Step 6: 全量验证 + 提交**

```bash
go test ./tools/sign-release/ -count=1 -v
go test ./internal/updater/ -count=1
go vet ./...
GIT_MASTER=1 git add tools/sign-release internal/updater/signing_pubkey.go internal/updater/signing_pubkey_test.go docs/release-signing.md .github/workflows/release.yml .gitignore
GIT_MASTER=1 git commit -m "feat(release): 更新包 Ed25519 签名基础设施——离线签名工具/嵌入公钥/CI 生成 SHA256SUMS/签名指南"
```

---

### Task 5: 前端下载 UI(事件登记 + 进度/取消/就绪态)

**Level:** L3
**Level Rationale:** 用户可见的下载闭环(PAC-1 的 UI 证据),涉及新事件类型登记、组件状态机扩展、既有测试扩展;跨 api/events 与设置面板。
**Linked Acceptance Items:** PAC-1、PAC-4
**Task Gate:** task reviewer + linked AC

**Files:**
- Modify: `frontend/src/api/events.ts`
- Modify: `frontend/src/components/settings/SettingsAboutPanel.vue`
- Modify: `frontend/src/components/settings/SettingsAboutPanel.test.ts`

**Interfaces:**
- Consumes: T3 的绑定 `UpdaterService.DownloadUpdate()/CancelDownload()` 与事件 `update:progress`;`updater.DownloadResult` 模型(T3 已再生成)。
- Produces: `UpdateProgressEvent` 类型与 `WailsEventMap['update:progress']`;下载态 UI(进度/取消/就绪)。

- [ ] **Step 1: 扩展事件登记(`frontend/src/api/events.ts`)**

在事件载荷类型区追加:

```ts
/** update:progress —— bridge.UpdaterService 下载进度(设计文档 §5.5;Go 端 progressDTO) */
export interface UpdateProgressEvent {
  phase: 'downloading' | 'verifying'
  received: number
  total: number
  percent: number
}
```

在 `WailsEventMap` 中登记:

```ts
export interface WailsEventMap {
  'console:events': ConsoleEvent[]
  'server:status': ServerStatusEvent
  'server:session': ServerSessionEvent
  'server:frame': ServerFrameEvent
  'server:warn': ServerWarnEvent
  'update:progress': UpdateProgressEvent
}
```

- [ ] **Step 2: 扩展组件(`SettingsAboutPanel.vue`)**

`<script setup>` 变更(在既有基础上):

```ts
// 追加导入
import { onWailsEvent, type UpdateProgressEvent } from '../../api/events'

// 追加状态(P1 既有状态保留)
const downloading = ref(false)
const progress = ref<UpdateProgressEvent | null>(null)
const ready = ref<updater.DownloadResult | null>(null)
let offProgress: (() => void) | null = null

// check() 中追加一行:ready.value = null(重新检查时复位)

async function download() {
  downloading.value = true
  progress.value = null
  offProgress = onWailsEvent('update:progress', (p) => {
    progress.value = p
  })
  try {
    ready.value = await UpdaterService.DownloadUpdate()
  } catch (e) {
    if (String(e) !== '已取消下载') message.error(String(e))
  } finally {
    downloading.value = false
    offProgress?.()
    offProgress = null
    progress.value = null
  }
}

async function cancelDownload() {
  try {
    await UpdaterService.CancelDownload()
  } catch {
    // 下载收尾由 DownloadUpdate 的拒绝统一处理
  }
}

const progressText = computed(() => {
  if (!progress.value) return '正在连接…'
  if (progress.value.phase === 'verifying') return '正在校验…'
  const { received, total } = progress.value
  return total > 0 ? `${formatBytes(received)} / ${formatBytes(total)}` : `已接收 ${formatBytes(received)}`
})
```

模板变更(把 `hasUpdate` 分支中原「about-actions」块替换为):

```html
            <div v-if="info.notes" class="about-notes" :class="{ expanded: notesExpanded }">
              <pre>{{ info.notes }}</pre>
            </div>

            <template v-if="ready">
              <span class="about-new">更新包已就绪:{{ ready.tag }}(下载与校验完成;安装与重启将在后续阶段开放)</span>
            </template>
            <template v-else-if="downloading">
              <div class="about-progress">
                <a-progress :percent="progress?.percent ?? 0" :show-info="false" size="small" />
                <span class="about-hint">{{ progressText }}</span>
                <a-button size="small" @click="cancelDownload">取消下载</a-button>
              </div>
            </template>
            <template v-else>
              <div class="about-actions">
                <a-button size="small" type="primary" :disabled="platformUnsupported" @click="download">下载更新</a-button>
                <a-button v-if="info.notes" size="small" type="link" @click="notesExpanded = !notesExpanded">
                  {{ notesExpanded ? '收起说明' : '展开说明' }}
                </a-button>
                <a-button size="small" @click="skip">跳过此版本</a-button>
              </div>
            </template>
```

样式追加:

```css
.about-progress {
  display: flex;
  align-items: center;
  gap: 8px;
}

.about-progress :deep(.ant-progress) {
  width: 160px;
  margin: 0;
}
```

- [ ] **Step 3: 扩展组件测试(`SettingsAboutPanel.test.ts`)**

mock 工厂扩展与新增用例:

```ts
// —— 顶部 mock 更新(既有 checkUpdate 保留)——
const downloadUpdate = vi.fn()
const cancelDownload = vi.fn()
vi.mock('../../../wailsjs/go/bridge/UpdaterService', () => ({
  CurrentVersion: vi.fn(async () => currentVersion),
  CheckUpdate: (...args: unknown[]) => checkUpdate(...args),
  DownloadUpdate: (...args: unknown[]) => downloadUpdate(...args),
  CancelDownload: (...args: unknown[]) => cancelDownload(...args),
}))

// 进度事件处理器捕获(vi.hoisted 供 mock 工厂引用)
const h = vi.hoisted(() => ({ progressHandler: null as null | ((p: unknown) => void) }))
vi.mock('../../api/events', () => ({
  onWailsEvent: vi.fn((_name: string, handler: (p: unknown) => void) => {
    h.progressHandler = handler
    return () => {
      h.progressHandler = null
    }
  }),
}))

// stubs 追加 a-progress:
const stubs = {
  'a-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  'a-progress': { template: '<div class="stub-progress"></div>' },
}
```

新增用例:

```ts
describe('下载与校验', () => {
  it('下载:进度事件驱动文案,完成后展示就绪态', async () => {
    downloadUpdate.mockResolvedValue({ tag: 'v0.2.0', assetName: 'gbt32960-simulator.app.zip', size: 200, sha256: 'x' })
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    await findBtn(wrapper, '检查更新').trigger('click')
    await flushPromises()

    await findBtn(wrapper, '下载更新').trigger('click')
    await flushPromises()
    h.progressHandler?.({ phase: 'downloading', received: 512, total: 1024, percent: 50 })
    await flushPromises()
    expect(wrapper.text()).toContain('512 B / 1.0 KB')

    await flushPromises()
    expect(wrapper.text()).toContain('更新包已就绪:v0.2.0')
    expect(downloadUpdate).toHaveBeenCalledTimes(1)
  })

  it('取消:不弹错误提示并回到可下载态', async () => {
    let rejectDownload: (e: unknown) => void = () => {}
    downloadUpdate.mockImplementation(
      () => new Promise((_, reject) => { rejectDownload = reject }),
    )
    cancelDownload.mockResolvedValue(undefined)

    const errSpy = vi.spyOn(message, 'error')
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    await findBtn(wrapper, '检查更新').trigger('click')
    await flushPromises()
    await findBtn(wrapper, '下载更新').trigger('click')
    await flushPromises()

    await findBtn(wrapper, '取消下载').trigger('click')
    rejectDownload('已取消下载')
    await flushPromises()

    expect(cancelDownload).toHaveBeenCalledTimes(1)
    expect(errSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('下载更新') // 回到可下载态
    errSpy.mockRestore()
  })
})
```

> 注:`message` 从 `ant-design-vue` 导入(既有 import 处);`beforeEach` 中追加 `downloadUpdate.mockReset()` / `cancelDownload.mockReset()`。若 `vi.spyOn(message, 'error')` 因冻结对象失败,改用 `vi.mock('ant-design-vue', async (orig) => ({ ...(await orig()), message: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))` 并断言 mock;二选一,以能稳定断言「取消不弹错误」为准。

- [ ] **Step 4: 前端四件套 + 提交**

```bash
cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build
GIT_MASTER=1 git add frontend/src/api/events.ts frontend/src/components/settings/SettingsAboutPanel.vue frontend/src/components/settings/SettingsAboutPanel.test.ts
GIT_MASTER=1 git commit -m "feat(frontend): 下载与校验 UI——update:progress 登记、进度条/取消/就绪态"
```

---

### Task 6: 真实链路集成测试与真机演练(PAC 证据)

**Level:** L3
**Level Rationale:** 端到端验收证据(真实 302 链、真机下载/取消/拒签路径),覆盖跨系统行为,不产生产品代码(测试与证据)。
**Linked Acceptance Items:** PAC-1、PAC-2、PAC-3、PAC-4、PAC-5
**Task Gate:** task reviewer + 全部证据留存 + 主控终验

**Files:**
- Create: `internal/updater/integration_test.go`(env 门控,默认跳过)
- Create(证据,不入库): `.superpowers/sdd/updater-p2/rehearsal-checklist.md`、`.superpowers/sdd/updater-p2/evidence/`

- [ ] **Step 1: 集成测试(真实 302 链,≥2 host)**

创建 `internal/updater/integration_test.go`:

```go
package updater

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestIntegrationReleaseChain 真实链路:GET v0.1.0 资产(github.com → release-assets.githubusercontent.com),
// 逐跳白名单校验;断言 ≥2 个 host 且全部合法。默认跳过;UPDATER_INTEGRATION=1 运行(需网络)。
func TestIntegrationReleaseChain(t *testing.T) {
	if os.Getenv("UPDATER_INTEGRATION") != "1" {
		t.Skip("设置 UPDATER_INTEGRATION=1 运行真实链路集成测试")
	}
	const assetURL = "https://github.com/sunsky74/gbt32960-simulator/releases/download/v0.1.0/gbt32960-simulator.app.zip"
	u, err := url.Parse(assetURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckDownloadURL(u); err != nil {
		t.Fatalf("初始 URL 不在白名单: %v", err)
	}
	var hops []string
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			hops = append(hops, req.URL.Hostname())
			return CheckDownloadURL(req.URL) // 逐跳校验:任一跳越界即失败
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-65535") // 只取前 64KiB
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	n, err := io.CopyN(io.Discard, resp.Body, 4096)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if n < 4096 {
		t.Fatalf("读取字节过少: %d", n)
	}
	if len(hops) < 1 || !strings.Contains(strings.Join(hops, ","), "release-assets.githubusercontent.com") {
		t.Fatalf("302 跳转链异常(期望 ≥2 host):%v", hops)
	}
}
```

```bash
UPDATER_INTEGRATION=1 go test ./internal/updater/ -run TestIntegrationReleaseChain -count=1 -v
```

预期:PASS(网络可达时);证据(命令+输出)留存。

- [ ] **Step 2: 真机演练清单(人工,主控执行并留存时间戳/输出)**

创建 `.superpowers/sdd/updater-p2/rehearsal-checklist.md`,逐项执行并记录:

1. **构建非 dev 版本**:`wails build -ldflags "-X main.version=v0.0.0"` → 启动 `build/bin/gbt32960-simulator.app`。
2. **启动清理证据**:启动前在 `~/Library/Caches/gbt32960-simulator/updates/` 预置 dummy 文件 → 启动后目录被清空。
3. **检查 → 下载(真实 302 链)**:关于面板检查更新(若限流则等窗口重试)→ 找到 v0.1.0 → 点“下载更新”→ 进度条推进(截图)。
4. **拒签路径(v0.1.0 未附校验资产)**:下载完成后验签阶段 → 文案 `发布未附校验信息,已拒绝更新`;检查缓存目录:无 `*.part`、无残留产物。
5. **取消路径**:再次下载(或任一大资产场景),中途点“取消下载”→ 无错误弹窗、回到可下载态;缓存目录无 `*.part`。
6. **回归**:关闭应用;`behind: 既有功能(连接/解析/服务端)不受影响` 由 PAC-6 自动化保证。

> 增强(推荐、不阻塞):按 `docs/release-signing.md` 对 v0.1.0 补传 SHA256SUMS + SHA256SUMS.sig(需在 GitHub Release 页上传)→ 重复第 3~4 步 → 预期落定“更新包已就绪:v0.1.0”(真实 happy path,证据留存)。

- [ ] **Step 3: 提交**

```bash
GIT_MASTER=1 git add internal/updater/integration_test.go
GIT_MASTER=1 git commit -m "test(updater): 真实 Release 302 链集成测试(env 门控)"
```

---

## Self-Review Record

- **Spec coverage:** 设计文档 §5.3(下载/校验契约)→ T2/T3;§5.5(update:progress 事件)→ T3/T5;§5.7(白名单/单飞/清理)→ T2/T3;§5.8(单元/桥接/真实链路测试)→ T1/T2/T3/T6;发布侧签名流程(§5.3 发布侧 + AC-12)→ T4。P3 内容(替换/回滚/toast)不在本计划,已由 Master Plan 映射。
- **Placeholder scan:** 无 TBD/TODO;唯一占位 = `signing_pubkey.go` 空公钥常量(T4 密钥仪式填实,`TestEmbeddedKeys` 作为完成门槛);`docs/release-signing.md` 的 keyid 空位在 T4 仪式时填写。
- **Type/interface consistency:** `Progress{Phase,Received,Total}`+`Percent()` 在 T2/T3 一致;`progressDTO` JSON 字段(phase/received/total/percent)与前端 `UpdateProgressEvent` 一致;`DownloadResult` 定义于 `updater` 包并被 bridge 透出(wailsjs 生成 `updater.DownloadResult`);错误文案在 Global Constraints、T1/T2 实现、PAC 三处一致;`CheckURL` 注入缝命名在 T2 定义、T3 测试使用一致。
- **Decomposition decision:** 本 Phase 为已批准的 4 阶段拆分之 P2;自身 6 个任务(≤8-10),不再二次拆分。
- **Master/Phase completeness:** Master Plan 含 Progress Ledger 与 Global AC 覆盖矩阵;本计划 PAC-1..6 已映射回 Global AC(逐项标注)。
- **Level completeness:** 6 个任务均含 Level 与 Level Rationale(T1 = L2;T2/T3/T4/T5/T6 = L3)。
- **Task Gate completeness:** L2 = task reviewer + focused checks;L3 = task reviewer + linked AC + 证据,均与 Level 匹配。
- **L3 AC binding:** T2(PAC-1/2/3)、T3(PAC-1/2/4)、T4(PAC-5)、T5(PAC-1/4)、T6(PAC-1..5)均非空绑定且 AC 存在。
- **Final acceptance coverage:** PAC-1~6 均绑定到非 L1 任务;跨模块项(绑定面/事件/清理)由 T3/T5 与 PAC-4 覆盖。
- **Executable final acceptance:** 每条 PAC refinement 均含具体命令、注入参数与边界值(如 `ProgressInterval=1h`、`StallTimeout=80ms`、恰 <300ms 判定、keyid 矩阵、≥2 host)。
- **Source consistency:** PAC 的 Source 名称与设计文档一致(Overall Business Flow / Current Requirement Flow / Development Architecture / New Architecture Enablement / Existing Architecture Fit)。
- **依赖完整性:** `signing_pubkey.go` 占位创建于 T1(供 T3 编译),真实公钥在 T4 仪式填实;T3 测试注入测试密钥,不依赖仪式完成。

## Execution Handoff

计划已保存至 `docs/superpowers/plans/2026-09-15-updater-phase-2-download-plan.md`。

三种执行方式:

**1. Quality-LTDD(推荐)** - Subagent-Driven + 验收门 + Final Intent Guard。每个任务先过实现者自检与 Task Reviewer;L3 任务额外跑 linked AC;全部完成后 Final Reviewer 与 plan 级 AC 终验。含两处人工步骤(T4 密钥仪式、T6 真机演练),由主控协调执行。

**2. Subagent-Driven** - 每任务新 subagent + 两阶段评审(spec 合规 + 代码质量)。验收门较轻。

**3. Inline Execution** - 本会话内批量执行 + 检查点人工确认。适合逐任务在场审阅。

Quality-LTDD 与 Subagent-Driven 的关系:前者在后者之上增加 Final Intent、任务级 AC、Final Reviewer、plan 级 AC 与失败裁决。Inline 为手动检查点路径。

Which approach?
