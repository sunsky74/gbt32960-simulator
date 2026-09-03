# Phase 1 · 扩展包核心引擎(internal/ext)Implementation Plan

> **For agentic workers:** Recommended execution: use superpowers:ltdd for Quality-LTDD. Alternatives: superpowers:subagent-driven-development, superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 交付纯 Go 包 `internal/ext`:扩展包类型定义、JSON 加载、带路径的静态校验、干跑、字段 DSL 编码器(黄金字节级)、GroupSchema 编译器——P2~P4 的全部地基。

**Architecture:** 独立新包,不触碰任何现有模块;消费 `internal/schema` 的导出类型(GroupSchema/FieldSchema/RowValue);数值契约 `物理值 = 线值×scale + offset`,反向编码,非整数报错。

**Tech Stack:** Go 1.25(标准库 encoding/json / math / os / regexp;errors.Join)

**Master 索引:** `docs/superpowers/plans/2026-08-31-extpack-master-plan.md`
**设计契约:** `docs/superpowers/specs/2026-08-31-extpack-design.md` §4(类型集/码位规则/编码规则)

## Global Constraints(继承 Master)

- 协议库 gb32960-go 零改动;除一处外不修改任何现有文件——`internal/schema/schema.go` 为 `FieldSchema` 增加 `Length int json:"length,omitempty"` 字段(标准组无 bytes 字段,零行为影响,服务 bytes 长度透传);其余只新增 `internal/ext/` 与 `testdata`
- 字段类型集冻结:u8/u16/u32/i8/i16/i32/f32/bits/bytes;scale 缺省 1、offset 缺省 0、scale≤0 非法、f32 禁 scale/offset、bits/bytes 禁 scale/offset
- unitCode:仅允许 0x80~0xFE(两版本同,与库自定义 TLV 区重合);拒绝标准占用(2016: 0x01~0x09;2025: 0x01~0x08、0x30、0x31、0x32、0xFF);**同包 unitCode 唯一**;maxRows ≥ 0
- 命令码:标准占用(2016 0x01~0x08 / 2025 0x01~0x0B)拒绝;本期仅上行预留区(2016 0x09~0x7F / 2025 0x0C~0x7F,错误文案版本感知);direction 仅 up
- 错误信息一律带 JSON 路径(如 `realtime.appendUnits[0].fields[3].scale`)
- 现有 `go test ./...` 必须保持全绿(本 Phase 纯新增,天然满足,收尾仍要验证)

## Final Acceptance Checklist (Refined from Spec) - MUST

- [PAC-1](源:AC-4) 非法包四类失败(语法/结构/语义/干跑)均产生含 JSON 路径的错误且不落盘
  Refinement: `go test ./internal/ext/ -v` 全绿;变异用例断言错误串包含期望路径;LoadFile 失败时不产生任何写盘副作用
- [PAC-2](源:AC-1/AC-2 字节级) 每种字段类型编码黄金字节;TLV = unitCode(u8)+len(u16 大端)+数据
  Refinement: 黄金 hex 见 Task 3/4 测试常量(含 scale/offset 换算、位段、hex 字节、整性检查报错)
- [PAC-3](源:AC-1) CompileUnit 产出与既有 schema 结构兼容的 GroupSchema(kind 映射正确,Min/Max 为物理量程)
  Refinement: u16+scale0.1 → **float**(仓库约定 ×0.1→float)、Min 0/Max 6553.5;i16+offset40(scale=1)→ int、Min -32728/Max 32807;bits → bitgroup;bytes → kind "bytes" 且 **Length=2**
- [PAC-4](源:AC-4) LoadDir 扫描目录、跳过非 JSON、坏文件不阻塞好文件
  Refinement: 混合目录(好包+坏包+txt)返回 1 个包且 error 非空并提及坏文件名;目录不存在返回 (nil, nil)

---

## File Structure(本 Phase 全部新增)

| 文件 | 职责 |
|---|---|
| `internal/ext/pack.go` | 包/单元/命令/字段声明类型(JSON 标签) |
| `internal/ext/validate.go` | 静态校验(码位/键唯一/类型集/取值域),路径化错误 |
| `internal/ext/encode.go` | 字段 DSL 编码器 + TLV 单元编码 |
| `internal/ext/compile.go` | DSL → schema.GroupSchema 编译 + 默认行值 |
| `internal/ext/load.go` | LoadFile(反序列化→校验→干跑) / LoadDir / DryRun |
| `internal/ext/*_test.go` + `testdata/demo.json` | 单测与黄金数据 |

---

### Task 1: 包类型定义与反序列化

**Level:** L2
**Level Rationale:** 新增独立包的类型层,局部行为(反序列化成功),单测可完整覆盖;无跨模块影响。
**Linked Acceptance Items:** AC-4(结构基础)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/ext/pack.go`
- Test: `internal/ext/pack_test.go`

**Interfaces:**
- Produces: `type Pack/Meta/Realtime/AppendUnit/Command/CommandBody/FieldSpec/BitSpec`(全部带 JSON 标签,字段名与设计文档 §4.1 一致;`Scale/Offset *float64` 区分未设置)

- [ ] **Step 1: 写失败测试**

`internal/ext/pack_test.go`:

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ext/ -v`
Expected: FAIL,`undefined: Pack`(编译错误)

- [ ] **Step 3: 最小实现**

`internal/ext/pack.go`:

```go
// Package ext 加载与编译 GB/T 32960 扩展包(单 JSON 文件):
// 0x02 实时报文尾部追加私有数据单元 + 私有命令帧声明。
package ext

// Pack 一个扩展包的完整声明。
type Pack struct {
	Meta     Meta      `json:"meta"`
	Realtime Realtime  `json:"realtime"`
	Commands []Command `json:"commands,omitempty"`
}

// Meta 包元信息。BaseVersion: "2016" | "2025"。
type Meta struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Vendor      string `json:"vendor,omitempty"`
	BaseVersion string `json:"baseVersion"`
}

// Realtime 0x02 实时报文的扩展段。
type Realtime struct {
	AppendUnits []AppendUnit `json:"appendUnits,omitempty"`
}

// AppendUnit 追加到标准体之后的私有数据单元(TLV)。
type AppendUnit struct {
	Key      string      `json:"key"`
	Title    string      `json:"title"`
	UnitCode int         `json:"unitCode"`
	Enabled  bool        `json:"enabled"`
	Multiple bool        `json:"multiple,omitempty"`
	MaxRows  int         `json:"maxRows,omitempty"`
	Fields   []FieldSpec `json:"fields"`
}

// Command 私有命令帧声明(Phase 3 实现发送,Phase 1 仅定义与校验)。
type Command struct {
	Key       string      `json:"key"`
	Label     string      `json:"label"`
	Code      int         `json:"code"`
	Direction string      `json:"direction"` // up | down
	Trigger   string      `json:"trigger"`   // manual | periodic | manual+periodic
	Body      CommandBody `json:"body"`
}

// CommandBody 命令体布局:平铺字段或实时报文同构(6B 十进制时间 + TLV 单元)。
type CommandBody struct {
	Type   string       `json:"type"` // fields | realtimeLike
	Fields []FieldSpec  `json:"fields,omitempty"`
	Units  []AppendUnit `json:"units,omitempty"`
}

// FieldSpec 字段 DSL 声明。物理值 = 线值×scale + offset。
type FieldSpec struct {
	Key    string    `json:"key"`
	Label  string    `json:"label"`
	Type   string    `json:"type"` // u8/u16/u32/i8/i16/i32/f32/bits/bytes
	Unit   string    `json:"unit,omitempty"`
	Scale  *float64  `json:"scale,omitempty"`
	Offset *float64  `json:"offset,omitempty"`
	Bits   []BitSpec `json:"bits,omitempty"`   // type=bits
	Length int       `json:"length,omitempty"` // type=bytes
}

// BitSpec 位段定义,index 0~31。
type BitSpec struct {
	Index int    `json:"index"`
	Label string `json:"label"`
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/ext/ -v`
Expected: PASS `TestUnmarshalPack`

- [ ] **Step 5: 提交**

```bash
git add internal/ext/pack.go internal/ext/pack_test.go
git commit -m "feat(ext): 扩展包声明类型与 JSON 反序列化"
```

---

### Task 2: 静态校验器(路径化错误)

**Level:** L2
**Level Rationale:** 新包内新增校验逻辑,局部行为;表驱动单测聚焦覆盖全部规则。
**Linked Acceptance Items:** AC-4
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/ext/validate.go`
- Test: `internal/ext/validate_test.go`

**Interfaces:**
- Consumes: Task 1 的全部类型
- Produces: `func Validate(p *Pack) error`(错误格式 `"<json路径>: <原因>"`);内部表 `standardUnitCodes`(集合判定)/ `standardCommandCodes`;同包 unitCode 唯一性由 Validate 内部共享 map 保证

- [ ] **Step 1: 写失败测试**

`internal/ext/validate_test.go`:

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ext/ -v`
Expected: FAIL,`undefined: Validate`

- [ ] **Step 3: 实现**

`internal/ext/validate.go`:

```go
package ext

import (
	"fmt"
	"regexp"
	"strings"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// 标准数据单元类型码(拒绝)。私有可用区两版本均为 0x80~0xFE(库的自定义 TLV 区)。
// 2025 的 0x30/0x31/0x32 为 燃料电池电堆/超级电容/超容极值,0xFF 为签名数据。
var standardUnitCodes = map[string]map[int]bool{
	"2016": {0x01: true, 0x02: true, 0x03: true, 0x04: true, 0x05: true, 0x06: true, 0x07: true, 0x08: true, 0x09: true},
	"2025": {0x01: true, 0x02: true, 0x03: true, 0x04: true, 0x05: true, 0x06: true, 0x07: true, 0x08: true, 0x30: true, 0x31: true, 0x32: true, 0xFF: true},
}

// 标准命令码占用区间。
var standardCommandCodes = map[string][2]int{"2016": {0x01, 0x08}, "2025": {0x01, 0x0B}}

func verrf(path, format string, args ...any) error {
	return fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...))
}

// Validate 静态校验包声明,错误信息带 JSON 路径。
// keys / unitCodes 在全包范围共享,保证 key 与 unitCode 同包唯一。
func Validate(p *Pack) error {
	if err := validateMeta(p.Meta); err != nil {
		return err
	}
	keys := map[string]bool{}
	unitCodes := map[int]bool{}
	for i, u := range p.Realtime.AppendUnits {
		path := fmt.Sprintf("realtime.appendUnits[%d]", i)
		if err := validateUnit(path, u, p.Meta.BaseVersion, keys, unitCodes); err != nil {
			return err
		}
	}
	for i, c := range p.Commands {
		path := fmt.Sprintf("commands[%d]", i)
		if err := validateCommand(path, c, p.Meta.BaseVersion, keys, unitCodes); err != nil {
			return err
		}
	}
	return nil
}

func validateMeta(m Meta) error {
	if !idPattern.MatchString(m.ID) {
		return verrf("meta.id", "须为 2~64 位小写字母/数字/连字符: %q", m.ID)
	}
	if strings.TrimSpace(m.Label) == "" {
		return verrf("meta.label", "不能为空")
	}
	if m.BaseVersion != "2016" && m.BaseVersion != "2025" {
		return verrf("meta.baseVersion", "须为 2016 或 2025: %q", m.BaseVersion)
	}
	return nil
}

func validateUnit(path string, u AppendUnit, base string, keys map[string]bool, unitCodes map[int]bool) error {
	if keys[u.Key] {
		return verrf(path+".key", "键重复: %q", u.Key)
	}
	keys[u.Key] = true
	if unitCodes[u.UnitCode] {
		return verrf(path+".unitCode", "同包内 unitCode 重复: 0x%02X", u.UnitCode)
	}
	unitCodes[u.UnitCode] = true
	if standardUnitCodes[base][u.UnitCode] {
		return verrf(path+".unitCode", "0x%02X 为标准数据单元,禁止占用", u.UnitCode)
	}
	if u.UnitCode < 0x80 || u.UnitCode > 0xFE {
		return verrf(path+".unitCode", "0x%02X 非法,私有单元须在自定义区 0x80~0xFE(0x0A~0x7F 为预留且库不可解码)", u.UnitCode)
	}
	if u.MaxRows < 0 {
		return verrf(path+".maxRows", "不能为负: %d", u.MaxRows)
	}
	if len(u.Fields) == 0 {
		return verrf(path+".fields", "至少一个字段")
	}
	return validateFields(path+".fields", u.Fields)
}

func validateFields(path string, fields []FieldSpec) error {
	seen := map[string]bool{}
	for i, f := range fields {
		fp := fmt.Sprintf("%s[%d]", path, i)
		if strings.TrimSpace(f.Key) == "" {
			return verrf(fp+".key", "不能为空")
		}
		if seen[f.Key] {
			return verrf(fp+".key", "字段键重复: %q", f.Key)
		}
		seen[f.Key] = true
		if strings.TrimSpace(f.Label) == "" {
			return verrf(fp+".label", "不能为空")
		}
		switch f.Type {
		case "u8", "u16", "u32", "i8", "i16", "i32":
			if f.Scale != nil && *f.Scale <= 0 {
				return verrf(fp+".scale", "须大于 0")
			}
		case "f32":
			if f.Scale != nil || f.Offset != nil {
				return verrf(fp+".scale", "f32 不支持 scale/offset")
			}
		case "bits", "bytes":
			if f.Scale != nil || f.Offset != nil {
				return verrf(fp+".scale", "%s 不支持 scale/offset", f.Type)
			}
			if f.Type == "bits" {
				if len(f.Bits) == 0 {
					return verrf(fp+".bits", "至少一个位定义")
				}
				idx := map[int]bool{}
				for j, b := range f.Bits {
					if b.Index < 0 || b.Index > 31 {
						return verrf(fmt.Sprintf("%s.bits[%d].index", fp, j), "须在 0~31: %d", b.Index)
					}
					if idx[b.Index] {
						return verrf(fmt.Sprintf("%s.bits[%d].index", fp, j), "位索引重复: %d", b.Index)
					}
					idx[b.Index] = true
				}
			}
			if f.Type == "bytes" && (f.Length < 1 || f.Length > 255) {
				return verrf(fp+".length", "须在 1~255 字节: %d", f.Length)
			}
		default:
			return verrf(fp+".type", "未知字段类型: %q", f.Type)
		}
	}
	return nil
}

func validateCommand(path string, c Command, base string, keys map[string]bool, unitCodes map[int]bool) error {
	if keys[c.Key] {
		return verrf(path+".key", "键重复: %q", c.Key)
	}
	keys[c.Key] = true
	if strings.TrimSpace(c.Label) == "" {
		return verrf(path+".label", "不能为空")
	}
	rng := standardCommandCodes[base]
	lo, hi := rng[0], rng[1]
	if c.Code >= lo && c.Code <= hi {
		return verrf(path+".code", "0x%02X 与标准命令冲突(标准占用 0x%02X~0x%02X)", c.Code, lo, hi)
	}
	reservedLo := hi + 1 // 上行预留区起点:2016=0x09,2025=0x0C
	if reservedLo < 0x09 {
		reservedLo = 0x09
	}
	if c.Code < reservedLo || c.Code > 0x7F {
		return verrf(path+".code", "本期仅支持上行预留区 0x%02X~0x7F: 0x%02X", reservedLo, c.Code)
	}
	if c.Direction != "up" {
		return verrf(path+".direction", "本期仅支持 up: %q", c.Direction)
	}
	switch c.Trigger {
	case "manual", "periodic", "manual+periodic":
	default:
		return verrf(path+".trigger", "须为 manual/periodic/manual+periodic: %q", c.Trigger)
	}
	switch c.Body.Type {
	case "fields":
		if len(c.Body.Fields) == 0 {
			return verrf(path+".body.fields", "至少一个字段")
		}
		return validateFields(path+".body.fields", c.Body.Fields)
	case "realtimeLike":
		if len(c.Body.Units) == 0 {
			return verrf(path+".body.units", "至少一个数据单元")
		}
		for i, u := range c.Body.Units {
			if err := validateUnit(fmt.Sprintf("%s.body.units[%d]", path, i), u, base, keys, unitCodes); err != nil {
				return err
			}
		}
		return nil
	default:
		return verrf(path+".body.type", "须为 fields 或 realtimeLike: %q", c.Body.Type)
	}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/ext/ -v`
Expected: PASS(`TestValidateOK` + 27 个子测试全绿)

- [ ] **Step 5: 提交**

```bash
git add internal/ext/validate.go internal/ext/validate_test.go
git commit -m "feat(ext): 静态校验器,错误带 JSON 路径定位"
```

---

### Task 3: 数值字段编码器

**Level:** L2
**Level Rationale:** 新包内编码逻辑,黄金字节单测聚焦覆盖;无跨模块影响。
**Linked Acceptance Items:** AC-1, AC-2(字节级)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/ext/encode.go`
- Test: `internal/ext/encode_test.go`

**Interfaces:**
- Consumes: `FieldSpec`(Task 1)、`schema.RowValue`
- Produces: `func EncodeFields(fields []FieldSpec, row schema.RowValue) ([]byte, error)`、`func ptrf(v float64) *float64`(后续 Task 与测试共用)、`var numericRange`(Task 5 编译量程复用)

- [ ] **Step 1: 写失败测试**

`internal/ext/encode_test.go`:

```go
package ext

import (
	"strings"
	"testing"

	"gbt32960-simulator/internal/schema"
)

func TestEncodeNumericGolden(t *testing.T) {
	fields := []FieldSpec{
		{Key: "u8v", Label: "a", Type: "u8"},
		{Key: "u16v", Label: "b", Type: "u16", Scale: ptrf(0.1)},
		{Key: "i16v", Label: "c", Type: "i16", Offset: ptrf(40)},
		{Key: "u32v", Label: "d", Type: "u32", Scale: ptrf(0.01)},
		{Key: "f32v", Label: "e", Type: "f32"},
	}
	row := schema.RowValue{"u8v": 80, "u16v": 3.3, "i16v": 25, "u32v": 12.34, "f32v": 1.5}
	got, err := EncodeFields(fields, row)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// 50 | 0021 | fff1 | 000004d2 | 3fc00000
	const want = "500021fff1000004d23fc00000"
	if hexStr(t, got) != want {
		t.Fatalf("got %s want %s", hexStr(t, got), want)
	}
}

func TestEncodeNumericErrors(t *testing.T) {
	cases := []struct {
		name   string
		field  FieldSpec
		value  any
		wantIn string
	}{
		{"换算非整数", FieldSpec{Key: "v", Label: "v", Type: "u16", Scale: ptrf(0.1)}, 3.35, "不是整数线值"},
		{"线值超范围", FieldSpec{Key: "v", Label: "v", Type: "u8"}, 300, "超出 u8 范围"},
		{"负值进无符号", FieldSpec{Key: "v", Label: "v", Type: "u8"}, -1, "超出 u8 范围"},
		{"值非数值", FieldSpec{Key: "v", Label: "v", Type: "u8"}, "abc", "不是数值"},
		{"f32 带 scale 拒绝", FieldSpec{Key: "v", Label: "v", Type: "f32", Scale: ptrf(2)}, 1.5, "f32 不支持"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EncodeFields([]FieldSpec{tc.field}, schema.RowValue{"v": tc.value})
			if err == nil || !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("err = %v, want contains %q", err, tc.wantIn)
			}
		})
	}
	t.Run("缺少值", func(t *testing.T) {
		_, err := EncodeFields([]FieldSpec{{Key: "v", Label: "v", Type: "u8"}}, schema.RowValue{})
		if err == nil || !strings.Contains(err.Error(), "缺少值") {
			t.Fatalf("err = %v", err)
		}
	})
}

func hexStr(t *testing.T, b []byte) string {
	t.Helper()
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, hexDigits[x>>4], hexDigits[x&0x0F])
	}
	return string(out)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ext/ -v`
Expected: FAIL,`undefined: EncodeFields` / `undefined: ptrf`

- [ ] **Step 3: 实现**

`internal/ext/encode.go`:

```go
package ext

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"gbt32960-simulator/internal/schema"
)

func ptrf(v float64) *float64 { return &v }

// toFloat 从 RowValue 取数值(JSON 数字可能解出 float64/int)。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

var numericRange = map[string][2]float64{
	"u8": {0, 255}, "u16": {0, 65535}, "u32": {0, 4294967295},
	"i8": {-128, 127}, "i16": {-32768, 32767}, "i32": {-2147483648, 2147483647},
}

var numericSize = map[string]int{"u8": 1, "i8": 1, "u16": 2, "i16": 2, "u32": 4, "i32": 4}

// EncodeFields 按声明顺序把一行配置编码为字节(不含任何头)。
func EncodeFields(fields []FieldSpec, row schema.RowValue) ([]byte, error) {
	out := make([]byte, 0, 64)
	for _, f := range fields {
		raw, err := encodeField(f, row)
		if err != nil {
			return nil, fmt.Errorf("字段 %s: %w", f.Key, err)
		}
		out = append(out, raw...)
	}
	return out, nil
}

func encodeField(f FieldSpec, row schema.RowValue) ([]byte, error) {
	switch f.Type {
	case "u8", "u16", "u32", "i8", "i16", "i32", "f32":
		return encodeNumeric(f, row)
	default:
		return nil, fmt.Errorf("未知字段类型 %q", f.Type)
	}
}

func scaleOf(f FieldSpec) (scale, offset float64) {
	scale, offset = 1, 0
	if f.Scale != nil {
		scale = *f.Scale
	}
	if f.Offset != nil {
		offset = *f.Offset
	}
	return
}

func encodeNumeric(f FieldSpec, row schema.RowValue) ([]byte, error) {
	v, ok := row[f.Key]
	if !ok {
		return nil, fmt.Errorf("缺少值")
	}
	phys, ok := toFloat(v)
	if !ok {
		return nil, fmt.Errorf("值不是数值: %v", v)
	}
	scale, offset := scaleOf(f)
	if f.Type == "f32" {
		if scale != 1 || offset != 0 {
			return nil, fmt.Errorf("f32 不支持 scale/offset")
		}
		bits := math.Float32bits(float32(phys))
		return []byte{byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits)}, nil
	}
	raw := (phys - offset) / scale
	wire := math.Round(raw)
	// 往返校验:用线值重建物理值再比对,免疫大数值下正向除法的浮点误差放大
	if math.Abs(wire*scale+offset-phys) > 1e-6 {
		return nil, fmt.Errorf("物理值 %v 换算后不是整数线值(最近线值 %v)", phys, wire)
	}
	iv := int64(wire)
	rng := numericRange[f.Type]
	lo, hi := rng[0], rng[1]
	if float64(iv) < lo || float64(iv) > hi {
		return nil, fmt.Errorf("线值 %d 超出 %s 范围 [%v, %v]", iv, f.Type, lo, hi)
	}
	return putInt(f.Type, iv), nil
}

func putInt(t string, v int64) []byte {
	size := numericSize[t]
	out := make([]byte, size)
	for i := 0; i < size; i++ {
		out[size-1-i] = byte(v >> (8 * i))
	}
	return out
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/ext/ -v`
Expected: PASS(黄金值 + 6 个错误分支)

- [ ] **Step 5: 提交**

```bash
git add internal/ext/encode.go internal/ext/encode_test.go
git commit -m "feat(ext): 数值字段 DSL 编码器(黄金字节 + 整性检查)"
```

---

### Task 4: bits/bytes 编码与 TLV 单元

**Level:** L2
**Level Rationale:** 同文件局部扩展,黄金字节单测聚焦覆盖。
**Linked Acceptance Items:** AC-1, AC-2(字节级)
**Task Gate:** task reviewer + focused checks

**Files:**
- Modify: `internal/ext/encode.go`(encodeField 增加 bits/bytes 分支;文件头新增 EncodeUnit)
- Test: `internal/ext/encode_test.go`(追加)

**Interfaces:**
- Consumes: Task 3 的 `encodeField`
- Produces: `func EncodeUnit(u AppendUnit, row schema.RowValue) ([]byte, error)`——返回完整 TLV(`unitCode u8 + len u16 大端 + 数据`),P2 组装管线直接消费

- [ ] **Step 1: 追加失败测试**

在 `internal/ext/encode_test.go` 追加:

```go
func TestEncodeBits(t *testing.T) {
	f2 := FieldSpec{Key: "flags", Label: "f", Type: "bits", Bits: []BitSpec{{Index: 0}, {Index: 1}}}
	got, err := encodeField(f2, schema.RowValue{"flags": map[string]any{"bit0": true}})
	if err != nil || hexStr(t, got) != "01" {
		t.Fatalf("got %s err %v", hexStr(t, got), err)
	}
	f9 := FieldSpec{Key: "flags", Label: "f", Type: "bits", Bits: []BitSpec{{Index: 9}}}
	got, err = encodeField(f9, schema.RowValue{"flags": map[string]any{"bit9": true}})
	if err != nil || hexStr(t, got) != "0002" {
		t.Fatalf("got %s err %v(第 9 位应占 2 字节)", hexStr(t, got), err)
	}
}

func TestEncodeBytesField(t *testing.T) {
	f := FieldSpec{Key: "sn", Label: "s", Type: "bytes", Length: 2}
	got, err := encodeField(f, schema.RowValue{"sn": "a1b2"})
	if err != nil || hexStr(t, got) != "a1b2" {
		t.Fatalf("got %s err %v", hexStr(t, got), err)
	}
	if _, err := encodeField(f, schema.RowValue{"sn": "a1"}); err == nil || !strings.Contains(err.Error(), "长度") {
		t.Fatalf("长度不匹配应报错, err=%v", err)
	}
	if _, err := encodeField(f, schema.RowValue{"sn": "zz"}); err == nil || !strings.Contains(err.Error(), "hex") {
		t.Fatalf("非法 hex 应报错, err=%v", err)
	}
}

func demoUnit() AppendUnit {
	var p Pack
	if err := jsonUnmarshalDemo(&p); err != nil {
		panic(err)
	}
	return p.Realtime.AppendUnits[0]
}

func TestEncodeUnitTLV(t *testing.T) {
	u := demoUnit()
	row := schema.RowValue{
		"soc2": 80, "packVolt": 3.3, "temp": 25,
		"flags": map[string]any{"bit0": true}, "sn": "a1b2",
	}
	got, err := EncodeUnit(u, row)
	if err != nil {
		t.Fatalf("encode unit: %v", err)
	}
	// 80 | 0008 | 50 0021 fff1 01 a1b2
	const want = "800008500021fff101a1b2"
	if hexStr(t, got) != want {
		t.Fatalf("got %s want %s", hexStr(t, got), want)
	}
}
```

并在测试文件顶部 import 区加入 `"encoding/json"`,追加辅助函数:

```go
func jsonUnmarshalDemo(p *Pack) error { return json.Unmarshal([]byte(demoPackJSON), p) }
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ext/ -v`
Expected: FAIL,`undefined: EncodeUnit`

- [ ] **Step 3: 实现**

`internal/ext/encode.go` 的 `encodeField` switch 改为:

```go
func encodeField(f FieldSpec, row schema.RowValue) ([]byte, error) {
	switch f.Type {
	case "u8", "u16", "u32", "i8", "i16", "i32", "f32":
		return encodeNumeric(f, row)
	case "bits":
		return encodeBits(f, row)
	case "bytes":
		return encodeBytes(f, row)
	default:
		return nil, fmt.Errorf("未知字段类型 %q", f.Type)
	}
}
```

文件末尾追加:

```go
func encodeBits(f FieldSpec, row schema.RowValue) ([]byte, error) {
	v, ok := row[f.Key]
	if !ok {
		return nil, fmt.Errorf("缺少值")
	}
	flags, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("值不是位组对象: %v", v)
	}
	maxIdx := 0
	for _, b := range f.Bits {
		if b.Index > maxIdx {
			maxIdx = b.Index
		}
	}
	size := 1
	if maxIdx >= 16 {
		size = 4
	} else if maxIdx >= 8 {
		size = 2
	}
	out := make([]byte, size)
	for _, b := range f.Bits {
		if on, _ := flags[fmt.Sprintf("bit%d", b.Index)].(bool); on {
			out[b.Index/8] |= 1 << (b.Index % 8)
		}
	}
	return out, nil
}

func encodeBytes(f FieldSpec, row schema.RowValue) ([]byte, error) {
	v, ok := row[f.Key]
	if !ok {
		return nil, fmt.Errorf("缺少值")
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("值不是 hex 字符串: %v", v)
	}
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("不是合法 hex: %w", err)
	}
	if len(b) != f.Length {
		return nil, fmt.Errorf("长度须为 %d 字节,实际 %d", f.Length, len(b))
	}
	return b, nil
}

// EncodeUnit 编码完整 TLV 数据单元: unitCode(u8) + 长度(u16 大端) + 数据。
// multiple 语义由调用方实现:每行调用一次,得到一个独立 TLV。
func EncodeUnit(u AppendUnit, row schema.RowValue) ([]byte, error) {
	data, err := EncodeFields(u.Fields, row)
	if err != nil {
		return nil, fmt.Errorf("单元 %s: %w", u.Key, err)
	}
	if len(data) > 0xFFFF {
		return nil, fmt.Errorf("单元 %s 数据超长: %d 字节", u.Key, len(data))
	}
	out := make([]byte, 3, 3+len(data))
	out[0] = byte(u.UnitCode)
	out[1] = byte(len(data) >> 8)
	out[2] = byte(len(data))
	return append(out, data...), nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/ext/ -v`
Expected: PASS(新增 3 个测试;黄金 TLV `800008500021fff101a1b2`)

- [ ] **Step 5: 提交**

```bash
git add internal/ext/encode.go internal/ext/encode_test.go
git commit -m "feat(ext): bits/bytes 编码与 TLV 单元编码器"
```

---

### Task 5: GroupSchema 编译器与默认行值

**Level:** L2
**Level Rationale:** DSL→schema 的纯映射逻辑,单测锁定 kind 映射与物理量程;消费既有导出类型,不修改 schema 包。
**Linked Acceptance Items:** AC-1(表单渲染地基)
**Task Gate:** task reviewer + focused checks

**Files:**
- Create: `internal/ext/compile.go`
- Modify: `internal/schema/schema.go`(FieldSchema 增加 Length 字段,一行)
- Test: `internal/ext/compile_test.go`

**Interfaces:**
- Consumes: `AppendUnit/FieldSpec`(Task 1)、`numericRange/scaleOf/ptrf`(Task 3)、`schema.GroupSchema/FieldSchema/BitDef/RowValue`
- Produces: `func CompileUnit(u AppendUnit) schema.GroupSchema`、`func CompileField(f FieldSpec) schema.FieldSchema`(bytes 字段携带 `Length`)、`func DefaultsFor(u AppendUnit) schema.RowValue`(P2 的 GetSchema 合并与 DefaultGroups 直接消费)

- [ ] **Step 1: 写失败测试**

`internal/ext/compile_test.go`:

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ext/ -v`
Expected: FAIL,`undefined: CompileUnit`

- [ ] **Step 3: 实现**

先修改 `internal/schema/schema.go`:在 `FieldSchema` 结构体的 `ScaleNote` 字段之后追加(bytes 长度透传,标准组不使用,零行为影响):

```go
	Length int                  `json:"length,omitempty"` // bytes 字段字节长度(扩展包编译器使用)
```

再创建 `internal/ext/compile.go`:

```go
package ext

import (
	"fmt"
	"strings"

	"gbt32960-simulator/internal/schema"
)

// CompileUnit 把 DSL 单元编译为前端表单渲染用的 GroupSchema。
func CompileUnit(u AppendUnit) schema.GroupSchema {
	fields := make([]schema.FieldSchema, len(u.Fields))
	for i, f := range u.Fields {
		fields[i] = CompileField(f)
	}
	maxRows := u.MaxRows
	if maxRows == 0 && u.Multiple {
		maxRows = 10
	}
	return schema.GroupSchema{
		Key: u.Key, Title: u.Title, Enabled: u.Enabled,
		Multiple: u.Multiple, MaxRows: maxRows, Fields: fields,
	}
}

// CompileField DSL 字段 → 前端 FieldSchema(kind 映射 + 物理量程 + 换算说明)。
func CompileField(f FieldSpec) schema.FieldSchema {
	out := schema.FieldSchema{Key: f.Key, Label: f.Label, Unit: f.Unit, ScaleNote: scaleNote(f)}
	switch f.Type {
	case "u8", "u16", "u32", "i8", "i16", "i32":
		rng := numericRange[f.Type]
	lo, hi := rng[0], rng[1]
		scale, offset := scaleOf(f)
		if scale == 1 { // 仓库约定:无 scale → int;×0.1 类 → float(如 speed/mileage)
			out.Kind = "int"
		} else {
			out.Kind = "float"
		}
		out.Min = ptrf(lo*scale + offset)
		out.Max = ptrf(hi*scale + offset)
	case "f32":
		out.Kind = "float"
	case "bits":
		out.Kind = "bitgroup"
		out.Bits = make([]schema.BitDef, len(f.Bits))
		for i, b := range f.Bits {
			out.Bits[i] = schema.BitDef{Index: b.Index, Label: b.Label}
		}
	case "bytes":
		out.Kind = "bytes"
		out.Length = f.Length
	}
	return out
}

func scaleNote(f FieldSpec) string {
	scale, offset := scaleOf(f)
	if scale == 1 && offset == 0 {
		return ""
	}
	var b strings.Builder
	if scale != 1 {
		fmt.Fprintf(&b, "×%g", scale)
	}
	if offset != 0 {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "偏移%+g", offset)
	}
	return b.String()
}

// DefaultsFor 生成单元默认行值:数值取 offset(线值 0),位全 false,bytes 为 00 填充。
func DefaultsFor(u AppendUnit) schema.RowValue {
	return defaultsForFields(u.Fields)
}

func defaultsForFields(fields []FieldSpec) schema.RowValue {
	row := schema.RowValue{}
	for _, f := range fields {
		switch f.Type {
		case "bits":
			flags := map[string]any{}
			for _, b := range f.Bits {
				flags[fmt.Sprintf("bit%d", b.Index)] = false
			}
			row[f.Key] = flags
		case "bytes":
			row[f.Key] = strings.Repeat("00", f.Length)
		default:
			offset := 0.0
			if f.Offset != nil {
				offset = *f.Offset
			}
			row[f.Key] = offset
		}
	}
	return row
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/ext/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/ext/compile.go internal/ext/compile_test.go internal/schema/schema.go
git commit -m "feat(ext): GroupSchema 编译器与默认行值;FieldSchema 增加 Length 字段"
```

---

### Task 6: LoadFile / LoadDir / DryRun 与夹具

**Level:** L2
**Level Rationale:** 组装既有能力(反序列化/校验/编码/默认值)的加载入口,行为局部;临时目录单测覆盖四类失败与目录扫描。
**Linked Acceptance Items:** AC-4
**Task Gate:** task reviewer + linked AC(PAC-1/PAC-4)

**Files:**
- Create: `internal/ext/load.go`、`internal/ext/testdata/demo.json`
- Test: `internal/ext/load_test.go`

**Interfaces:**
- Consumes: `Validate`(Task 2)、`EncodeUnit/EncodeFields`(Task 3/4)、`DefaultsFor/defaultsForFields`(Task 5)
- Produces: `func LoadFile(path string) (*Pack, error)`(反序列化→静态校验→干跑,失败即返回、零写盘)、`func LoadDir(dir string) ([]*Pack, error)`(坏文件不阻塞好文件,error 为聚合)、`func DryRun(p *Pack) error`——P2 的 Runtime/ExtService 直接消费

- [ ] **Step 1: 建夹具**

`internal/ext/testdata/demo.json`(内容与 Task 1 的 `demoPackJSON` 常量逐字一致):

```json
{
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
}
```

- [ ] **Step 2: 写失败测试**

`internal/ext/load_test.go`:

```go
package ext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileOK(t *testing.T) {
	p, err := LoadFile(filepath.Join("testdata", "demo.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Meta.ID != "demo" {
		t.Fatalf("id = %q", p.Meta.ID)
	}
}

func TestLoadFileFailures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("语法错误", func(t *testing.T) {
		_, err := LoadFile(write("bad.json", "{not json"))
		if err == nil || !strings.Contains(err.Error(), "JSON 语法错误") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("语义错误带路径", func(t *testing.T) {
		src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
		broken := strings.Replace(string(src), `"unitCode": 128`, `"unitCode": 9`, 1)
		_, err := LoadFile(write("sem.json", broken))
		if err == nil || !strings.Contains(err.Error(), "realtime.appendUnits[0].unitCode") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("干跑失败", func(t *testing.T) {
		src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
		broken := strings.Replace(string(src), `"type": "u8"`, `"type": "u999"`, 1)
		_, err := LoadFile(write("dry.json", broken))
		if err == nil {
			t.Fatal("未知类型应在静态校验拦截;若未来放宽静态校验,干跑必须兜底")
		}
	})
	t.Run("文件不存在", func(t *testing.T) {
		if _, err := LoadFile(filepath.Join(dir, "nope.json")); err == nil {
			t.Fatal("应报错")
		}
	})
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile(filepath.Join("testdata", "demo.json"))
	_ = os.WriteFile(filepath.Join(dir, "good.json"), src, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("skip"), 0o644)

	packs, err := LoadDir(dir)
	if len(packs) != 1 || packs[0].Meta.ID != "demo" {
		t.Fatalf("packs = %d, want 1", len(packs))
	}
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("err = %v, want 提及 broken.json", err)
	}

	if packs, err := LoadDir(filepath.Join(dir, "不存在")); packs != nil || err != nil {
		t.Fatalf("目录不存在应返回 (nil, nil), 实际 (%v, %v)", packs, err)
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/ext/ -v`
Expected: FAIL,`undefined: LoadFile`

- [ ] **Step 4: 实现**

`internal/ext/load.go`:

```go
package ext

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadFile 加载并完整校验一个扩展包:反序列化 → 静态校验 → 干跑。
// 任一步失败即返回错误,不产生写盘副作用。
func LoadFile(path string) (*Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	name := filepath.Base(path)
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: JSON 语法错误: %w", name, err)
	}
	if err := Validate(&p); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if err := DryRun(&p); err != nil {
		return nil, fmt.Errorf("%s: 干跑失败: %w", name, err)
	}
	return &p, nil
}

// LoadDir 扫描目录下全部 *.json;坏文件不阻塞好文件,错误聚合返回。
// 目录不存在返回 (nil, nil)。
func LoadDir(dir string) ([]*Pack, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var packs []*Pack
	var errs []error
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		p, err := LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		packs = append(packs, p)
	}
	return packs, errors.Join(errs...)
}

// DryRun 用默认值把每个单元/命令体编码一遍,兜底静态校验发现不了的布局错误。
func DryRun(p *Pack) error {
	for i, u := range p.Realtime.AppendUnits {
		if _, err := EncodeUnit(u, DefaultsFor(u)); err != nil {
			return fmt.Errorf("realtime.appendUnits[%d]: %w", i, err)
		}
	}
	for i, c := range p.Commands {
		if err := dryRunCommand(c); err != nil {
			return fmt.Errorf("commands[%d]: %w", i, err)
		}
	}
	return nil
}

func dryRunCommand(c Command) error {
	switch c.Body.Type {
	case "fields":
		_, err := EncodeFields(c.Body.Fields, defaultsForFields(c.Body.Fields))
		return err
	case "realtimeLike":
		for j, u := range c.Body.Units {
			if _, err := EncodeUnit(u, DefaultsFor(u)); err != nil {
				return fmt.Errorf("units[%d]: %w", j, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("body.type 非法: %q", c.Body.Type)
	}
}
```

- [ ] **Step 5: 跑全部测试确认通过**

Run: `go test ./internal/ext/ -v`
Expected: PASS(全部 Task 1~6 测试)

- [ ] **Step 6: 全仓回归**

Run: `go test ./...`
Expected: 全绿(现有 golden/单测不受影响)

- [ ] **Step 7: Phase 级验收(逐条核对)**

- [PAC-1] `go test ./internal/ext/ -v` 全绿;四类失败(语法/语义/干跑/不存在)错误信息含文件名或 JSON 路径 ✓
- [PAC-2] 黄金 hex:数值串 `500021fff1000004d23fc00000`、TLV `800008500021fff101a1b2`、位段 `01`/`0002` ✓
- [PAC-3] kind 映射 int×3/bitgroup/bytes;量程 0~6553.5 与 -32728~32807;ScaleNote `×0.1`、`偏移+40` ✓
- [PAC-4] 混合目录返回 1 包 + 聚合错误提及坏文件;目录不存在 (nil, nil) ✓

- [ ] **Step 8: 提交**

```bash
git add internal/ext/load.go internal/ext/load_test.go internal/ext/testdata/demo.json
git commit -m "feat(ext): 包加载入口(校验+干跑)与 demo 夹具"
```

---

## Self-Review 记录

1. **Spec 覆盖**:设计 §4.1 格式(Task 1)、§4.2 类型集与数值契约(Task 2/3)、§4.3 编码规则(Task 4)、§4.4 码位规则(Task 2)、GroupSchema 编译(Task 5)、加载/干跑(Task 6)——无缺口。
2. **占位符扫描**:无 TBD/TODO;所有代码步骤含完整代码。
3. **类型一致性**:`EncodeUnit(u AppendUnit, row schema.RowValue) ([]byte, error)` 在 Task 4 定义、Task 5/6 消费一致;`ptrf` Task 3 定义、Task 3/5 消费一致;`defaultsForFields` Task 5 定义、Task 6 消费一致。
4. **拆分决策**:已拆分(见 Master Plan);Phase 1 独立可验收。
5. **Level/Gate 完整**:6 任务全部 L2 + rationale + gate;无行为变化为零的任务,故无 L1。
6. **AC 覆盖**:PAC-1~4 均有非 L1 任务承载(PAC-1←Task2/6,PAC-2←Task3/4,PAC-3←Task5,PAC-4←Task6)。
7. **AC 可执行**:每条 PAC 带命令/数据/边界。
8. **Oracle 评审修订(2026-09-01)**:已落实 P0-1(unitCode 区修正为 0x80~0xFE + 2025 标准集合含 0x30~0x32/0xFF)、P1-4(FieldSchema.Length + 唯一现有文件修改豁免)、P1-5(scale≠1→float)、P1-6(同包 unitCode 唯一)、P2-1(命令码区间文案版本感知)、P2-2(整性检查改往返校验)、P2-6(示例 code 为 JSON 数字)、P2-7(maxRows≥0);设计文档 §4.1~§4.6/§6/§7 同步修订(含 P1-1 十进制时间、P1-2 精确码副本契约、P1-3 四路管线,属 P2/P3 契约)。
