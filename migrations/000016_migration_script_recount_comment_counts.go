package migrations

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"

	"github.com/mlogclub/simple/sqls"
)

// migrate_recount_comment_counts 修复历史评论计数：
// 文章评论从未累加，评论删除也未回退，导致列表计数与详情不一致。
func migrate_recount_comment_counts() error {
	db := sqls.DB()

	if err := db.Exec(`
		UPDATE t_topic
		SET comment_count = (
			SELECT COUNT(*) FROM t_comment c
			WHERE c.entity_type = ? AND c.entity_id = t_topic.id AND c.status = ?
		)`, constants.EntityTopic, constants.StatusOk).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE t_article
		SET comment_count = (
			SELECT COUNT(*) FROM t_comment c
			WHERE c.entity_type = ? AND c.entity_id = t_article.id AND c.status = ?
		)`, constants.EntityArticle, constants.StatusOk).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE t_user
		SET comment_count = (
			SELECT COUNT(*) FROM t_comment c
			WHERE c.user_id = t_user.id AND c.status = ?
		)`, constants.StatusOk).Error; err != nil {
		return err
	}

	var comments []models.Comment
	if err := db.Find(&comments).Error; err != nil {
		return err
	}
	for _, comment := range comments {
		var count int64
		if err := db.Model(&models.Comment{}).
			Where("entity_type = ? AND entity_id = ? AND status = ?", constants.EntityComment, comment.Id, constants.StatusOk).
			Count(&count).Error; err != nil {
			return err
		}
		if comment.CommentCount == count {
			continue
		}
		if err := db.Model(&models.Comment{}).Where("id = ?", comment.Id).
			UpdateColumn("comment_count", count).Error; err != nil {
			return err
		}
	}
	return nil
}
