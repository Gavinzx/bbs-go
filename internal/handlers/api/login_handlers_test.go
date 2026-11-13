package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func setupLoginHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:login_handler_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "t_",
			SingularTable: true,
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	sqls.SetDB(db)
	if err := db.AutoMigrate(&models.SysConfig{}, &models.User{}); err != nil {
		t.Fatalf("auto migrate sys config: %v", err)
	}
	return db
}

func setInstalledTestConfig(t *testing.T) {
	t.Helper()

	oldConfig := config.Instance
	config.Instance = &config.Config{Installed: true, Language: config.LanguageZhCN}
	t.Cleanup(func() {
		config.Instance = oldConfig
	})
}

func setLoginConfig(t *testing.T, value string) {
	t.Helper()

	cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	t.Cleanup(func() {
		cache.SysConfigCache.Invalidate(constants.SysConfigLoginConfig)
	})
	if err := services.SysConfigService.Set(constants.SysConfigLoginConfig, value); err != nil {
		t.Fatalf("set login config: %v", err)
	}
}

func postSignup(t *testing.T) map[string]any {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	form := url.Values{
		"username":   {"newuser"},
		"email":      {"new-user@example.com"},
		"nickname":   {"new-user"},
		"password":   {"Pass@12345"},
		"rePassword": {"Pass@12345"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/login/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	LoginSignup(ctx)

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return result
}

func TestLoginSignupRejectedWhenRegisterClosed(t *testing.T) {
	db := setupLoginHandlerTestDB(t)
	setInstalledTestConfig(t)
	setLoginConfig(t, `{"registerEnabled":false,"passwordLogin":{"enabled":true}}`)

	result := postSignup(t)
	if result["success"] == true {
		t.Fatalf("expected signup to be rejected when register is closed, got %#v", result)
	}
	if result["message"] != "auth.register_disabled" {
		t.Fatalf("expected register-disabled message, got %#v", result)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created, got %d", userCount)
	}
}

func TestLoginSignupNotBlockedWhenRegisterOpen(t *testing.T) {
	db := setupLoginHandlerTestDB(t)
	setInstalledTestConfig(t)
	setLoginConfig(t, `{"registerEnabled":true,"passwordLogin":{"enabled":true}}`)

	result := postSignup(t)
	if result["success"] == true {
		t.Fatalf("expected signup to fail on captcha without a valid captcha, got %#v", result)
	}
	if result["message"] == "auth.register_disabled" {
		t.Fatalf("expected register gate to allow signup when open, got %#v", result)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created without captcha, got %d", userCount)
	}
}
