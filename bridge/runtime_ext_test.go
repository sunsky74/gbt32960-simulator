package bridge

import (
	"testing"

	"gbt32960-simulator/internal/ext"
)

func extTestPack(id string) *ext.Pack {
	return &ext.Pack{
		Meta: ext.Meta{ID: id, Label: id, BaseVersion: "2016"},
		Realtime: ext.Realtime{AppendUnits: []ext.AppendUnit{{
			Key: "telemetry", Title: "私有遥测", UnitCode: 128, Fields: []ext.FieldSpec{
				{Key: "soc2", Label: "SOC2", Type: "u8"},
			},
		}}},
	}
}

func TestRuntimePackResolution(t *testing.T) {
	rt := NewRuntime()
	rt.SetPacks([]*ext.Pack{extTestPack("demo"), extTestPack("other")})

	cases := []struct {
		name string
		cfg  *ConnectionConfig
		want string // 期望激活包 id,"" 表示 nil
	}{
		{"按 id 激活", &ConnectionConfig{ExtensionPack: "demo"}, "demo"},
		{"未绑定", &ConnectionConfig{ExtensionPack: ""}, ""},
		{"id 不存在", &ConnectionConfig{ExtensionPack: "nope"}, ""},
		{"配置为 nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt.SetConnCfg(tc.cfg)
			got := rt.Pack()
			if tc.want == "" {
				if got != nil {
					t.Fatalf("期望 nil,实际 %s", got.Meta.ID)
				}
			} else if got == nil || got.Meta.ID != tc.want {
				t.Fatalf("期望 %s,实际 %v", tc.want, got)
			}
		})
	}

	t.Run("后设包集合同样按当前配置解析", func(t *testing.T) {
		rt.SetConnCfg(&ConnectionConfig{ExtensionPack: "other"})
		rt.SetPacks([]*ext.Pack{extTestPack("demo"), extTestPack("other")})
		if p := rt.Pack(); p == nil || p.Meta.ID != "other" {
			t.Fatalf("期望 other,实际 %v", p)
		}
	})
}
