package updater

import (
	"crypto/ed25519"
	"encoding/base64"
)

// signingPubKeyB64 发布签名公钥(Ed25519,std base64;由 tools/sign-release -gen 生成)。
// 私钥离线保管、绝不入库;轮换:保留旧钥条目直至所有在用版本都升级到含新钥的版本。
const signingPubKeyB64 = "AVkLcnNunEYb9BqmkjRSwTXTvJjhBqbIOCWpBIDXSt4="

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
