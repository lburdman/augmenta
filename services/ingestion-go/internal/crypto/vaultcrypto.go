package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKeySize = errors.New("invalid key size: must be 32 bytes for AES-256")
	ErrDecryptFailed  = errors.New("decryption failed (wrong key or corrupted data)")
)

// GenerateDEK creates a secure 32-byte Data Encryption Key for AES-256.
func GenerateDEK() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// EncryptValue encrypts the plaintext using AES-GCM with the provided DEK and AAD.
// Returns the randomly generated nonce and the ciphertext.
func EncryptValue(dek, plaintext, aad []byte) (nonce []byte, ciphertext []byte, err error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, nil, fmt.Errorf("aes new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("cipher new gcm: %w", err)
	}

	nonce = make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and auth tag to the first argument (dst).
	// We pass nil so it allocates a new slice.
	ciphertext = aead.Seal(nil, nonce, plaintext, aad)

	return nonce, ciphertext, nil
}

// DecryptValue decrypts the ciphertext using AES-GCM with the provided DEK, nonce, and AAD.
func DecryptValue(dek, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher new gcm: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
}

// WrapDEK_DEV encrypts a generated DEK using a statically provided Master Key (for local dev).
// We include the nonce inside the returned wrapped payload so it's a single blob.
func WrapDEK_DEV(masterKey, dek []byte) ([]byte, error) {
	if len(masterKey) != 32 {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher new gcm: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, dek, nil)
	
	// Prepend nonce to the wrapped blob
	wrapped := make([]byte, 0, len(nonce)+len(ciphertext))
	wrapped = append(wrapped, nonce...)
	wrapped = append(wrapped, ciphertext...)

	return wrapped, nil
}

// UnwrapDEK_DEV decrypts a DEK that was wrapped using the DEV Master Key.
func UnwrapDEK_DEV(masterKey, wrapped []byte) ([]byte, error) {
	if len(masterKey) != 32 {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher new gcm: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(wrapped) < nonceSize {
		return nil, errors.New("wrapped key too short")
	}

	nonce, ciphertext := wrapped[:nonceSize], wrapped[nonceSize:]

	dek, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return dek, nil
}
