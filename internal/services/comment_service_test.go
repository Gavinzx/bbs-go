package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/req"
	"bbs-go/internal/permissions"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/repositories"
	"testing"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

func setupCommentServiceTestDB(t *testing.T) {
	t.Helper()
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Comment{}, &models.Topic{}, &models.Article{}, &models.Role{}, &models.UserRole{}, &models.Permission{}, &models.RolePermission{}); err != nil {
		t.Fatalf("auto migrate comment: %v", err)
	}
	PermissionService.ClearCache()
}

func mustCreateComment(t *testing.T, comment *models.Comment) *models.Comment {
	t.Helper()
	if comment.Status == 0 {
		comment.Status = constants.StatusOk
	}
	if err := repositories.CommentRepository.Create(sqls.DB(), comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return comment
}

func TestCommentService_DeleteByUserRejectsNonAuthorWithoutPermission(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityTopic,
		EntityId:    20,
		Content:     "hello",
		ContentType: constants.ContentTypeText,
	})

	regularUser := &models.User{Roles: ""}
	if err := CommentService.DeleteByUser(regularUser, comment.Id); err == nil {
		t.Fatalf("expected permission error for regular user")
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusOk {
		t.Fatalf("expected comment status ok, got %d", got.Status)
	}
}

func TestCommentService_DeleteByUserAllowsAuthor(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityTopic,
		EntityId:    20,
		Content:     "hello",
		ContentType: constants.ContentTypeText,
	})

	author := &models.User{Model: models.Model{Id: 10}}
	if err := CommentService.DeleteByUser(author, comment.Id); err != nil {
		t.Fatalf("delete by author: %v", err)
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusDeleted {
		t.Fatalf("expected comment status deleted, got %d", got.Status)
	}
}

func TestCommentService_DeleteByUserAllowsCommentDeletePermission(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityTopic,
		EntityId:    20,
		Content:     "hello",
		ContentType: constants.ContentTypeText,
	})
	now := dates.NowTimestamp()
	moderator := mustCreateUser(t, now)
	role := mustCreateRole(t, "comment-moderator", constants.StatusOk)
	permission := mustCreatePermission(t, permissions.PermissionCommentDelete.Code, constants.StatusOk)
	mustAssignRole(t, moderator, role)
	mustGrantPermission(t, role, permission)

	if err := CommentService.DeleteByUser(moderator, comment.Id); err != nil {
		t.Fatalf("delete with comment permission: %v", err)
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusDeleted {
		t.Fatalf("expected comment status deleted, got %d", got.Status)
	}
}

func TestCommentService_DeleteByUserAllowsOwner(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityComment,
		EntityId:    20,
		Content:     "reply",
		ContentType: constants.ContentTypeText,
	})

	ownerUser := &models.User{Roles: constants.RoleOwner}
	if err := CommentService.DeleteByUser(ownerUser, comment.Id); err != nil {
		t.Fatalf("delete by owner: %v", err)
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusDeleted {
		t.Fatalf("expected comment status deleted, got %d", got.Status)
	}
}

func TestCommentService_PublishArticleIncrementsAndDeleteDecrements(t *testing.T) {
	setupCommentServiceTestDB(t)
	db := sqls.DB()
	if err := db.AutoMigrate(&models.Article{}); err != nil {
		t.Fatalf("auto migrate article: %v", err)
	}
	if err := db.Create(&models.Article{
		Model:       models.Model{Id: 100},
		ContentType: constants.ContentTypeText,
		Status:      constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}
	user := mustCreateUser(t, dates.NowTimestamp())

	comment, err := CommentService.Publish(user.Id, req.CreateCommentReq{
		EntityType: constants.EntityArticle,
		EntityId:   "100",
		Content:    "article comment",
	})
	if err != nil {
		t.Fatalf("publish article comment: %v", err)
	}
	article := ArticleService.Get(100)
	if article == nil || article.CommentCount != 1 {
		t.Fatalf("expected article comment_count=1, got %#v", article)
	}

	if err := CommentService.Delete(comment.Id); err != nil {
		t.Fatalf("delete article comment: %v", err)
	}
	article = ArticleService.Get(100)
	if article == nil || article.CommentCount != 0 {
		t.Fatalf("expected article comment_count=0, got %#v", article)
	}
}

func TestCommentService_DeleteTopicCommentDecrementsTopicAndUserCounts(t *testing.T) {
	setupCommentServiceTestDB(t)
	db := sqls.DB()
	if err := db.AutoMigrate(&models.Topic{}); err != nil {
		t.Fatalf("auto migrate topic: %v", err)
	}
	if err := db.Create(&models.Topic{
		Model:        models.Model{Id: 200},
		CommentCount: 3,
		Status:       constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	user := mustCreateUser(t, dates.NowTimestamp())
	if err := db.Model(user).Update("comment_count", 3).Error; err != nil {
		t.Fatalf("set user comment count: %v", err)
	}
	comment := mustCreateComment(t, &models.Comment{
		UserId:     user.Id,
		EntityType: constants.EntityTopic,
		EntityId:   200,
		Content:    "topic comment",
	})

	if err := CommentService.Delete(comment.Id); err != nil {
		t.Fatalf("delete topic comment: %v", err)
	}
	if err := CommentService.Delete(comment.Id); err != nil {
		t.Fatalf("delete topic comment twice: %v", err)
	}

	topic := TopicService.Get(200)
	if topic == nil || topic.CommentCount != 2 {
		t.Fatalf("expected topic comment_count=2, got %#v", topic)
	}
	gotUser := UserService.Get(user.Id)
	if gotUser == nil || gotUser.CommentCount != 2 {
		t.Fatalf("expected user comment_count=2, got %#v", gotUser)
	}
}

func TestCommentService_DeleteReplyDecrementsParentCommentCount(t *testing.T) {
	setupCommentServiceTestDB(t)
	parent := mustCreateComment(t, &models.Comment{
		UserId:       10,
		EntityType:   constants.EntityTopic,
		EntityId:     300,
		Content:      "parent",
		CommentCount: 2,
	})
	reply := mustCreateComment(t, &models.Comment{
		UserId:     11,
		EntityType: constants.EntityComment,
		EntityId:   parent.Id,
		Content:    "reply",
	})

	if err := CommentService.Delete(reply.Id); err != nil {
		t.Fatalf("delete reply: %v", err)
	}
	got := CommentService.Get(parent.Id)
	if got == nil || got.CommentCount != 1 {
		t.Fatalf("expected parent comment_count=1, got %#v", got)
	}
}
