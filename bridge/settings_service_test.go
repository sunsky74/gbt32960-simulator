package bridge

import (
	"fmt"
	"testing"

	"gbt32960-simulator/internal/engine"
	"gbt32960-simulator/internal/store"
)

func TestSettingsServiceDefaults(t *testing.T) {
	tempHome(t)
	fwd := NewForwarder(nil)
	svc := NewSettingsService(fwd)
	if got := svc.GetAppSettings(); got.ConsoleExportCap != defaultConsoleExportCap {
		t.Fatalf("默认导出缓冲 = %d, want %d", got.ConsoleExportCap, defaultConsoleExportCap)
	}
	fwd.bufMu.Lock()
	gotCap := fwd.cap
	fwd.bufMu.Unlock()
	if gotCap != defaultConsoleExportCap {
		t.Fatalf("转发器容量 = %d, want %d", gotCap, defaultConsoleExportCap)
	}
}

func TestSettingsServiceLoadsInvalidFileAsDefault(t *testing.T) {
	tempHome(t)
	if err := store.Save(appSettingsFile, AppSettings{ConsoleExportCap: 10}); err != nil {
		t.Fatal(err)
	}
	svc := NewSettingsService(NewForwarder(nil))
	if got := svc.GetAppSettings(); got.ConsoleExportCap != defaultConsoleExportCap {
		t.Fatalf("非法存量设置应回落默认: %+v", got)
	}
}

func TestSettingsServiceRoundtrip(t *testing.T) {
	tempHome(t)
	svc := NewSettingsService(NewForwarder(nil))
	if err := svc.SetAppSettings(AppSettings{ConsoleExportCap: 1234}); err != nil {
		t.Fatal(err)
	}
	// 重新加载(模拟重启):从 settings.json 读回
	reloaded := NewSettingsService(NewForwarder(nil))
	if got := reloaded.GetAppSettings(); got.ConsoleExportCap != 1234 {
		t.Fatalf("重载设置 = %+v, want 1234", got)
	}
}

func TestSettingsServiceInvalidRange(t *testing.T) {
	tempHome(t)
	svc := NewSettingsService(NewForwarder(nil))
	for _, n := range []int{0, -1, 999, 500001} {
		if err := svc.SetAppSettings(AppSettings{ConsoleExportCap: n}); err == nil || err.Error() != "导出缓冲条数须在 1000~500000" {
			t.Fatalf("n=%d err = %v, want 范围错误", n, err)
		}
	}
	if got := svc.GetAppSettings(); got.ConsoleExportCap != defaultConsoleExportCap {
		t.Fatalf("非法设置不应生效: %+v", got)
	}
}

func TestSettingsServiceAppliesCapAndTrims(t *testing.T) {
	tempHome(t)
	fwd := NewForwarder(nil)
	svc := NewSettingsService(fwd)
	for i := 0; i < minConsoleExportCap+5; i++ {
		fwd.mirror(engine.Event{Message: fmt.Sprintf("e%d", i)})
	}
	if err := svc.SetAppSettings(AppSettings{ConsoleExportCap: minConsoleExportCap}); err != nil {
		t.Fatal(err)
	}
	snap := fwd.Snapshot(nil)
	if len(snap) != minConsoleExportCap {
		t.Fatalf("裁剪后条数 = %d, want %d", len(snap), minConsoleExportCap)
	}
	if snap[0].Message != "e5" || snap[len(snap)-1].Message != fmt.Sprintf("e%d", minConsoleExportCap+4) {
		t.Fatalf("应保留最近 %d 条: first=%s last=%s", minConsoleExportCap, snap[0].Message, snap[len(snap)-1].Message)
	}
}
