package sms

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"MedFollow/internal/middleware"
)

// 患者端点击 Token（PRD 第十章）：复用登录 JWT secret，claim 携带 task_id 与
// typ=sms_click；48h 内可重复点击（幂等重开，状态机只推进一次），推进与否由
// sms_logs.clicked 标志判定（见 store.go）
const (
	TokenTTL    = 48 * time.Hour
	TypSmsClick = "sms_click"
)

// ClickClaims 短信链接 Token 载荷：patient_id + task_id + typ（防止与登录 Token 互用）
type ClickClaims struct {
	PatientID string `json:"patient_id"`
	TaskID    string `json:"task_id"`
	Typ       string `json:"typ"`
	jwt.RegisteredClaims
}

// SignClickToken 签发 48h 一次性点击 Token；jti 随机化保证同任务多次签发互不相同
func SignClickToken(taskID, patientID string) (string, error) {
	now := time.Now()
	jti := make([]byte, 8)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	claims := &ClickClaims{
		PatientID: patientID,
		TaskID:    taskID,
		Typ:       TypSmsClick,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        hex.EncodeToString(jti),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
}

// ParseClickToken 校验签名、exp 与 typ=sms_click，返回载荷；任一不满足即拒绝
func ParseClickToken(raw string) (*ClickClaims, error) {
	claims := &ClickClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return middleware.JWTSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	if claims.Typ != TypSmsClick || claims.TaskID == "" {
		return nil, errors.New("invalid token type")
	}
	return claims, nil
}
