//go:build windows

package hotkey

import hk "golang.design/x/hotkey"

// Windows OEM 键 VK 值（与键盘布局无关的物理键位）。
// 参考：https://learn.microsoft.com/en-us/windows/win32/inputdev/virtual-key-codes
func oemKeys() map[string]hk.Key {
	return map[string]hk.Key{
		"`":             hk.Key(0xC0), // VK_OEM_3  — ` / ~
		"-":             hk.Key(0xBD), // VK_OEM_MINUS
		"=":             hk.Key(0xBB), // VK_OEM_PLUS  (物理键是 =)
		"[":             hk.Key(0xDB), // VK_OEM_4
		"]":             hk.Key(0xDD), // VK_OEM_6
		"\\":            hk.Key(0xDC), // VK_OEM_5
		";":             hk.Key(0xBA), // VK_OEM_1
		"'":             hk.Key(0xDE), // VK_OEM_7
		",":             hk.Key(0xBC), // VK_OEM_COMMA
		".":             hk.Key(0xBE), // VK_OEM_PERIOD
		"/":             hk.Key(0xBF), // VK_OEM_2
		"grave":         hk.Key(0xC0),
		"graveaccent":   hk.Key(0xC0),
		"backquote":     hk.Key(0xC0),
		"semicolon":     hk.Key(0xBA),
		"apostrophe":    hk.Key(0xDE),
		"quote":         hk.Key(0xDE),
		"comma":         hk.Key(0xBC),
		"period":        hk.Key(0xBE),
		"slash":         hk.Key(0xBF),
		"backslash":     hk.Key(0xDC),
		"bracketleft":   hk.Key(0xDB),
		"bracketright":  hk.Key(0xDD),
		"minus":         hk.Key(0xBD),
		"equal":         hk.Key(0xBB),
	}
}
