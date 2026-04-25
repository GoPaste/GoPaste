//go:build darwin

package hotkey

import hk "golang.design/x/hotkey"

// macOS Carbon/HIToolbox keycode（kVK_ANSI_xxx），与键盘布局无关的物理键位。
// 参考：HIToolbox/Events.h
func oemKeys() map[string]hk.Key {
	return map[string]hk.Key{
		"`":             hk.Key(50), // kVK_ANSI_Grave
		"-":             hk.Key(27), // kVK_ANSI_Minus
		"=":             hk.Key(24), // kVK_ANSI_Equal
		"[":             hk.Key(33), // kVK_ANSI_LeftBracket
		"]":             hk.Key(30), // kVK_ANSI_RightBracket
		"\\":            hk.Key(42), // kVK_ANSI_Backslash
		";":             hk.Key(41), // kVK_ANSI_Semicolon
		"'":             hk.Key(39), // kVK_ANSI_Quote
		",":             hk.Key(43), // kVK_ANSI_Comma
		".":             hk.Key(47), // kVK_ANSI_Period
		"/":             hk.Key(44), // kVK_ANSI_Slash
		"grave":         hk.Key(50),
		"graveaccent":   hk.Key(50),
		"backquote":     hk.Key(50),
		"semicolon":     hk.Key(41),
		"apostrophe":    hk.Key(39),
		"quote":         hk.Key(39),
		"comma":         hk.Key(43),
		"period":        hk.Key(47),
		"slash":         hk.Key(44),
		"backslash":     hk.Key(42),
		"bracketleft":   hk.Key(33),
		"bracketright":  hk.Key(30),
		"minus":         hk.Key(27),
		"equal":         hk.Key(24),
	}
}
