// SPDX-License-Identifier: MPL-2.0

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"

	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	aes256KeyLength            = 32
	defaultContentType         = "application/octet-stream"
	minimumGCMCiphertextLength = 12 + 16
)

type StateDocument struct {
	Ciphertext    []byte
	ContentLength int64
	ContentType   string
	SHA256        string
}

func GenerateWrappedGroupKey(masterKey []byte) (string, error) {
	if len(masterKey) != aes256KeyLength {
		return "", errors.New("invalid master key")
	}

	groupKey := make([]byte, aes256KeyLength)
	if _, err := rand.Read(groupKey); err != nil {
		return "", err
	}

	return security.Encrypt(groupKey, string(masterKey))
}

func UnwrapGroupKey(masterKey []byte, wrapped string) ([]byte, error) {
	if len(masterKey) != aes256KeyLength || !validCiphertext(wrapped) {
		return nil, errors.New("invalid wrapped group key")
	}

	key, err := security.Decrypt(wrapped, string(masterKey))
	if err != nil || len(key) != aes256KeyLength {
		return nil, errors.New("invalid wrapped group key")
	}

	return key, nil
}

func EncryptState(plaintext, groupKey []byte, contentType string) (StateDocument, error) {
	if len(groupKey) != aes256KeyLength {
		return StateDocument{}, errors.New("invalid group key")
	}
	if contentType == "" {
		contentType = defaultContentType
	}

	ciphertext, err := security.Encrypt(plaintext, string(groupKey))
	if err != nil {
		return StateDocument{}, err
	}

	digest := sha256.Sum256(plaintext)
	return StateDocument{
		Ciphertext:    []byte(ciphertext),
		ContentLength: int64(len(plaintext)),
		ContentType:   contentType,
		SHA256:        hex.EncodeToString(digest[:]),
	}, nil
}

func DecryptState(document StateDocument, groupKey []byte) ([]byte, error) {
	if len(groupKey) != aes256KeyLength || !validCiphertext(string(document.Ciphertext)) {
		return nil, errors.New("invalid encrypted state")
	}

	plaintext, err := security.Decrypt(string(document.Ciphertext), string(groupKey))
	if err != nil {
		return nil, errors.New("invalid encrypted state")
	}

	digest := sha256.Sum256(plaintext)
	expectedDigest, err := hex.DecodeString(document.SHA256)
	if err != nil || len(expectedDigest) != len(digest) || subtle.ConstantTimeCompare(digest[:], expectedDigest) != 1 {
		return nil, errors.New("state integrity check failed")
	}
	if int64(len(plaintext)) != document.ContentLength {
		return nil, errors.New("state length check failed")
	}

	return plaintext, nil
}

func validCiphertext(ciphertext string) bool {
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	return err == nil && len(decoded) >= minimumGCMCiphertextLength
}
