package tlsconf

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	errCA       = "CA 证书解析失败"
	errCARead   = "CA 证书: 读取"
	errCertRead = "客户端证书: 读取"
	errMaterial = "客户端证书/私钥缺失或不可读"
	errPair     = "客户端证书装配失败"
)

// certPair 测试用自签证书材料。
type certPair struct {
	certPEM []byte
	keyPEM  []byte
	cert    *x509.Certificate
}

// genSelfSignedCert 在内存中生成自签 ECDSA P-256 证书,避免提交二进制夹具。
func genSelfSignedCert(t *testing.T, cn string) certPair {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("解析证书失败: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("序列化私钥失败: %v", err)
	}
	return certPair{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		cert:    cert,
	}
}

// writeFile 把材料写入临时目录,返回路径。
func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
	return path
}

// assertRootCA 断言 RootCAs 非空且确实信任给定 CA(按 DER RawSubject 比对)。
func assertRootCA(t *testing.T, cfg *tls.Config, ca *x509.Certificate) {
	t.Helper()
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs 未装配")
	}
	subjects := cfg.RootCAs.Subjects()
	if len(subjects) < 1 {
		t.Fatal("RootCAs.Subjects() 为空")
	}
	for _, s := range subjects {
		if bytes.Equal(s, ca.RawSubject) {
			return
		}
	}
	t.Fatalf("生成的 CA 未被加入 RootCAs: %x", subjects)
}

// assertClientCert 断言 Certificates 恰好装配了给定证书。
func assertClientCert(t *testing.T, cfg *tls.Config, cert *x509.Certificate) {
	t.Helper()
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates 数量 = %d, want 1", len(cfg.Certificates))
	}
	chain := cfg.Certificates[0].Certificate
	if len(chain) != 1 || !bytes.Equal(chain[0], cert.Raw) {
		t.Fatal("装配的客户端证书与输入不匹配")
	}
}

func TestConfigBuild(t *testing.T) {
	ca := genSelfSignedCert(t, "test-ca")
	client := genSelfSignedCert(t, "client")
	other := genSelfSignedCert(t, "other-client")

	// allow: SIZE_OK — 文件主体为表驱动用例数据(17 个子测试),逻辑代码仅 helpers + runner。
	tests := []struct {
		name    string
		config  func(t *testing.T) *Config
		wantErr string
		wantNil bool
		check   func(t *testing.T, cfg *tls.Config)
	}{
		{
			name:    "nil 接收者返回 nil,nil",
			config:  func(*testing.T) *Config { return nil },
			wantNil: true,
		},
		{
			name: "Enabled=false 时忽略材料返回 nil,nil",
			config: func(t *testing.T) *Config {
				return &Config{Enabled: false, CA: string(ca.certPEM)}
			},
			wantNil: true,
		},
		{
			name:   "Enabled=true 无任何材料:合法空配置",
			config: func(*testing.T) *Config { return &Config{Enabled: true} },
			check: func(t *testing.T, cfg *tls.Config) {
				if cfg.RootCAs != nil || len(cfg.Certificates) != 0 {
					t.Fatalf("空材料不应装配根池或证书: RootCAs=%v certs=%d", cfg.RootCAs, len(cfg.Certificates))
				}
			},
		},
		{
			name: "CA 为 PEM 内容",
			config: func(*testing.T) *Config {
				return &Config{Enabled: true, CA: string(ca.certPEM)}
			},
			check: func(t *testing.T, cfg *tls.Config) { assertRootCA(t, cfg, ca.cert) },
		},
		{
			name: "CA 为文件路径(首尾空白被裁剪)",
			config: func(t *testing.T) *Config {
				return &Config{Enabled: true, CA: " \n" + writeFile(t, "ca.pem", ca.certPEM) + "\n"}
			},
			check: func(t *testing.T, cfg *tls.Config) { assertRootCA(t, cfg, ca.cert) },
		},
		{
			name: "CA 文件存在但内容非 PEM",
			config: func(t *testing.T) *Config {
				return &Config{Enabled: true, CA: writeFile(t, "garbage.bin", []byte("这不是证书"))}
			},
			wantErr: errCA,
		},
		{
			name: "CA 文件不可读:报读取错误而不再静默忽略",
			config: func(t *testing.T) *Config {
				return &Config{Enabled: true, CA: filepath.Join(t.TempDir(), "missing-ca.pem")}
			},
			wantErr: errCARead,
		},
		{
			name: "CA 形似 PEM 但解析失败",
			config: func(*testing.T) *Config {
				return &Config{
					Enabled: true,
					CA:      "-----BEGIN CERTIFICATE-----\n!!!not-base64!!!\n-----END CERTIFICATE-----\n",
				}
			},
			wantErr: errCA,
		},
		{
			name: "客户端证书/私钥为 PEM 内容",
			config: func(*testing.T) *Config {
				return &Config{Enabled: true, ClientCert: string(client.certPEM), ClientKey: string(client.keyPEM)}
			},
			check: func(t *testing.T, cfg *tls.Config) { assertClientCert(t, cfg, client.cert) },
		},
		{
			name: "客户端证书/私钥为文件路径",
			config: func(t *testing.T) *Config {
				return &Config{
					Enabled:    true,
					ClientCert: writeFile(t, "client.pem", client.certPEM),
					ClientKey:  writeFile(t, "client.key", client.keyPEM),
				}
			},
			check: func(t *testing.T, cfg *tls.Config) { assertClientCert(t, cfg, client.cert) },
		},
		{
			name: "CA 与客户端证书同时装配",
			config: func(t *testing.T) *Config {
				return &Config{
					Enabled:    true,
					CA:         writeFile(t, "ca.pem", ca.certPEM),
					ClientCert: string(client.certPEM),
					ClientKey:  string(client.keyPEM),
				}
			},
			check: func(t *testing.T, cfg *tls.Config) {
				assertRootCA(t, cfg, ca.cert)
				assertClientCert(t, cfg, client.cert)
			},
		},
		{
			name: "只给客户端证书",
			config: func(*testing.T) *Config {
				return &Config{Enabled: true, ClientCert: string(client.certPEM)}
			},
			wantErr: errMaterial,
		},
		{
			name: "只给客户端私钥",
			config: func(*testing.T) *Config {
				return &Config{Enabled: true, ClientKey: string(client.keyPEM)}
			},
			wantErr: errMaterial,
		},
		{
			name: "客户端证书/私钥路径不可读",
			config: func(t *testing.T) *Config {
				dir := t.TempDir()
				return &Config{
					Enabled:    true,
					ClientCert: filepath.Join(dir, "missing.pem"),
					ClientKey:  filepath.Join(dir, "missing.key"),
				}
			},
			wantErr: errCertRead,
		},
		{
			name: "证书与私钥不配对",
			config: func(*testing.T) *Config {
				return &Config{Enabled: true, ClientCert: string(client.certPEM), ClientKey: string(other.keyPEM)}
			},
			wantErr: errPair,
		},
		{
			name: "ServerName 与 Insecure 透传",
			config: func(*testing.T) *Config {
				return &Config{Enabled: true, ServerName: "example.test", Insecure: true}
			},
			check: func(t *testing.T, cfg *tls.Config) {
				if cfg.ServerName != "example.test" {
					t.Fatalf("ServerName = %q, want example.test", cfg.ServerName)
				}
				if !cfg.InsecureSkipVerify {
					t.Fatal("InsecureSkipVerify 未透传")
				}
			},
		},
		{
			name:   "MinVersion 为 TLS 1.2",
			config: func(*testing.T) *Config { return &Config{Enabled: true} },
			check: func(t *testing.T, cfg *tls.Config) {
				if cfg.MinVersion != tls.VersionTLS12 {
					t.Fatalf("MinVersion = %#x, want %#x", cfg.MinVersion, tls.VersionTLS12)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.config(t).Build()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Build() error = nil, want 包含 %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Build() error = %q, want 包含 %q", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("出错时应返回 nil config, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("Build() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("Build() = nil, want 非 nil 配置")
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestBuildUnreadableMaterialsReportFileError 回归 ⑨:材料文件不可读时,
// 必须报出"读取 <路径>"的文件读取错误,与"内容非法/解析失败"可区分。
func TestBuildUnreadableMaterialsReportFileError(t *testing.T) {
	dir := t.TempDir()
	missingCA := filepath.Join(dir, "missing-ca.pem")
	missingCert := filepath.Join(dir, "missing-cert.pem")
	missingKey := filepath.Join(dir, "missing-key.pem")
	valid := genSelfSignedCert(t, "client")

	tests := []struct {
		name   string
		cfg    *Config
		substr []string
	}{
		{
			name:   "CA 文件不可读",
			cfg:    &Config{Enabled: true, CA: missingCA},
			substr: []string{"CA 证书", "读取", missingCA},
		},
		{
			name:   "客户端证书文件不可读",
			cfg:    &Config{Enabled: true, ClientCert: missingCert, ClientKey: string(valid.keyPEM)},
			substr: []string{"客户端证书", "读取", missingCert},
		},
		{
			name:   "客户端私钥文件不可读",
			cfg:    &Config{Enabled: true, ClientCert: string(valid.certPEM), ClientKey: missingKey},
			substr: []string{"客户端私钥", "读取", missingKey},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := tc.cfg.Build()
			if err == nil {
				t.Fatal("应报文件读取错误")
			}
			if cfg != nil {
				t.Fatalf("出错时应返回 nil config, got %+v", cfg)
			}
			for _, s := range tc.substr {
				if !strings.Contains(err.Error(), s) {
					t.Fatalf("err = %q, 应包含 %q", err, s)
				}
			}
			if strings.Contains(err.Error(), "解析失败") {
				t.Fatalf("文件读取失败不应被误报为解析失败: %q", err)
			}
		})
	}
}

func TestLoadPEM(t *testing.T) {
	pemText := "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"
	dir := t.TempDir()
	filePath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(filePath, []byte(pemText), 0o600); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	missing := filepath.Join(dir, "nope.pem")

	tests := []struct {
		name    string
		in      string
		want    []byte
		wantErr string
	}{
		{name: "空串返回 nil,nil", in: ""},
		{name: "PEM 内容原样返回", in: pemText, want: []byte(pemText)},
		{name: "文件路径读取内容", in: filePath, want: []byte(pemText)},
		{name: "路径首尾空白被裁剪", in: "\n " + filePath + " \n", want: []byte(pemText)},
		{name: "文件不存在报读取错误并含路径", in: missing, wantErr: missing},
		{name: "非 PEM 且非文件路径报读取错误", in: "just-a-string", wantErr: "just-a-string"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadPEM(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("loadPEM(%q) err = %v, want 包含 %q", tc.in, err, tc.wantErr)
				}
				if !strings.HasPrefix(err.Error(), "读取 ") {
					t.Fatalf("loadPEM(%q) err = %v, want 文件读取错误", tc.in, err)
				}
				if got != nil {
					t.Fatalf("出错时应返回 nil, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadPEM(%q) error = %v", tc.in, err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("loadPEM(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestLooksLikePEM(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "", want: false},
		{in: "/tmp/ca.pem", want: false},
		{in: "-----BEGIN CERTIFICATE-----\n", want: true},
		{in: "前缀文本 -----BEGIN 出现在中间", want: true},
	}
	for _, tc := range tests {
		if got := looksLikePEM(tc.in); got != tc.want {
			t.Fatalf("looksLikePEM(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
