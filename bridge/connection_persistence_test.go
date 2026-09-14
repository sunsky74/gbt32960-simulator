package bridge

import (
	"fmt"
	"sync"
	"testing"
)

// TestConcurrentSaveConfigNoLostUpdate 并发保存不同档案时不得丢更新。
// 缺陷背景:SaveConfig/SwitchProfile/DeleteProfile 是「读 profiles.json →
// 改 → 写回」的无锁 RMW;Wails 每个绑定 RPC 一个 goroutine,Connect(持
// connectMu)与前端「保存配置」可并发进入,5s 的 GetProfiles 轮询同时在读。
// 无锁时代后写覆盖先写,档案整体丢失。
func TestConcurrentSaveConfigNoLostUpdate(t *testing.T) {
	tempHome(t)
	cs := NewConnectionService(NewRuntime())

	const n = 12
	start := make(chan struct{}) // 起始栅栏:最大化 RMW 窗口重叠
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := *DefaultConnectionConfig()
			cfg.Name = fmt.Sprintf("profile-%02d", i)
			<-start
			if err := cs.SaveConfig(cfg); err != nil {
				errs <- fmt.Errorf("profile-%02d: %w", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发 SaveConfig 失败: %v", err)
	}

	got, err := cs.GetProfiles()
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(got))
	for _, p := range got {
		names[p.Name] = true
	}
	for i := 0; i < n; i++ {
		if name := fmt.Sprintf("profile-%02d", i); !names[name] {
			t.Errorf("并发保存丢失档案 %s(现存 %d 个: %v)", name, len(got), names)
		}
	}
}

// TestConcurrentReadsDuringSaves 保存风暴期间持续读取不得报错
// (依赖原子写:rename 前读者永远读到某个完整版本)。
func TestConcurrentReadsDuringSaves(t *testing.T) {
	tempHome(t)
	cs := NewConnectionService(NewRuntime())

	stop := make(chan struct{})
	var readerWg sync.WaitGroup
	readerErr := make(chan error, 1)
	for i := 0; i < 4; i++ {
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := cs.GetProfiles(); err != nil {
					select {
					case readerErr <- err:
					default:
					}
					return
				}
			}
		}()
	}

	var writerWg sync.WaitGroup
	for i := 0; i < 8; i++ {
		writerWg.Add(1)
		go func(i int) {
			defer writerWg.Done()
			for j := 0; j < 20; j++ {
				cfg := *DefaultConnectionConfig()
				cfg.Name = fmt.Sprintf("w-%d-%d", i, j)
				_ = cs.SaveConfig(cfg)
			}
		}(i)
	}
	writerWg.Wait()
	close(stop)
	readerWg.Wait()
	select {
	case err := <-readerErr:
		t.Fatalf("并发读取失败(原子写回归?): %v", err)
	default:
	}
}
