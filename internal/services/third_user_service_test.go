package services

import (
	"testing"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"
)

func setLoginConfigForTest(t *testing.T, value string) {
	t.Helper()

	oldConfig := config.Instance
	config.Instance = &config.Config{Language: config.LanguageZhCN}
	t.Cleanup(func() {
		config.Instance = oldConfig
	})

	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.SysConfig{}); err != nil {
		t.Fatalf("auto migrate sys config: %v", err)
	}
	cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	t.Cleanup(func() {
		cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	})
	if err := SysConfigService.Set(constants.SysConfigLoginConfig, value); err != nil {
		t.Fatalf("set login config: %v", err)
	}
}

func TestEnsureRegisterEnabledRejectsWhenRegisterClosed(t *testing.T) {
	setLoginConfigForTest(t, `{"registerEnabled":false,"passwordLogin":{"enabled":true}}`)

	if err := ensureRegisterEnabled(); err == nil {
		t.Fatalf("expected third-party registration to be rejected when register is closed")
	}
}

func TestEnsureRegisterEnabledAllowsWhenRegisterOpen(t *testing.T) {
	setLoginConfigForTest(t, `{"passwordLogin":{"enabled":true}}`)

	if err := ensureRegisterEnabled(); err != nil {
		t.Fatalf("expected registration to be open by default, got %v", err)
	}
}

// 管理员后台创建用户走 UserService.SignUp，不应被注册开关影响。
func TestUserSignUpIgnoresRegisterSwitch(t *testing.T) {
	setLoginConfigForTest(t, `{"registerEnabled":false,"passwordLogin":{"enabled":true}}`)

	user, err := UserService.SignUp("admincreated", "admin-created@example.com", "admin-created", "Pass@12345", "Pass@12345")
	if err != nil {
		t.Fatalf("expected admin-side SignUp to keep working, got %v", err)
	}
	if user == nil || user.Id <= 0 {
		t.Fatalf("expected created user, got %#v", user)
	}
}

func TestEnsureGithubRegisterAllowed(t *testing.T) {
	setLoginConfigForTest(t, `{"registerEnabled":false,"passwordLogin":{"enabled":true}}`)

	if err := ensureGithubRegisterAllowed(nil); err == nil {
		t.Fatalf("expected new github user creation to be rejected when register is closed")
	}
	existing := &models.User{}
	if err := ensureGithubRegisterAllowed(existing); err != nil {
		t.Fatalf("expected github email binding path to stay allowed, got %v", err)
	}
}

func TestUserSignUpRejectsTooLongNickname(t *testing.T) {
	setLoginConfigForTest(t, `{"passwordLogin":{"enabled":true}}`)

	_, err := UserService.SignUp("longnick", "longnick@example.com", "12345678901234567", "Pass@12345", "Pass@12345")
	if err == nil {
		t.Fatalf("expected too long nickname to be rejected")
	}
}
