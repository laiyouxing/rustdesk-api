package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/pquerna/otp/totp"
	"github.com/sirupsen/logrus"
)

// TestVerifyMfaCode_Regression 回归测试：覆盖 MFA 动态码校验的正确/错误码、空密钥守卫，
// 以及时钟偏移（Skew 加固）场景。无需数据库，纯内存校验。
func TestVerifyMfaCode_Regression(t *testing.T) {
	// 避免修复后 Logger.Warnf 在测试中 nil panic
	Logger = logrus.New()

	us := &UserService{}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "RustDesk", AccountName: "alice"})
	if err != nil {
		t.Fatalf("totp.Generate: %v", err)
	}

	u := &model.User{Username: "alice", MfaEnabled: true, MfaSecret: key.Secret()}

	// 1) 正确动态码应通过
	code, err := totp.GenerateCode(u.MfaSecret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !us.VerifyMfaCode(u, code) {
		t.Fatalf("正确动态码 %q 应校验通过", code)
	}

	// 2) 错误动态码应被拒绝
	if us.VerifyMfaCode(u, "000000") {
		t.Fatalf("错误动态码不应通过")
	}

	// 3) 空密钥应被守卫拒绝（并打告警），不会静默误判
	empty := &model.User{MfaEnabled: true, MfaSecret: ""}
	if us.VerifyMfaCode(empty, code) {
		t.Fatalf("空密钥不应通过校验")
	}

	// 4) 时钟偏移 60s（超出默认 Skew=1 的 ±30s）场景下，加固后的 Skew=3（±90s）应可通过
	skewedCode, err := totp.GenerateCode(u.MfaSecret, time.Now().Add(-60*time.Second))
	if err != nil {
		t.Fatalf("GenerateCode(skewed): %v", err)
	}
	// 旧行为（标准 totp.Validate, Skew=1）在此偏移下必然失败 —— 复现原 Bug 场景
	if totp.Validate(skewedCode, u.MfaSecret) {
		t.Fatalf("预期旧逻辑在 60s 偏移下失败，实际却通过（与根因不符）")
	}
	// 新行为（VerifyMfaCode, Skew=3）在此偏移下应通过 —— 验证加固生效
	if !us.VerifyMfaCode(u, skewedCode) {
		t.Fatalf("加固后 VerifyMfaCode 在 60s 偏移下应能通过")
	}
}

// TestVerifyMfaCode_ConfigurableSkew 验证 mfa_totp_skew 配置生效：
// 运维可通过 config.yaml 的 mfa_totp_skew 调整 TOTP 时钟容差（Skew=N → 容忍 ±30s×N），
// 无需重新编译。本测试同时覆盖“放大容差接受更大偏移”与“默认容差拒绝过大偏移”两种情形。
func TestVerifyMfaCode_ConfigurableSkew(t *testing.T) {
	// 避免 Logger.Warnf 在测试中 nil panic
	Logger = logrus.New()
	defer func() { Config = nil }() // 还原全局，避免污染其他测试

	us := &UserService{}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "RustDesk", AccountName: "alice"})
	if err != nil {
		t.Fatalf("totp.Generate: %v", err)
	}
	u := &model.User{Username: "alice", MfaEnabled: true, MfaSecret: key.Secret()}

	const offset = 150 * time.Second // 150s 偏移：仅 Skew>=5（±150s）才能容忍

	// 1) 配置较大容差 Skew=6（±180s）：150s 偏移应通过
	Config = &config.Config{MfaTotpSkew: 6}
	code, err := totp.GenerateCode(u.MfaSecret, time.Now().Add(-offset))
	if err != nil {
		t.Fatalf("GenerateCode(skewed): %v", err)
	}
	if !us.VerifyMfaCode(u, code) {
		t.Fatalf("配置 Skew=6 时，%v 偏移的验证码应能通过", offset)
	}

	// 2) 对照：回退为默认 Skew=3（±90s）< 150s：应被拒绝
	Config = &config.Config{MfaTotpSkew: 3}
	if us.VerifyMfaCode(u, code) {
		t.Fatalf("默认 Skew=3 时，%v 偏移的验证码不应通过", offset)
	}

	// 3) 边界：配置非法值（0 / 负数）应回退默认 Skew=3，150s 偏移仍被拒绝
	Config = &config.Config{MfaTotpSkew: 0}
	if us.VerifyMfaCode(u, code) {
		t.Fatalf("非法配置 Skew=0 应回退默认(3)，%v 偏移的验证码不应通过", offset)
	}
}
