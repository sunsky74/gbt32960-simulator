package bridge

import (
	"sync"
	"testing"

	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/track"
)

// TA1:Groups() 快照必须与后续轨迹写入隔离 —— 轨迹推进替换 Rows[0] 后,
// 旧快照读到的仍是拿快照时的值(修复前快照与运行时共享 Rows 底层数组)。
func TestTA1GroupsSnapshotIsolationOnAdvance(t *testing.T) {
	rt := NewRuntime()
	rt.SetGroups(map[string]schema.GroupConfig{
		"location": {Enabled: true, Rows: []map[string]any{{"longitude": 1.0, "latitude": 1.0, "valid": true}}},
	})
	snap := rt.Groups()

	s := NewTrackService(rt, &fakeTrackDeps{})
	s.points = []track.Point{{Lng: 9, Lat: 8}}
	s.active = true
	s.advanceForReport()

	if got := snap["location"].Rows[0]["longitude"]; got != 1.0 {
		t.Fatalf("快照被就地写穿:longitude = %v, want 1.0", got)
	}
	if got := rt.Groups()["location"].Rows[0]["longitude"]; got != 9.0 {
		t.Fatalf("运行时位置组未更新:longitude = %v, want 9", got)
	}

	// nil Rows 语义保持:nil 不得被转换为空切片。
	rt.SetGroups(map[string]schema.GroupConfig{"bare": {Enabled: true}})
	if g := rt.Groups()["bare"]; g.Rows != nil {
		t.Fatalf("nil Rows 被改写:%#v", g.Rows)
	}
}

// TA2:轨迹推进与 Groups() 并发读,在 -race 下不得报竞态
// (修复前 applyLocationLocked 在锁外写共享 Rows 底层数组元素)。
func TestTA2ConcurrentAdvanceAndGroupsRead(t *testing.T) {
	rt := NewRuntime()
	rt.SetGroups(map[string]schema.GroupConfig{
		"location": {Enabled: true, Rows: []map[string]any{{"longitude": 1.0, "latitude": 1.0, "valid": true}}},
	})
	s := NewTrackService(rt, &fakeTrackDeps{})
	pts := make([]track.Point, 4)
	for i := range pts {
		pts[i] = track.Point{Lng: float64(116 + i), Lat: float64(39 + i)}
	}
	s.points, s.active, s.loop = pts, true, true

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			s.advanceForReport()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			if g, ok := rt.Groups()["location"]; ok && len(g.Rows) > 0 {
				_ = g.Rows[0]["longitude"]
			}
		}
	}()
	wg.Wait()
}
