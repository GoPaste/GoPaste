// Package crypto 提供 AES-256-GCM 加解密能力。
//
// 密钥由 keyring 模块负责生成与持久化（优先系统 Keychain，不可用时落到本地文件并用机器指纹派生 KEK）。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// KeySize AES-256 密钥长度。
const KeySize = 32

// Cipher 基于 AES-256-GCM 的对称加密器。
type Cipher struct {
	aead cipher.AEAD
}

// New 使用给定的 32 字节密钥创建 Cipher。
func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: key size must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt 加密明文，返回 nonce(12) || ciphertext。
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: read nonce: %w", err)
	}
	out := make([]byte, 0, len(nonce)+len(plaintext)+c.aead.Overhead())
	out = append(out, nonce...)
	out = c.aead.Seal(out, nonce, plaintext, nil)
	return out, nil
}

// Decrypt 解密 nonce(12) || ciphertext 格式的数据。
func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("crypto: data too short")
	}
	nonce, ct := data[:ns], data[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return pt, nil
}

// GenerateKey 生成一个 32 字节的随机密钥。
func GenerateKey() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil, err
	}
	return k, nil
}
