package admin

import (
	"bbs-go/internal/cache"
	"bbs-go/internal/models/constants"
	modelReq "bbs-go/internal/models/req"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/errs"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/repositories"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"bbs-go/internal/models"

	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/pkg/params"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"github.com/mlogclub/simple/web"
	"github.com/spf13/cast"

	"bbs-go/internal/services"
)

// 禁言
// 修改自己的密码
// PostResetPassword 重置密码
func userBuildUserItem(user *models.User, buildRoleIds bool) map[string]interface{} {
	// 管理端响应不返回密码哈希
	b := web.NewRspBuilderExcludes(user, "password").
		Put("idEncode", idcodec.Encode(user.Id)).
		Put("roles", user.GetRoles()).
		Put("username", user.Username.String).
		Put("email", user.Email.String).
		Put("forbidden", user.IsForbidden())
	if buildRoleIds {
		b.Put("roleIds", services.UserRoleService.GetUserRoleIds(user.Id))
	}
	return b.Build()
}

// normalizeRoleIds 去重并过滤非法 ID
func normalizeRoleIds(roleIds []int64) []int64 {
	seen := make(map[int64]struct{}, len(roleIds))
	result := make([]int64, 0, len(roleIds))
	for _, id := range roleIds {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// checkAssignableRoles 校验角色是否存在、启用，且非 Owner 不得分配系统角色。
// 管理员创建/编辑用户共用，避免通过 roleIds 绕过系统角色保护。
func checkAssignableRoles(ctx *gin.Context, roleIds []int64) ([]int64, error) {
	roleIds = normalizeRoleIds(roleIds)
	if len(roleIds) == 0 {
		return nil, nil
	}

	current := common.GetCurrentUser(ctx)
	if current == nil {
		return nil, errs.NotLogin()
	}

	roles := services.RoleService.Find(sqls.NewCnd().In("id", roleIds).Eq("status", constants.StatusOk))
	if len(roles) != len(roleIds) {
		return nil, errors.New("角色不存在或已禁用")
	}

	if !current.IsOwner() {
		for _, role := range roles {
			if role.Code == constants.RoleOwner || role.Type == constants.RoleTypeSystem {
				return nil, errors.New("无权分配系统角色")
			}
		}
	}

	return roleIds, nil
}

func UserSynccount(ctx *gin.Context) {

	go func() {
		services.UserService.Scan(func(users []models.User) {
			for _, user := range users {
				topicCount := repositories.TopicRepository.Count(sqls.DB(), sqls.NewCnd().Eq("user_id", user.Id).Eq("status", constants.StatusOk))
				commentCount := repositories.CommentRepository.Count(sqls.DB(), sqls.NewCnd().Eq("user_id", user.Id).Eq("status", constants.StatusOk))
				_ = repositories.UserRepository.UpdateColumn(sqls.DB(), user.Id, "topic_count", topicCount)
				_ = repositories.UserRepository.UpdateColumn(sqls.DB(), user.Id, "comment_count", commentCount)
				cache.UserCache.Invalidate(user.Id)
			}
		})
	}()
	ginx.WriteJSON(ctx, nil)

}

func UserDetail(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	t := services.UserService.Get(id)
	if t == nil {
		ginx.WriteJSON(ctx, ginx.ErrorMessage("Not found, id="+strconv.FormatInt(id, 10)))
		return
	}
	ginx.WriteJSON(ctx, userBuildUserItem(t, true))

}

func UserList(ctx *gin.Context) {
	cnd := params.NewPagedSqlCnd(ctx,
		params.QueryFilter{
			ParamName: "id",
			Op:        params.Eq,
			ValueWrapper: func(origin string) string {
				if id := idcodec.Decode(origin); id > 0 {
					return cast.ToString(id)
				}
				return ""
			},
		},
		params.QueryFilter{
			ParamName: "nickname",
			Op:        params.Like,
		},
		params.QueryFilter{
			ParamName: "email",
			Op:        params.Eq,
		},
		params.QueryFilter{
			ParamName: "username",
			Op:        params.Eq,
		},
	)
	if forbiddenValue := params.QueryValue(ctx, "forbidden"); forbiddenValue != "" {
		now := dates.NowTimestamp()
		if cast.ToBool(forbiddenValue) {
			cnd.Where("(forbidden_end_time = ? OR forbidden_end_time > ?)", -1, now)
		} else {
			cnd.Where("(forbidden_end_time >= ? AND forbidden_end_time <= ?)", 0, now)
		}
	}
	list, paging := services.UserService.FindPageByCnd(cnd.Desc("id"))
	var itemList []map[string]interface{}
	for _, user := range list {
		itemList = append(itemList, userBuildUserItem(&user, false))
	}
	ginx.WriteJSON(ctx, &web.PageResult{Results: itemList, Page: paging})

}

func UserCreate(ctx *gin.Context) {
	var req modelReq.AdminUserCreateReq
	if err := ginx.Bind(ctx, &req); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	roleIds, err := checkAssignableRoles(ctx, modelReq.SplitCommaInt64s(req.RoleIds))
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	user, err := services.UserService.SignUp(req.Username, req.Email, req.Nickname, req.Password, req.Password)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	if len(roleIds) > 0 {
		if err := services.UserRoleService.UpdateUserRoles(user.Id, roleIds); err != nil {
			ginx.WriteJSON(ctx, ginx.ErrorMessage("用户已创建，但角色分配失败："+err.Error()))
			return
		}
		user = services.UserService.Get(user.Id)
	}
	ginx.WriteJSON(ctx, userBuildUserItem(user, true))

}

func UserUpdate(ctx *gin.Context) {
	var req modelReq.AdminUserUpdateReq
	if err := ginx.Bind(ctx, &req); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	user := services.UserService.Get(req.Id)
	if user == nil {
		ginx.WriteJSON(ctx, ginx.ErrorMessage("entity not found"))
		return
	}

	roleIds, err := checkAssignableRoles(ctx, modelReq.SplitCommaInt64s(req.RoleIds))
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	user.Username = sqls.SqlNullString(req.Username)
	user.Email = sqls.SqlNullString(req.Email)
	user.Nickname = req.Nickname
	user.Avatar = req.Avatar
	user.Gender = constants.Gender(req.Gender)
	user.HomePage = req.HomePage
	user.Description = req.Description
	user.Status = req.Status

	if err := services.UserService.Update(user); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	if err := services.UserRoleService.UpdateUserRoles(user.Id, roleIds); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	user = services.UserService.Get(user.Id)
	ginx.WriteJSON(ctx, userBuildUserItem(user, true))

}

func UserForbidden(ctx *gin.Context) {
	user := common.GetCurrentUser(ctx)
	if user == nil {
		ginx.WriteJSON(ctx, errs.NotLogin())
		return
	}
	var req modelReq.AdminUserForbiddenReq
	if err := ginx.Bind(ctx, &req); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	if !services.PermissionService.CanForbiddenUser(user, req.Days) {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("errors.no_permission")))
		return
	}
	if req.UserId < 0 {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.user_id_required")))
		return
	}
	if req.Days == 0 {
		services.UserService.RemoveForbidden(user.Id, req.UserId, ctx.Request)
	} else {
		if err := services.UserService.Forbidden(user.Id, req.UserId, req.Days, req.Reason, ctx.Request); err != nil {
			ginx.WriteJSON(ctx, err)
			return
		}
	}
	ginx.WriteJSON(ctx, nil)

}

func UserUpdatePassword(ctx *gin.Context) {
	user := common.GetCurrentUser(ctx)
	if user == nil {
		ginx.WriteJSON(ctx, errs.NotLogin())
		return
	}
	var req modelReq.PasswordUpdateReq
	if err := ginx.Bind(ctx, &req); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	if err := services.UserService.UpdatePassword(user.Id, req.OldPassword, req.Password, req.RePassword); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	ginx.WriteJSON(ctx, nil)

}

func UserResetPassword(ctx *gin.Context) {
	userId, _ := params.GetInt64(ctx, "userId")

	if userId <= 0 {
		ginx.WriteJSON(ctx, ginx.ErrorMessage("invalid param: userId"))
		return
	}

	newPassword, err := services.UserService.ResetPassword(userId)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	ginx.WriteJSON(ctx, map[string]interface{}{
		"password": newPassword,
	})

}
