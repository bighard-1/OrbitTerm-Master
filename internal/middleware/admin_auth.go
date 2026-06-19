package middleware

import (
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/model"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserRoleKey = "auth_user_role"
)

func RequireAdminRole(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[role] = struct{}{}
	}

	return func(ctx *gin.Context) {
		value, exists := ctx.Get(ContextUserRoleKey)
		if !exists {
			common.Error(ctx, http.StatusForbidden, "缺少管理端权限")
			ctx.Abort()
			return
		}

		role, ok := value.(string)
		if !ok || role == "" {
			common.Error(ctx, http.StatusForbidden, "管理端权限无效")
			ctx.Abort()
			return
		}

		if len(allowed) == 0 {
			if IsAdminRole(role) {
				ctx.Next()
				return
			}
		} else if _, ok := allowed[role]; ok {
			ctx.Next()
			return
		}

		common.Error(ctx, http.StatusForbidden, "没有执行该管理操作的权限")
		ctx.Abort()
	}
}

func IsAdminRole(role string) bool {
	switch role {
	case model.UserRoleSuperAdmin, model.UserRoleAdmin, model.UserRoleSupport:
		return true
	default:
		return false
	}
}
