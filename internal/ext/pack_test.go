package ext

import (
	"encoding/json"
	"testing"
)

const demoPackJSON = `{
	"meta": {"id": "demo", "label": "演示包", "vendor": "Demo", "baseVersion": "2016"},
	"realtime": {"appendUnits": [{
		"key": "telemetry", "title": "私有遥测", "unitCode": 128,
		"enabled": false, "multiple": false, "maxRows": 10,
		"fields": [
			{"key": "soc2", "label": "SOC2", "type": "u8", "unit": "%"},
			{"key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V"},
			{"key": "temp", "label": "温度", "type": "i16", "offset": 40, "unit": "°C"},
			{"key": "flags", "label": "标志", "type": "bits", "bits": [{"index": 0, "label": "充电"}, {"index": 1, "label": "加热"}]},
			{"key": "sn", "label": "序列号", "type": "bytes", "length": 2}
		]
	}]},
	"commands": [{
		"key": "extData09", "label": "扩展数据", "code": 9,
		"direction": "up", "trigger": "manual+periodic",
		"body": {"type": "fields", "fields": [{"key": "seq", "label": "流水号", "type": "u16"}]}
	}]
}`

func TestUnmarshalPack(t *testing.T) {
	var p Pack
	if err := json.Unmarshal([]byte(demoPackJSON), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Meta.ID != "demo" || p.Meta.Vendor != "Demo" || p.Meta.BaseVersion != "2016" {
		t.Fatalf("meta = %+v", p.Meta)
	}
	u := p.Realtime.AppendUnits[0]
	if u.Key != "telemetry" || u.UnitCode != 128 || u.Multiple || len(u.Fields) != 5 {
		t.Fatalf("unit = %+v", u)
	}
	if u.Fields[1].Scale == nil || *u.Fields[1].Scale != 0.1 {
		t.Fatalf("scale = %+v", u.Fields[1].Scale)
	}
	if u.Fields[2].Offset == nil || *u.Fields[2].Offset != 40 {
		t.Fatalf("offset = %+v", u.Fields[2].Offset)
	}
	if len(u.Fields[3].Bits) != 2 || u.Fields[3].Bits[1].Label != "加热" {
		t.Fatalf("bits = %+v", u.Fields[3].Bits)
	}
	c := p.Commands[0]
	if c.Code != 9 || c.Direction != "up" || c.Trigger != "manual+periodic" || c.Body.Type != "fields" || len(c.Body.Fields) != 1 {
		t.Fatalf("command = %+v", c)
	}
}
