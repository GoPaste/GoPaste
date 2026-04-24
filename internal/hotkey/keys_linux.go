//go:build linux

package hotkey

import hk "golang.design/x/hotkey"

// Linux X11 keysym 值（XK_xxx），参考 /usr/include/X11/keysymdef.h。
func oemKeys() map[string]hk.Key {
	return map[string]hk.Key{
		"`":             hk.Key(0x0060), // XK_grave
		"-":             hk.Key(0x002D), // XK_minus
		"=":             hk.Key(0x003D), // XK_equal
		"[":             hk.Key(0x005B), // XK_bracketleft
		"]":             hk.Key(0x005D), // XK_bracketright
		"\\":            hk.Key(0x005C), // XK_backslash
		";":             hk.Key(0x003B), // XK_semicolon
		"'":             hk.Key(0x0027), // XK_apostrophe
		",":             hk.Key(0x002C), // XK_comma
		".":             hk.Key(0x002E), // XK_period
		"/":             hk.Key(0x002F), // XK_slash
		"grave":         hk.Key(0x0060),
		"graveaccent":   hk.Key(0x0060),
		"backquote":     hk.Key(0x0060),
		"semicolon":     hk.Key(0x003B),
		"apostrophe":    hk.Key(0x0027),
		"quote":         hk.Key(0x0027),
		"comma":         hk.Key(0x002C),
		"period":        hk.Key(0x002E),
		"slash":         hk.Key(0x002F),
		"backslash":     hk.Key(0x005C),
		"bracketleft":   hk.Key(0x005B),
		"bracketright":  hk.Key(0x005D),
		"minus":         hk.Key(0x002D),
		"equal":         hk.Key(0x003D),
	}
}
