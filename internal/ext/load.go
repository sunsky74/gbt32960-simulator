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
	return LoadText(string(data), filepath.Base(path))
}

// LoadText 从 JSON 文本加载并完整校验一个扩展包(粘贴导入共用入口)。
// name 仅用于错误前缀展示。
func LoadText(text, name string) (*Pack, error) {
	var p Pack
	if err := json.Unmarshal([]byte(text), &p); err != nil {
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
