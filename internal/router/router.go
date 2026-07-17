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
	healthController *controller.HealthController,
	jwtManager *utils.JWTManager,
	userRepo repository.UserRepository,
) {
	adminweb.Register(engine)
	engine.GET("/healthz", healthController.Health)

	v1 := engine.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", middleware.IPRateLimit(10, time.Hour), authController.Register)
			auth.POST("/login", middleware.IPRateLimit(30, time.Minute), authController.Login)
			auth.POST("/refresh", middleware.IPRateLimit(120, time.Minute), authController.Refresh)
			auth.GET("/recovery-info", middleware.IPRateLimit(60, time.Minute), authController.RecoveryInfo)
			auth.GET("/registration-policy", middleware.IPRateLimit(60, time.Minute), authController.RegistrationPolicy)
			auth.POST("/password", middleware.JWTAuthMiddleware(jwtManager, userRepo), middleware.IPRateLimit(10, time.Hour), authController.ChangePassword)
		}

		configGroup := v1.Group("/config")
		configGroup.Use(middleware.JWTAuthMiddleware(jwtManager, userRepo))
		{
			configGroup.POST("/upload", configController.Upload)
			configGroup.POST("/master-key/rotate", middleware.IPRateLimit(5, time.Hour), configController.RotateMasterKey)
			configGroup.GET("/pull", configController.Pull)
			configGroup.GET("/trash", configController.Trash)
			configGroup.GET("/sync/pull", configController.PullChanges)
			configGroup.POST("/sync/ack", configController.AcknowledgeSync)
			configGroup.GET("/identity-match", configController.IdentityMatches)
			configGroup.POST("/assets/:asset_id/delete", configController.DeleteAsset)
			configGroup.POST("/assets/:asset_id/restore", configController.RestoreAsset)
			configGroup.POST("/assets/:asset_id/purge", configController.PurgeAsset)
			configGroup.DELETE("/:id", configController.Delete)
		}

		adminPublic := v1.Group("/admin")
		{
			adminPublic.GET("/bootstrap/status", middleware.IPRateLimit(60, time.Minute), adminController.BootstrapStatus)
			adminPublic.POST("/bootstrap/super-admin", middleware.IPRateLimit(5, time.Hour), adminController.BootstrapSuperAdmin)
			adminPublic.POST("/auth/login", middleware.IPRateLimit(20, time.Minute), adminController.Login)
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
			adminGroup.GET(
				"/system/runtime",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				healthController.RuntimeStatus,
			)
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
			adminGroup.GET("/system/audit-policy", adminController.GetAuditPolicy)
			adminGroup.PUT(
				"/system/audit-policy",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.UpdateAuditPolicy,
			)
			adminGroup.GET("/system/asset-deletion-policy", adminController.GetAssetDeletionPolicy)
			adminGroup.PUT(
				"/system/asset-deletion-policy",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.UpdateAssetDeletionPolicy,
			)
			adminGroup.POST(
				"/system/asset-trash/cleanup",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.CleanupExpiredAssetTrash,
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
			adminGroup.POST(
				"/system/migration-bundle/export",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin),
				adminController.ExportMigrationBundle,
			)
			adminGroup.POST(
				"/system/migration-bundle/restore",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin),
				adminController.RestoreMigrationBundle,
			)
			adminGroup.GET("/users", adminController.ListUsers)
			adminGroup.GET(
				"/registration-invites",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.ListRegistrationInvites,
			)
			adminGroup.POST(
				"/registration-invites",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.CreateRegistrationInvite,
			)
			adminGroup.POST(
				"/registration-invites/:id/revoke",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.RevokeRegistrationInvite,
			)
			adminGroup.POST(
				"/audit-logs/cleanup",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.CleanupAuditLogs,
			)
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
			adminGroup.POST(
				"/users/force-logout-regular",
				middleware.RequireAdminRole(model.UserRoleSuperAdmin, model.UserRoleAdmin),
				adminController.ForceLogoutRegularUsers,
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
