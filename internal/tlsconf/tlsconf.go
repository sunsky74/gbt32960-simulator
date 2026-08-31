// Package tlsconf 把用户输入的 TLS 材料(文件路径或 PEM 内容)装配为 tls.Config。
package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

// Config TLS 配置的持久化形态(bridge DTO 直接复用)。
type Config struct {
	Enabled    bool   `json:"enabled"`
	CA         string `json:"ca"`         // CA 证书:文件路径或 PEM 内容
	ClientCert string `json:"clientCert"` // 客户端证书:文件路径或 PEM 内容(双向认证)
	ClientKey  string `json:"clientKey"`  // 客户端私钥:文件路径或 PEM 内容
	ServerName string `json:"serverName"` // SNI / 校验名
	Insecure   bool   `json:"insecure"`   // 跳过证书校验(仅调试)
}

// Build 装配 tls.Config。Enabled=false 或材料为空时返回 nil。
func (c *Config) Build() (*tls.Config, error) {
	if c == nil || !c.Enabled {
		return nil, nil
	}
	cfg := &tls.Config{
		ServerName:         c.ServerName,
		InsecureSkipVerify: c.Insecure, //nolint:gosec // 用户显式勾选的调试选项
		MinVersion:         tls.VersionTLS12,
	}

	if ca := loadPEM(c.CA); len(ca) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("CA 证书解析失败(既不是有效文件也不是 PEM 内容)")
		}
		cfg.RootCAs = pool
	}

	if c.ClientCert != "" || c.ClientKey != "" {
		certPEM := loadPEM(c.ClientCert)
		keyPEM := loadPEM(c.ClientKey)
		if certPEM == nil || keyPEM == nil {
			return nil, fmt.Errorf("客户端证书/私钥缺失或不可读")
		}
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("客户端证书装配失败: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return cfg, nil
}

// looksLikePEM 判断是否直接就是 PEM 文本。
func looksLikePEM(s string) bool {
	return strings.Contains(s, "-----BEGIN")
}

// loadPEM 输入可以是 PEM 内容或文件路径;空串返回 nil。
func loadPEM(s string) []byte {
	if s == "" {
		return nil
	}
	if looksLikePEM(s) {
		return []byte(s)
	}
	b, err := os.ReadFile(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return b
}
