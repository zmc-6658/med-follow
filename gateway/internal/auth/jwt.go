package auth

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/golang-jwt/jwt/v5"

	"MedFollow/internal/middleware"
)

func newJTI() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func jwtWithClaims(claims *middleware.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
}
