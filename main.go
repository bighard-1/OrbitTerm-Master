package main

import (
	"context"
	"log"
	"time"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/controller"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/router"
	"orbitterm-server/internal/service"
	"orbitterm-server/internal/utils"
	"orbitterm-server/internal/worker"

	"github.com/gin-gonic/gin"
)

// main 是 OrbitTerm-Server 的启动入口。
// 职责：
// 1) 加载运行配置；
// 2) 初始化数据库连接并执行迁移；
// 3) 初始化依赖（仓储层、服务层、控制器）；
// 4) 挂载路由并启动 HTTP 服务。
func main() {
	startedAt := time.Now().UTC()

	// 加载环境配置（端口、数据库、JWT 等）。
	cfg := config.Load()

	// 初始化数据库连接。
	db, err := config.NewDatabase(cfg)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 执行集中式兼容迁移；历史密文配置会保持 active，不需要服务端解密。
	if err := config.MigrateDatabase(db); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 安全组件初始化：JWT 签发器与校验器。
	jwtManager := utils.NewJWTManager(
		cfg.JWTSecret,
		cfg.JWTIssuer,
		cfg.JWTAccessExpireMinutes,
		cfg.JWTRefreshExpireDays,
		cfg.JWTExpireHours,
	)

	// 组装领域依赖。
	userRepo := repository.NewUserRepository(db)
	adminAuditRepo := repository.NewAdminAuditRepository(db)
	adminAuditService := service.NewAdminAuditService(adminAuditRepo)
	systemSettingRepo := repository.NewSystemSettingRepository(db)
	securityPolicyService := service.NewSecurityPolicyService(systemSettingRepo, adminAuditService)
	recoveryPolicyService := service.NewRecoveryPolicyService(systemSettingRepo, adminAuditService)
	auditPolicyService := service.NewAuditPolicyService(systemSettingRepo, adminAuditRepo, adminAuditService)
	backupReadinessService := service.NewBackupReadinessService(db, cfg, adminAuditService)
	authService := service.NewAuthService(userRepo, jwtManager, securityPolicyService)
	authController := controller.NewAuthController(authService, recoveryPolicyService)

	configRepo := repository.NewServerConfigRepository(db)
	configService := service.NewConfigService(configRepo)
	configController := controller.NewConfigController(configService)
	adminDashboardService := service.NewAdminDashboardService(userRepo, configRepo, adminAuditService, backupReadinessService)
	systemHealthService := service.NewSystemHealthService(db, cfg, startedAt)
	healthController := controller.NewHealthController(systemHealthService)

	adminAuthService := service.NewAdminAuthService(userRepo, jwtManager, adminAuditService)
	adminUserService := service.NewAdminUserService(userRepo, adminAuditService)
	adminController := controller.NewAdminController(
		adminAuditService,
		adminUserService,
		adminAuthService,
		securityPolicyService,
		recoveryPolicyService,
		auditPolicyService,
		backupReadinessService,
		adminDashboardService,
		cfg.AdminBootstrapToken,
	)

	// 创建 Gin 引擎并注册路由。
	engine := gin.Default()
	router.Register(engine, authController, configController, adminController, healthController, jwtManager, userRepo)
	worker.StartExpiredBanWorker(context.Background(), cfg, adminUserService)

	log.Printf("OrbitTerm-Server 启动成功，监听端口: %s", cfg.ServerPort)
	if err := engine.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
