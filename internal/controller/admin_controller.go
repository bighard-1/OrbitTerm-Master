package controller

import (
	"orbitterm-server/internal/service"
)

type AdminController struct {
	auditService        service.AdminAuditService
	userService         service.AdminUserService
	adminAuthService    service.AdminAuthService
	securityPolicy      service.SecurityPolicyService
	recoveryPolicy      service.RecoveryPolicyService
	auditPolicy         service.AuditPolicyService
	assetDeletionPolicy service.AssetDeletionPolicyManager
	backupReadiness     service.BackupReadinessService
	dashboard           service.AdminDashboardService
	registrationInvites service.RegistrationInviteService
	migrationBundles    service.MigrationBundleService
	adminBootstrapToken string
}

func NewAdminController(
	auditService service.AdminAuditService,
	userService service.AdminUserService,
	adminAuthService service.AdminAuthService,
	securityPolicy service.SecurityPolicyService,
	recoveryPolicy service.RecoveryPolicyService,
	auditPolicy service.AuditPolicyService,
	assetDeletionPolicy service.AssetDeletionPolicyManager,
	backupReadiness service.BackupReadinessService,
	dashboard service.AdminDashboardService,
	registrationInvites service.RegistrationInviteService,
	migrationBundles service.MigrationBundleService,
	adminBootstrapToken string,
) *AdminController {
	return &AdminController{
		auditService:        auditService,
		userService:         userService,
		adminAuthService:    adminAuthService,
		securityPolicy:      securityPolicy,
		recoveryPolicy:      recoveryPolicy,
		auditPolicy:         auditPolicy,
		assetDeletionPolicy: assetDeletionPolicy,
		backupReadiness:     backupReadiness,
		dashboard:           dashboard,
		registrationInvites: registrationInvites,
		migrationBundles:    migrationBundles,
		adminBootstrapToken: adminBootstrapToken,
	}
}
