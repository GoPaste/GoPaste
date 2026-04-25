//go:build !darwin

package tray

// SetDockClickCallback is a no-op on non-macOS platforms.
func SetDockClickCallback(fn func()) {}
