package engine

import (
	"testing"

	"github.com/sunsky74/gb32960/api"
	"github.com/sunsky74/gb32960/types"
)

func TestCommandForRegisteredExactCode(t *testing.T) {
	RegisterCommand(api.V2016, 0x0A, "TEST_EXT")
	rt := commandFor(api.V2016, 0x0A)
	c, ok := rt.(*types.CommandV2016)
	if !ok || c.Code != 0x0A || c.Min != 0x0A || c.Max != 0x0A || c.Name != "TEST_EXT" {
		t.Fatalf("rt = %+v, want 精确副本(Code=Min=Max=0x0A)", rt)
	}
	ResetExtCommands()
	if _, ok := extCommandName(api.V2016, 0x0A); ok {
		t.Fatal("Reset 后自定义命令应清除")
	}
	if name, ok := extCommandName(api.V2016, 0x8A); !ok || name != "REMOTE_CONTROL" {
		t.Fatal("私有远控内置条目应保留(AC-3)")
	}
}

func TestCommandForFallbackStandard(t *testing.T) {
	ResetExtCommands()
	rt := commandFor(api.V2016, 0x02)
	if _, ok := rt.(*types.CommandV2016); !ok {
		t.Fatalf("标准命令应回退库枚举,实际 %T", rt)
	}
}

func TestCommandForV2025Registered(t *testing.T) {
	RegisterCommand(api.V2025, 0x0C, "TEST_V2025")
	rt := commandFor(api.V2025, 0x0C)
	c, ok := rt.(*types.CommandV2025)
	if !ok || c.Code != 0x0C || c.Min != 0x0C || c.Max != 0x0C {
		t.Fatalf("rt = %+v", rt)
	}
	ResetExtCommands()
}
