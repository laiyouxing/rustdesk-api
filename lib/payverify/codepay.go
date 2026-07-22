// Package payverify 码支付签名工具（易支付标准）
//
// 签名算法：
//  1. 移除 sign 和 sign_type 字段，移除空值字段
//  2. 按 key 字典序升序排列
//  3. 拼接 k1=v1&k2=v2…（去掉末尾 &）
//  4. 末尾追加 secret_key
//  5. 取 MD5 小写 hex
package payverify

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"
)

// Sign 生成签名
func Sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	first := true
	for _, k := range keys {
		v := params[k]
		if v == "" {
			continue
		}
		if !first {
			sb.WriteString("&")
		}
		first = false
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}
	sb.WriteString(secret)

	h := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(h[:])
}

// Verify 校验签名
func Verify(params map[string]string, secret string) bool {
	clientSign, ok := params["sign"]
	if !ok || clientSign == "" {
		return false
	}
	expected := Sign(params, secret)
	return clientSign == expected
}
