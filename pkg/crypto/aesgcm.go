package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Cipher performs AES-256-GCM authenticated encryption for secrets at rest
// (WireGuard private keys, VNC passwords, runner tokens).
type Cipher struct {
	aead cipher.AEAD
}

// NewCipherFromBase64 builds an AES-256-GCM Cipher from a base64-encoded 32-byte key.
func NewCipherFromBase64(b64Key string) (*Cipher, error) {
	raw, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt encodes plaintext as (nonce || ciphertext || tag).
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decodes a (nonce || ciphertext || tag) payload produced by Encrypt.
func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(blob) < ns+c.aead.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := blob[:ns], blob[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// GenerateToken returns a cryptographically random URL-safe token and its SHA-256 hex digest.
func GenerateToken(numBytes int) (raw string, hashHex string, err error) {
	if numBytes < 16 {
		numBytes = 32
	}
	buf := make([]byte, numBytes)
	if _, err = io.ReadFull(rand.Reader, buf); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken returns the lowercase SHA-256 hex digest of a raw secret token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
