package bridge

import (
	"strings"
	"testing"
)

func platformCfg() *ConnectionConfig {
	return &ConnectionConfig{
		Name: "t", Host: "127.0.0.1", Port: 32960, Version: "2016",
		VIN: "LSV00000000000001",
	}
}

func TestValidateConnPlatform(t *testing.T) {
	ok := platformCfg()
	ok.PlatformMode = true
	ok.PlatformVIN = "PLT00000000000001"
	ok.PlatformUser = "entuser"
	ok.PlatformPass = "entpass"
	if err := validateConn(ok); err != nil {
		t.Fatalf("valid platform cfg rejected: %v", err)
	}

	shortVIN := platformCfg()
	shortVIN.PlatformMode = true
	shortVIN.PlatformVIN = "PLT1"
	shortVIN.PlatformUser = "u"
	if err := validateConn(shortVIN); err == nil || !strings.Contains(err.Error(), "平台标识") {
		t.Fatalf("err = %v, want platform VIN length error", err)
	}

	noUser := platformCfg()
	noUser.PlatformMode = true
	noUser.PlatformVIN = "PLT00000000000001"
	if err := validateConn(noUser); err == nil || !strings.Contains(err.Error(), "平台账号") {
		t.Fatalf("err = %v, want empty user error", err)
	}

	longUser := platformCfg()
	longUser.PlatformMode = true
	longUser.PlatformVIN = "PLT00000000000001"
	longUser.PlatformUser = "0123456789abcdef" // 16 > 12
	if err := validateConn(longUser); err == nil || !strings.Contains(err.Error(), "12 位") {
		t.Fatalf("err = %v, want user length error", err)
	}

	// 未开平台模式时平台字段不校验
	plain := platformCfg()
	plain.PlatformVIN = ""
	if err := validateConn(plain); err != nil {
		t.Fatalf("plain cfg rejected: %v", err)
	}
}
