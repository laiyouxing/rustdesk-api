package jwt

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type Jwt struct {
	Key                 []byte
	TokenExpireDuration time.Duration
}

type UserClaims struct {
	UserId uint `json:"user_id"`
	jwt.RegisteredClaims
}

func NewJwt(key string, tokenExpireDuration time.Duration) *Jwt {
	return &Jwt{
		Key:                 []byte(key),
		TokenExpireDuration: tokenExpireDuration,
	}
}

func (s *Jwt) GenerateToken(userId uint) string {
	if len(s.Key) == 0 {
		fmt.Println("jwt key is nil")
		return ""
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		UserClaims{
			UserId: userId,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.TokenExpireDuration)),
			},
		})
	token, err := t.SignedString(s.Key)
	if err != nil {
		fmt.Printf("jwt token generate error: %v", err)
		return ""
	}
	return token
}

func (s *Jwt) ParseToken(tokenString string) (uint, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// SECURITY: 显式校验签名算法，防止算法混淆攻击（algorithm confusion）。
		// 若不校验，攻击者可把 alg 改为 "none" 或用 RSA 公钥伪装成 HMAC 密钥来伪造令牌。
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.Key, nil
	})
	if err != nil {
		return 0, err
	}
	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return claims.UserId, nil
	}
	return 0, err
}

// MfaClaims 用于登录二次验证的临时令牌（短时效）
type MfaClaims struct {
	UserId uint `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateMfaToken 生成 MFA 临时令牌，有效期 5 分钟
func (s *Jwt) GenerateMfaToken(userId uint) string {
	if len(s.Key) == 0 {
		return ""
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, MfaClaims{
		UserId: userId,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	})
	token, err := t.SignedString(s.Key)
	if err != nil {
		return ""
	}
	return token
}

// ParseMfaToken 解析 MFA 临时令牌
func (s *Jwt) ParseMfaToken(tokenString string) (uint, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MfaClaims{}, func(token *jwt.Token) (interface{}, error) {
		// SECURITY: 同上，MFA 临时令牌同样需要校验签名算法，防止伪造。
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.Key, nil
	})
	if err != nil {
		return 0, err
	}
	if claims, ok := token.Claims.(*MfaClaims); ok && token.Valid {
		return claims.UserId, nil
	}
	return 0, err
}
