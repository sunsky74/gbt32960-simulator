package servermode

import (
	"testing"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/model"
	"github.com/sunsky74/gb32960/model/gbt2016"
)

const vin17 = "LVBV3J7B0LY000001"

// emptyBody 0x07/0x08 等无载荷体的测试空体(自包含,不依赖 Task 4 的 rawBody)。
type emptyBody struct{}

func (emptyBody) Version() api.GBTVersion { return api.V2016 }
func (emptyBody) Bytes() ([]byte, error)  { return nil, nil }

// loginFrame 造一帧真实 2016 登入(载荷体与引擎 sendLogin 同构:
// 库内 gbt2016.VehicleLogin,ICCID 定长 20,Codes 须与 Count×Length 匹配)。
func loginFrame(t *testing.T) []byte {
	t.Helper()
	raw, _, err := engine.BuildFrame(api.V2016, vin17, 0x01, &gbt2016.VehicleLogin{
		BeanTime:  model.BeanTime{Year: 26, Month: 9, Day: 3, Hour: 10, Minute: 0, Second: 0},
		SerialNum: 1,
		ICCID:     "12345678901234567890",
		Count:     1,
		Length:    1,
		Codes:     []string{"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func heartbeatFrame(t *testing.T) []byte {
	t.Helper()
	raw, _, err := engine.BuildFrame(api.V2016, vin17, 0x07, emptyBody{})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDecodeNormal(t *testing.T) {
	d := decodeFrame(loginFrame(t))
	if d.Err != nil || d.Kind != KindNormal || d.Cmd != 0x01 || d.VIN != vin17 {
		t.Fatalf("decode 异常: %+v err=%v", d, d.Err)
	}
}

func TestDecodeHeartbeatIsNormal(t *testing.T) {
	// 心跳(0x07)载荷类型在库中为 nil("no decodable body"),但属正常命令——
	// 必须判 normal,不得落入 unknown(评审 B3)
	if d := decodeFrame(heartbeatFrame(t)); d.Kind != KindNormal {
		t.Fatalf("心跳 kind = %s, want normal", d.Kind)
	}
}

func TestDecodeUnknownCommand(t *testing.T) {
	// 0x30 在上行预留区但不在服务端已知命令白名单 → unknown
	raw := loginFrame(t)
	raw[2] = 0x30
	raw[len(raw)-1] ^= 0x01 ^ 0x30 // 命令字节改动后同步修正 BCC(否则库 Decode 先报 BCC 错)
	d := decodeFrame(raw)
	if d.Err != nil {
		t.Fatalf("预留区命令应可帧解码: %v", d.Err)
	}
	if d.Kind != KindUnknown {
		t.Fatalf("kind = %s, want unknown", d.Kind)
	}
}

func TestDecodeEncryptedShortCircuit(t *testing.T) {
	// 加密帧(加密标志 ≠ 0x01)必须在进入 ProtocolCodec.Decode 前短路分类:
	// 库对非 0x01 加密直接报 ErrEncryptionNotSupported(评审 B2)
	raw := loginFrame(t)
	raw[21] = 0x02 // 加密方式字节位于偏移 21(2 起始 + 1 cmd + 1 resp + 17 VIN)
	d := decodeFrame(raw)
	if d.Err != nil {
		t.Fatalf("加密帧不应报解码错误: %v", d.Err)
	}
	if d.Kind != KindEncrypted || !d.Encrypted {
		t.Fatalf("加密帧 kind = %s, want encrypted", d.Kind)
	}
}

func TestDecodeBCCFail(t *testing.T) {
	raw := loginFrame(t)
	raw[len(raw)-1] ^= 0xFF // 破坏 BCC
	d := decodeFrame(raw)
	if d.Err == nil || d.PM != nil {
		t.Fatal("BCC 损坏应解码失败")
	}
}
