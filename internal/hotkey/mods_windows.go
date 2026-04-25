//go:build windows

package hotkey

import hk "golang.design/x/hotkey"

func modCtrl() hk.Modifier { return hk.ModCtrl }
func modAlt() hk.Modifier  { return hk.ModAlt }
func modCmd() hk.Modifier  { return hk.ModWin } // Win 键映射到 "cmd"
