package core

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

func XorDecipher(hexStr string, key int) string {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return ""
	}

	var builder strings.Builder
	for _, b := range data {
		builder.WriteRune(rune(int(b) ^ key))
	}

	return builder.String()
}

// SafeCompare
func SafeCompare(input, target string) bool {
	inputByte := []byte(input)
	targetByte := []byte(target)
	inputHash := sha256.Sum256(inputByte)
	targetHash := sha256.Sum256(targetByte)
	if subtle.ConstantTimeCompare(inputHash[:], targetHash[:]) == 1 {
		return true
	}
	return false
}
