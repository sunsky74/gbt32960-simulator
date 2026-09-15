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
