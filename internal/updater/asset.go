package updater

import "errors"

// ErrUnsupportedPlatform 当前平台无匹配的更新产物。
var ErrUnsupportedPlatform = errors.New("当前平台暂不支持自动更新")

// expectedAssetName 发布资产命名契约(设计文档 §5.2;精确匹配,fail-closed):
//
//	darwin(universal) → gbt32960-simulator.app.zip
//	windows/amd64     → gbt32960-simulator.exe(精确匹配天然排除 *-installer.exe)
//	linux/amd64       → gbt32960-simulator(无扩展名)
func expectedAssetName(goos, goarch string) (string, bool) {
	switch {
	case goos == "darwin":
		return "gbt32960-simulator.app.zip", true
	case goos == "windows" && goarch == "amd64":
		return "gbt32960-simulator.exe", true
	case goos == "linux" && goarch == "amd64":
		return "gbt32960-simulator", true
	default:
		return "", false
	}
}

// MatchAsset 从发布资产中选出当前平台的更新产物;无匹配时返回 ErrUnsupportedPlatform。
func MatchAsset(goos, goarch string, assets []Asset) (Asset, error) {
	want, ok := expectedAssetName(goos, goarch)
	if !ok {
		return Asset{}, ErrUnsupportedPlatform
	}
	for _, a := range assets {
		if a.Name == want {
			return a, nil
		}
	}
	return Asset{}, ErrUnsupportedPlatform
}
