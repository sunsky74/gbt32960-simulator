package ext

import (
	"testing"
)

func TestCompileUnit(t *testing.T) {
	g := CompileUnit(demoUnit())
	if g.Key != "telemetry" || g.Title != "私有遥测" || g.Enabled || g.Multiple || g.MaxRows != 10 {
		t.Fatalf("group = %+v", g)
	}
	// packVolt(scale=0.1)→ float(仓库约定 ×0.1→float);temp(offset=40,scale=1)→ int
	wantKinds := []string{"int", "float", "int", "bitgroup", "bytes"}
	for i, want := range wantKinds {
		if g.Fields[i].Kind != want {
			t.Fatalf("fields[%d].Kind = %q want %q", i, g.Fields[i].Kind, want)
		}
	}
	volt := g.Fields[1] // u16 scale 0.1 → 物理量程 0~6553.5
	if volt.Min == nil || *volt.Min != 0 || volt.Max == nil || *volt.Max != 6553.5 {
		t.Fatalf("volt min/max = %v/%v", volt.Min, volt.Max)
	}
	temp := g.Fields[2] // i16 offset 40 → -32768+40 ~ 32767+40
	if temp.Min == nil || *temp.Min != -32728 || temp.Max == nil || *temp.Max != 32807 {
		t.Fatalf("temp min/max = %v/%v", temp.Min, temp.Max)
	}
	if len(g.Fields[3].Bits) != 2 || g.Fields[3].Bits[0].Index != 0 {
		t.Fatalf("bits = %+v", g.Fields[3].Bits)
	}
	if g.Fields[4].Kind != "bytes" || g.Fields[4].Length != 2 {
		t.Fatalf("bytes 字段 = %+v(Length 应透传为 2)", g.Fields[4])
	}
	if g.Fields[1].ScaleNote != "×0.1" {
		t.Fatalf("scaleNote = %q", g.Fields[1].ScaleNote)
	}
	if g.Fields[2].ScaleNote != "偏移+40" {
		t.Fatalf("scaleNote = %q", g.Fields[2].ScaleNote)
	}
}

func TestDefaultsFor(t *testing.T) {
	row := DefaultsFor(demoUnit())
	if row["temp"] != 40.0 { // offset 40 → 线值 0
		t.Fatalf("temp 默认 = %v want 40", row["temp"])
	}
	flags, ok := row["flags"].(map[string]any)
	if !ok || flags["bit0"] != false || flags["bit1"] != false {
		t.Fatalf("flags 默认 = %+v", row["flags"])
	}
	if row["sn"] != "0000" {
		t.Fatalf("sn 默认 = %v", row["sn"])
	}
	if _, err := EncodeUnit(demoUnit(), row); err != nil { // 默认值可完整编码
		t.Fatalf("默认值编码失败: %v", err)
	}
}
