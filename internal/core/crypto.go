package core

import (
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
