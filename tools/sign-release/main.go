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
