package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateOTP returns a uniform 6-digit code in 000000-999999 using
// crypto/rand — math/rand is predictable and was previously also capped at
// Intn(100000), so the leading digit was always 0 (see important.todo
// LAUNCH BLOCKER 4).
func GenerateOTP() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		// crypto/rand failing means the OS entropy source is broken — not a
		// case to silently fall back from for a security-sensitive code.
		panic("utils.GenerateOTP: crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%06d", n.Int64())
}
