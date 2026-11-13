package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
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
	"bbs-go/internal/pkg/xlsx"
	"bbs-go/internal/services"

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
	if result.Message != "admin.role_no_permission" {
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
	if result.Message != "admin.role_not_found_or_disabled" {
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

func postUserBatchCreate(t *testing.T, body string) (bool, map[string]any) {
	t.Helper()
	return postUserBatchCreateAs(t, body, &models.User{
		Model:  models.Model{Id: 99},
		Roles:  constants.RoleOwner,
		Status: constants.StatusOk,
	})
}

func postUserBatchCreateAs(t *testing.T, body string, current *models.User) (bool, map[string]any) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, current)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/batch_create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	UserBatchCreate(ctx)

	var result struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		return false, map[string]any{"message": result.Message}
	}
	return true, result.Data
}

func TestUserBatchCreateRejectsEmptyUsers(t *testing.T) {
	setupAdminUserTestDB(t)

	ok, data := postUserBatchCreate(t, `{"users":[]}`)
	if ok {
		t.Fatalf("expected empty batch to be rejected, got %#v", data)
	}
	if data["message"] != "admin.user_batch_empty" {
		t.Fatalf("expected empty message key, got %#v", data)
	}
}

func TestUserBatchCreateRejectsTooManyUsers(t *testing.T) {
	setupAdminUserTestDB(t)

	users := make([]map[string]string, 0, maxBatchCreateUsers+1)
	for i := 0; i < maxBatchCreateUsers+1; i++ {
		users = append(users, map[string]string{
			"username": fmt.Sprintf("batchuser%d", i),
			"email":    fmt.Sprintf("batchuser%d@example.com", i),
			"nickname": fmt.Sprintf("batchuser%d", i),
			"password": "Pass@12345",
		})
	}
	body, err := json.Marshal(map[string]any{"users": users})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	ok, data := postUserBatchCreate(t, string(body))
	if ok {
		t.Fatalf("expected oversized batch to be rejected, got %#v", data)
	}
	if data["message"] != "admin.user_batch_limit" {
		t.Fatalf("expected limit message key, got %#v", data)
	}
}

func TestUserBatchCreateCreatesUsers(t *testing.T) {
	db := setupAdminUserTestDB(t)

	body := `{"users":[
		{"username":"batchone","email":"batchone@example.com","nickname":"Batch One","password":"Pass@12345"},
		{"username":"batchtwo","email":"batchtwo@example.com","nickname":"Batch Two","password":"Pass@12345"}
	]}`
	ok, data := postUserBatchCreate(t, body)
	if !ok {
		t.Fatalf("expected batch create success, got %#v", data)
	}
	if data["created"] != float64(2) || data["roleFailed"] != float64(0) || data["failed"] != float64(0) {
		t.Fatalf("expected created=2 failed=0, got %#v", data)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 2 {
		t.Fatalf("expected 2 users, got %d", userCount)
	}
}

func TestUserBatchCreateReturnsPerUserErrors(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.Create(&models.User{
		Model:    models.Model{Id: 1},
		Username: sqlNullString("duplicated"),
		Email:    sqlNullString("duplicated@example.com"),
		Nickname: "duplicated",
		Status:   constants.StatusOk,
		Password: "hashed",
	}).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	body := `{"users":[
		{"username":"freshuser","email":"freshuser@example.com","nickname":"Fresh","password":"Pass@12345"},
		{"username":"duplicated","email":"duplicated2@example.com","nickname":"Duplicated","password":"Pass@12345"}
	]}`
	ok, data := postUserBatchCreate(t, body)
	if !ok {
		t.Fatalf("expected partial success response, got %#v", data)
	}
	if data["created"] != float64(1) || data["roleFailed"] != float64(0) || data["failed"] != float64(1) {
		t.Fatalf("expected created=1 failed=1, got %#v", data)
	}
	errs, _ := data["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error detail, got %#v", data["errors"])
	}
	detail := errs[0].(map[string]any)
	if detail["index"] != float64(1) || detail["username"] != "duplicated" {
		t.Fatalf("unexpected error detail: %#v", detail)
	}
}

func TestUserBatchCreateAssignsRoles(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 2},
		Type:   1,
		Name:   "Tester",
		Code:   "tester",
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	body := `{"users":[{"username":"batchrole","email":"batchrole@example.com","nickname":"Batch Role","password":"Pass@12345","roleIds":"2"}]}`
	ok, data := postUserBatchCreate(t, body)
	if !ok {
		t.Fatalf("expected batch create success, got %#v", data)
	}
	if data["created"] != float64(1) {
		t.Fatalf("expected created=1, got %#v", data)
	}

	var roleCount int64
	if err := db.Model(&models.UserRole{}).Count(&roleCount).Error; err != nil {
		t.Fatalf("count user roles: %v", err)
	}
	if roleCount != 1 {
		t.Fatalf("expected 1 user role, got %d", roleCount)
	}
}

func TestUserBatchCreateRejectsSystemRoleForNonOwner(t *testing.T) {
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
		t.Fatalf("create role: %v", err)
	}

	body := `{"users":[{"username":"batchescalate","email":"batchescalate@example.com","nickname":"Escalate","password":"Pass@12345","roleIds":"1"}]}`
	ok, data := postUserBatchCreateAs(t, body, &models.User{
		Model:  models.Model{Id: 10},
		Roles:  "member",
		Status: constants.StatusOk,
	})
	if !ok {
		t.Fatalf("expected per-user failure response, got %#v", data)
	}
	if data["created"] != float64(0) || data["failed"] != float64(1) {
		t.Fatalf("expected created=0 failed=1, got %#v", data)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created, got %d", userCount)
	}
}

func TestUserBatchCreateRejectsInvalidRoleIds(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}

	body := `{"users":[{"username":"batchbadrole","email":"batchbadrole@example.com","nickname":"Bad Role","password":"Pass@12345","roleIds":"999"}]}`
	ok, data := postUserBatchCreate(t, body)
	if !ok {
		t.Fatalf("expected per-user failure response, got %#v", data)
	}
	if data["created"] != float64(0) || data["failed"] != float64(1) {
		t.Fatalf("expected created=0 failed=1, got %#v", data)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created, got %d", userCount)
	}
}

func TestUserBatchCreateRejectsEmptyUsername(t *testing.T) {
	db := setupAdminUserTestDB(t)

	body := `{"users":[{"username":"","email":"emptyuser@example.com","nickname":"Empty","password":"Pass@12345"}]}`
	ok, data := postUserBatchCreate(t, body)
	if !ok {
		t.Fatalf("expected per-user failure response, got %#v", data)
	}
	if data["created"] != float64(0) || data["failed"] != float64(1) {
		t.Fatalf("expected created=0 failed=1, got %#v", data)
	}
	errs, _ := data["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["message"] != "admin.username_required" {
		t.Fatalf("unexpected error details: %#v", data["errors"])
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user to be created, got %d", userCount)
	}
}

func TestUserBatchCreateReportsRoleAssignmentFailure(t *testing.T) {
	db := setupAdminUserTestDB(t)
	// 仅迁移 Role，不迁移 UserRole，使角色写入失败
	if err := db.AutoMigrate(&models.Role{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 2},
		Type:   1,
		Name:   "Tester",
		Code:   "tester",
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	body := `{"users":[{"username":"rolefail","email":"rolefail@example.com","nickname":"Role Fail","password":"Pass@12345","roleIds":"2"}]}`
	ok, data := postUserBatchCreate(t, body)
	if !ok {
		t.Fatalf("expected per-user failure response, got %#v", data)
	}
	if data["created"] != float64(0) || data["roleFailed"] != float64(1) || data["failed"] != float64(0) {
		t.Fatalf("expected created=0 roleFailed=1 failed=0, got %#v", data)
	}
	errs, _ := data["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["message"] != "admin.user_role_assign_failed" {
		t.Fatalf("unexpected error details: %#v", data["errors"])
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected the user to be created despite role failure, got %d", userCount)
	}
}

func postUserImport(t *testing.T, data []byte, defaultPassword, roleID string, current *models.User) (bool, map[string]any) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	filePart, err := writer.CreateFormFile("file", "users.xlsx")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := filePart.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	_ = writer.WriteField("defaultPassword", defaultPassword)
	_ = writer.WriteField("roleId", roleID)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, current)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request = req

	UserImport(ctx)

	var result struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		return false, map[string]any{"message": result.Message}
	}
	return true, result.Data
}

func ownerUser() *models.User {
	return &models.User{
		Model:  models.Model{Id: 99},
		Roles:  constants.RoleOwner,
		Status: constants.StatusOk,
	}
}

func TestUserImportCreatesUsersWithRowOrDefaultPassword(t *testing.T) {
	db := setupAdminUserTestDB(t)
	data, err := xlsx.Write([][]string{
		{"用户名", "邮箱", "昵称", "密码"},
		{"import_one", "import_one@example.com", "Import One", "Row@12345"},
		{"import_two", "import_two@example.com", "Import Two", ""},
	})
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	ok, result := postUserImport(t, data, "Default@123", "", ownerUser())
	if !ok {
		t.Fatalf("expected import success, got %#v", result)
	}
	if result["created"] != float64(2) || result["failed"] != float64(0) {
		t.Fatalf("expected created=2 failed=0, got %#v", result)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 2 {
		t.Fatalf("expected 2 users, got %d", userCount)
	}

	if _, err := services.UserService.SignIn("import_one", "Row@12345"); err != nil {
		t.Fatalf("expected row password to be used: %v", err)
	}
	if _, err := services.UserService.SignIn("import_two", "Default@123"); err != nil {
		t.Fatalf("expected default password to be used: %v", err)
	}
}

func TestUserImportContinuesAfterRowFailure(t *testing.T) {
	db := setupAdminUserTestDB(t)
	data, err := xlsx.Write([][]string{
		{"用户名", "邮箱", "昵称", "密码"},
		{"bad1", "bad1@example.com", "Bad One", "Pass@12345"},
		{"import_ok", "import_ok@example.com", "Import OK", "Pass@12345"},
	})
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	ok, result := postUserImport(t, data, "", "", ownerUser())
	if !ok {
		t.Fatalf("expected partial success, got %#v", result)
	}
	if result["created"] != float64(1) || result["failed"] != float64(1) {
		t.Fatalf("expected created=1 failed=1, got %#v", result)
	}
	errs, _ := result["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error detail, got %#v", result["errors"])
	}
	detail := errs[0].(map[string]any)
	if detail["username"] != "bad1" || detail["row"] != float64(2) {
		t.Fatalf("unexpected error detail: %#v", detail)
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected only 1 user, got %d", userCount)
	}
}

func TestUserImportAssignsSelectedRole(t *testing.T) {
	db := setupAdminUserTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate roles: %v", err)
	}
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 2},
		Type:   1,
		Name:   "Tester",
		Code:   "tester",
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	data, err := xlsx.Write([][]string{
		{"用户名", "邮箱", "昵称", "密码"},
		{"import_role", "import_role@example.com", "Import Role", "Pass@12345"},
	})
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	ok, result := postUserImport(t, data, "", "2", ownerUser())
	if !ok {
		t.Fatalf("expected import success, got %#v", result)
	}
	if result["created"] != float64(1) {
		t.Fatalf("expected created=1, got %#v", result)
	}

	var roleCount int64
	if err := db.Model(&models.UserRole{}).Count(&roleCount).Error; err != nil {
		t.Fatalf("count user roles: %v", err)
	}
	if roleCount != 1 {
		t.Fatalf("expected 1 user role, got %d", roleCount)
	}
}

func TestUserImportRejectsInvalidHeader(t *testing.T) {
	setupAdminUserTestDB(t)
	data, err := xlsx.Write([][]string{{"name", "mail", "nick", "password"}})
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	ok, result := postUserImport(t, data, "", "", ownerUser())
	if ok {
		t.Fatalf("expected invalid header to be rejected, got %#v", result)
	}
	if result["message"] != "admin.import_header_invalid" {
		t.Fatalf("expected header error key, got %#v", result)
	}
}

func TestUserImportTemplateReturnsXLSX(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/user/import_template", nil)

	UserImportTemplate(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if contentType := w.Header().Get("Content-Type"); contentType == "" {
		t.Fatalf("expected xlsx content type")
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		t.Fatalf("expected xlsx zip payload")
	}
}

func TestUserImportRequiresPasswordWhenNoDefault(t *testing.T) {
	db := setupAdminUserTestDB(t)
	data, err := xlsx.Write([][]string{
		{"用户名", "邮箱", "昵称"},
		{"no_password", "no_password@example.com", "No Password"},
	})
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	ok, result := postUserImport(t, data, "", "", ownerUser())
	if !ok {
		t.Fatalf("expected per-row failure response, got %#v", result)
	}
	if result["created"] != float64(0) || result["failed"] != float64(1) {
		t.Fatalf("expected created=0 failed=1, got %#v", result)
	}
	errs, _ := result["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["message"] != "admin.import_password_required" {
		t.Fatalf("unexpected error details: %#v", result["errors"])
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user, got %d", userCount)
	}
}

func TestUserCreateRejectsInvalidUsername(t *testing.T) {
	db := setupAdminUserTestDB(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, ownerUser())
	body := "username=bad1&email=bad1%40example.com&nickname=Bad&password=Pass%4012345"
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
	if result.Success || result.Message != "user.username_invalid" {
		t.Fatalf("expected username validation error, got %s", w.Body.String())
	}

	var userCount int64
	if err := db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected no user, got %d", userCount)
	}
}

func TestUserUpdateRejectsInvalidChangedUsername(t *testing.T) {
	db := setupAdminUserTestDB(t)
	mustCreateUser(t, db, &models.User{
		Model:    models.Model{Id: 1},
		Username: sqlNullString("olduser"),
		Email:    sqlNullString("olduser@example.com"),
		Nickname: "Old User",
		Status:   constants.StatusOk,
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	common.SetCurrentUser(ctx, ownerUser())
	body := "id=1&username=bad1&email=olduser%40example.com&nickname=Old+User&status=0"
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserUpdate(ctx)

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if result.Success || result.Message != "user.username_invalid" {
		t.Fatalf("expected username validation error, got %s", w.Body.String())
	}
}
