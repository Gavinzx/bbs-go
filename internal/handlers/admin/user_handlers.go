package admin

import (
	"bbs-go/internal/cache"
	"bbs-go/internal/models/constants"
	modelReq "bbs-go/internal/models/req"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/errs"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/pkg/validate"
	"bbs-go/internal/pkg/xlsx"
	"bbs-go/internal/repositories"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"bbs-go/internal/models"

	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/pkg/params"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"github.com/mlogclub/simple/web"
	"github.com/spf13/cast"

	"bbs-go/internal/services"
	"log/slog"
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
		return nil, errors.New(locales.Get("admin.role_not_found_or_disabled"))
	}

	if !current.IsOwner() {
		for _, role := range roles {
			if role.Code == constants.RoleOwner || role.Type == constants.RoleTypeSystem {
				return nil, errors.New(locales.Get("admin.role_no_permission"))
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

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("user.username_required")))
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

	req.Username = strings.TrimSpace(req.Username)
	if req.Username != "" && req.Username != user.Username.String {
		if err := validate.IsUsername(req.Username); err != nil {
			ginx.WriteJSON(ctx, err)
			return
		}
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

const maxBatchCreateUsers = 100

type batchCreateUserError struct {
	Index    int    `json:"index"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

type batchCreateUserResult struct {
	Created    int                    `json:"created"`
	RoleFailed int                    `json:"roleFailed"`
	Failed     int                    `json:"failed"`
	Errors     []batchCreateUserError `json:"errors"`
}

// UserBatchCreate 批量创建用户（非事务：逐条创建并返回成功/失败明细）
func UserBatchCreate(ctx *gin.Context) {
	var req modelReq.AdminUserBatchCreateReq
	if err := ginx.BindJSON(ctx, &req); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	if len(req.Users) == 0 {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.user_batch_empty")))
		return
	}
	if len(req.Users) > maxBatchCreateUsers {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.user_batch_limit")))
		return
	}

	result := batchCreateUserResult{Errors: make([]batchCreateUserError, 0)}
	for index, item := range req.Users {
		item.Username = strings.TrimSpace(item.Username)
		if item.Username == "" {
			result.Failed++
			result.Errors = append(result.Errors, batchCreateUserError{
				Index:    index,
				Username: item.Username,
				Message:  locales.Get("admin.username_required"),
			})
			continue
		}

		roleIds, err := checkAssignableRoles(ctx, modelReq.SplitCommaInt64s(item.RoleIds))
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, batchCreateUserError{
				Index:    index,
				Username: item.Username,
				Message:  err.Error(),
			})
			continue
		}

		user, err := services.UserService.SignUp(item.Username, item.Email, item.Nickname, item.Password, item.Password)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, batchCreateUserError{
				Index:    index,
				Username: item.Username,
				Message:  err.Error(),
			})
			continue
		}

		if len(roleIds) > 0 {
			if err := services.UserRoleService.UpdateUserRoles(user.Id, roleIds); err != nil {
				slog.Error("batch create user role assignment failed", slog.Any("err", err), slog.String("username", item.Username))
				result.RoleFailed++
				result.Errors = append(result.Errors, batchCreateUserError{
					Index:    index,
					Username: item.Username,
					Message:  locales.Get("admin.user_role_assign_failed"),
				})
				continue
			}
		}
		result.Created++
	}

	ginx.WriteJSON(ctx, result)
}

const (
	maxImportFileSize = 5 << 20
	maxImportRows     = 1000
)

type importUserSuccess struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

type importUserError struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

type importUserResult struct {
	Created    int                 `json:"created"`
	RoleFailed int                 `json:"roleFailed"`
	Failed     int                 `json:"failed"`
	Success    []importUserSuccess `json:"success"`
	Partial    []importUserError   `json:"partial"`
	Errors     []importUserError   `json:"errors"`
}

// UserImportTemplate 下载 Excel 导入模板。
func UserImportTemplate(ctx *gin.Context) {
	data, err := xlsx.Write([][]string{{"用户名", "邮箱", "昵称", "密码"}})
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	ctx.Header("Content-Disposition", `attachment; filename="user-import-template.xlsx"`)
	ctx.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// UserImport 导入 Excel 创建用户（逐行创建，单行失败不影响其他行）。
func UserImport(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxImportFileSize+1<<20)
	if err := ctx.Request.ParseMultipartForm(maxImportFileSize); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_file_too_large")))
		} else {
			ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_file_invalid")))
		}
		return
	}

	fileHeader, err := ctx.FormFile("file")
	if err != nil || fileHeader == nil {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_file_required")))
		return
	}
	if strings.ToLower(filepath.Ext(fileHeader.Filename)) != ".xlsx" {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_file_type_invalid")))
		return
	}
	if fileHeader.Size > maxImportFileSize {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_file_too_large")))
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportFileSize+1))
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	if len(data) > maxImportFileSize {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_file_too_large")))
		return
	}

	var roleIds []int64
	if roleID := strings.TrimSpace(ctx.PostForm("roleId")); roleID != "" {
		id, err := strconv.ParseInt(roleID, 10, 64)
		if err != nil {
			ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.role_not_found_or_disabled")))
			return
		}
		roleIds = []int64{id}
	}
	roleIds, err = checkAssignableRoles(ctx, roleIds)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	defaultPassword := strings.TrimSpace(ctx.PostForm("defaultPassword"))

	rows, err := xlsx.ReadRows(data)
	if err != nil {
		slog.Error("parse import excel failed", slog.Any("err", err))
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_parse_failed")))
		return
	}

	headerIndex := -1
	for index, row := range rows {
		if !isBlankImportRow(row.Values) {
			headerIndex = index
			break
		}
	}
	if headerIndex < 0 {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_empty")))
		return
	}

	header := rows[headerIndex].Values
	if len(header) < 3 ||
		strings.TrimSpace(header[0]) != "用户名" ||
		strings.TrimSpace(header[1]) != "邮箱" ||
		strings.TrimSpace(header[2]) != "昵称" {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_header_invalid")))
		return
	}
	passwordColumn := -1
	for index, value := range header {
		if strings.TrimSpace(value) == "密码" {
			passwordColumn = index
			break
		}
	}

	dataRows := 0
	for _, row := range rows[headerIndex+1:] {
		if !isBlankImportRow(row.Values) {
			dataRows++
		}
	}
	if dataRows == 0 {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_empty")))
		return
	}
	if dataRows > maxImportRows {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("admin.import_row_limit")))
		return
	}

	result := importUserResult{
		Success: make([]importUserSuccess, 0),
		Partial: make([]importUserError, 0),
		Errors:  make([]importUserError, 0),
	}
	for index := headerIndex + 1; index < len(rows); index++ {
		row := rows[index]
		if isBlankImportRow(row.Values) {
			continue
		}
		rowNumber := row.Number
		username := importCell(row.Values, 0)
		email := importCell(row.Values, 1)
		nickname := importCell(row.Values, 2)
		password := ""
		if passwordColumn >= 0 {
			password = importCell(row.Values, passwordColumn)
		}
		if password == "" {
			password = defaultPassword
		}

		if username == "" {
			result.Failed++
			result.Errors = append(result.Errors, importUserError{Row: rowNumber, Username: username, Message: locales.Get("admin.username_required")})
			continue
		}
		if password == "" {
			result.Failed++
			result.Errors = append(result.Errors, importUserError{Row: rowNumber, Username: username, Message: locales.Get("admin.import_password_required")})
			continue
		}

		user, err := services.UserService.SignUp(username, email, nickname, password, password)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, importUserError{Row: rowNumber, Username: username, Message: err.Error()})
			continue
		}

		if len(roleIds) > 0 {
			if err := services.UserRoleService.UpdateUserRoles(user.Id, roleIds); err != nil {
				slog.Error("import user role assignment failed", slog.Any("err", err), slog.String("username", username))
				result.RoleFailed++
				result.Partial = append(result.Partial, importUserError{Row: rowNumber, Username: username, Message: locales.Get("admin.user_role_assign_failed")})
				continue
			}
		}

		result.Created++
		result.Success = append(result.Success, importUserSuccess{Row: rowNumber, Username: username, Email: email, Nickname: nickname})
	}

	ginx.WriteJSON(ctx, result)
}

func importCell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func isBlankImportRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
