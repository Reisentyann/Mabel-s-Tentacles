// 文件：mcp-server-go/internal/service/auth.go —— 认证服务：bcrypt 哈希 / JWT 签发与解析
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTokenInvalid       = errors.New("invalid token")
)

// HashPassword 生成 bcrypt 哈希。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验明文密码与 bcrypt 哈希。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// Claims JWT 载荷。前端只读 sub 和 exp，refresh 流程依赖 jti。
type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func newJTI() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func issueToken(secretKey string, userID int64, username string, lifetime time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
			ID:        newJTI(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secretKey))
}

// GenerateTokens 签发 access + refresh。
func GenerateTokens(cfg *config.Config, userID int64, username string) (*TokenPair, error) {
	access, err := issueToken(cfg.Security.SecretKey, userID, username,
		time.Duration(cfg.Security.AccessTokenExpireMin)*time.Minute)
	if err != nil {
		return nil, err
	}
	refresh, err := issueToken(cfg.Security.SecretKey, userID, username,
		time.Duration(cfg.Security.RefreshTokenExpireDays)*24*time.Hour)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh, TokenType: "bearer"}, nil
}

// ParseToken 校验签名与有效期，返回 claims。
func ParseToken(secretKey, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return []byte(secretKey), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
