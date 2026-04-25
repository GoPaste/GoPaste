//go:build darwin

package hotkey

import hk "golang.design/x/hotkey"

func modCtrl() hk.Modifier { return hk.ModCtrl }
func modAlt() hk.Modifier  { return hk.ModOption }
func modCmd() hk.Modifier  { return hk.ModCmd }
