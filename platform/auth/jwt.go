package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gogu-x/gogs/config"
)

type claims struct {
	UID uint64 `json:"uid"`
	jwt.RegisteredClaims
}

func Sign(uid uint64) (string, error) {
	c := claims{
		UID: uid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(config.JWTSecret))
}

func Verify(token string) (uint64, error) {
	t, err := jwt.ParseWithClaims(token, &claims{}, func(*jwt.Token) (interface{}, error) {
		return []byte(config.JWTSecret), nil
	})
	if err != nil {
		return 0, err
	}
	c, ok := t.Claims.(*claims)
	if !ok || !t.Valid {
		return 0, errors.New("invalid token")
	}
	return c.UID, nil
}
