// Package secret 负责静态加密（上游 API key）和随机令牌的生成与摘要。
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

// Box 用 AES-256-GCM 加解密；密文格式为 nonce || ciphertext。
type Box struct {
	aead cipher.AEAD
}

func NewBox(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("主密钥必须是 32 字节")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

func (b *Box) Open(sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("密文过短")
	}
	return b.aead.Open(nil, sealed[:n], sealed[n:], nil)
}

// Token 返回 32 字节随机数的 base64url 编码（43 个字符）。
func Token() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand 在受支持的平台上不会失败
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// Hash 用于存储会话令牌与分发 key：随机值熵足够，sha256 即可，不需要慢哈希。
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Hint 返回密钥末 4 个字符，便于管理员辨认。按 rune 切，不能把中文/emoji 截成半个。
func Hint(s string) string {
	runes := []rune(s)
	if len(runes) <= 4 {
		return "****"
	}
	return "…" + string(runes[len(runes)-4:])
}
