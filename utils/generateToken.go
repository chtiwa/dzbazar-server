package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func GenerateToken(ID uuid.UUID, multiplier uint, role string) *jwt.Token {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  ID,
		"role": role,
		"exp":  time.Now().Add(time.Second * time.Duration(multiplier)).Unix(),
	})

	return token
}
