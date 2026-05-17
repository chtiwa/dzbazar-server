package utils

import (
	"fmt"
	"math/rand"
)

func GenerateOTP() string {
	return fmt.Sprintf("%06d", rand.Intn(100000))
}
