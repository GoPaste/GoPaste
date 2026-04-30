// Package hotkey 提供全局快捷键注册能力。
//
// 实际调用由 golang.design/x/hotkey 完成，平台行为差异（尤其是 Linux Wayland）会在 Register 时返回错误。
package hotkey

import (
	"context"
	"fmt"
	"strings"

	hk "golang.design/x/hotkey"
)

// Manager 快捷键管理器。
//
// 历史上只持有单个快捷键（用于唤出主面板）；现支持额外注册多个"附加"快捷键
// （比如 Alt+0..6 用于唤起并切到对应 tab）。Close 时会一并注销。
type Manager struct {
	hotkey *hk.Hotkey
	cancel context.CancelFunc

	// 附加快捷键：每个元素都有自己的 hotkey 句柄 + 监听 goroutine 的 cancel
	extras []*Manager
}

// New 根据给定的修饰键 + 主键注册全局快捷键。
//
// modifiers 支持 "ctrl" / "shift" / "alt" / "cmd"（大小写不敏感）；
// key 支持 A-Z、0-9（其他保留字符后续扩展）。
// 成功后 fn 会在每次按下时被调用。调用方应在不再需要时调用 Close。
func New(ctx context.Context, modifiers []string, key string, fn func()) (*Manager, error) {
	mods, err := parseModifiers(modifiers)
	if err != nil {
		return nil, err
	}
	k, err := ParseKey(key)
	if err != nil {
		return nil, err
	}

	h := hk.New(mods, k)
	if err := h.Register(); err != nil {
		return nil, err
	}

	cctx, cancel := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case <-cctx.Done():
				return
			case <-h.Keydown():
				fn()
			}
		}
	}()
	return &Manager{hotkey: h, cancel: cancel}, nil
}

// Add 在已有 Manager 上追加注册一个快捷键。
//
// 用于在主热键之外再挂多个全局热键（如 Alt+0..6 切 tab）。
// 失败时单个快捷键忽略（返回 error 但不影响其它已注册项）。
// 调用 Close 时会一并注销所有附加项。
func (m *Manager) Add(ctx context.Context, modifiers []string, key string, fn func()) error {
	if m == nil {
		return fmt.Errorf("hotkey: nil manager")
	}
	sub, err := New(ctx, modifiers, key, fn)
	if err != nil {
		return err
	}
	m.extras = append(m.extras, sub)
	return nil
}

// Close 注销快捷键（包括所有附加项）。
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	// 先关附加项（互不依赖，逐个关，错误丢弃）
	for _, e := range m.extras {
		_ = e.Close()
	}
	m.extras = nil

	if m.hotkey == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	return m.hotkey.Unregister()
}

func parseModifiers(mods []string) ([]hk.Modifier, error) {
	if len(mods) == 0 {
		return nil, fmt.Errorf("hotkey: empty modifiers")
	}
	out := make([]hk.Modifier, 0, len(mods))
	for _, m := range mods {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "ctrl", "control":
			out = append(out, modCtrl())
		case "shift":
			out = append(out, hk.ModShift)
		case "alt", "option":
			out = append(out, modAlt())
		case "cmd", "command", "super", "win":
			out = append(out, modCmd())
		default:
			return nil, fmt.Errorf("hotkey: unknown modifier %q", m)
		}
	}
	return out, nil
}
