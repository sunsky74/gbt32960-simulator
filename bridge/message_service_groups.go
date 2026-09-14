// message_service_groups.go 报文组配置的保存/加载与扩展行干跑校验。
package bridge

import (
	"fmt"
	"time"

	"gbt32960-simulator/internal/ext"
	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/store"
)

const groupsFile = "message.json"

// SaveGroups 校验(标准组装 + 扩展行干跑)、持久化并快照报文配置。
func (s *MessageService) SaveGroups(payload schema.GroupsPayload) error {
	groups := payload.ToMap()
	if _, err := schema.Assemble(s.version(), groups, time.Now()); err != nil {
		return err
	}
	if err := s.validateExtRows(groups); err != nil {
		return err
	}
	if err := s.saveExtGroups(groups); err != nil {
		return err
	}
	s.rt.SetGroups(groups) // 内存快照保留扩展组:组装需要扩展值
	// message.json 只落标准组:扩展组值按包独立存 extgroups.json,
	// 混写会在切换到同键扩展包时被当作新包的值读出(跨包串值)。
	standard := make(map[string]schema.GroupConfig, len(groups))
	for k, g := range groups {
		if schema.IsStandardGroupKey(k) {
			standard[k] = g
		}
	}
	return store.Save(groupsFile, &standard)
}

// validateExtRows 对激活包的扩展组行值做字段级编码干跑:非法值在保存点拦截,不落盘。
// 跳过语义与组装一致(assembleBody/assembleCommandBody):未配置或未启用的组不校验。
func (s *MessageService) validateExtRows(groups map[string]schema.GroupConfig) error {
	p := s.rt.Pack()
	if p == nil {
		return nil
	}
	for _, u := range p.Realtime.AppendUnits {
		g, ok := groups[u.Key]
		if !ok || !g.Enabled {
			continue
		}
		for ri, row := range g.Rows {
			if _, err := ext.EncodeUnit(u, row); err != nil {
				return fmt.Errorf("扩展单元 %s 第 %d 行: %w", u.Key, ri+1, err)
			}
		}
	}
	for _, c := range p.Commands {
		switch c.Body.Type {
		case "fields":
			g, ok := groups[c.Key]
			if !ok || !g.Enabled || len(g.Rows) == 0 {
				continue
			}
			if _, err := ext.EncodeFields(c.Body.Fields, g.Rows[0]); err != nil {
				return fmt.Errorf("扩展命令 %s: %w", c.Key, err)
			}
		case "realtimeLike":
			for _, u := range c.Body.Units {
				g, ok := groups[c.Key+":"+u.Key]
				if !ok || !g.Enabled {
					continue
				}
				for ri, row := range g.Rows {
					if _, err := ext.EncodeUnit(u, row); err != nil {
						return fmt.Errorf("扩展命令 %s 单元 %s 第 %d 行: %w", c.Key, u.Key, ri+1, err)
					}
				}
			}
		}
	}
	return nil
}

// extKeys 激活包的扩展组键集合(实时追加单元 + 命令组),与 GetSchema 的键命名一致。
func (s *MessageService) extKeys(p *ext.Pack) map[string]bool {
	if p == nil {
		return nil
	}
	out := map[string]bool{}
	for _, u := range p.Realtime.AppendUnits {
		out[u.Key] = true
	}
	for _, c := range p.Commands {
		switch c.Body.Type {
		case "fields":
			out[c.Key] = true
		case "realtimeLike":
			for _, u := range c.Body.Units {
				out[c.Key+":"+u.Key] = true
			}
		}
	}
	return out
}

// saveExtGroups 把激活包的扩展组配置按包独立存储。
// 未绑包时直接返回(解绑后的保存不触碰扩展组配置);绑包时以包 id 为键
// **逐键合并**(评审 P0-1:禁止整体覆盖——实时面板保存的 payload 只含标准组与
// 追加单元、不含命令组,整体覆盖会抹掉命令组配置),part 为空则不写。
func (s *MessageService) saveExtGroups(groups map[string]schema.GroupConfig) error {
	keys := s.extKeys(s.rt.Pack())
	if len(keys) == 0 {
		return nil
	}
	var all map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &all); err != nil || all == nil {
		all = map[string]map[string]schema.GroupConfig{}
	}
	part := map[string]schema.GroupConfig{}
	for k, g := range groups {
		if keys[k] {
			part[k] = g
		}
	}
	if len(part) == 0 {
		return nil
	}
	if all[s.rt.Pack().Meta.ID] == nil {
		all[s.rt.Pack().Meta.ID] = map[string]schema.GroupConfig{}
	}
	for k, v := range part {
		all[s.rt.Pack().Meta.ID][k] = v
	}
	return store.Save(extGroupsFile, &all)
}

// loadAllGroups 读标准组(message.json)并叠加激活包的扩展组配置(extgroups.json)。
// 两个文件都无数据时返回 nil,触发 GetGroups 的默认值兜底。
func (s *MessageService) loadAllGroups() map[string]schema.GroupConfig {
	var g map[string]schema.GroupConfig
	if err := store.Load(groupsFile, &g); err != nil || g == nil {
		g = map[string]schema.GroupConfig{}
	}
	// 剔除历史遗留的扩展键:旧版本曾把扩展组值混写进 message.json,
	// 绑同键包时会被当成该包的值读出;扩展组值只从 extgroups.json 叠加。
	for k := range g {
		if !schema.IsStandardGroupKey(k) {
			delete(g, k)
		}
	}
	for k, v := range s.loadExtGroups() {
		g[k] = v
	}
	if len(g) == 0 {
		return nil
	}
	return g
}

// loadExtGroups 读取激活包的扩展组配置(extgroups.json 按包 id 取;未绑包/无数据返回 nil)。
func (s *MessageService) loadExtGroups() map[string]schema.GroupConfig {
	p := s.rt.Pack()
	if p == nil {
		return nil
	}
	var all map[string]map[string]schema.GroupConfig
	if err := store.Load(extGroupsFile, &all); err != nil || all == nil {
		return nil
	}
	return all[p.Meta.ID]
}
