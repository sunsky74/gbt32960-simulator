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
		"长度不符":           []byte("deadbeef YWJj"), // "abc" = 3 字节
		"单字段":            []byte("deadbeef"),
		"空内容":            []byte(""),
		"多字段":            []byte("a b c"),
	} {
		if !errors.Is(VerifySumSignature(sums, raw, keys), ErrSigVerify) {
			t.Fatalf("%s 应验签失败", name)
		}
	}

	// 多公钥表命中(keyid 配对,轮换场景)
	keys2 := map[string]ed25519.PublicKey{
		KeyID(pub): pub,
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
