package crypto

import (
	"bytes"
	"testing"
)

func TestPayloadEncryptionRoundtrip(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK failed: %v", err)
	}

	plaintext := []byte("john.doe@example.com")
	aad := []byte("tenantId=A|requestId=123|token=[[AUG:EMAIL:1]]|entityType=EMAIL_ADDRESS")

	nonce, ciphertext, err := EncryptValue(dek, plaintext, aad)
	if err != nil {
		t.Fatalf("EncryptValue failed: %v", err)
	}

	decrypted, err := DecryptValue(dek, nonce, ciphertext, aad)
	if err != nil {
		t.Fatalf("DecryptValue failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Roundtrip mismatch: expected %s, got %s", string(plaintext), string(decrypted))
	}

	// Test AAD mismatch
	badAAD := []byte("tenantId=B|requestId=123|token=[[AUG:EMAIL:1]]|entityType=EMAIL_ADDRESS")
	_, err = DecryptValue(dek, nonce, ciphertext, badAAD)
	if err == nil {
		t.Fatalf("Expected decryption to fail with wrong AAD")
	}
}

func TestDEKDevWrapping(t *testing.T) {
	masterKey, _ := GenerateDEK() // 32 bytes valid
	
	originalDEK, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK failed: %v", err)
	}

	wrapped, err := WrapDEK_DEV(masterKey, originalDEK)
	if err != nil {
		t.Fatalf("WrapDEK_DEV failed: %v", err)
	}

	unwrapped, err := UnwrapDEK_DEV(masterKey, wrapped)
	if err != nil {
		t.Fatalf("UnwrapDEK_DEV failed: %v", err)
	}

	if !bytes.Equal(originalDEK, unwrapped) {
		t.Errorf("Unwrap mismatch: expected %x, got %x", originalDEK, unwrapped)
	}

	// Test invalid master key
	badMasterKey := make([]byte, 32)
	_, err = UnwrapDEK_DEV(badMasterKey, wrapped)
	if err == nil {
		t.Fatalf("Expected unwrap to fail with wrong master key")
	}
}
