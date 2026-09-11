package bridge

// console_decode_test.go:bridge 转发层解码(tx/rx 帧 → 中文键保序树)与三种导出格式消费该树的测试。

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gbt32960-simulator/internal/engine"
)

// loadConsoleFrameHex 读取 schema golden 帧 hex;文件缺失时跳过(与 internal/parser 测试同风格)。
func loadConsoleFrameHex(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../internal/schema/testdata/" + name)
	if err != nil {
		t.Skipf("golden missing: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// TestDecodeEventEnrichesTxRx 校验 tx/rx 帧事件经 DecodeEvent 后带上保序中文键树。
func TestDecodeEventEnrichesTxRx(t *testing.T) {
	hexStr := loadConsoleFrameHex(t, "prod_login_v2016_01.hex")

	for _, kind := range []engine.EventKind{engine.EventTx, engine.EventRx} {
		t.Run(string(kind), func(t *testing.T) {
			// Given 一条仅含原始 hex 的帧事件
			// When 经 DecodeEvent 补解码
			got := DecodeEvent(engine.Event{Kind: kind, Hex: hexStr})
			// Then Decoded 为可序列化的保序树
			if got.Decoded == nil {
				t.Fatal("Decoded 为空")
			}
			b, err := json.Marshal(got.Decoded)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			s := string(b)
			if !strings.Contains(s, `"起始符":"##"`) {
				t.Errorf("缺少起始符: %s", s)
			}
			if i, j := strings.Index(s, "起始符"), strings.Index(s, "命令单元"); i < 0 || j < 0 || i > j {
				t.Errorf("键序错误: 起始符@%d 命令单元@%d", i, j)
			}
			if !strings.Contains(s, `"命令标识"`) || !strings.Contains(s, `"应答标志"`) {
				t.Errorf("命令单元缺少子键: %s", s)
			}
		})
	}
}

// TestDecodeEventSkipsNonFrames 非帧事件与非法 hex 保持原样,不 panic。
func TestDecodeEventSkipsNonFrames(t *testing.T) {
	// Given 连接事件(无 hex)
	// When 经 DecodeEvent
	got := DecodeEvent(engine.Event{Kind: engine.EventConn, Message: "TCP 已连接"})
	// Then 原样返回,Decoded 仍为空
	if got.Decoded != nil {
		t.Errorf("conn 事件不应有 Decoded: %v", got.Decoded)
	}

	// error 事件同理
	if got := DecodeEvent(engine.Event{Kind: engine.EventError, Message: "超时"}); got.Decoded != nil {
		t.Errorf("error 事件不应有 Decoded: %v", got.Decoded)
	}

	// 非法 hex:解析失败静默保持 nil
	for _, bad := range []string{"zz", "2323"} {
		got := DecodeEvent(engine.Event{Kind: engine.EventTx, Hex: bad})
		if got.Decoded != nil {
			t.Errorf("非法 hex %q 不应有 Decoded: %v", bad, got.Decoded)
		}
	}
}

// TestExportUsesOrderedTree CSV/LOG/JSON 三种导出都消费 DecodeEvent 填充的保序树。
func TestExportUsesOrderedTree(t *testing.T) {
	hexStr := loadConsoleFrameHex(t, "prod_login_v2016_01.hex")
	// Given 已补解码的 tx 事件
	enriched := DecodeEvent(engine.Event{Kind: engine.EventTx, Cmd: "0x01 VEHICLE_LOGIN", Hex: hexStr, Bytes: 55})
	if enriched.Decoded == nil {
		t.Fatal("Decoded 为空")
	}

	// When/Then CSV:解码列承载紧凑 JSON,且键序为 起始符 → 命令单元
	csvData, err := formatEventsCSV([]engine.Event{enriched})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(csvData))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[1]) != 7 {
		t.Fatalf("csv 形状异常: %v", rows)
	}
	decodedCol := rows[1][6]
	if !strings.Contains(decodedCol, `"起始符":"##"`) {
		t.Errorf("csv 解码列缺少紧凑树: %s", decodedCol)
	}
	if i, j := strings.Index(decodedCol, "起始符"), strings.Index(decodedCol, "命令单元"); i < 0 || j < 0 || i > j {
		t.Errorf("csv 键序错误: 起始符@%d 命令单元@%d", i, j)
	}

	// LOG:同一紧凑树
	logData, err := formatEventsLog([]engine.Event{enriched})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), `DECODED: {"起始符":"##"`) {
		t.Errorf("log 缺少紧凑树: %s", logData)
	}

	// JSON:MarshalIndent 缩进后仍带中文键
	jsonData, err := formatEventsJSON([]engine.Event{enriched})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonData), `"起始符": "##"`) {
		t.Errorf("json 缺少缩进树: %s", jsonData)
	}
}
