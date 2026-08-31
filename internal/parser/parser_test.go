package parser

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sunsky74/gb32960/codec"
	_ "github.com/sunsky74/gb32960/codec/all"
	"github.com/sunsky74/gb32960/frame"
	mdl "github.com/sunsky74/gb32960/model/gbt2016"
	"github.com/sunsky74/gb32960/utils"
)

func loadHex(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../internal/schema/testdata/" + name)
	if err != nil {
		t.Skipf("golden missing: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func TestParseGoldenLogin(t *testing.T) {
	r, err := Parse(loadHex(t, "prod_login_v2016_01.hex"))
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalBytes != 55 || r.Version != "V2016" {
		t.Errorf("basic = %d/%s", r.TotalBytes, r.Version)
	}
	if r.Command != "0x01 VEHICLE_LOGIN" || r.VIN != "H3V21BA29SZ003109" {
		t.Errorf("cmd/vin = %s / %s", r.Command, r.VIN)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v", r.Warnings)
	}
	// 交叉验证:与库 codec 解码结果一致
	msg, _ := codec.ProtocolCodec.Decode(utils.NewByteReader(mustHex(t, loadHex(t, "prod_login_v2016_01.hex"))))
	pm := msg.(*frame.ProtocolMessage)
	_ = pm.DecodePayload()
	login := pm.Payload.(*mdl.VehicleLogin)

	find := func(name string) *Field {
		for i := range r.Fields {
			if r.Fields[i].Name == name {
				return &r.Fields[i]
			}
		}
		return nil
	}
	if f := find("登入流水号"); f == nil || f.RawValue != "2" {
		t.Errorf("serial field = %+v (codec says %d)", f, login.SerialNum)
	}
	if f := find("数据采集时间"); f == nil || f.Translate != login.BeanTime.String() {
		t.Errorf("beanTime field = %+v (codec says %s)", f, login.BeanTime.String())
	}
	if f := find("ICCID"); f == nil || f.RawValue != login.ICCID {
		t.Errorf("iccid = %+v (codec says %s)", f, login.ICCID)
	}
}

func TestParseGoldenRealtimeCrossValidate(t *testing.T) {
	hexStr := loadHex(t, "prod_realtime_v2016_01.hex")
	r, err := Parse(hexStr)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v", r.Warnings)
	}

	msg, _ := codec.ProtocolCodec.Decode(utils.NewByteReader(mustHex(t, hexStr)))
	pm := msg.(*frame.ProtocolMessage)
	if err := pm.DecodePayload(); err != nil {
		t.Fatal(err)
	}
	rt := pm.Payload.(*mdl.RealTimeData)

	// 物理值交叉验证:解析表中的偏移值必须等于 codec 解码值
	cv := map[string]float64{}
	if rt.VehicleData != nil {
		cv["车速"] = rt.VehicleData.Speed
		cv["累计里程"] = rt.VehicleData.Mileage
		cv["总电压"] = rt.VehicleData.Voltage
		cv["总电流"] = rt.VehicleData.Current
	}
	if rt.LocationData != nil {
		cv["经度(东经)"] = rt.LocationData.Longitude
		cv["纬度(北纬)"] = rt.LocationData.Latitude
	}
	if rt.ExtremumData != nil {
		cv["单体电池电压最高值"] = rt.ExtremumData.MaxVoltage
		cv["最高温度值"] = rt.ExtremumData.MaxTemperature
	}
	for name, want := range cv {
		f := findField(r.Fields, name)
		if f == nil {
			t.Errorf("field %s not found", name)
			continue
		}
		got := parseFloat(t, f.OffsetVal)
		if abs(got-want) > 0.01 {
			t.Errorf("%s: parser=%s codec=%v", name, f.OffsetVal, want)
		}
	}

	// 枚举翻译抽查
	if f := findField(r.Fields, "DC/DC 状态"); f != nil && f.Translate != "工作" {
		t.Errorf("DC translate = %s", f.Translate)
	}
	// BCC 通过
	if f := findField(r.Fields, "校验码 BCC"); f != nil && f.Translate != "校验通过" {
		t.Errorf("bcc = %s", f.Translate)
	}
	// 字段行覆盖量合理(9 组全开的实时报文应有大量字段行)
	if len(r.Fields) < 60 {
		t.Errorf("fields too few: %d", len(r.Fields))
	}
}

func TestParseHexFormats(t *testing.T) {
	raw := loadHex(t, "prod_logout_v2016_01.hex")
	variants := []string{
		raw,
		spaced(raw),
		strings.ToUpper(raw),
		"0x" + raw[:2] + " 0x" + raw[2:],
		joinEvery(raw, 4, "-"),
	}
	for i, v := range variants {
		if _, err := Parse(v); err != nil {
			t.Errorf("variant %d failed: %v", i, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Error("empty should fail")
	}
	if _, err := Parse("23 01 zz"); err == nil {
		t.Error("illegal char should fail")
	}
	if _, err := Parse("2323"); err == nil {
		t.Error("too short should fail")
	}
	if _, err := Parse("232301"); err == nil {
		t.Error("odd bytes should fail")
	}
}

func TestParseV2025HeaderOnly(t *testing.T) {
	r, err := Parse(loadHex(t, "prod_realtime_v2025_01.hex"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "V2025" || r.VersionByte != "$$" {
		t.Errorf("version = %s %s", r.Version, r.VersionByte)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v", r.Warnings)
	}
	found := false
	for _, f := range r.Fields {
		if strings.Contains(f.Name, "V2025") && f.Offset == 24 {
			found = true
		}
	}
	if !found {
		t.Error("v2025 payload placeholder row missing")
	}
}

func TestNormalizeHex(t *testing.T) {
	b, err := NormalizeHex("23 23\n01,fe\tdead")
	if err != nil || len(b) != 6 || b[0] != 0x23 || b[5] != 0xad {
		t.Errorf("normalize = %x err=%v", b, err)
	}
}

/* helpers */

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := utils.HexToBytes(strings.TrimSpace(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func findField(fs []Field, name string) *Field {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}

func parseFloat(t *testing.T, s string) float64 {
	t.Helper()
	var v float64
	if _, err := fmt.Sscan(s, &v); err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func abs(x float64) float64 { if x < 0 { return -x }; return x }

func spaced(raw string) string {
	var out strings.Builder
	for i, c := range raw {
		if i%2 == 0 && i > 0 {
			out.WriteByte(' ')
		}
		out.WriteRune(c)
	}
	return out.String()
}

func joinEvery(s string, n int, sep string) string {
	var parts []string
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		parts = append(parts, s[i:end])
	}
	return strings.Join(parts, sep)
}
