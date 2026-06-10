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
	ResetFilterOnShow  bool     `json:"resetFilterOnShow"`
	ClearSearchOnShow bool     `json:"clearSearchOnShow"` // 激活时清空搜索栏
	SilentStart       bool     `json:"silentStart"`       // 静默启动（启动时隐藏窗口）
	ShowTrayIcon      bool     `json:"showTrayIcon"`      // 显示菜单栏/托盘图标
	ShowTaskbarIcon   bool     `json:"showTaskbarIcon"`   // 显示任务栏图标
	TrayIconStyle     string   `json:"trayIconStyle"`     // 菜单栏图标风格："color"（彩色）| "gray"（灰色）
	AutoStart         bool     `json:"autoStart"`         // 开机自启动
	TabHotkeysEnabled bool     `json:"tabHotkeysEnabled"` // 是否启用 Alt+1..6 全局切分类热键（关掉避免与其它软件冲突）

	// 扩展功能 —— macOS Cmd+Q 防误触
	// CmdQBehavior：
	//   "default" | ""（缺省）— 保留系统原生行为，按下立即退出
	//   "confirm"            — 二次确认：按一次 Cmd+Q 弹 toast，时间窗内再按一次才退出
	//   "disable"            — 完全禁用，仅通过托盘/标题栏等其他方式退出
	// 仅 macOS 生效；非 macOS 平台前端隐藏相关 UI，字段保留以便跨设备同步配置。
	CmdQBehavior      string `json:"cmdQBehavior"`
	CmdQConfirmWindow int    `json:"cmdQConfirmWindow"` // confirm 模式下的时间窗（毫秒），默认 1500

	// EmojiEnabled Emoji 功能总开关（默认开启）。
	// 关闭：前端不挂载 EmojiPicker、不显示 emoji tab，组件 onBeforeUnmount 会释放
	//      已构建的 sprite blob URL 等资源；同时不会触发任何 prewarm。
	// 开启：恢复 emoji tab 与面板，按需触发 tone 0 prewarm（tones 1..5 仍受
	//      ExtendedEmoji 开关 + 用户首次进入 emoji 视图的双重门控）。
	EmojiEnabled bool `json:"emojiEnabled"`
	// ExtendedEmoji 显示完整 Emoji 库。
	// 关闭（默认）：在表情面板隐藏 "物品 / 旗帜" 两个分类，并隐藏右上角肤色切换按钮，
	//             同时跳过 tone 1..5 sprite 的预热，可省下约 50MB GPU 内存。
	// 开启：显示全部分类与肤色按钮，组件会防抖触发一次 tones 1..5 prewarm。
	ExtendedEmoji bool `json:"extendedEmoji"`
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
		ResetFilterOnShow:  true,
		ClearSearchOnShow: true,
		SilentStart:       false,
		ShowTrayIcon:      true,
		ShowTaskbarIcon:   false,
		TrayIconStyle:     "color",
		AutoStart:         false,
		TabHotkeysEnabled: true,
		CmdQBehavior:      "default",
		CmdQConfirmWindow: 1500,
		EmojiEnabled:      true,
		ExtendedEmoji:     false,
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
	// 兼容旧配置：CmdQ 相关字段缺失时回填默认值
	if s.cur.CmdQBehavior == "" {
		s.cur.CmdQBehavior = "default"
	}
	if s.cur.CmdQConfirmWindow <= 0 {
		s.cur.CmdQConfirmWindow = 1500
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
