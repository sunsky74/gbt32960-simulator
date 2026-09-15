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
	if !ok || len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, sums, sig) {
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
