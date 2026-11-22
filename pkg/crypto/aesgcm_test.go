package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	c, err := NewCipherFromBase64(key)
	if err != nil {
		t.Fatalf("NewCipherFromBase64: %v", err)
	}

	plain := []byte("[Interface]\nPrivateKey = secret\n")
	ct, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ct, plain) {
		t.Fatal("ciphertext must not contain plaintext")
	}

	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q, want %q", got, plain)
	}

	// Tampered ciphertext must fail authentication
	ct[len(ct)-1] ^= 0xFF
	if _, err := c.Decrypt(ct); err == nil {
		t.Fatal("expected decryption failure on tampered ciphertext")
	}
}

func TestGenerateToken(t *testing.T) {
	raw1, hash1, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	raw2, hash2, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if raw1 == raw2 || hash1 == hash2 {
		t.Fatal("expected unique tokens")
	}
	if HashToken(raw1) != hash1 {
		t.Fatal("HashToken mismatch")
	}
}
