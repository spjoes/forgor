package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/nacl/box"
)

const (
	// Argon2id parameters
	Argon2Time    = 3
	Argon2Memory  = 64 * 1024 // This would be 64 MB
	Argon2Threads = 4
	Argon2KeyLen  = 32

	SaltSize  = 16
	NonceSize = 24
)

var (
	ErrDecryptionFailed = errors.New("decryption failed: invalid password or corrupted data")
	ErrInvalidCiphertext = errors.New("ciphertext too short")
)

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		Argon2Time,
		Argon2Memory,
		Argon2Threads,
		Argon2KeyLen,
	)
}

func Encrypt(key, plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func Decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < NonceSize {
		return nil, ErrInvalidCiphertext
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	nonce := ciphertext[:NonceSize]
	encrypted := ciphertext[NonceSize:]

	plaintext, err := aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func GenerateBoxKeyPair() (pub, priv *[32]byte, err error) {
	pub, priv, err = box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key pair: %w", err)
	}
	return pub, priv, nil
}

func BoxSeal(message []byte, recipientPub, senderPriv *[32]byte) ([]byte, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := box.Seal(nonce[:], message, &nonce, recipientPub, senderPriv)
	return ciphertext, nil
}

func BoxOpen(ciphertext []byte, senderPub, recipientPriv *[32]byte) ([]byte, error) {
	if len(ciphertext) < 24 {
		return nil, ErrInvalidCiphertext
	}

	var nonce [24]byte
	copy(nonce[:], ciphertext[:24])

	plaintext, ok := box.Open(nil, ciphertext[24:], &nonce, senderPub, recipientPriv)
	if !ok {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func GenerateSignKeyPair() (pub [32]byte, priv [64]byte, err error) {
	pubSlice, privSlice, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return pub, priv, fmt.Errorf("failed to generate signing key pair: %w", err)
	}
	copy(pub[:], pubSlice)
	copy(priv[:], privSlice)
	return pub, priv, nil
}

func Sign(privateKey [64]byte, message []byte) []byte {
	return ed25519.Sign(privateKey[:], message)
}

func Verify(publicKey [32]byte, message, signature []byte) bool {
	return ed25519.Verify(publicKey[:], message, signature)
}

func ComputeDeviceID(pubkeySign [32]byte) string {
	hash := sha256.Sum256(pubkeySign[:])
	return hex.EncodeToString(hash[:])
}

func DeviceIDBytes(pubkeySign [32]byte) []byte {
	hash := sha256.Sum256(pubkeySign[:])
	return hash[:]
}

func DeviceIDToBytes(hexID string) ([]byte, error) {
	return hex.DecodeString(hexID)
}
