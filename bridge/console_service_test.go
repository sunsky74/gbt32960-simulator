package bridge

import (
	"strings"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
