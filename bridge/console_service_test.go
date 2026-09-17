package bridge

import (
	"strings"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
)

func sampleEvents() []engine.Event {
	return []engine.Event{
		{Time: time.Date(2026, 8, 27, 10, 0, 0, 0, time.Local), Kind: engine.EventConn, Message: "TCP 已连接"},
		{Time: time.Date(2026, 8, 27, 10, 0, 1, 0, time.Local), Kind: engine.EventTx, Cmd: "0x01 VEHICLE_LOGIN", Hex: "232301fe", Bytes: 55},
		{Time: time.Date(2026, 8, 27, 10, 0, 2, 0, time.Local), Kind: engine.EventError, Message: "超时"},
	}
}

func TestForwarderSnapshotFilter(t *testing.T) {
	f := NewForwarder(nil)
	for _, e := range sampleEvents() {
		f.mirror(e)
	}
	all := f.Snapshot(nil)
	if len(all) != 3 {
		t.Fatalf("all = %d", len(all))
	}
	tx := f.Snapshot([]string{"tx"})
	if len(tx) != 1 || tx[0].Kind != engine.EventTx {
		t.Fatalf("tx filter = %+v", tx)
	}
	f.Clear()
	if len(f.Snapshot(nil)) != 0 {
		t.Fatal("clear failed")
	}
}

func TestFormatEventsCSVAndLog(t *testing.T) {
	events := sampleEvents()

	csvData, err := formatEventsCSV(events)
	if err != nil {
		t.Fatal(err)
	}
	csvStr := string(csvData)
	if !strings.Contains(csvStr, "时间") || !strings.Contains(csvStr, "0x01 VEHICLE_LOGIN") || !strings.Contains(csvStr, "232301fe") {
		t.Fatalf("csv missing columns: %s", csvStr[:min(200, len(csvStr))])
	}

	logData, err := formatEventsLog(events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "[conn]") || !strings.Contains(string(logData), "HEX: 232301fe") {
		t.Fatalf("log format wrong: %s", logData)
	}

	jsonData, err := formatEventsJSON(events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonData), `"kind": "tx"`) {
		t.Fatalf("json format wrong")
	}
}

// TestConsoleServiceProtocolMetadata 参数定义表与应答码列表按版本分发,未知版本回落 2016。
func TestConsoleServiceProtocolMetadata(t *testing.T) {
	s := NewConsoleService(nil)
	if got := s.GetParamSpecs("2025"); len(got) != len(engine.ParamSpecs(api.V2025)) {
		t.Fatalf("2025 参数条目 = %d", len(got))
	}
	if got := s.GetResponseCodes("2025"); len(got) != 7 {
		t.Fatalf("2025 应答码条目 = %d, want 7", len(got))
	}
	if got := s.GetResponseCodes("2016"); len(got) != 3 {
		t.Fatalf("2016 应答码条目 = %d, want 3", len(got))
	}
	if got := s.GetResponseCodes("unknown"); len(got) != 3 {
		t.Fatalf("未知版本应回落 2016, got %d", len(got))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
