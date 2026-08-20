package calendar

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

/*
tokenCipher seals the OAuth tokens that are stored in the database.

A refresh token is the whole feature's key: it grants read and write access to four Google
calendars and it does not expire. The database is dumped by backups and read by whoever can
reach Postgres, so storing that in plaintext would make every copy of a backup a copy of the
grant. AES-256-GCM with a key that lives only in the environment keeps the ciphertext useless
on its own, and the nonce is prepended so rotating the key is the only thing that ever needs
migrating.
*/
type tokenCipher struct {
	aead cipher.AEAD
}

// newTokenCipher takes the 32-byte key as hex (openssl rand -hex 32).
func newTokenCipher(hexKey string) (*tokenCipher, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("token key is not valid hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("token key must be 32 bytes (64 hex chars), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &tokenCipher{aead: aead}, nil
}

func (c *tokenCipher) seal(plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (c *tokenCipher) open(sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	if len(sealed) < c.aead.NonceSize() {
		return "", errors.New("sealed token is truncated")
	}
	nonce, ct := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		// Almost always a changed GOOGLE_TOKEN_KEY. Say so, because the alternative is
		// hunting a "sync stopped working" with no clue.
		return "", fmt.Errorf("cannot decrypt stored token (has GOOGLE_TOKEN_KEY changed?): %w", err)
	}
	return string(plain), nil
}
