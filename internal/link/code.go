package link

import (
	"crypto/rand"
	"math/big"
)

const (
	codeAlphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	codeLength   = 7
)

// generateCode returns a random base62 short code of fixed length.
func generateCode() (string, error) {
	buf := make([]byte, codeLength)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		buf[i] = codeAlphabet[n.Int64()]
	}
	return string(buf), nil
}
