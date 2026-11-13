package migrations

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestMigrateRecountCommentCounts(t *testing.T) {
	dsn := fmt.Sprintf("file:migration_recount_comment_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "t_",
			SingularTable: true,
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)

	if err := db.AutoMigrate(&models.User{}, &models.Topic{}, &models.Article{}, &models.Comment{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.User{Model: models.Model{Id: 1}, Status: constants.StatusOk, CommentCount: 9}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&models.Topic{Model: models.Model{Id: 1}, Status: constants.StatusOk, CommentCount: 9}).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if err := db.Create(&models.Article{Model: models.Model{Id: 1}, Status: constants.StatusOk, CommentCount: 9}).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	comments := []models.Comment{
		{Model: models.Model{Id: 1}, UserId: 1, EntityType: constants.EntityTopic, EntityId: 1, Status: constants.StatusOk},
		{Model: models.Model{Id: 2}, UserId: 1, EntityType: constants.EntityTopic, EntityId: 1, Status: constants.StatusOk},
		{Model: models.Model{Id: 3}, UserId: 1, EntityType: constants.EntityTopic, EntityId: 1, Status: constants.StatusDeleted},
		{Model: models.Model{Id: 4}, UserId: 1, EntityType: constants.EntityArticle, EntityId: 1, Status: constants.StatusOk},
		{Model: models.Model{Id: 10}, UserId: 1, EntityType: constants.EntityTopic, EntityId: 1, Status: constants.StatusOk, CommentCount: 9},
		{Model: models.Model{Id: 11}, UserId: 1, EntityType: constants.EntityComment, EntityId: 10, Status: constants.StatusOk},
		{Model: models.Model{Id: 12}, UserId: 1, EntityType: constants.EntityComment, EntityId: 10, Status: constants.StatusOk},
	}
	if err := db.Create(&comments).Error; err != nil {
		t.Fatalf("create comments: %v", err)
	}

	if err := migrate_recount_comment_counts(); err != nil {
		t.Fatalf("recount: %v", err)
	}

	var topic models.Topic
	if err := db.First(&topic, 1).Error; err != nil {
		t.Fatalf("load topic: %v", err)
	}
	if topic.CommentCount != 3 {
		t.Fatalf("expected topic comment_count=3, got %d", topic.CommentCount)
	}
	var article models.Article
	if err := db.First(&article, 1).Error; err != nil {
		t.Fatalf("load article: %v", err)
	}
	if article.CommentCount != 1 {
		t.Fatalf("expected article comment_count=1, got %d", article.CommentCount)
	}
	var parent models.Comment
	if err := db.First(&parent, 10).Error; err != nil {
		t.Fatalf("load parent comment: %v", err)
	}
	if parent.CommentCount != 2 {
		t.Fatalf("expected parent comment_count=2, got %d", parent.CommentCount)
	}
	var user models.User
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if user.CommentCount != 6 {
		t.Fatalf("expected user comment_count=6, got %d", user.CommentCount)
	}
}
