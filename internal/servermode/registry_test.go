package servermode

import (
	"sync"
	"testing"
	"time"
)

func TestRegistryDuplicateLogin(t *testing.T) {
	r := NewRegistry()
	now := time.Unix(1_700_000_000, 0)
	if !r.Register(vin17, "1.2.3.4:5", now, false) {
		t.Fatal("首次登入应成功")
	}
	if r.Register(vin17, "5.6.7.8:9", now.Add(time.Second), false) {
		t.Fatal("同 VIN 重复登入应被拒绝")
	}
	s := r.Snapshot()
	if len(s) != 1 || s[0].Peer != "1.2.3.4:5" {
		t.Fatalf("重复登入不应覆盖原会话: %+v", s)
	}
}

func TestRegistryRemoveAndTouch(t *testing.T) {
	r := NewRegistry()
	now := time.Unix(1_700_000_000, 0)
	r.Register(vin17, "p", now, false)
	r.Touch(vin17, now.Add(time.Minute))
	if got := r.Snapshot()[0].LastSeen; !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("Touch 未刷新: %v", got)
	}
	if _, ok := r.Remove(vin17); !ok {
		t.Fatal("Remove 应成功")
	}
	if _, ok := r.Remove(vin17); ok {
		t.Fatal("重复 Remove 应失败")
	}
	if r.Register(vin17, "p2", now, false) != true {
		t.Fatal("登出后应可重新登入")
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Register(vin17, "p", time.Now(), false)
			r.Touch(vin17, time.Now())
			r.Snapshot()
		}()
	}
	wg.Wait()
	if len(r.Snapshot()) != 1 {
		t.Fatal("并发下会话数应为 1")
	}
}
