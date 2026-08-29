// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateEncryptionRoundTripPreservesOriginalMetadata(t *testing.T) {
	masterKey := bytes.Repeat([]byte{1}, 32)
	wrapped, err := GenerateWrappedGroupKey(masterKey)
	require.NoError(t, err)

	groupKey, err := UnwrapGroupKey(masterKey, wrapped)
	require.NoError(t, err)

	plaintext := []byte(`{"version":4}`)
	document, err := EncryptState(plaintext, groupKey, "application/json")
	require.NoError(t, err)
	require.Equal(t, int64(len(plaintext)), document.ContentLength)
	require.Equal(t, "application/json", document.ContentType)

	decrypted, err := DecryptState(document, groupKey)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestDecryptStateRejectsTamperedCiphertextOrDigest(t *testing.T) {
	groupKey := bytes.Repeat([]byte{2}, 32)
	document, err := EncryptState([]byte(`{"version":4}`), groupKey, "application/json")
	require.NoError(t, err)

	tamperedCiphertext := document
	tamperedCiphertext.Ciphertext = append([]byte(nil), document.Ciphertext...)
	tamperedCiphertext.Ciphertext[0] ^= 0xff
	_, err = DecryptState(tamperedCiphertext, groupKey)
	require.Error(t, err)

	tamperedDigest := document
	tamperedDigest.SHA256 = "not-a-digest"
	_, err = DecryptState(tamperedDigest, groupKey)
	require.Error(t, err)
}
