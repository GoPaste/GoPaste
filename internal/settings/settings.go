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
	HotkeyModifiers   []string `json:"hotkeyModifiers"`
	HotkeyKey         string   `json:"hotkeyKey"`
	MaxItems          int      `json:"maxItems"`
	MaxDays           int      `json:"maxDays"`
	Theme             string   `json:"theme"`
	Language          string   `json:"language"`
	AutoPaste         bool     `json:"autoPaste"`   // Deprecated: 保留字段以兼容旧配置反序列化；新版由 PasteTrigger 控制
	HideOnPaste       bool     `json:"hideOnPaste"` // Deprecated: 同上
	PasteTrigger      string   `json:"pasteTrigger"` // "single" | "double"，决定列表项何种点击动作触发粘贴
	WindowPosition    string   `json:"windowPosition"`
	ScrollTopOnShow   bool     `json:"scrollTopOnShow"`
	ResetFilterOnShow bool     `json:"resetFilterOnShow"`
	SilentStart       bool     `json:"silentStart"`       // 静默启动（启动时隐藏窗口）
	ShowTrayIcon      bool     `json:"showTrayIcon"`      // 显示菜单栏/托盘图标
	ShowTaskbarIcon   bool     `json:"showTaskbarIcon"`   // 显示任务栏图标
	TrayIconStyle     string   `json:"trayIconStyle"`     // 菜单栏图标风格："color"（彩色）| "gray"（灰色）
	AutoStart         bool     `json:"autoStart"`         // 开机自启动
	TabHotkeysEnabled bool     `json:"tabHotkeysEnabled"` // 是否启用 Alt+1..6 全局切分类热键（关掉避免与其它软件冲突）
}

// Default 返回默认设置。
func Default() Settings {
	return Settings{
		HotkeyModifiers:   []string{"alt"},
		HotkeyKey:         "`",
		MaxItems:          1000,
		MaxDays:           30,
		Theme:             "dark",
		Language:          "zh",
		AutoPaste:         true,
		HideOnPaste:       true,
		PasteTrigger:      "double",
		WindowPosition:    "center",
		ScrollTopOnShow:   true,
		ResetFilterOnShow: true,
		SilentStart:       false,
		ShowTrayIcon:      true,
		ShowTaskbarIcon:   false,
		TrayIconStyle:     "color",
		AutoStart:         false,
		TabHotkeysEnabled: true,
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
	// 先填充默认值，再用 JSON 覆盖——缺失的字段自动保持默认
	merged := Default()
	if err := json.Unmarshal(data, &merged); err != nil {
		// 文件损坏：回退默认值，但不覆盖原文件
		return s, nil
	}
	s.cur = merged
	// 兼容旧配置：如果未设置 PasteTrigger，默认使用双击（与旧版 autoPaste=true 等价）
	if s.cur.PasteTrigger == "" {
		s.cur.PasteTrigger = "double"
	}
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
