//go:build !darwin

package clipboard

import (
	"fmt"

	"golang.design/x/clipboard"
)

// WriteText 非 darwin 平台沿用 golang.design/x/clipboard。
func WriteText(b []byte) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}
	clipboard.Write(clipboard.FmtText, b)
	return nil
}

// WriteImage 非 darwin 平台沿用 golang.design/x/clipboard。
func WriteImage(b []byte) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}
	clipboard.Write(clipboard.FmtImage, b)
	return nil
}
