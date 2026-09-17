package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 缓存根目录内的固定文件名(状态文件与 helper 日志;CleanupCache 白名单同源)。
const (
	lastResultName = "last-result.json"
	helperLogName  = "helper.log"
)

// helperLogLimit 单文件日志上限:打开时超过即截断(无轮转;§5.4)。
const helperLogLimit = 1 << 20 // 1MiB

// LastResult 更新结果文件 last-result.json 的固定 schema(§5.4;字段名即落盘契约)。
type LastResult struct {
	OK            bool   `json:"ok"`
	TargetVersion string `json:"targetVersion"`
	Reason        string `json:"reason,omitempty"`
	LogPath       string `json:"logPath"`
}

// ApplyOutcome 绑定面返回(比 LastResult 多 present 标志:区分"无记录"与"失败记录")。
type ApplyOutcome struct {
	Present       bool   `json:"present"`
	OK            bool   `json:"ok"`
	TargetVersion string `json:"targetVersion"`
	Reason        string `json:"reason"`
	LogPath       string `json:"logPath"`
}

// AsOutcome 投影结果文件为绑定面返回值;nil(无记录)→ {Present:false}。
func AsOutcome(r *LastResult) ApplyOutcome {
	if r == nil {
		return ApplyOutcome{Present: false}
	}
	return ApplyOutcome{
		Present:       true,
		OK:            r.OK,
		TargetVersion: r.TargetVersion,
		Reason:        r.Reason,
		LogPath:       r.LogPath,
	}
}

// UpdatesRoot 更新缓存根目录(UserCacheDir()/gbt32960-simulator/updates;§5.3)。
func UpdatesRoot() (string, error) {
	cd, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cd, "gbt32960-simulator", "updates"), nil
}

// LastResultPath 结果文件路径(<root>/last-result.json)。
func LastResultPath() (string, error) {
	root, err := UpdatesRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, lastResultName), nil
}

// HelperLogPath helper 日志路径(<root>/helper.log)。
func HelperLogPath() (string, error) {
	root, err := UpdatesRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, helperLogName), nil
}

// WriteLastResult 原子写结果文件:MkdirAll 根目录 → 临时文件 → rename;任一步失败清理临时文件。
func WriteLastResult(r LastResult) error {
	p, err := LastResultPath()
	if err != nil {
		return err
	}
	root := filepath.Dir(p)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(root, "."+lastResultName+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// ReadLastResult 读结果文件;不存在 → (nil, nil),非法 JSON → error。
func ReadLastResult() (*LastResult, error) {
	p, err := LastResultPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r LastResult
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ClearLastResult 删除结果文件(读取即清除;不存在视为成功)。
func ClearLastResult() error {
	p, err := LastResultPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// OpenHelperLog 以追加写打开 helper 日志(MkdirAll 根目录;>1MiB 先截断)。
func OpenHelperLog() (*os.File, error) {
	p, err := HelperLogPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err == nil && fi.Size() > helperLogLimit {
		if err := f.Truncate(0); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return f, nil
}
