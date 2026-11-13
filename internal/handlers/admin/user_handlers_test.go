package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/search"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestUserListFiltersForbiddenUsers(t *testing.T) {
	db := setupAdminUserTestDB(t)
	now := time.Now().UnixMilli()
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 1},
		Nickname:         "normal",
		ForbiddenEndTime: 0,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 2},
		Nickname:         "forbidden",
		ForbiddenEndTime: now + 3600_000,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 3},
		Nickname:         "forever",
		ForbiddenEndTime: -1,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 4},
		Nickname:         "expired",
		ForbiddenEndTime: now - 3600_000,
	})

	users := postUserList(t, "forbidden=true")

	gotIDs := make([]int64, 0, len(users))
	for _, user := range users {
		gotIDs = append(gotIDs, int64(user["id"].(float64)))
		if forbidden, ok := user["forbidden"].(bool); !ok || !forbidden {
			t.Fatalf("expected only forbidden users, got %#v", user)
		}
	}
	wantIDs := []int64{3, 2}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
		}
	}
}

func TestUserListFiltersNonForbiddenUsers(t *testing.T) {
	db := setupAdminUserTestDB(t)
	now := time.Now().UnixMilli()
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 1},
		Nickname:         "normal",
		ForbiddenEndTime: 0,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 2},
		Nickname:         "forbidden",
		ForbiddenEndTime: now + 3600_000,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 3},
		Nickname:         "forever",
		ForbiddenEndTime: -1,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 4},
		Nickname:         "expired",
		ForbiddenEndTime: now - 3600_000,
	})

	users := postUserList(t, "forbidden=false")

	gotIDs := make([]int64, 0, len(users))
	for _, user := range users {
		gotIDs = append(gotIDs, int64(user["id"].(float64)))
		if forbidden, ok := user["forbidden"].(bool); !ok || forbidden {
			t.Fatalf("expected only non-forbidden users, got %#v", user)
		}
	}
	wantIDs := []int64{4, 1}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
		}
	}
}

func TestUserResetPasswordDisablesUserTokens(t *testing.T) {
	db := setupAdminUserTestDB(t)
	mustCreateUser(t, db, &models.User{
		Model:    models.Model{Id: 1},
		Nickname: "target",
		Status:   constants.StatusOk,
		Password: "old-password",
	})
	mustCreateUser(t, db, &models.User{
		Model:    models.Model{Id: 2},
		Nickname: "other",
		Status:   constants.StatusOk,
		Password: "old-password",
	})

	now := time.Now().UnixMilli()
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "target-active-1",
		UserId:     1,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusOk,
		CreateTime: now,
	})
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "target-active-2",
		UserId:     1,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusOk,
		CreateTime: now,
	})
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "target-deleted",
		UserId:     1,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusDeleted,
		CreateTime: now,
	})
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "other-active",
		UserId:     2,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusOk,
		CreateTime: now,
	})

	postUserResetPassword(t, "userId=1")

	var targetActiveCount int64
	if err := db.Model(&models.UserToken{}).
		Where("user_id = ? AND status = ?", 1, constants.StatusOk).
		Count(&targetActiveCount).Error; err != nil {
		t.Fatalf("count target active tokens: %v", err)
	}
	if targetActiveCount != 0 {
		t.Fatalf("expected target user active tokens to be disabled, got %d", targetActiveCount)
	}

	var otherActiveCount int64
	if err := db.Model(&models.UserToken{}).
		Where("user_id = ? AND status = ?", 2, constants.StatusOk).
		Count(&otherActiveCount).Error; err != nil {
		t.Fatalf("count other active tokens: %v", err)
	}
	if otherActiveCount != 1 {
		t.Fatalf("expected other user active token to remain, got %d", otherActiveCount)
	}
}

func setupAdminUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	idcodec.Init(1)
	config.Instance = &config.Config{
		Search: config.SearchConfig{
			IndexPath: filepath.Join(t.TempDir(), "index"),
		},
	}
	search.Init()

	dsn := fmt.Sprintf("file:admin_user_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	if err := db.AutoMigrate(&models.User{}, &models.UserToken{}); err != nil {
		t.Fatalf("auto migrate users: %v", err)
	}
	return db
}

func mustCreateUser(t *testing.T, db *gorm.DB, user *models.User) {
	t.Helper()

	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func mustCreateUserToken(t *testing.T, db *gorm.DB, userToken *models.UserToken) {
	t.Helper()

	if err := db.Create(userToken).Error; err != nil {
		t.Fatalf("create user token: %v", err)
	}
}

func postUserResetPassword(t *testing.T, body string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/reset_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserResetPassword(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Password string `json:"password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}
	if result.Data.Password == "" {
		t.Fatalf("expected reset password in response, got %s", w.Body.String())
	}
}

func postUserList(t *testing.T, body string) []map[string]interface{} {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/list", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserList(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Results []map[string]interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}
	return result.Data.Results
}

func TestUserBuildUserItemOmitsPassword(t *testing.T) {
	setupAdminUserTestDB(t)
	item := userBuildUserItem(&models.User{
		Model:    models.Model{Id: 1},
		Username: sqlNullString("tester"),
		Password: "hashed-password",
	}, false)

	if _, ok := item["password"]; ok {
		t.Fatalf("expected password hash to be omitted, got %#v", item["password"])
	}
}

func TestUserCreateAssignsRolesAndOmitsPassword(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 1},
		Type:   1,
		Name:   "Tester",
		Code:   "tester",
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, &models.User{
		Model:  models.Model{Id: 99},
		Roles:  constants.RoleOwner,
		Status: constants.StatusOk,
	})
	body := "username=createduser&email=createduser%40example.com&nickname=createduser&password=Pass%4012345&roleIds=1"
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserCreate(ctx)

	var result struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected create success, got %s", w.Body.String())
	}
	if _, ok := result.Data["password"]; ok {
		t.Fatalf("expected password hash to be omitted, got %#v", result.Data["password"])
	}
	if result.Data["username"] != "createduser" {
		t.Fatalf("expected username in response, got %#v", result.Data)
	}

	var roleCount int64
	if err := db.Model(&models.UserRole{}).Count(&roleCount).Error; err != nil {
		t.Fatalf("count user roles: %v", err)
	}
	if roleCount != 1 {
		t.Fatalf("expected 1 user role, got %d", roleCount)
	}
}

func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func TestUserCreateRejectsSystemRoleForNonOwner(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 1},
		Type:   constants.RoleTypeSystem,
		Name:   "Owner",
		Code:   constants.RoleOwner,
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create owner role: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, &models.User{
		Model:  models.Model{Id: 10},
		Roles:  "member",
		Status: constants.StatusOk,
	})
	body := "username=escalated&email=escalated%40example.com&nickname=escalated&password=Pass%4012345&roleIds=1"
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserCreate(ctx)

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if result.Success {
		t.Fatalf("expected system role assignment to be rejected, got %s", w.Body.String())
	}
	if result.Message != "无权分配系统角色" {
		t.Fatalf("expected role permission error, got %s", w.Body.String())
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created, got %d", userCount)
	}
}

func TestUserCreateRejectsInvalidRoleIds(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, &models.User{
		Model:  models.Model{Id: 99},
		Roles:  constants.RoleOwner,
		Status: constants.StatusOk,
	})
	body := "username=badrole&email=badrole%40example.com&nickname=badrole&password=Pass%4012345&roleIds=999"
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserCreate(ctx)

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if result.Success {
		t.Fatalf("expected invalid role ids to be rejected, got %s", w.Body.String())
	}
	if result.Message != "角色不存在或已禁用" {
		t.Fatalf("expected invalid role error, got %s", w.Body.String())
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created, got %d", userCount)
	}
}
