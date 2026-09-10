// Package track 解析轨迹文件(GPX / Excel .xlsx / CSV 文本)为经纬度点序列。
// 纯解析无副作用,供 bridge 层调用。
package track

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Point 一个轨迹点(十进制度:经度 Lng、纬度 Lat)。
type Point struct {
	Lng float64
	Lat float64
}

// Result 解析结果:有效点序列、被跳过的行数(空行不计)、格式标识。
type Result struct {
	Points  []Point
	Skipped int
	Format  string
}

// ParseFile 按文件扩展名分发解析。
// .gpx → GPX;.xlsx → Excel(第一个工作表);.csv/.txt → 文本;
// .xls(旧格式)不支持,提示另存为 .xlsx。
func ParseFile(name string, data []byte) (Result, error) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".gpx"):
		return parseGPX(data)
	case strings.HasSuffix(lower, ".xlsx"):
		return parseSheet(data)
	case strings.HasSuffix(lower, ".csv"), strings.HasSuffix(lower, ".txt"):
		return parseText(data)
	case strings.HasSuffix(lower, ".xls"):
		return Result{}, fmt.Errorf("旧版 .xls 不受支持,请在 Excel 中另存为 .xlsx 后重试")
	default:
		return Result{}, fmt.Errorf("不支持的轨迹文件类型: %s(支持 .gpx / .xlsx / .csv)", name)
	}
}

// ---------------------------------------------------------------- GPX

// gpxDoc 只取关心的元素:轨迹点/路径点/航点(带 lat/lon 属性)。
// Go xml 无命名空间标签按本地名匹配,兼容各 GPX 版本的默认命名空间。
type gpxDoc struct {
	TRK []struct {
		SEGS []struct {
			PTS []gpxPoint `xml:"trkpt"`
		} `xml:"trkseg"`
	} `xml:"trk"`
	RTE []struct {
		PTS []gpxPoint `xml:"rtept"`
	} `xml:"rte"`
	WPTS []gpxPoint `xml:"wpt"`
}

type gpxPoint struct {
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
}

func parseGPX(data []byte) (Result, error) {
	var doc gpxDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return Result{}, fmt.Errorf("GPX 解析失败: %w", err)
	}
	res := Result{Format: "gpx"}
	add := func(pts []gpxPoint) {
		for _, p := range pts {
			pt := Point{Lng: p.Lon, Lat: p.Lat}
			if valid(pt) {
				res.Points = append(res.Points, pt)
			} else {
				res.Skipped++
			}
		}
	}
	// 主轨迹在前,路径/航点兜底(部分导出工具只写 wpt)。
	for _, trk := range doc.TRK {
		for _, seg := range trk.SEGS {
			add(seg.PTS)
		}
	}
	for _, rte := range doc.RTE {
		add(rte.PTS)
	}
	add(doc.WPTS)
	return doneCheck(res)
}

// ---------------------------------------------------------------- Excel / 文本

// parseSheet 解析 .xlsx 第一个工作表。单元格取字符串值,逐行走与文本
// 相同的解析路径:单列 "经度,纬度" 字符串与经度|纬度两列布局均支持。
func parseSheet(data []byte) (Result, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return Result{}, fmt.Errorf("Excel 打开失败(仅支持 .xlsx): %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return Result{}, fmt.Errorf("Excel 工作簿中没有工作表")
	}
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return Result{}, fmt.Errorf("读取工作表失败: %w", err)
	}
	defer rows.Close()
	res := Result{Format: "xlsx"}
	for rows.Next() {
		cells, err := rows.Columns()
		if err != nil {
			return Result{}, fmt.Errorf("读取行失败: %w", err)
		}
		nonEmpty := make([]string, 0, len(cells))
		for _, c := range cells {
			if s := strings.TrimSpace(c); s != "" {
				nonEmpty = append(nonEmpty, s)
			}
		}
		if len(nonEmpty) == 0 {
			continue // 空行不计入跳过
		}
		if p, ok := parseLine(strings.Join(nonEmpty, ",")); ok {
			res.Points = append(res.Points, p)
		} else {
			res.Skipped++
		}
	}
	return doneCheck(res)
}

// parseText 解析 CSV/纯文本:每行 "经度,纬度"(兼容中文逗号/分号/制表符/
// 空格分隔;多余列取前两个数字;表头等非数字行跳过)。
func parseText(data []byte) (Result, error) {
	res := Result{Format: "csv"}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if p, ok := parseLine(line); ok {
			res.Points = append(res.Points, p)
		} else {
			res.Skipped++
		}
	}
	return doneCheck(res)
}

var separators = strings.NewReplacer(
	"，", ",", "；", ",", ";", ",",
	"\t", ",", " ", ",",
)

// parseLine 从一行文本提取 (经度, 纬度):归一化分隔符后取前两个可解析
// 的数字。提取失败或范围非法返回 false。
func parseLine(line string) (Point, bool) {
	parts := strings.Split(separators.Replace(line), ",")
	vals := make([]float64, 0, 2)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			continue // 非数字 token(表头等)跳过
		}
		vals = append(vals, f)
		if len(vals) == 2 {
			break
		}
	}
	if len(vals) < 2 {
		return Point{}, false
	}
	pt := Point{Lng: vals[0], Lat: vals[1]}
	return pt, valid(pt)
}

// valid 范围校验:经度 [-180,180],纬度 [-90,90]。
func valid(p Point) bool {
	return p.Lng >= -180 && p.Lng <= 180 && p.Lat >= -90 && p.Lat <= 90
}

// doneCheck 至少一个有效点才算解析成功。
func doneCheck(res Result) (Result, error) {
	if len(res.Points) == 0 {
		if res.Skipped > 0 {
			return Result{}, fmt.Errorf("未解析到有效坐标点(跳过 %d 行;每行应为\"经度,纬度\")", res.Skipped)
		}
		return Result{}, fmt.Errorf("未解析到有效坐标点(每行应为\"经度,纬度\")")
	}
	return res, nil
}
