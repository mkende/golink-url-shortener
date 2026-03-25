// Package links provides link business logic including name validation and
// random name generation.
package links

import (
	"crypto/rand"
	"math/big"
)

const quickLinkChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// GenerateQuickName generates a random link name of the given length using
// lowercase letters and digits. If length is less than 1, it defaults to 6.
func GenerateQuickName(length int) (string, error) {
	if length < 1 {
		length = 6
	}
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(quickLinkChars))))
		if err != nil {
			return "", err
		}
		result[i] = quickLinkChars[n.Int64()]
	}
	return string(result), nil
}
