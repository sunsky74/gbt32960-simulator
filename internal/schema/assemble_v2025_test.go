package schema

import (
	"reflect"
	"testing"
	"time"

	"github.com/sunsky74/gb32960/api"
	_ "github.com/sunsky74/gb32960/codec/all"
	mdl "github.com/sunsky74/gb32960/model/gbt2025"
	mdl16 "github.com/sunsky74/gb32960/model/gbt2016"
	"github.com/sunsky74/gb32960/utils"
)

func decodeV2025Realtime(t *testing.T, body *mdl.RealTimeV2025Data) *mdl.RealTimeV2025Data {
	t.Helper()
	raw, err := body.Bytes()
	if err != nil {
		t.Fatalf("encode v2025 realtime: %v", err)
	}
	c := api.GetCodec(api.V2025, reflect.TypeOf((*mdl.RealTimeV2025Data)(nil)).Elem())
	if c == nil {
		t.Fatal("v2025 codec not registered")
	}
	m, err := c.Decode(utils.NewByteReader(raw))
	if err != nil {
		t.Fatalf("decode v2025 realtime: %v", err)
	}
	return m.(*mdl.RealTimeV2025Data)
}

func fullGroupsV2025() GroupsConfig {
	return GroupsConfig{
		"vehicle": {Enabled: true, Rows: []RowValue{{
			"operatingState": 1, "chargingState": 1, "operationMode": 1,
			"speed": 88.8, "mileage": 55555.5, "voltage": 350.5, "current": -120.5,
			"soc": 66, "dc": 1, "gear": 4, "drivingForce": true, "brakingForce": false,
		}}},
		"motor": {Enabled: true, Rows: []RowValue{
			{"seq": 1, "state": 1, "controllerTemp": 41.5, "speed": 5000, "torque": 210.5, "motorTemp": 55},
		}},
		"location": {Enabled: true, Rows: []RowValue{{"valid": true, "longitude": -121.4737, "latitude": 31.2304}}},
		"alarm": {Enabled: true, Rows: []RowValue{{
			"maxAlarmLevel": 2,
			"bits":          map[string]any{"bit0": true, "bit19": true, "bit27": true},
			"batteryFaults": []any{42.0},
		}}},
		"minparallel": {Enabled: true, Rows: []RowValue{{
			"batteryPackSeq": 2, "voltage": 350.5, "current": -120.5,
			"batteryVoltages": []any{3.331, 3.332, 3.333},
		}}},
		"batterytemp": {Enabled: true, Rows: []RowValue{{
			"batteryPackSeq": 2, "probeTemps": []any{25, 26, 27, 28},
		}}},
		"supercap": {Enabled: true, Rows: []RowValue{{
			"managementSystemNumber": 1, "totalVoltage": 48.5, "totalCurrent": -30.2,
			"capacitorVoltages":     []any{2.71, 2.72}, "probeTemperatures": []any{31, 32},
		}}},
		"supercapextremum": {Enabled: true, Rows: []RowValue{{
			"voltageMaxSubsystem": 1, "voltageMaxBattery": 3, "maxVoltage": 2.75,
			"voltageMinSubsystem": 1, "voltageMinBattery": 4, "minVoltage": 2.65,
			"tempMaxSubsystem": 1, "tempMaxProbe": 2, "maxTemp": 33,
			"tempMinSubsystem": 1, "tempMinProbe": 3, "minTemp": 30,
		}}},
	}
}

func TestAssembleV2025Roundtrip(t *testing.T) {
	at := time.Date(2026, 8, 28, 16, 0, 0, 0, time.Local)
	body, err := AssembleRealtimeV2025(fullGroupsV2025(), at)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if body.VehicleData == nil || body.MotorDataList == nil || body.LocationData == nil ||
		body.AlarmData == nil || body.MinParallelCellVoltages == nil ||
		body.BatteryPackTemperatures == nil || body.SuperCapacitorData == nil ||
		body.SuperCapacitorExtremumData == nil {
		t.Fatal("enabled groups must be assembled")
	}
	if body.FuelCellData != nil || body.EngineData != nil || body.FuelCellStackDataList != nil {
		t.Fatal("disabled groups must be nil")
	}

	got := decodeV2025Realtime(t, body)

	if got.BeanTime.String() != "2026-08-28 16:00:00" {
		t.Errorf("BeanTime = %s", got.BeanTime.String())
	}
	v := got.VehicleData
	if v.Speed != 88.8 || v.SOC != 66 || v.Current != -120.5 {
		t.Errorf("vehicle = speed:%v soc:%d cur:%v", v.Speed, v.SOC, v.Current)
	}
	if v.GearPosition.GP != 4 || !v.GearPosition.DrivingForceActive {
		t.Errorf("gear = %+v", v.GearPosition)
	}
	// V2025 整车无加速/制动踏板字段(codec 不写),检查编码长度侧效应由帧级测试覆盖

	m := got.MotorDataList
	if m.MotorCount != 1 || m.Items[0].MotorSpeed != 5000 || m.Items[0].MotorTorque != 210.5 {
		t.Errorf("motor = %+v", m.Items[0])
	}

	loc := got.LocationData
	if loc.OriginLongitude != -121.4737 || loc.OriginLatitude != 31.2304 {
		t.Errorf("location = %v/%v", loc.OriginLongitude, loc.OriginLatitude)
	}
	if loc.EastFlag || !loc.NorthernFlag {
		t.Errorf("hemisphere flags wrong (西经应 east=false, 北纬应 north=true): east=%v north=%v", loc.EastFlag, loc.NorthernFlag)
	}

	a := got.AlarmData
	if !a.TemperatureDifferential || !a.DriveMotorOverSpeed || !a.FuelCellStackOverTemperature {
		t.Errorf("v2025 alarm bits wrong (19/27): %+v", a)
	}
	if a.BatteryFaultNum != 1 || a.BatteryFaultDatas[0] != 42 {
		t.Errorf("faults = %+v", a.BatteryFaultDatas)
	}

	mp := got.MinParallelCellVoltages
	if mp.BatteryPackCount != 1 || mp.Items[0].BatteryPackSeq != 2 {
		t.Errorf("minparallel = %+v", mp)
	}
	if mp.Items[0].MinParallelUnits != 3 || mp.Items[0].BatteryVoltages[2] != 3.333 {
		t.Errorf("parallel volts = %+v", mp.Items[0].BatteryVoltages)
	}

	bt := got.BatteryPackTemperatures
	if bt.Items[0].TemperatureProbeCount != 4 || bt.Items[0].ProbeTemperatures[3] != 28 {
		t.Errorf("battery temps = %+v", bt.Items[0])
	}

	sc := got.SuperCapacitorData
	if sc.TotalVoltage != 48.5 || sc.CapacitorCount != 2 || sc.CapacitorVoltages[1] != 2.72 {
		t.Errorf("supercap = %+v", sc)
	}

	sce := got.SuperCapacitorExtremumData
	if sce.MaxVoltage != 2.75 || sce.MinTemperature != 30 {
		t.Errorf("supercap extremum = %+v", sce)
	}
}

func TestAssembleV2025MotorOffsetSemantics(t *testing.T) {
	// V2025 转速偏移 +32000:填 0 应可无损往返(0+32000 在 u16 范围内)
	cfg := GroupsConfig{
		"motor": {Enabled: true, Rows: []RowValue{{"seq": 1, "state": 3, "speed": 0, "torque": 0}}},
	}
	body, err := AssembleRealtimeV2025(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := decodeV2025Realtime(t, body)
	if got.MotorDataList.Items[0].MotorSpeed != 0 || got.MotorDataList.Items[0].MotorTorque != 0 {
		t.Errorf("motor zero roundtrip = %+v", got.MotorDataList.Items[0])
	}
}

func TestAssembleDispatchByVersion(t *testing.T) {
	// 同一份配置经分发器应产出对应版本的报文体
	cfg := GroupsConfig{
		"vehicle": {Enabled: true, Rows: []RowValue{{"soc": 50, "gear": 3}}},
	}
	b16, err := Assemble(api.V2016, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b25, err := Assemble(api.V2025, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b16.(*mdl16.RealTimeData); !ok {
		t.Errorf("v2016 dispatch type = %T", b16)
	}
	if _, ok := b25.(*mdl.RealTimeV2025Data); !ok {
		t.Errorf("v2025 dispatch type = %T", b25)
	}
}

func TestV2025GroupsSchemaSanity(t *testing.T) {
	groups := V2025Groups()
	keys := map[string]bool{}
	for _, g := range groups {
		keys[g.Key] = true
		if len(g.Fields) == 0 {
			t.Errorf("group %s has no fields", g.Key)
		}
	}
	for _, want := range []string{"vehicle", "motor", "fuelcell", "engine", "location", "alarm", "minparallel", "batterytemp", "fcstack", "supercap", "supercapextremum"} {
		if !keys[want] {
			t.Errorf("missing group %s", want)
		}
	}
	// 2016 独有组不应出现在 2025
	for _, gone := range []string{"extremum", "voltage", "temperature"} {
		if keys[gone] {
			t.Errorf("2016-only group %s leaked into v2025 schema", gone)
		}
	}
	// 报警位组应为 28 位
	for _, g := range groups {
		if g.Key == "alarm" {
			for _, f := range g.Fields {
				if f.Kind == "bitgroup" && len(f.Bits) != 28 {
					t.Errorf("v2025 alarm bits = %d, want 28", len(f.Bits))
				}
			}
		}
	}
}
