//go:build !darwin

package tray

// startPlatform 在非 macOS 平台上不处理，交由 systray 路径处理。
func startPlatform(_ Callbacks) (cleanup func(), ok bool) {
	return nil, false
}

// setIconStylePlatform 在非 macOS 平台为 no-op。
func setIconStylePlatform(_ string) {}
