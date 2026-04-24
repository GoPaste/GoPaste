//go:build !darwin

package tray

// setTrayIconSizePt 在非 darwin 平台上是 no-op。
func setTrayIconSizePt(_ float64) {}
