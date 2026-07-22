package payverify

import (
	"testing"
)

func TestSign(t *testing.T) {
	params := map[string]string{
		"pid":          "1000",
		"out_trade_no": "SUB20260722001",
		"money":        "10.00",
		"name":         "pro订阅",
		"trade_status": "TRADE_SUCCESS",
	}
	secret := "test_secret_key_123"

	sign := Sign(params, secret)
	if sign == "" {
		t.Fatal("sign should not be empty")
	}

	// Verify with sign added to params
	params["sign"] = sign
	if !Verify(params, secret) {
		t.Fatal("verify should pass with correct sign and secret")
	}

	// Verify with wrong secret
	params["sign"] = sign
	if Verify(params, "wrong_secret") {
		t.Fatal("verify should fail with wrong secret")
	}
}

func TestSign_ExcludesSignField(t *testing.T) {
	params := map[string]string{
		"pid":   "1000",
		"money": "10.00",
		"sign":  "should_not_affect",
	}
	first := Sign(params, "secret")

	params["sign"] = "different_value"
	second := Sign(params, "secret")

	if first != second {
		t.Fatal("sign field should be excluded from calculation")
	}
}

func TestSign_ExcludesSignType(t *testing.T) {
	params := map[string]string{
		"pid":       "1000",
		"money":     "10.00",
		"sign_type": "MD5",
	}
	sign := Sign(params, "secret")
	if sign == "" {
		t.Fatal("sign should skip sign_type field")
	}
}

func TestSign_DifferentOrderSameResult(t *testing.T) {
	a := map[string]string{"a": "1", "b": "2"}
	b := map[string]string{"b": "2", "a": "1"}

	sa := Sign(a, "key")
	sb := Sign(b, "key")

	if sa != sb {
		t.Fatal("sign should be deterministic regardless of param order")
	}
}

func TestVerify_MissingSign(t *testing.T) {
	params := map[string]string{"pid": "1000"}
	if Verify(params, "secret") {
		t.Fatal("verify should fail when sign is missing")
	}
}

func TestVerify_EmptySign(t *testing.T) {
	params := map[string]string{"pid": "1000", "sign": ""}
	if Verify(params, "secret") {
		t.Fatal("verify should fail when sign is empty")
	}
}

func TestVerify_EmptySecret(t *testing.T) {
	params := map[string]string{"pid": "1000", "sign": "abc"}
	if Verify(params, "") {
		t.Fatal("verify should fail with empty secret")
	}
}
