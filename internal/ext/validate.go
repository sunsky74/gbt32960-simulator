package ext

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// reservedGroupKeys 标准报文组键(与 internal/schema 的 V2016Groups/V2025Groups 同步,协议冻结不会变)。
// 扩展命令/追加单元的组键若与之同名,会在 GetSchema 合并时与标准组冲突(键重叠导致配置错乱)。
var reservedGroupKeys = map[string]bool{
	"vehicle": true, "motor": true, "fuelcell": true, "engine": true, "location": true,
	"extremum": true, "alarm": true, "voltage": true, "temperature": true,
	"minparallel": true, "batterytemp": true, "fcstack": true, "supercap": true, "supercapextremum": true,
}

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
	cmdCodes := map[int]bool{}
	for i, u := range p.Realtime.AppendUnits {
		path := fmt.Sprintf("realtime.appendUnits[%d]", i)
		if err := validateUnit(path, u, p.Meta.BaseVersion, keys, unitCodes); err != nil {
			return err
		}
	}
	for i, c := range p.Commands {
		path := fmt.Sprintf("commands[%d]", i)
		if err := validateCommand(path, c, p.Meta.BaseVersion, keys, unitCodes, cmdCodes); err != nil {
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
	seen := map[string]bool{}
	for i, s := range m.Scope {
		if s != ScopeClient && s != ScopeParser {
			return verrf(fmt.Sprintf("meta.scope[%d]", i), "须为 %q 或 %q: %q", ScopeClient, ScopeParser, s)
		}
		if seen[s] {
			return verrf(fmt.Sprintf("meta.scope[%d]", i), "重复的应用范围: %q", s)
		}
		seen[s] = true
	}
	return nil
}

func validateUnit(path string, u AppendUnit, base string, keys map[string]bool, unitCodes map[int]bool) error {
	if strings.TrimSpace(u.Key) == "" {
		return verrf(path+".key", "不能为空")
	}
	if strings.Contains(u.Key, ":") {
		return verrf(path+".key", "键不能包含冒号(与命令组键命名空间冲突): %q", u.Key)
	}
	if reservedGroupKeys[u.Key] {
		return verrf(path+".key", "键与标准报文组冲突(保留键): %q", u.Key)
	}
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
			// G-1 裁决(方案A):offset 限整数。半整数偏移改写为 scale 表达(如 offset:0.5 → scale:0.5),值域等价且 Kind=float 表单可用。
			if f.Offset != nil && *f.Offset != math.Trunc(*f.Offset) {
				return verrf(fp+".offset", "须为整数(非整数偏移请改写为 scale,如 offset:0.5 → scale:0.5)")
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
		case "tail":
			if f.Scale != nil || f.Offset != nil || f.Length != 0 || len(f.Bits) > 0 {
				return verrf(fp+".tail", "变长 hex 尾部不支持 scale/offset/length/bits")
			}
		default:
			return verrf(fp+".type", "未知字段类型: %q", f.Type)
		}
	}
	return nil
}

func validateCommand(path string, c Command, base string, keys map[string]bool, unitCodes map[int]bool, cmdCodes map[int]bool) error {
	if strings.TrimSpace(c.Key) == "" {
		return verrf(path+".key", "不能为空")
	}
	if strings.Contains(c.Key, ":") {
		return verrf(path+".key", "键不能包含冒号(与命令组键命名空间冲突): %q", c.Key)
	}
	if reservedGroupKeys[c.Key] {
		return verrf(path+".key", "键与标准报文组冲突(保留键): %q", c.Key)
	}
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
	// 私有远控 0x8A 应答模板:仅 2016 版放行(私有远控应答),RemoteSub 必填且子指令码 1~17
	if c.Code == 0x8A {
		if base != "2016" {
			return verrf(path+".code", "0x8A 仅支持 baseVersion 2016")
		}
		if c.RemoteSub < 1 || c.RemoteSub > 0x11 {
			return verrf(path+".remoteSub", "0x8A 应答模板须声明子指令码 0x01~0x11: %d", c.RemoteSub)
		}
		if c.Body.Type != "fields" {
			return verrf(path+".body.type", "0x8A 应答模板仅支持平铺 fields 体: %q", c.Body.Type)
		}
		if len(c.Body.Fields) == 0 {
			return verrf(path+".body.fields", "至少一个字段")
		}
		return validateFields(path+".body.fields", c.Body.Fields)
	}
	reservedLo := hi + 1 // 上行预留区起点:2016=0x09,2025=0x0C
	if c.Code < reservedLo || c.Code > 0x7F {
		return verrf(path+".code", "本期仅支持上行预留区 0x%02X~0x7F: 0x%02X", reservedLo, c.Code)
	}
	if cmdCodes[c.Code] {
		return verrf(path+".code", "命令码重复: 0x%02X 已被同包其他命令占用", c.Code)
	}
	cmdCodes[c.Code] = true
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
