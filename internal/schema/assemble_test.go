package schema

import (
	"reflect"
	"testing"
	"time"

	"gbt32960-simulator/internal/engine"
	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/frame"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	mdlrt16 "github.com/sunsky74/gb32960/model/gbt2016/realtime"
	mdlrt25 "github.com/sunsky74/gb32960/model/gbt2025/realtime"
	"github.com/sunsky74/gb32960/utils"
)

func decodeRealtime(t *testing.T, body *mdl.RealTimeData) *mdl.RealTimeData {
	t.Helper()
	raw, err := body.Bytes()
	if err != nil {
		t.Fatalf("encode realtime: %v", err)
	}
	c := api.GetCodec(api.V2016, reflect.TypeOf((*mdl.RealTimeData)(nil)).Elem())
	if c == nil {
		t.Fatal("codec not registered (missing blank import)")
	}
	m, err := c.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("decode realtime: %v", err)
	}
	return m.(*mdl.RealTimeData)
}

func fullGroups() GroupsConfig {
	return GroupsConfig{
		"vehicle": {Enabled: true, Rows: []RowValue{{
			"operatingState": 1, "chargingState": 1, "operationMode": 1,
			"speed": 66.6, "mileage": 12345.6, "voltage": 400.5, "current": -50.2,
			"soc": 78, "dc": 1, "gear": 4, "drivingForce": true, "brakingForce": false,
			"insulance": 3000, "accelerationValue": 22, "brakePedal": 5,
		}}},
		"motor": {Enabled: true, Rows: []RowValue{
			{"seq": 1, "state": 1, "controllerTemp": 35.5, "speed": 3000, "torque": 120.5, "motorTemp": 42, "controllerVoltage": 380.2, "controllerCurrent": -30},
			{"seq": 2, "state": 2, "controllerTemp": 36.5, "speed": 1500.5, "torque": -20.3, "motorTemp": 41, "controllerVoltage": 379.1, "controllerCurrent": 15.5},
		}},
		"location": {Enabled: true, Rows: []RowValue{{"valid": true, "longitude": 121.4737, "latitude": 31.2304}}},
		"extremum": {Enabled: true, Rows: []RowValue{{
			"voltageMaxSubsystem": 1, "voltageMaxBattery": 12, "maxVoltage": 3.65,
			"voltageMinSubsystem": 1, "voltageMinBattery": 13, "minVoltage": 3.55,
			"tempMaxSubsystem": 1, "tempMaxProbe": 3, "maxTemp": 38.5,
			"tempMinSubsystem": 1, "tempMinProbe": 4, "minTemp": 22.5,
		}}},
		"alarm": {Enabled: true, Rows: []RowValue{{
			"maxAlarmLevel": 1,
			"bits":          map[string]any{"bit0": true, "bit4": true, "bit18": true},
			"batteryFaults": []any{100.0, 200.0},
			"otherFaults":   []any{300.0},
		}}},
		"voltage": {Enabled: true, Rows: []RowValue{{
			"subsystem": 1, "voltage": 400.5, "current": -50.2, "batteryTotal": 96,
			"frameStartSeq": 1, "batteryVoltages": []any{3.65, 3.64, 3.63},
		}}},
		"temperature": {Enabled: true, Rows: []RowValue{{
			"subsystem": 1, "probeTemps": []any{22.5, 23.0, 24.5, 25.0},
		}}},
	}
}

func TestAssembleRoundtripAllGroups(t *testing.T) {
	at := time.Date(2026, 8, 26, 10, 30, 15, 0, time.Local)
	body, err := AssembleRealtime(fullGroups(), at)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if body.VehicleData == nil || body.MotorDataList == nil || body.LocationData == nil ||
		body.ExtremumData == nil || body.AlarmData == nil ||
		body.ChargeableSubsystemElectricList == nil || body.ChargeableSubsystemTemperatureList == nil {
		t.Fatal("enabled groups must be assembled")
	}
	if body.FuelCellData != nil || body.EngineData != nil {
		t.Fatal("disabled groups must be nil")
	}

	got := decodeRealtime(t, body)

	if got.BeanTime.String() != "2026-08-26 10:30:15" {
		t.Errorf("BeanTime = %s", got.BeanTime.String())
	}
	if got.VehicleData.Speed != 66.6 || got.VehicleData.SOC != 78 {
		t.Errorf("vehicle speed/soc = %v/%v", got.VehicleData.Speed, got.VehicleData.SOC)
	}
	if got.VehicleData.GearPosition.GP != 4 {
		t.Errorf("gear = %v", got.VehicleData.GearPosition.GP)
	}
	if !got.VehicleData.GearPosition.DrivingForceActive || got.VehicleData.GearPosition.BrakingTorqueApplied {
		t.Errorf("gear flags = %v/%v", got.VehicleData.GearPosition.DrivingForceActive, got.VehicleData.GearPosition.BrakingTorqueApplied)
	}
	if len(got.MotorDataList.Items) != 2 || got.MotorDataList.Items[0].MotorTorque != 120.5 {
		t.Errorf("motor list wrong: %+v", got.MotorDataList)
	}
	if got.LocationData.Longitude != 121.4737 || got.LocationData.Latitude != 31.2304 {
		t.Errorf("location = %v/%v", got.LocationData.Longitude, got.LocationData.Latitude)
	}
	if got.ExtremumData.MaxVoltage != 3.65 || got.ExtremumData.MinTemperature != 22 {
		t.Errorf("extremum wrong: %+v", got.ExtremumData)
	}
	if got.ExtremumData.VoltageMaxSubsystem != 1 || got.ExtremumData.TemperatureMaxProbe != 3 {
		t.Errorf("extremum subsystem ints wrong: %+v", got.ExtremumData)
	}
	if !got.AlarmData.TemperatureDifferential || !got.AlarmData.SocLow || !got.AlarmData.DeviceTypeOverFilling {
		t.Errorf("alarm bits wrong, mask=%b", got.AlarmData.AlarmBitIdentify)
	}
	if got.AlarmData.BatteryFaultNum != 2 || got.AlarmData.OtherFaultNum != 1 {
		t.Errorf("alarm fault counts = %d/%d", got.AlarmData.BatteryFaultNum, got.AlarmData.OtherFaultNum)
	}
	volt := got.ChargeableSubsystemElectricList.Items[0]
	if volt.Voltage != 400.5 || len(volt.BatteryVoltages) != 3 || volt.BatteryVoltages[2] != 3.63 {
		t.Errorf("subsystem electric wrong: %+v", volt)
	}
	if len(got.ChargeableSubsystemTemperatureList.Items[0].ProbeTemperatures) != 4 {
		t.Errorf("probe temps wrong: %+v", got.ChargeableSubsystemTemperatureList)
	}
}

func TestAssembleDisabledGroupsOmitted(t *testing.T) {
	cfg := GroupsConfig{
		"vehicle": {Enabled: true, Rows: []RowValue{{"soc": 50, "gear": 3}}},
	}
	body, err := AssembleRealtime(cfg, time.Now())
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if body.VehicleData == nil {
		t.Fatal("vehicle should be present")
	}
	if body.AlarmData != nil || body.MotorDataList != nil {
		t.Fatal("omitted groups must be nil")
	}
}

func TestAssembleInvalidSOC(t *testing.T) {
	cfg := GroupsConfig{
		"vehicle": {Enabled: true, Rows: []RowValue{{"soc": 150}}},
	}
	if _, err := AssembleRealtime(cfg, time.Now()); err == nil {
		t.Fatal("soc=150 should fail validation")
	}
}

// TestV2016DocCompliance 锁定 2016 文档语义修复:
// 挡位按附录 A.1 位域编码、充电状态含 0x04 充电完成、储能电压超 200 单体自动拆帧。
func TestV2016DocCompliance(t *testing.T) {
	t.Run("gear A.1 nibble", func(t *testing.T) {
		cfg := GroupsConfig{"vehicle": {Enabled: true, Rows: []RowValue{
			{"gear": 0x0F, "drivingForce": true, "brakingForce": true},
		}}}
		body, err := AssembleRealtime(cfg, time.Now())
		if err != nil {
			t.Fatalf("assemble P挡: %v", err)
		}
		// bit5 驱动力 | bit4 制动力 | nibble 0xF(停车P挡)
		if got := body.VehicleData.GearPosition.Origin; got != 0x3F {
			t.Errorf("P挡 origin = 0x%02X, want 0x3F", got)
		}
		d := decodeRealtime(t, body)
		if d.VehicleData.GearPosition.GP != 0x0F ||
			!d.VehicleData.GearPosition.DrivingForceActive ||
			!d.VehicleData.GearPosition.BrakingTorqueApplied {
			t.Errorf("P挡 decode = %+v", d.VehicleData.GearPosition)
		}

		cfg["vehicle"] = GroupConfig{Enabled: true, Rows: []RowValue{{"gear": 0x00}}}
		body, err = AssembleRealtime(cfg, time.Now())
		if err != nil {
			t.Fatalf("assemble 空挡: %v", err)
		}
		if got := body.VehicleData.GearPosition.Origin; got != 0x00 {
			t.Errorf("空挡 origin = 0x%02X, want 0x00", got)
		}

		// A.1 未定义的挡位码(如 0x07)应被拒绝
		cfg["vehicle"] = GroupConfig{Enabled: true, Rows: []RowValue{{"gear": 0x07}}}
		if _, err := AssembleRealtime(cfg, time.Now()); err == nil {
			t.Error("gear=0x07 (A.1 预留) should fail validation")
		}
	})

	t.Run("charging state 0x04 充电完成", func(t *testing.T) {
		cfg := GroupsConfig{"vehicle": {Enabled: true, Rows: []RowValue{{"chargingState": 4}}}}
		body, err := AssembleRealtime(cfg, time.Now())
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if got := byte(body.VehicleData.ChargingState); got != 0x04 {
			t.Errorf("chargingState = 0x%02X, want 0x04", got)
		}
		d := decodeRealtime(t, body)
		if got := byte(d.VehicleData.ChargingState); got != 0x04 {
			t.Errorf("roundtrip chargingState = 0x%02X, want 0x04", got)
		}
	})

	t.Run("voltage frames split at 200 cells", func(t *testing.T) {
		volts := make([]any, 250)
		for i := range volts {
			volts[i] = 3.5
		}
		cfg := GroupsConfig{"voltage": {Enabled: true, Rows: []RowValue{{
			"subsystem": 1, "voltage": 400, "current": -10, "batteryTotal": 250,
			"frameStartSeq": 1, "batteryVoltages": volts,
		}}}}
		body, err := AssembleRealtime(cfg, time.Now())
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		l := body.ChargeableSubsystemElectricList
		if l.ElectricCount != 2 || len(l.Items) != 2 {
			t.Fatalf("expect 2 frames after split, got count=%d items=%d", l.ElectricCount, len(l.Items))
		}
		if l.Items[0].BatteryCount != 200 || l.Items[0].FrameStartBatterySeq != 1 {
			t.Errorf("frame0 = %d cells @seq %d, want 200 @1", l.Items[0].BatteryCount, l.Items[0].FrameStartBatterySeq)
		}
		if l.Items[1].BatteryCount != 50 || l.Items[1].FrameStartBatterySeq != 201 {
			t.Errorf("frame1 = %d cells @seq %d, want 50 @201", l.Items[1].BatteryCount, l.Items[1].FrameStartBatterySeq)
		}
		// 整帧编解码闭环:线上按个数读条目,拆帧后仍应可完整解码
		d := decodeRealtime(t, body)
		if len(d.ChargeableSubsystemElectricList.Items) != 2 ||
			len(d.ChargeableSubsystemElectricList.Items[0].BatteryVoltages) != 200 {
			t.Errorf("roundtrip split frames wrong: %+v", d.ChargeableSubsystemElectricList)
		}
	})
}

// TestAlarmBoolSettersReflect 锁定反射写入口径:两版已知字段均可写入,
// 未知字段两版均返回错误(v2016 旧 switch 实现曾静默忽略)。
func TestAlarmBoolSettersReflect(t *testing.T) {
	a := &mdlrt16.AlarmData{}
	if err := setAlarmBool(a, "SocLow", true); err != nil || !a.SocLow {
		t.Fatalf("v2016 已知字段写入失败: err=%v", err)
	}
	if err := setAlarmBool(a, "NoSuchAlarmBit", true); err == nil {
		t.Fatal("v2016 未知字段应返回错误")
	}

	b := &mdlrt25.AlarmV2025Data{}
	if err := setAlarmV2025Bool(b, "DCStatus", true); err != nil || !b.DCStatus {
		t.Fatalf("v2025 已知字段写入失败: err=%v", err)
	}
	if err := setAlarmV2025Bool(b, "NoSuchAlarmBit", true); err == nil {
		t.Fatal("v2025 未知字段应返回错误")
	}
}

// TestFrameRoundtripViaEngine 验证 engine.BuildFrame → 协议库解码 的整帧闭环。
func TestFrameRoundtripViaEngine(t *testing.T) {
	at := time.Date(2026, 8, 26, 10, 0, 0, 0, time.Local)
	body, err := AssembleRealtime(fullGroups(), at)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	raw, cmd, err := engine.BuildFrame(api.V2016, "LSV00000000000001", 0x02, body)
	if err != nil {
		t.Fatalf("build frame: %v", err)
	}
	if cmd != "0x02 REAL_TIME" {
		t.Errorf("cmd = %s", cmd)
	}
	msg, err := codec.ProtocolCodec.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	pm, ok := msg.(*frame.ProtocolMessage)
	if !ok {
		t.Fatalf("unexpected message type %T", msg)
	}
	if err := pm.DecodePayload(); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if pm.VIN != "LSV00000000000001" {
		t.Errorf("vin = %s", pm.VIN)
	}
}
