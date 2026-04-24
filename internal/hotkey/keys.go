package hotkey

import (
	"fmt"
	"strings"

	hk "golang.design/x/hotkey"
)

// keyMap 所有支持的键名 → hk.Key 映射。
// 键名统一小写。前端录制后发送的 key.code / key.key 需对应到此表。
// OEM 符号键（` - = [ ] \ ; ' , . /）通过各平台 keys_<os>.go 的 oemKeys() 在 init() 里合并进来。
var keyMap = map[string]hk.Key{
	"a": hk.KeyA, "b": hk.KeyB, "c": hk.KeyC, "d": hk.KeyD,
	"e": hk.KeyE, "f": hk.KeyF, "g": hk.KeyG, "h": hk.KeyH,
	"i": hk.KeyI, "j": hk.KeyJ, "k": hk.KeyK, "l": hk.KeyL,
	"m": hk.KeyM, "n": hk.KeyN, "o": hk.KeyO, "p": hk.KeyP,
	"q": hk.KeyQ, "r": hk.KeyR, "s": hk.KeyS, "t": hk.KeyT,
	"u": hk.KeyU, "v": hk.KeyV, "w": hk.KeyW, "x": hk.KeyX,
	"y": hk.KeyY, "z": hk.KeyZ,

	"0": hk.Key0, "1": hk.Key1, "2": hk.Key2, "3": hk.Key3,
	"4": hk.Key4, "5": hk.Key5, "6": hk.Key6, "7": hk.Key7,
	"8": hk.Key8, "9": hk.Key9,

	"f1": hk.KeyF1, "f2": hk.KeyF2, "f3": hk.KeyF3, "f4": hk.KeyF4,
	"f5": hk.KeyF5, "f6": hk.KeyF6, "f7": hk.KeyF7, "f8": hk.KeyF8,
	"f9": hk.KeyF9, "f10": hk.KeyF10, "f11": hk.KeyF11, "f12": hk.KeyF12,

	"space":     hk.KeySpace,
	" ":         hk.KeySpace,
	"tab":       hk.KeyTab,
	"return":    hk.KeyReturn,
	"enter":     hk.KeyReturn,
	"escape":    hk.KeyEscape,
	"esc":       hk.KeyEscape,
	"delete":    hk.KeyDelete,
	"backspace": hk.KeyDelete,
	"up":        hk.KeyUp,
	"down":      hk.KeyDown,
	"left":      hk.KeyLeft,
	"right":     hk.KeyRight,
	"arrowup":    hk.KeyUp,
	"arrowdown":  hk.KeyDown,
	"arrowleft":  hk.KeyLeft,
	"arrowright": hk.KeyRight,
}

func init() {
	// 合并各平台 OEM 符号键（` - = [ ] \ ; ' , . /）
	for k, v := range oemKeys() {
		keyMap[k] = v
	}
}

func letterKey(c byte) hk.Key {
	k, ok := keyMap[strings.ToLower(string(c))]
	if ok {
		return k
	}
	return hk.KeyV
}

func digitKey(c byte) hk.Key {
	k, ok := keyMap[string(c)]
	if ok {
		return k
	}
	return hk.Key0
}

// ParseKey 解析键名字符串为 hk.Key。支持 A-Z, 0-9, F1-F12, Space, Tab, Enter, Escape, Delete,
// 方向键，以及符号键 ` - = [ ] \ ; ' , . /（通过平台 OEM 键映射）。
func ParseKey(name string) (hk.Key, error) {
	if name == "" {
		return 0, fmt.Errorf("hotkey: empty key")
	}
	k, ok := keyMap[strings.ToLower(name)]
	if ok {
		return k, nil
	}
	// 兼容单字符
	if len(name) == 1 {
		c := strings.ToUpper(name)[0]
		if c >= 'A' && c <= 'Z' {
			return letterKey(c), nil
		}
		if c >= '0' && c <= '9' {
			return digitKey(c), nil
		}
	}
	return 0, fmt.Errorf("hotkey: unsupported key %q", name)
}

// SupportedKeys 返回所有支持的键名列表（供前端展示）。
func SupportedKeys() []string {
	seen := make(map[hk.Key]bool)
	var out []string
	// 优先顺序
	for _, name := range []string{
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
		"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
		"Space", "Tab", "Enter", "Escape", "Delete",
		"Up", "Down", "Left", "Right",
		"`", "-", "=", "[", "]", `\`, ";", "'", ",", ".", "/",
	} {
		k, err := ParseKey(name)
		if err == nil && !seen[k] {
			seen[k] = true
			out = append(out, name)
		}
	}
	return out
}
