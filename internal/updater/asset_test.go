package updater

import (
	"errors"
	"testing"
)

var sampleAssets = []Asset{
	{Name: "gbt32960-simulator", Size: 14155776},
	{Name: "gbt32960-simulator-amd64-installer.exe", Size: 7920000},
	{Name: "gbt32960-simulator.app.zip", Size: 11430000},
	{Name: "gbt32960-simulator.exe", Size: 15940000},
}

func TestMatchAsset(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
	}{
		{"darwin", "arm64", "gbt32960-simulator.app.zip"},
		{"darwin", "amd64", "gbt32960-simulator.app.zip"},
		{"windows", "amd64", "gbt32960-simulator.exe"}, // 精确匹配天然排除 installer
		{"linux", "amd64", "gbt32960-simulator"},
	}
	for _, c := range cases {
		got, err := MatchAsset(c.goos, c.goarch, sampleAssets)
		if err != nil || got.Name != c.want {
			t.Fatalf("MatchAsset(%s,%s) = %q,%v want %q", c.goos, c.goarch, got.Name, err, c.want)
		}
	}
}

func TestMatchAssetUnsupported(t *testing.T) {
	for _, c := range [][2]string{{"windows", "arm64"}, {"linux", "arm64"}, {"freebsd", "amd64"}} {
		if _, err := MatchAsset(c[0], c[1], sampleAssets); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Fatalf("MatchAsset(%s,%s) err = %v", c[0], c[1], err)
		}
	}
	if _, err := MatchAsset("darwin", "arm64", nil); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal("空资产列表应返回 ErrUnsupportedPlatform")
	}
}
