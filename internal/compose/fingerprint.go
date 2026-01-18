package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Fingerprint computes a SHA-256 hash for the given compose bytes.
func Fingerprint(body []byte) (string, error) {
	if len(body) == 0 {
		return "", errors.New("compose body is empty")
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
