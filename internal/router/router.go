package router

import (
	"time"

	"orbitterm-server/internal/adminweb"
	"orbitterm-server/internal/controller"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"

	"github.com/gin-gonic/gin"
)

// Register 统一挂载 API 路由。
func Register(
	engine *gin.Engine,
	authController *controller.AuthController,
	configController *controller.ConfigController,
	adminController *controller.AdminController,
	jwtManager *utils.JWTManager,
	userRepo repository.UserRepository,
) {
	adminweb.Register(engine)

	v1 := engine.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authController.Register)
			auth.POST("/login", authController.Login)
			auth.POST("/refresh", authController.Refresh)
			auth.GET("/recovery-info", authController.RecoveryInfo)
		}

		configGroup := v1.Group("/config")
		configGroup.Use(middleware.JWTAuthMiddleware(jwtManager, userRepo))
		{
			configGroup.POST("/upload", configController.Upload)
			configGroup.GET("/pull", configController.Pull)
			configGroup.DELETE("/:id", configController.Delete)
		}

		adminPublic := v1.Group("/admin")
		{
			adminPublic.GET("/bootstrap/status", adminController.BootstrapStatus)
			adminPublic.POST("/bootstrap/super-admin", adminController.BootstrapSuperAdmin)
			adminPublic.POST("/auth/login", adminController.Login)
		}

		adminGroup := v1.Group("/admin")
		adminGroup.Use(
			middleware.IPRateLimit(180, time.Minute),
			middleware.JWTAuthMiddleware(jwtManager, userRepo),
			middleware.RequireAdminRole(),
		)
		{
			adminGroup.GET("/me", adminController.Me)
			adminGroup.GET("/dashboard/overview", adminController.DashboardOverview)
			adminGroup.GET("/audit-logs", adminController.AuditLogs)
			adminGroup.GET("/system/security-policy", adminController.GetSecurityPolicy)
			adminGroup.PUT(
				"/system/security-policy",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.UpdateSecurityPolicy,
			)
			adminGroup.GET("/system/recovery-policy", adminController.GetRecoveryPolicy)
			adminGroup.PUT(
				"/system/recovery-policy",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.UpdateRecoveryPolicy,
			)
			adminGroup.GET(
				"/system/backup-readiness",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.BackupReadiness,
			)
			adminGroup.GET(
				"/system/diagnostics",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.Diagnostics,
			)
			adminGroup.GET("/users", adminController.ListUsers)
			adminGroup.POST(
				"/users/managed",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin),
				adminController.CreateManagedUser,
			)
			adminGroup.POST(
				"/users/expired-bans/scan",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.ScanExpiredBans,
			)
			adminGroup.GET("/users/:id", adminController.GetUser)
			adminGroup.POST(
				"/users/:id/role",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin),
				adminController.UpdateUserRole,
			)
			adminGroup.POST(
				"/users/:id/ban",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.BanUser,
			)
			adminGroup.POST(
				"/users/:id/unban",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.UnbanUser,
			)
			adminGroup.POST(
				"/users/:id/reset-password",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.ResetPassword,
			)
			adminGroup.POST(
				"/users/:id/force-logout",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.ForceLogout,
			)
			adminGroup.POST(
				"/users/:id/soft-delete",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.SoftDeleteUser,
			)
			adminGroup.POST(
				"/users/:id/restore",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.RestoreUser,
			)
		}
	}
}
