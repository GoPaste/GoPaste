// Package settings 管理用户偏好设置，持久化为 JSON 文件。
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Settings 用户偏好。
type Settings struct {
	HotkeyModifiers []string `json:"hotkeyModifiers"` // ["ctrl","shift"] 或 ["cmd","shift"]
	HotkeyKey       string   `json:"hotkeyKey"`       // "V"
	MaxItems        int      `json:"maxItems"`        // 非收藏/置顶的最大保留条数，0 表示不限制
	MaxDays         int      `json:"maxDays"`         // 保留天数，0 表示不限制
	Theme           string   `json:"theme"`           // "dark" | "light" | "auto"
	AutoPaste       bool     `json:"autoPaste"`       // 选中后是否模拟发送粘贴键
	HideOnPaste     bool     `json:"hideOnPaste"`     // 粘贴后是否自动隐藏窗口
}

// Default 返回默认设置。
func Default() Settings {
	return Settings{
		HotkeyModifiers: []string{"ctrl", "shift"},
		HotkeyKey:       "V",
		MaxItems:        1000,
		MaxDays:         30,
		Theme:           "dark",
		AutoPaste:       true,
		HideOnPaste:     true,
	}
}

// Store 线程安全的设置存储。
type Store struct {
	path string
	mu   sync.RWMutex
	cur  Settings
}

// Open 加载或创建配置文件。
func Open(path string) (*Store, error) {
	s := &Store{path: path, cur: Default()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// 使用默认并持久化
			if err := s.save(); err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, err
	}
	var loaded Settings
	if err := json.Unmarshal(data, &loaded); err != nil {
		// 文件损坏：回退默认值，但不覆盖原文件（避免丢用户数据）
		return s, nil
	}
	// 合并：用户未设置的字段保持默认
	merged := Default()
	if len(loaded.HotkeyModifiers) > 0 {
		merged.HotkeyModifiers = loaded.HotkeyModifiers
	}
	if loaded.HotkeyKey != "" {
		merged.HotkeyKey = loaded.HotkeyKey
	}
	merged.MaxItems = loaded.MaxItems
	merged.MaxDays = loaded.MaxDays
	if loaded.Theme != "" {
		merged.Theme = loaded.Theme
	}
	merged.AutoPaste = loaded.AutoPaste
	merged.HideOnPaste = loaded.HideOnPaste
	s.cur = merged
	return s, nil
}

// Get 返回当前设置快照。
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Set 更新并持久化。
func (s *Store) Set(ns Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = ns
	return s.save()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
