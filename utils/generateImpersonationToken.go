package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// GenerateImpersonationToken mints a short-lived access token that grants a
// super admin Owner-equivalent access to exactly one shop. RequireAuthentication
// re-validates user.PlatformRole == "super_admin" on every request, so demoting
// a super admin immediately invalidates any outstanding impersonation token.
func GenerateImpersonationToken(userID uuid.UUID, shopID uuid.UUID, ttlSeconds uint) *jwt.Token {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":           userID,
		"role":          "owner",
		"impersonating": true,
		"shopId":        shopID.String(),
		"exp":           time.Now().Add(time.Second * time.Duration(ttlSeconds)).Unix(),
	})
}
