package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// hmacSign 用 HMAC-SHA256 对 signingString 签名，返回 RawURLEncoding 结果（模拟攻击者以“公钥”为密钥的混淆签名）。
func hmacSign(signingString string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingString))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// buildUnsignedNoneToken 手工构造一个 alg:none 的 JWT（header.payload. 形式，签名留空）。
// 这是最朴素的伪造令牌：攻击者不提供任何签名即可伪装任意 user_id。
func buildUnsignedNoneToken(t *testing.T, userID uint) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(
		fmt.Sprintf(`{"user_id":%d,"exp":%d}`, userID, time.Now().Add(time.Hour).Unix()),
	))
	return header + "." + payload + "."
}

// TestParseToken_HS256_HappyPath 确认正常 HS256 令牌可被正确解析（修复不应破坏正常功能）。
func TestParseToken_HS256_HappyPath(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)
	token := svc.GenerateToken(12345)
	if token == "" {
		t.Fatal("HS256 令牌生成失败")
	}
	uid, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("正常 HS256 令牌解析应成功，实际错误: %v", err)
	}
	if uid != 12345 {
		t.Fatalf("期望解析出 user_id=12345，实际 %d", uid)
	}
}

// TestParseToken_NoneAlgorithm_Manual 手工构造的 alg:none 令牌必须被拒绝（不得解析出有效 user_id）。
func TestParseToken_NoneAlgorithm_Manual(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)
	noneTok := buildUnsignedNoneToken(t, 999)
	uid, err := svc.ParseToken(noneTok)
	if err == nil {
		t.Fatalf("alg:none 令牌不应被接受，但解析成功 (uid=%d)", uid)
	}
	if uid != 0 {
		t.Fatalf("被拒的 alg:none 令牌应返回 uid 0，实际 %d", uid)
	}
}

// TestParseToken_NoneAlgorithm_Library 用 jwt 库构造带 SigningMethodNone 的令牌，同样必须被拒绝。
func TestParseToken_NoneAlgorithm_Library(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)
	claims := UserClaims{
		UserId: 7,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("构造 alg:none 令牌失败: %v", err)
	}
	uid, err := svc.ParseToken(signed)
	if err == nil {
		t.Fatalf("alg:none 令牌不应被接受，但解析成功 (uid=%d)", uid)
	}
	if uid != 0 {
		t.Fatalf("被拒的 alg:none 令牌应返回 uid 0，实际 %d", uid)
	}
}

// TestParseToken_RS256Rejected 用 RSA 私钥签名的 RS256 令牌必须被拒绝（算法混淆防护）。
func TestParseToken_RS256Rejected(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)

	rsaKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pk))
	if err != nil {
		t.Fatalf("解析测试用 RSA 私钥失败: %v", err)
	}
	claims := UserClaims{
		UserId: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	rs256Token, err := tok.SignedString(rsaKey)
	if err != nil {
		t.Fatalf("构造 RS256 令牌失败: %v", err)
	}

	uid, err := svc.ParseToken(rs256Token)
	if err == nil {
		t.Fatalf("RS256 令牌不应被接受（算法不匹配），但解析成功 (uid=%d)", uid)
	}
	if uid != 0 {
		t.Fatalf("被拒的 RS256 令牌应返回 uid 0，实际 %d", uid)
	}
}

// TestParseToken_RS256ConfusionWithHmacSecret 模拟经典算法混淆攻击：
// 攻击者把 alg 设为 RS256，并用 HMAC 以“公钥”作密钥签名。即使签名可被 HMAC 校验通过，
// 也必须因算法不匹配被拒（验证 keyfunc 在返回密钥前先校验方法）。
func TestParseToken_RS256ConfusionWithHmacSecret(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)

	claims := UserClaims{
		UserId: 42,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	// 伪造成 RS256 头，但用 HMAC 以服务端“公钥/密钥”字符串签名：
	// 手工拼装 “RS256 头 + HMAC 签名” 的混淆令牌（绕过 RS256 需要 RSA 私钥的限制）。
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(
		fmt.Sprintf(`{"user_id":%d,"exp":%d}`, claims.UserId, claims.ExpiresAt.Unix()),
	))
	signingString := header + "." + payload
	// 用 HMAC-SHA256（以 test-secret 为密钥）制造一个“能通过 HMAC 校验”的签名串
	mac := hmacSign(signingString, []byte("test-secret"))
	confusedToken := signingString + "." + mac

	uid, err := svc.ParseToken(confusedToken)
	if err == nil {
		t.Fatalf("算法混淆攻击（RS256 头 + HMAC 签名）不应被接受，但解析成功 (uid=%d)", uid)
	}
	if uid != 0 {
		t.Fatalf("被拒的混淆令牌应返回 uid 0，实际 %d", uid)
	}
}

// TestParseMfaToken_NoneAlgorithm MFA 临时令牌同样必须拒绝 alg:none。
func TestParseMfaToken_NoneAlgorithm(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)
	noneTok := buildUnsignedNoneToken(t, 555)
	uid, err := svc.ParseMfaToken(noneTok)
	if err == nil {
		t.Fatalf("MFA alg:none 令牌不应被接受，但解析成功 (uid=%d)", uid)
	}
	if uid != 0 {
		t.Fatalf("被拒的 MFA alg:none 令牌应返回 uid 0，实际 %d", uid)
	}
}

// TestParseMfaToken_RS256Rejected MFA 临时令牌同样必须拒绝 RS256。
func TestParseMfaToken_RS256Rejected(t *testing.T) {
	svc := NewJwt("test-secret", time.Hour)

	rsaKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pk))
	if err != nil {
		t.Fatalf("解析测试用 RSA 私钥失败: %v", err)
	}
	claims := MfaClaims{
		UserId: 88,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	rs256Token, err := tok.SignedString(rsaKey)
	if err != nil {
		t.Fatalf("构造 MFA RS256 令牌失败: %v", err)
	}
	uid, err := svc.ParseMfaToken(rs256Token)
	if err == nil {
		t.Fatalf("MFA RS256 令牌不应被接受，但解析成功 (uid=%d)", uid)
	}
	if uid != 0 {
		t.Fatalf("被拒的 MFA RS256 令牌应返回 uid 0，实际 %d", uid)
	}
}

// TestParseMfaToken_HS256_HappyPath 确认正常 MFA HS256 令牌可被正确解析。
func TestParseMfaToken_HS256_HappyPath(t *testing.T) {
	svc := NewJwt("test-secret", 5*time.Minute)
	token := svc.GenerateMfaToken(777)
	if token == "" {
		t.Fatal("MFA HS256 令牌生成失败")
	}
	uid, err := svc.ParseMfaToken(token)
	if err != nil {
		t.Fatalf("正常 MFA HS256 令牌解析应成功，实际错误: %v", err)
	}
	if uid != 777 {
		t.Fatalf("期望解析出 user_id=777，实际 %d", uid)
	}
}
