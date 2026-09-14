package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gbt32960-simulator/internal/schema"
	"gbt32960-simulator/internal/track"
)

// fakeTrackDeps 测试替身:记录 ensureAutoReport 调用;DefaultGroups 只含 location 组。
type fakeTrackDeps struct {
	mu        sync.Mutex
	ensured   int
	ensureErr error
}

func (f *fakeTrackDeps) ensureAutoReport() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensured++
	return f.ensureErr
}

func (f *fakeTrackDeps) DefaultGroups(string) *schema.GroupsPayload {
	return schema.FromMap(map[string]schema.GroupConfig{
		"location": {Enabled: true, Rows: []map[string]any{{"valid": false, "longitude": 0.0, "latitude": 0.0}}},
	}, []string{"location"})
}

func (f *fakeTrackDeps) ensuredCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ensured
}

func newTestTrackService(points int) (*TrackService, *fakeTrackDeps, *Runtime) {
	rt := NewRuntime()
	deps := &fakeTrackDeps{}
	s := NewTrackService(rt, deps)
	pts := make([]track.Point, points)
	for i := range pts {
		pts[i] = track.Point{Lng: float64(116 + i), Lat: float64(39 + i)}
	}
	s.points = pts
	s.info = TrackInfo{Count: points}
	return s, deps, rt
}

func locationRow(t *testing.T, rt *Runtime) map[string]any {
	t.Helper()
	g, ok := rt.Groups()["location"]
	if !ok || len(g.Rows) == 0 {
		t.Fatalf("location group missing: %+v", g)
	}
	return g.Rows[0]
}

func TestStartReplayAppliesPreviewPoint(t *testing.T) {
	s, deps, rt := newTestTrackService(3)
	if err := s.StartReplay(false); err != nil {
		t.Fatal(err)
	}
	if deps.ensuredCalls() != 1 {
		t.Fatalf("ensureAutoReport calls = %d, want 1", deps.ensuredCalls())
	}
	st := s.ReplayStatus()
	if !st.Running || st.Index != 0 || st.Total != 3 {
		t.Fatalf("status = %+v", st)
	}
	row := locationRow(t, rt)
	if row["longitude"] != 116.0 || row["latitude"] != 39.0 || row["valid"] != true {
		t.Errorf("preview row = %v, want first point 116/39/valid", row)
	}
}

func TestAdvanceProgressionAndFinish(t *testing.T) {
	s, _, rt := newTestTrackService(3)
	if err := s.StartReplay(false); err != nil {
		t.Fatal(err)
	}
	want := [][2]float64{{116, 39}, {117, 40}, {118, 41}}
	for i, w := range want {
		s.advanceForReport()
		row := locationRow(t, rt)
		if row["longitude"] != w[0] || row["latitude"] != w[1] {
			t.Errorf("advance %d: row = %v, want %v", i+1, row, w)
		}
	}
	if st := s.ReplayStatus(); st.Index != 3 || !st.Running {
		t.Fatalf("after 3 advances: %+v", st)
	}
	s.advanceForReport() // 播完 → 自动停止
	if st := s.ReplayStatus(); st.Running || st.Index != 3 || st.LastError != "" {
		t.Fatalf("after finish: %+v", st)
	}
	// 停止后再推进为空操作,位置保留最后一点。
	s.advanceForReport()
	if row := locationRow(t, rt); row["longitude"] != 118.0 {
		t.Errorf("position changed after finish: %v", row)
	}
}

func TestAdvanceLoop(t *testing.T) {
	s, _, _ := newTestTrackService(3)
	if err := s.StartReplay(true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		s.advanceForReport()
	}
	st := s.ReplayStatus()
	if !st.Running {
		t.Fatal("loop replay should stay running")
	}
	if st.Index != 1 { // 7 mod 3
		t.Fatalf("index = %d, want 1", st.Index)
	}
}

func TestFailOnSendStopsReplay(t *testing.T) {
	s, _, _ := newTestTrackService(5)
	if err := s.StartReplay(false); err != nil {
		t.Fatal(err)
	}
	s.failOnSend(errors.New("connection reset"))
	st := s.ReplayStatus()
	if st.Running || st.LastError == "" {
		t.Fatalf("status = %+v, want stopped with reason", st)
	}
	s.failOnSend(errors.New("again")) // 未激活时忽略
	if strings.Contains(s.ReplayStatus().LastError, "again") {
		t.Fatal("inactive failOnSend should be ignored")
	}
}

func TestStopReplayKeepsLastPosition(t *testing.T) {
	s, _, _ := newTestTrackService(3)
	_ = s.StartReplay(false)
	s.advanceForReport()
	s.StopReplay()
	s.StopReplay() // 幂等
	st := s.ReplayStatus()
	if st.Running || st.Lng != 116.0 || st.Lat != 39.0 {
		t.Fatalf("status = %+v, want stopped at first advanced point", st)
	}
}

func TestClearTrack(t *testing.T) {
	s, _, _ := newTestTrackService(2)
	if err := s.StartReplay(false); err != nil {
		t.Fatal(err)
	}
	s.ClearTrack()
	st := s.ReplayStatus()
	if st.Running || st.Total != 0 || st.Index != 0 {
		t.Fatalf("after clear: %+v", st)
	}
	if err := s.StartReplay(false); err == nil {
		t.Fatal("start after clear should fail")
	}
}

func TestImportAutoEnsuresReport(t *testing.T) {
	s, deps, _ := newTestTrackService(0)
	dir := t.TempDir()
	path := filepath.Join(dir, "a.csv")
	if err := os.WriteFile(path, []byte("116.1,39.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportTrack(path); err != nil {
		t.Fatal(err)
	}
	if deps.ensuredCalls() != 1 {
		t.Fatalf("ensureAutoReport calls = %d, want 1", deps.ensuredCalls())
	}

	// 离线等失败静默跳过,导入仍成功。
	deps.ensureErr = errors.New("未连接")
	if _, err := s.ImportTrack(path); err != nil {
		t.Fatalf("import should tolerate ensure failure: %v", err)
	}
}

func TestImportTrackStopsActiveReplay(t *testing.T) {
	s, _, _ := newTestTrackService(3)
	if err := s.StartReplay(false); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "route.csv")
	if err := os.WriteFile(path, []byte("116.1,39.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportTrack(path); err != nil {
		t.Fatal(err)
	}
	st := s.ReplayStatus()
	if st.Running {
		t.Fatal("import should stop active replay")
	}
	if st.Total != 1 || st.Index != 0 {
		t.Fatalf("status = %+v, want fresh 1-point track", st)
	}
}

func TestStartReplayGuards(t *testing.T) {
	s, _, _ := newTestTrackService(0)
	if err := s.StartReplay(false); err == nil {
		t.Fatal("want error: no track imported")
	}

	s2, deps, _ := newTestTrackService(2)
	deps.ensureErr = errors.New("未连接或未登录")
	err := s2.StartReplay(false)
	if err == nil || !strings.Contains(err.Error(), "开启周期上报失败") {
		t.Fatalf("err = %v, want wrapped ensure failure", err)
	}
	if s2.ReplayStatus().Running {
		t.Fatal("replay must not activate when ensure fails")
	}
}

func TestImportTrackFormats(t *testing.T) {
	s, _, _ := newTestTrackService(0)
	dir := t.TempDir()

	csv := filepath.Join(dir, "a.csv")
	if err := os.WriteFile(csv, []byte("经度,纬度\n116.1,39.2\n116.2,39.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := s.ImportTrack(csv)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "csv" || info.Count != 2 || info.Skipped != 1 {
		t.Fatalf("csv info = %+v", info)
	}

	gpx := filepath.Join(dir, "a.gpx")
	if err := os.WriteFile(gpx, []byte(`<gpx><trk><trkseg><trkpt lat="39.5" lon="116.6"/><trkpt lat="39.6" lon="116.7"/></trkseg></trk></gpx>`), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err = s.ImportTrack(gpx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "gpx" || info.Count != 2 {
		t.Fatalf("gpx info = %+v", info)
	}

	if _, err := s.ImportTrack(filepath.Join(dir, "none.csv")); err == nil {
		t.Fatal("want error for missing file")
	}
}

// fakeHook MessageService 停报联动测试用的 trackHook 替身。
type fakeHook struct {
	stopped bool
}

func (f *fakeHook) advanceForReport() {}
func (f *fakeHook) failOnSend(error)  {}
func (f *fakeHook) StopReplay()       { f.stopped = true }

func TestSetAutoReportStopStopsTrackReplay(t *testing.T) {
	rt := NewRuntime()
	ms := NewMessageService(rt)
	h := &fakeHook{}
	ms.setTrackReplay(h)
	// 无客户端:调用报"未连接",但停报联动仍须发生。
	if err := ms.SetAutoReport(false, 0); err == nil {
		t.Fatal("want error: not connected")
	}
	if !h.stopped {
		t.Fatal("SetAutoReport(false) should stop track replay")
	}
}

func TestEnsureAutoReportRequiresOnlineClient(t *testing.T) {
	rt := NewRuntime()
	ms := NewMessageService(rt)
	if err := ms.ensureAutoReport(); err == nil {
		t.Fatal("want error without client")
	}
}
