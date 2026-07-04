// Package crypto provides AES-256-GCM field encryption for secrets stored at
// rest (channel/notifier credentials). A nil *Cipher means "no encryption"
// (dev mode) and callers must handle that gracefully.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Cipher wraps an AES-GCM AEAD.
type Cipher struct {
	aead cipher.AEAD
}

// GenerateKey returns a fresh base64-encoded 32-byte key.
func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// NewCipher builds a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// NewCipherFromBase64 builds a Cipher from a base64-encoded 32-byte key. An
// empty string yields (nil, nil) meaning "no encryption".
func NewCipherFromBase64(s string) (*Cipher, error) {
	if s == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid base64 master key: %w", err)
	}
	return NewCipher(key)
}

const marker = "enc:v1:"

// Encrypt encrypts plain and returns a self-describing string. A nil Cipher
// returns the plaintext unchanged.
func (c *Cipher) Encrypt(plain []byte) (string, error) {
	if c == nil {
		return string(plain), nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.aead.Seal(nonce, nonce, plain, nil)
	return marker + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt. Values without the marker are returned as-is so a
// database written in dev mode (no key) can later be read with a key set.
func (c *Cipher) Decrypt(s string) ([]byte, error) {
	if len(s) < len(marker) || s[:len(marker)] != marker {
		return []byte(s), nil // plaintext (dev mode or pre-encryption)
	}
	if c == nil {
		return nil, errors.New("crypto: encrypted value but no master key configured")
	}
	raw, err := base64.StdEncoding.DecodeString(s[len(marker):])
	if err != nil {
		return nil, err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	return c.aead.Open(nil, nonce, ct, nil)
}

// EncryptString is a string-typed convenience wrapper around Encrypt.
func (c *Cipher) EncryptString(s string) (string, error) { return c.Encrypt([]byte(s)) }

// DecryptString is a string-typed convenience wrapper around Decrypt.
func (c *Cipher) DecryptString(s string) (string, error) {
	b, err := c.Decrypt(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
