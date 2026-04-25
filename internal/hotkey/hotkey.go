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
type Manager struct {
	hotkey *hk.Hotkey
	cancel context.CancelFunc
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

// Close 注销快捷键。
func (m *Manager) Close() error {
	if m == nil || m.hotkey == nil {
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
