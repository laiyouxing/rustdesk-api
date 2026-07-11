package service

import (
	"testing"
	"time"

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
