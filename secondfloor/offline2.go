package secondfloor

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

const (
	offline2PrefixSize = 10
	offline2SaltSize   = 16
	offline2MACSize    = sha1.Size
	offline2HeaderSize = 128
	offline2EntrySize  = 36
	offline2Iterations = 256
)

type ContentKey [16]byte

func ReadOfflineKeys(offline2Path string, deviceID string) (map[FileID]ContentKey, error) {
	data, err := os.ReadFile(offline2Path)
	if err != nil {
		return nil, err
	}
	if len(data) < offline2PrefixSize+offline2SaltSize+offline2HeaderSize+offline2MACSize {
		return nil, fmt.Errorf("offline2 is too short (%d bytes)", len(data))
	}
	salt := data[offline2PrefixSize : offline2PrefixSize+offline2SaltSize]
	if !bytes.Equal(data[:offline2PrefixSize], salt[:offline2PrefixSize]) {
		return nil, errors.New("offline2 prefix does not match salt")
	}
	ciphertext := data[offline2PrefixSize+offline2SaltSize : len(data)-offline2MACSize]
	tag := data[len(data)-offline2MACSize:]
	if (len(ciphertext)-offline2HeaderSize)%offline2EntrySize != 0 {
		return nil, fmt.Errorf("offline2 has invalid payload size %d", len(ciphertext))
	}

	masterKey := sha1.Sum([]byte(deviceID))
	raw := pbkdf2.Key(masterKey[:], salt, offline2Iterations, 48, sha1.New)

	mac := hmac.New(sha1.New, raw[:20])
	mac.Write(ciphertext)
	if !hmac.Equal(mac.Sum(nil), tag) {
		return nil, errors.New("offline2 has bad hmac")
	}

	block, err := aes.NewCipher(raw[20:36])
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	copy(iv, raw[40:48])
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCTR(block, iv).XORKeyStream(plaintext, ciphertext)

	keys := make(map[FileID]ContentKey)
	for off := offline2HeaderSize; off < len(plaintext); off += offline2EntrySize {
		entry := plaintext[off : off+offline2EntrySize]
		keys[FileID(entry[:20])] = ContentKey(entry[20:])
	}
	return keys, nil
}
