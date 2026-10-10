package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"MedFollow/internal/config"
	"MedFollow/internal/model"
)

const userContextKey = "user_context"

// 错误码：40100 缺失/格式错误，40101 无效或过期，40300 权限不足
const (
	errMissingToken = 40100
	errInvalidToken = 40101
	errForbidden    = 40300
)

func JWTSecret() []byte { return []byte(config.Get().JWT.Secret) }

func GetUserContext(c *gin.Context) *model.UserContext {
	uc, ok := c.Get(userContextKey)
	if !ok {
		return nil
	}
	user, _ := uc.(*model.UserContext)
	return user
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			model.AbortErr(c, 401, errMissingToken, "missing token")
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			model.AbortErr(c, 401, errMissingToken, "malformed authorization header, expected: Bearer <token>")
			return
		}

		claims := &Claims{}
		_, err := jwt.ParseWithClaims(parts[1], claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return JWTSecret(), nil
		})

		if err != nil || claims.Subject == "" {
			model.AbortErr(c, 401, errInvalidToken, "invalid or expired token")
			return
		}

		c.Set(userContextKey, claims.ToUserContext())
		c.Next()
	}
}

func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uc := GetUserContext(c)
		if uc == nil {
			model.AbortErr(c, 401, errMissingToken, "missing token")
			return
		}
		if !HasPermission(uc.Role, perm) {
			model.AbortErr(c, 403, errForbidden, "forbidden")
			return
		}
		c.Next()
	}
}
