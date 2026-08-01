package vault

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	kdfAlgorithm      = "argon2id"
	kdfMemoryKiB      = 64 * 1024
	kdfIterations     = 3
	kdfParallelism    = 4
	kdfSaltLen        = 16
	keyLen            = 32
	maxKDFMemoryKiB   = 128 * 1024
	maxKDFIterations  = 8
	maxKDFParallelism = 8
)

// Envelope is the encrypted blob stored remotely.
type Envelope struct {
	Version     int    `json:"version"`
	KDF         string `json:"kdf"`
	Memory      uint32 `json:"memory"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        string `json:"salt"`       // base64
	Nonce       string `json:"nonce"`      // base64
	Ciphertext  string `json:"ciphertext"` // base64
}

func Encrypt(doc Document, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errors.New("vault passphrase is required")
	}
	plain, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, kdfSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(passphrase), salt, kdfIterations, kdfMemoryKiB, kdfParallelism, keyLen)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, plain, nil)
	env := Envelope{
		Version:     DocumentVersion,
		KDF:         kdfAlgorithm,
		Memory:      kdfMemoryKiB,
		Iterations:  kdfIterations,
		Parallelism: kdfParallelism,
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Nonce:       base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:  base64.StdEncoding.EncodeToString(ciphertext),
	}
	return json.Marshal(env)
}

func Decrypt(blob []byte, passphrase string) (Document, error) {
	var env Envelope
	if err := json.Unmarshal(blob, &env); err != nil {
		return Document{}, fmt.Errorf("invalid vault envelope: %w", err)
	}
	if env.KDF != kdfAlgorithm {
		return Document{}, fmt.Errorf("unsupported vault KDF %q", env.KDF)
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return Document{}, errors.New("invalid vault salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return Document{}, errors.New("invalid vault nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return Document{}, errors.New("invalid vault ciphertext")
	}
	memory := env.Memory
	if memory == 0 {
		memory = kdfMemoryKiB
	}
	if memory > maxKDFMemoryKiB {
		return Document{}, errors.New("vault envelope memory parameter exceeds limit")
	}
	iterations := env.Iterations
	if iterations == 0 {
		iterations = kdfIterations
	}
	if iterations > maxKDFIterations {
		return Document{}, errors.New("vault envelope iterations parameter exceeds limit")
	}
	parallelism := env.Parallelism
	if parallelism == 0 {
		parallelism = kdfParallelism
	}
	if parallelism > maxKDFParallelism {
		return Document{}, errors.New("vault envelope parallelism parameter exceeds limit")
	}
	key := argon2.IDKey([]byte(passphrase), salt, iterations, memory, parallelism, keyLen)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Document{}, err
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return Document{}, errors.New("incorrect vault passphrase or corrupted vault data")
	}
	var doc Document
	if err := json.Unmarshal(plain, &doc); err != nil {
		return Document{}, fmt.Errorf("invalid vault document: %w", err)
	}
	return doc, nil
}
