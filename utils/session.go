package utils

import (
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/golang-jwt/jwt/v5"
)

// RevokeToken denylists a single token by its jti until the token's own
// expiry — after that it would be rejected on exp anyway, so the key can
// safely expire too. Best-effort: a Redis outage means the token stays
// valid for its remaining lifetime, which is the pre-existing behavior.
func RevokeToken(claims jwt.MapClaims) {
	jti, _ := claims["jti"].(string)
	exp, _ := claims["exp"].(float64)
	if jti == "" || exp == 0 {
		return
	}
	ttl := time.Until(time.Unix(int64(exp), 0))
	if ttl <= 0 {
		return
	}
	initializers.RClient.Set(initializers.Ctx, "session:revoked:"+jti, "1", ttl)
}

// IsTokenRevoked checks the denylist set by RevokeToken.
func IsTokenRevoked(claims jwt.MapClaims) bool {
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return false
	}
	n, err := initializers.RClient.Exists(initializers.Ctx, "session:revoked:"+jti).Result()
	return err == nil && n > 0
}

// RevokeAllSessions invalidates every token already issued to a user (e.g.
// on password reset) by recording a cutoff timestamp — any token with an
// iat before this is rejected regardless of jti. TTL matches the longest
// possible refresh-token life, past which iat comparisons are moot anyway.
func RevokeAllSessions(userID string) {
	initializers.RClient.Set(initializers.Ctx, "session:valid_after:"+userID, time.Now().Unix(), 7*24*time.Hour)
}

// IsTokenBeforeRevokeAll checks a token's iat against RevokeAllSessions' cutoff.
func IsTokenBeforeRevokeAll(userID string, claims jwt.MapClaims) bool {
	iat, ok := claims["iat"].(float64)
	if !ok {
		return false
	}
	val, err := initializers.RClient.Get(initializers.Ctx, "session:valid_after:"+userID).Int64()
	if err != nil {
		return false
	}
	return int64(iat) < val
}
