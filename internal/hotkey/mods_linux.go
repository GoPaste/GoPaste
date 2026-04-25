//go:build linux

package hotkey

import hk "golang.design/x/hotkey"

// Linux 下 Alt 是 Mod1，没有 Cmd 概念。
func modCtrl() hk.Modifier { return hk.ModCtrl }
func modAlt() hk.Modifier  { return hk.Mod1 }
func modCmd() hk.Modifier  { return hk.Mod4 } // 映射到 Super/Win
