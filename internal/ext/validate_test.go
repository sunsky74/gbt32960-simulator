package ext

import (
	"encoding/json"
	"strings"
	"testing"
)

func validPack() *Pack {
	var p Pack
	if err := json.Unmarshal([]byte(demoPackJSON), &p); err != nil {
		panic(err)
	}
	return &p
}

func TestValidateOK(t *testing.T) {
	if err := Validate(validPack()); err != nil {
		t.Fatalf("合法包不应报错: %v", err)
	}
}

func TestValidate2025ReservedCommandOK(t *testing.T) {
	p := validPack()
	p.Meta.BaseVersion = "2025"
	p.Commands[0].Code = 0x0C // 2025 上行预留区起点,应合法
	if err := Validate(p); err != nil {
		t.Fatalf("2025 + 0x0C 应合法: %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Pack)
		wantPath string
	}{
		{"meta.id 非法", func(p *Pack) { p.Meta.ID = "Demo!" }, "meta.id"},
		{"meta.label 空", func(p *Pack) { p.Meta.Label = " " }, "meta.label"},
		{"baseVersion 非法", func(p *Pack) { p.Meta.BaseVersion = "2017" }, "meta.baseVersion"},
		{"unitCode 撞 2016 标准", func(p *Pack) { p.Realtime.AppendUnits[0].UnitCode = 9 }, "realtime.appendUnits[0].unitCode"},
		{"unitCode 在预留区 0x0A", func(p *Pack) { p.Realtime.AppendUnits[0].UnitCode = 0x0A }, "realtime.appendUnits[0].unitCode"},
		{"unitCode 0xFF 越自定义区", func(p *Pack) { p.Realtime.AppendUnits[0].UnitCode = 0xFF }, "realtime.appendUnits[0].unitCode"},
		{"unitCode 越上界", func(p *Pack) { p.Realtime.AppendUnits[0].UnitCode = 256 }, "realtime.appendUnits[0].unitCode"},
		{"同包 unitCode 重复", func(p *Pack) {
			dup := p.Realtime.AppendUnits[0]
			dup.Key = "telemetry2"
			p.Realtime.AppendUnits = append(p.Realtime.AppendUnits, dup)
		}, "realtime.appendUnits[1].unitCode"},
		{"maxRows 为负", func(p *Pack) { p.Realtime.AppendUnits[0].MaxRows = -1 }, "realtime.appendUnits[0].maxRows"},
		{"单元 key 重复", func(p *Pack) {
			dup := p.Realtime.AppendUnits[0]
			p.Realtime.AppendUnits = append(p.Realtime.AppendUnits, dup)
		}, "realtime.appendUnits[1].key"},
		{"单元无字段", func(p *Pack) { p.Realtime.AppendUnits[0].Fields = nil }, "realtime.appendUnits[0].fields"},
		{"字段类型未知", func(p *Pack) { p.Realtime.AppendUnits[0].Fields[0].Type = "s8" }, "realtime.appendUnits[0].fields[0].type"},
		{"字段 key 重复", func(p *Pack) {
			p.Realtime.AppendUnits[0].Fields[1].Key = p.Realtime.AppendUnits[0].Fields[0].Key
		}, "realtime.appendUnits[0].fields[1].key"},
		{"scale 非正", func(p *Pack) { p.Realtime.AppendUnits[0].Fields[1].Scale = new(float64) }, "realtime.appendUnits[0].fields[1].scale"},
		{"f32 带 scale", func(p *Pack) {
			p.Realtime.AppendUnits[0].Fields[0].Type = "f32"
			p.Realtime.AppendUnits[0].Fields[0].Scale = new(float64)
			*p.Realtime.AppendUnits[0].Fields[0].Scale = 1
		}, "realtime.appendUnits[0].fields[0].scale"},
		{"bits 带 scale", func(p *Pack) {
			p.Realtime.AppendUnits[0].Fields[3].Scale = new(float64)
			*p.Realtime.AppendUnits[0].Fields[3].Scale = 1
		}, "realtime.appendUnits[0].fields[3].scale"},
		{"bits 索引越界", func(p *Pack) {
			p.Realtime.AppendUnits[0].Fields[3].Bits[0].Index = 32
		}, "realtime.appendUnits[0].fields[3].bits[0].index"},
		{"bits 索引重复", func(p *Pack) {
			p.Realtime.AppendUnits[0].Fields[3].Bits[1].Index = 0
		}, "realtime.appendUnits[0].fields[3].bits[1].index"},
		{"bytes 长度 0", func(p *Pack) { p.Realtime.AppendUnits[0].Fields[4].Length = 0 }, "realtime.appendUnits[0].fields[4].length"},
		{"命令码撞 2016 标准", func(p *Pack) { p.Commands[0].Code = 2 }, "commands[0].code"},
		{"命令码在下行区", func(p *Pack) { p.Commands[0].Code = 0x8A }, "commands[0].code"},
		{"direction down", func(p *Pack) { p.Commands[0].Direction = "down" }, "commands[0].direction"},
		{"trigger 非法", func(p *Pack) { p.Commands[0].Trigger = "auto" }, "commands[0].trigger"},
		{"body.type 非法", func(p *Pack) { p.Commands[0].Body.Type = "raw" }, "commands[0].body.type"},
		{"命令 key 与单元重复", func(p *Pack) { p.Commands[0].Key = "telemetry" }, "commands[0].key"},
		{"2025 unitCode 撞标准(电堆 0x30)", func(p *Pack) {
			p.Meta.BaseVersion = "2025"
			p.Realtime.AppendUnits[0].UnitCode = 0x30
		}, "realtime.appendUnits[0].unitCode"},
		{"2025 命令码撞标准", func(p *Pack) {
			p.Meta.BaseVersion = "2025"
			p.Commands[0].Code = 0x0B
		}, "commands[0].code"},
		{"单元 key 为空", func(p *Pack) { p.Realtime.AppendUnits[0].Key = "" }, "realtime.appendUnits[0].key"},
		{"命令 key 为空", func(p *Pack) { p.Commands[0].Key = " " }, "commands[0].key"},
		{"offset 非整数", func(p *Pack) {
			p.Realtime.AppendUnits[0].Fields[2].Offset = ptrf(0.5)
		}, "realtime.appendUnits[0].fields[2].offset"},
		{"命令体 offset 非整数", func(p *Pack) {
			p.Commands[0].Body.Fields[0].Offset = ptrf(0.5)
		}, "commands[0].body.fields[0].offset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validPack()
			tc.mutate(p)
			err := Validate(p)
			if err == nil {
				t.Fatalf("期望报错,实际 nil")
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Fatalf("错误 %q 未包含路径 %q", err.Error(), tc.wantPath)
			}
		})
	}
}
