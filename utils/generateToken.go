package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// GenerateToken signs a token carrying a unique jti (so a single token can be
// revoked by id on logout) and iat (so RequireAuthentication can reject any
// token issued before a "revoke all sessions" event, e.g. a password reset).
func GenerateToken(ID uuid.UUID, multiplier uint, role string) *jwt.Token {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  ID,
		"role": role,
		"iat":  time.Now().Unix(),
		"jti":  uuid.NewString(),
		"exp":  time.Now().Add(time.Second * time.Duration(multiplier)).Unix(),
	})

	return token
}
