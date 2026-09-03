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
		{"命令码在下行区", func(p *Pack) { p.Commands[0].Code = 0x83 }, "commands[0].code"},
		{"0x8A 缺子指令码", func(p *Pack) { p.Commands[0].Code = 0x8A }, "commands[0].remoteSub"},
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

// TestValidateNewGuardErrors 评审 A/B 新增门禁:命令码同包唯一、key 禁冒号、标准组键保留。
func TestValidateNewGuardErrors(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Pack)
		wantPath string
		wantSub  string
	}{
		{"命令码同包重复", func(p *Pack) {
			dup := p.Commands[0]
			dup.Key = "extData2"
			p.Commands = append(p.Commands, dup)
		}, "commands[1].code", "命令码重复"},
		{"命令 key 含冒号", func(p *Pack) { p.Commands[0].Key = "ext:data" }, "commands[0].key", "冒号"},
		{"单元 key 含冒号", func(p *Pack) { p.Realtime.AppendUnits[0].Key = "tele:metry" }, "realtime.appendUnits[0].key", "冒号"},
		{"命令 key 撞标准组保留键", func(p *Pack) { p.Commands[0].Key = "vehicle" }, "commands[0].key", "标准报文组"},
		{"单元 key 撞标准组保留键", func(p *Pack) { p.Realtime.AppendUnits[0].Key = "vehicle" }, "realtime.appendUnits[0].key", "标准报文组"},
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
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误 %q 未包含子串 %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestValidateMultiCommandDistinctCodes 命令码不同的多命令包应通过(extcmd 夹具同构)。
func TestValidateMultiCommandDistinctCodes(t *testing.T) {
	p := validPack()
	p.Commands = append(p.Commands, Command{
		Key: "extReport0A", Label: "扩展报表", Code: 0x0A,
		Direction: "up", Trigger: "manual",
		Body: CommandBody{Type: "fields", Fields: []FieldSpec{{Key: "seq", Label: "流水号", Type: "u16"}}},
	})
	if err := Validate(p); err != nil {
		t.Fatalf("多命令不同码应通过: %v", err)
	}
}

func TestValidateScope(t *testing.T) {
	mk := func(scope []string) *Pack {
		return &Pack{Meta: Meta{ID: "ok-pack", Label: "x", BaseVersion: "2016", Scope: scope}}
	}
	if err := Validate(mk(nil)); err != nil {
		t.Errorf("缺省 scope 应合法: %v", err)
	}
	if err := Validate(mk([]string{"client", "parser"})); err != nil {
		t.Errorf("client+parser 应合法: %v", err)
	}
	if err := Validate(mk([]string{"parser"})); err != nil {
		t.Errorf("仅 parser 应合法: %v", err)
	}
	if err := Validate(mk([]string{"server"})); err == nil {
		t.Error("非法 scope 值应报错")
	}
	if err := Validate(mk([]string{"client", "client"})); err == nil {
		t.Error("重复 scope 应报错")
	}
}

func TestScopeHas(t *testing.T) {
	p := &Pack{Meta: Meta{Scope: []string{ScopeParser}}}
	if !ScopeHas(p, ScopeParser) || ScopeHas(p, ScopeClient) {
		t.Error("scope=[parser] 判定错误")
	}
	def := &Pack{}
	if !ScopeHas(def, ScopeClient) || ScopeHas(def, ScopeParser) {
		t.Error("缺省 scope 应视为仅 client")
	}
	if ScopeHas(nil, ScopeClient) {
		t.Error("nil 包应返回 false")
	}
}
