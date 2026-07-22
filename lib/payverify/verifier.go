package payverify

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/lejianwen/rustdesk-api/v2/config"
)

// PaymentVerifier 支付回调验签接口（通用可配置，不写死平台）
type PaymentVerifier interface {
	// Verify 校验回调参数签名是否合法
	// params: 平台回调的完整参数 map（含 sign 字段）
	Verify(params map[string]string) (bool, error)
}

// GenericProvider 基于配置的通用验签实现
type GenericProvider struct {
	cfg config.PaymentConfig
}

// NewGenericProvider 创建通用验签提供者
func NewGenericProvider(cfg config.PaymentConfig) *GenericProvider {
	return &GenericProvider{cfg: cfg}
}

// Verify 验签：从 params 中取出签名 → 用其余参数计算签名 → 比对
func (p *GenericProvider) Verify(params map[string]string) (bool, error) {
	if params == nil {
		return false, fmt.Errorf("verify params is nil")
	}

	// 1. 提取签名
	clientSign, ok := params[p.cfg.SignField]
	if !ok || clientSign == "" {
		return false, fmt.Errorf("sign field %q not found in params", p.cfg.SignField)
	}

	// 2. 计算服务端签名
	expectedSign := p.BuildSign(params)

	// 3. 恒定时间比对（防时序攻击）
	if !constantTimeEqual(clientSign, expectedSign) {
		return false, fmt.Errorf("sign mismatch: got %s, expected %s", clientSign, expectedSign)
	}
	return true, nil
}

// BuildSign 构造签名：
// 1. 排除 sign 字段本身
// 2. 按 key 字典序排序
// 3. 拼接成 k1=v1&k2=v2...&key=secret 格式
// 4. 按指定算法计算摘要
func (p *GenericProvider) BuildSign(params map[string]string) string {
	// 收集待签名参数（排除 sign 字段）
	var keys []string
	for k := range params {
		if k == p.cfg.SignField {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 拼接 key=value 对
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(params[k])
	}
	// 追加密钥
	sb.WriteString("&key=")
	sb.WriteString(p.cfg.Secret)

	payload := sb.String()

	switch strings.ToLower(p.cfg.SignAlgo) {
	case "sha256":
		return sha256Hex(payload)
	case "hmac-sha256":
		return hmacSha256Hex(payload, p.cfg.Secret)
	default: // md5 及默认
		return md5Hex(payload)
	}
}

func md5Hex(data string) string {
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

func sha256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func hmacSha256Hex(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// constantTimeEqual 恒定时间字符串比较，防止时序攻击
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	h := sha256.New()
	h.Write([]byte(a))
	digestA := h.Sum(nil)

	h.Reset()
	h.Write([]byte(b))
	digestB := h.Sum(nil)

	return hmac.Equal(digestA, digestB)
}
