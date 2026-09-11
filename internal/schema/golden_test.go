package schema

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/frame"
	"gbt32960-simulator/internal/engine"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	"github.com/sunsky74/gb32960/utils"
)

// loadGolden 读取生产报文金标准 hex。
// 来源: 协议库 golden 测试集 —— 真实国标终端发出、网关日志采集的报文。
func loadGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Skipf("golden file missing: %v", err)
	}
	data, err := utils.HexToBytes(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	return data
}

// TestGoldenReencodeByteIdentical 是编解码正确性的终极验证:
// 生产报文 → 解码 → 重新编码 → 逐字节相等。
// 字段顺序/宽度/转换器/BCC 任何偏差都会导致字节不等。
func TestGoldenReencodeByteIdentical(t *testing.T) {
	for _, name := range []string{
		"prod_login_v2016_01.hex",
		"prod_logout_v2016_01.hex",
		"prod_realtime_v2016_01.hex",
		"prod_realtime_v2025_01.hex",
		"prod_reissue_v2025_01.hex",
	} {
		t.Run(name, func(t *testing.T) {
			raw := loadGolden(t, name)

			msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
			if err != nil {
				t.Fatalf("decode frame: %v", err)
			}
			pm := msg.(*frame.ProtocolMessage)
			if err := pm.DecodePayload(); err != nil {
				t.Fatalf("decode payload: %v", err)
			}

			reencoded, err := pm.Bytes()
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}

			if len(reencoded) != len(raw) {
				t.Fatalf("length mismatch: got %d bytes, want %d", len(reencoded), len(raw))
			}
			for i := range raw {
				if reencoded[i] != raw[i] {
					t.Fatalf("byte %d/%d mismatch: got %02X, want %02X",
						i, len(raw), reencoded[i], raw[i])
				}
			}
		})
	}
}

// TestGoldenBeanTimeIsDecimalNotBCD 线上时间为十进制字节而非 BCD:
// login 报文 1a 07 07 06 37 0f = 2026-07-07 06:55:15。
func TestGoldenBeanTimeIsDecimalNotBCD(t *testing.T) {
	raw := loadGolden(t, "prod_login_v2016_01.hex")
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	pm := msg.(*frame.ProtocolMessage)
	if err := pm.DecodePayload(); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	login, ok := pm.Payload.(*mdl.VehicleLogin)
	if !ok {
		t.Fatalf("payload type %T", pm.Payload)
	}
	bt := login.BeanTime
	if bt.Year != 0x1a || bt.Month != 7 || bt.Day != 7 || bt.Hour != 6 || bt.Minute != 0x37 || bt.Second != 0x0f {
		t.Fatalf("BeanTime = %+v, want raw decimal 26/7/7/6/55/15", bt)
	}
	if len(login.ICCID) != 20 {
		t.Errorf("ICCID len = %d, want 20", len(login.ICCID))
	}
}

// TestGoldenChargeableCurrentSemantics 储能电流语义(offset 修复后):
// 生产报文储能电流 raw=0x2729(10025) 解码 = 2.5A,与同一报文整车总电流
// (raw=0x2729, 2.5A)完全一致 —— 同车同电流,物理自洽。
// 修复前(offset −1000)该字段解出 1202.5A,与整车电流自相矛盾。
func TestGoldenChargeableCurrentSemantics(t *testing.T) {
	raw := loadGolden(t, "prod_realtime_v2016_01.hex")
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	pm := msg.(*frame.ProtocolMessage)
	if err := pm.DecodePayload(); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	rt, ok := pm.Payload.(*mdl.RealTimeData)
	if !ok {
		t.Fatalf("payload type %T", pm.Payload)
	}
	if rt.VehicleData == nil {
		t.Fatal("vehicle data missing")
	}
	el := rt.ChargeableSubsystemElectricList
	if el == nil || len(el.Items) == 0 {
		t.Fatal("chargeable electric list missing")
	}

	vehicleCur := rt.VehicleData.Current
	chargeCur := el.Items[0].Current
	if chargeCur < -1000 || chargeCur > 1000 {
		t.Fatalf("chargeable current = %.1fA — outside physical range, offset sign wrong", chargeCur)
	}
	if diff := chargeCur - vehicleCur; diff < -0.11 || diff > 0.11 {
		t.Errorf("chargeable current %.2fA != vehicle total current %.2fA (same car, same wire raw 0x2729)", chargeCur, vehicleCur)
	}
	t.Logf("vehicle current = %.2fA, chargeable current = %.2fA (both raw 0x2729)", vehicleCur, chargeCur)
}

// TestSimulatorFrameMatchesGoldenShape 模拟器产出帧与生产帧的帧头布局对照。
func TestSimulatorFrameMatchesGoldenShape(t *testing.T) {
	loadGolden(t, "prod_realtime_v2016_01.hex") // 存在性检查,缺失则 skip

	cfg := GroupsConfig{
		"vehicle": {Enabled: true, Rows: []RowValue{{
			"operatingState": 1, "chargingState": 1, "operationMode": 1,
			"speed": 30, "soc": 60, "gear": 3,
		}}},
	}
	body, err := AssembleRealtime(cfg, time.Date(2026, 8, 26, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	sim, _, err := engine.BuildFrame(api.V2016, "H3V21AA24RZ016158", 0x02, body)
	if err != nil {
		t.Fatalf("build frame: %v", err)
	}

	wantHead := []byte{0x23, 0x23, 0x02, 0xFE}
	for i, w := range wantHead {
		if sim[i] != w {
			t.Errorf("head[%d] = %02X, want %02X", i, sim[i], w)
		}
	}
	if vin := string(sim[4:21]); vin != "H3V21AA24RZ016158" {
		t.Errorf("vin = %q", vin)
	}
	if sim[21] != 0x01 {
		t.Errorf("encryption = %02X, want 01 (不加密)", sim[21])
	}
	payloadLen := int(sim[22])<<8 | int(sim[23])
	if payloadLen != len(sim)-25 {
		t.Errorf("payloadLen field = %d, actual %d", payloadLen, len(sim)-25)
	}
}
