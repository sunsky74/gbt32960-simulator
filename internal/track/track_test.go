package track

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseGPX(t *testing.T) {
	gpx := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><name>路线</name>
    <trkseg>
      <trkpt lat="39.531791" lon="116.664053"></trkpt>
      <trkpt lat="39.53199" lon="116.66438"/>
      <trkpt lat="95.0" lon="116.7"/>
    </trkseg>
  </trk>
  <wpt lat="39.6" lon="116.8"/>
</gpx>`
	res, err := ParseFile("路线.gpx", []byte(gpx))
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "gpx" || len(res.Points) != 3 || res.Skipped != 1 {
		t.Fatalf("format=%s points=%d skipped=%d, want gpx/3/1", res.Format, len(res.Points), res.Skipped)
	}
	if res.Points[0].Lng != 116.664053 || res.Points[0].Lat != 39.531791 {
		t.Errorf("points[0] = %+v", res.Points[0])
	}
}

func TestParseGPXBadXML(t *testing.T) {
	if _, err := ParseFile("a.gpx", []byte("not xml")); err == nil {
		t.Fatal("want error for invalid xml")
	}
}

func TestParseText(t *testing.T) {
	cases := []struct {
		name  string
		lines string
		want  []Point
	}{
		{"逗号", "116.1,39.2\n116.2,39.3", []Point{{116.1, 39.2}, {116.2, 39.3}}},
		{"中文逗号与空格", "116.1，39.2\n116.2 39.3", []Point{{116.1, 39.2}, {116.2, 39.3}}},
		{"多余列取前两个数字", "116.1,39.2,1725000000", []Point{{116.1, 39.2}}},
		{"负坐标", "-116.1,-39.2", []Point{{-116.1, -39.2}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := ParseFile("a.csv", []byte(c.lines))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Points) != len(c.want) {
				t.Fatalf("points = %v, want %v", res.Points, c.want)
			}
			for i, p := range res.Points {
				if p != c.want[i] {
					t.Errorf("points[%d] = %v, want %v", i, p, c.want[i])
				}
			}
		})
	}
}

func TestParseTextHeaderSkipped(t *testing.T) {
	res, err := ParseFile("a.csv", []byte("经度,纬度\n116.1,39.2\n\n116.2,39.3\n无效行"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 2 || res.Skipped != 2 { // 表头 + 无效行;空行不计
		t.Fatalf("points=%d skipped=%d, want 2/2", len(res.Points), res.Skipped)
	}
}

func TestParseTextOutOfRange(t *testing.T) {
	if _, err := ParseFile("a.csv", []byte("200.1,39.2")); err == nil {
		t.Fatal("want error when all rows invalid")
	}
}

func TestParseSheetSingleColumnPairs(t *testing.T) {
	// 测试人员实际格式:单列,每行 "经度,纬度" 字符串。
	f := excelize.NewFile()
	for i, v := range []string{"经度,纬度", "116.664053,39.531791", "116.66438,39.53199", ""} {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetCellValue("Sheet1", cell, v); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := ParseFile("行驶路线1.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "xlsx" || len(res.Points) != 2 || res.Skipped != 1 {
		t.Fatalf("format=%s points=%d skipped=%d, want xlsx/2/1", res.Format, len(res.Points), res.Skipped)
	}
	if res.Points[0].Lng != 116.664053 || res.Points[0].Lat != 39.531791 {
		t.Errorf("points[0] = %+v", res.Points[0])
	}
}

func TestParseSheetTwoColumns(t *testing.T) {
	// 双列布局:经度 | 纬度(数字单元格)。
	f := excelize.NewFile()
	rows := [][]interface{}{{116.1, 39.2}, {116.2, 39.3}}
	for i, r := range rows {
		if err := f.SetSheetRow("Sheet1", "A"+string(rune('1'+i)), &r); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := ParseFile("a.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 2 || res.Points[1].Lng != 116.2 || res.Points[1].Lat != 39.3 {
		t.Fatalf("points = %v", res.Points)
	}
}

func TestParseFileLegacyXls(t *testing.T) {
	if _, err := ParseFile("a.xls", []byte("junk")); err == nil ||
		!bytes.Contains([]byte(err.Error()), []byte(".xlsx")) {
		t.Fatalf("want .xlsx hint error, got %v", err)
	}
}

func TestParseFileUnknownExt(t *testing.T) {
	if _, err := ParseFile("a.kml", []byte("x")); err == nil {
		t.Fatal("want error for unsupported ext")
	}
}

// TestParseFileRealFile 端到端:临时目录落盘真实文件后解析。
func TestParseFileRealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "route.csv")
	if err := os.WriteFile(path, []byte("116.1,39.2\n116.2,39.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ParseFile(filepath.Base(path), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 2 {
		t.Fatalf("points = %v", res.Points)
	}
}
