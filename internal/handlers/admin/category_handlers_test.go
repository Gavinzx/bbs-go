package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/resp"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestFilterCategoryListByCategoryIDIncludesSelectedCategoryWithAncestors(t *testing.T) {
	categories := []models.Category{
		{Model: models.Model{Id: 1}, Name: "root", ParentId: 0},
		{Model: models.Model{Id: 2}, Name: "child", ParentId: 1},
		{Model: models.Model{Id: 3}, Name: "sibling", ParentId: 1},
		{Model: models.Model{Id: 4}, Name: "grandchild", ParentId: 2},
		{Model: models.Model{Id: 5}, Name: "other-root", ParentId: 0},
	}

	got := filterCategoryListByCategoryID(categories, 2)
	gotIDs := categoryIDs(got)
	wantIDs := []int64{1, 2, 4}

	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
		}
	}
}

func categoryIDs(categories []models.Category) []int64 {
	ids := make([]int64, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.Id)
	}
	return ids
}

func TestCategoryListFiltersByStatus(t *testing.T) {
	db := setupAdminCategoryTestDB(t)
	mustCreateCategory(t, db, &models.Category{
		Model:  models.Model{Id: 1},
		Name:   "active-root",
		Status: constants.StatusOk,
	})
	mustCreateCategory(t, db, &models.Category{
		Model:    models.Model{Id: 2},
		Name:     "active-child",
		ParentId: 1,
		Status:   constants.StatusOk,
	})
	mustCreateCategory(t, db, &models.Category{
		Model:  models.Model{Id: 3},
		Name:   "deleted-root",
		Status: constants.StatusDeleted,
	})

	data := postCategoryList(t, "status=1")
	if len(data) != 1 {
		t.Fatalf("expected one deleted root category, got %#v", data)
	}
	if data[0].Id != 3 {
		t.Fatalf("expected deleted category id 3, got %#v", data)
	}
}

func setupAdminCategoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:admin_category_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	if err := db.AutoMigrate(&models.Category{}); err != nil {
		t.Fatalf("auto migrate categories: %v", err)
	}
	return db
}

func mustCreateCategory(t *testing.T, db *gorm.DB, category *models.Category) {
	t.Helper()

	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
}

func postCategoryList(t *testing.T, body string) []resp.CategoryTreeItem {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/category/list", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	CategoryList(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result struct {
		Success bool                    `json:"success"`
		Data    []resp.CategoryTreeItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}
	return result.Data
}

func TestCategoryCreatePersistsQATypeAndChildInherits(t *testing.T) {
	db := setupAdminCategoryTestDB(t)

	root := createCategoryViaHandler(t, db, "parentId=0&type=qa&name=qa-root&description=qa+root&status=0")
	if root.Type != constants.CategoryTypeQA {
		t.Fatalf("expected root type qa, got %q", root.Type)
	}

	child := createCategoryViaHandler(t, db, fmt.Sprintf("parentId=%d&type=normal&name=qa-child&description=qa+child&status=0", root.Id))
	if child.Type != constants.CategoryTypeQA {
		t.Fatalf("expected child to inherit qa, got %q", child.Type)
	}
}

func createCategoryViaHandler(t *testing.T, db *gorm.DB, body string) models.Category {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/category/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	CategoryCreate(ctx)

	var result struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    models.Category `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}

	var category models.Category
	if err := db.First(&category, result.Data.Id).Error; err != nil {
		t.Fatalf("load category: %v", err)
	}
	return category
}

func TestCategoryUpdateAllowsEmptyDescriptionAndChangesType(t *testing.T) {
	db := setupAdminCategoryTestDB(t)
	mustCreateCategory(t, db, &models.Category{
		Model:  models.Model{Id: 1},
		Name:   "empty-description",
		Type:   constants.CategoryTypeNormal,
		Status: constants.StatusOk,
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/category/update", strings.NewReader("id=1&type=qa&name=empty-description&status=0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	CategoryUpdate(ctx)

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected update success, got %s", w.Body.String())
	}

	var category models.Category
	if err := db.First(&category, 1).Error; err != nil {
		t.Fatalf("load category: %v", err)
	}
	if category.Type != constants.CategoryTypeQA {
		t.Fatalf("expected type qa, got %q", category.Type)
	}
}
