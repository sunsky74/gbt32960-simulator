package updater

import "testing"

// TestEmbeddedKeys 嵌入公钥至少 1 个且可解析(密钥仪式完成标志;失败=仪式未完成)。
// 轮换期允许多钥共存(旧钥+新钥),故断言 >=1 而非 ==1。
func TestEmbeddedKeys(t *testing.T) {
	keys := EmbeddedKeys()
	if len(keys) < 1 {
		t.Fatalf("嵌入公钥数 = %d, want >=1(密钥仪式未完成?)", len(keys))
	}
	for id, pub := range keys {
		if id == "" || len(pub) != 32 {
			t.Fatalf("公钥异常: id=%q len=%d", id, len(pub))
		}
	}
}
