package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

type Box struct{ aead cipher.AEAD }

func New(master string) (*Box, error) {
	k := sha256.Sum256([]byte(master))
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: a}, nil
}
func (b *Box) Seal(v string) (string, error) {
	n := make([]byte, b.aead.NonceSize())
	if _, e := io.ReadFull(rand.Reader, n); e != nil {
		return "", e
	}
	out := b.aead.Seal(n, n, []byte(v), nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}
func (b *Box) Open(v string) (string, error) {
	raw, e := base64.RawURLEncoding.DecodeString(v)
	if e != nil {
		return "", e
	}
	n := b.aead.NonceSize()
	if len(raw) < n {
		return "", fmt.Errorf("invalid secret")
	}
	p, e := b.aead.Open(nil, raw[:n], raw[n:], nil)
	return string(p), e
}
func Random(prefix string, bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}
func Hash(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
