// Package store 提供 JSON 文件持久化(连接配置 + 报文配置)。
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Dir 返回配置目录(os.UserConfigDir()/gbt32960-simulator),不存在则创建。
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "gbt32960-simulator")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Save 把 v 以 JSON 原子写入配置目录下的 name 文件:
// 先写同目录临时文件,再 os.Rename 覆盖目标。并发读者(frontend 轮询)不会
// 读到截断/半截内容,写入过程中崩溃也不会留下损坏的目标文件。
func Save(name string, v any) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() { _ = os.Remove(tmpName) }() // 失败路径清理;rename 成功后为 no-op
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// CreateTemp 默认 0600,修正为与其他配置文件一致的 0644。
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}

// Load 从配置目录读取 name 文件到 v;文件不存在时返回 nil(v 保持零值)。
func Load(name string, v any) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
