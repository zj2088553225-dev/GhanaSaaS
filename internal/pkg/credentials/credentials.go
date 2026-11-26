package credentials

import (
	"crypto/rand"
	"encoding/hex"
)

func GenerateAppKey() (string, error) {
	return randomHex(16)
}

func GenerateAppSecret() (string, error) {
	return randomHex(32)
}

func randomHex(byteLength int) (string, error) {
	value := make([]byte, byteLength)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
