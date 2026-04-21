package crypto

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "gopaste"
	keyringUser    = "dek" // Data Encryption Key
)

// LoadOrCreateKey 返回持久化的 32 字节数据密钥。
//
// 优先从系统 Keychain 读取；不可用时落盘到 fallbackPath（应为用户数据目录内权限 0600 的文件）。
// 首次调用会自动生成并持久化。
func LoadOrCreateKey(fallbackPath string) ([]byte, error) {
	// 1) 尝试 Keychain
	if hexKey, err := keyring.Get(keyringService, keyringUser); err == nil {
		if k, err := hex.DecodeString(hexKey); err == nil && len(k) == KeySize {
			return k, nil
		}
	}

	// 2) 尝试回退文件
	if fallbackPath != "" {
		if data, err := os.ReadFile(fallbackPath); err == nil {
			if k, err := hex.DecodeString(string(data)); err == nil && len(k) == KeySize {
				// 同步到 Keychain（若支持）
				_ = keyring.Set(keyringService, keyringUser, string(data))
				return k, nil
			}
		}
	}

	// 3) 生成新密钥
	k, err := GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("keyring: generate: %w", err)
	}
	hexKey := hex.EncodeToString(k)

	// 优先存 Keychain
	if err := keyring.Set(keyringService, keyringUser, hexKey); err != nil {
		// 回退到文件
		if fallbackPath == "" {
			return nil, fmt.Errorf("keyring: no fallback path and keyring unavailable: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(fallbackPath), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(fallbackPath, []byte(hexKey), 0o600); err != nil {
			return nil, fmt.Errorf("keyring: write fallback: %w", err)
		}
	}
	return k, nil
}

// ErrNoKey 表示密钥尚未初始化。
var ErrNoKey = errors.New("crypto: no key loaded")
