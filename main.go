package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}
	log.Printf("OrbitTerm-Server 运行模式: %s", cfg.AppEnv)

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
	refreshSessionRepo := repository.NewRefreshSessionRepository(db)
	jwtManager.SetRefreshSessionStore(refreshSessionRepo, cfg.AllowLegacyRefresh)

	// 组装领域依赖。
	userRepo := repository.NewUserRepository(db)
	adminAuditRepo := repository.NewAdminAuditRepository(db)
	adminAuditService := service.NewAdminAuditService(adminAuditRepo)
	systemSettingRepo := repository.NewSystemSettingRepository(db)
	securityPolicyService := service.NewSecurityPolicyService(systemSettingRepo, adminAuditService, db)
	registrationInviteRepo := repository.NewRegistrationInviteRepository(db)
	registrationInviteService := service.NewRegistrationInviteService(registrationInviteRepo, adminAuditService, db)
	recoveryPolicyService := service.NewRecoveryPolicyService(systemSettingRepo, adminAuditService, db)
	auditPolicyService := service.NewAuditPolicyService(systemSettingRepo, adminAuditRepo, adminAuditService, db)
	backupReadinessService := service.NewBackupReadinessService(db, cfg, adminAuditService)
	migrationBundleService := service.NewMigrationBundleService(db, cfg, adminAuditService)
	authService := service.NewAuthService(userRepo, jwtManager, securityPolicyService, registrationInviteService)
	authController := controller.NewAuthController(authService, recoveryPolicyService, securityPolicyService)

	configRepo := repository.NewServerConfigRepository(db)
	assetDeletionPolicyService := service.NewAssetDeletionPolicyService(systemSettingRepo)
	assetDeletionPolicyManager := service.NewAssetDeletionPolicyManager(systemSettingRepo, configRepo, adminAuditService, db)
	configService := service.NewConfigService(configRepo, assetDeletionPolicyService)
	masterKeyRotationService := service.NewMasterKeyRotationService(configRepo, jwtManager)
	configCipherMigrationService := service.NewConfigCipherMigrationService(configRepo)
	configController := controller.NewConfigController(configService, masterKeyRotationService, configCipherMigrationService)
	adminDashboardService := service.NewAdminDashboardService(userRepo, configRepo, adminAuditService, backupReadinessService)
	systemHealthService := service.NewSystemHealthService(db, cfg, startedAt)
	healthController := controller.NewHealthController(systemHealthService)

	adminAuthService := service.NewAdminAuthServiceWithPolicy(userRepo, jwtManager, adminAuditService, securityPolicyService, db)
	adminUserService := service.NewAdminUserServiceWithPolicies(userRepo, adminAuditService, securityPolicyService, recoveryPolicyService, db)
	adminController := controller.NewAdminController(
		adminAuditService,
		adminUserService,
		adminAuthService,
		securityPolicyService,
		recoveryPolicyService,
		auditPolicyService,
		assetDeletionPolicyManager,
		backupReadinessService,
		adminDashboardService,
		registrationInviteService,
		migrationBundleService,
		cfg.AdminBootstrapToken,
	)

	// 创建 Gin 引擎并注册路由。
	engine := gin.Default()
	// Gin 默认信任全部代理，攻击者可伪造 X-Forwarded-For。
	// 这里只信任显式配置的 1Panel/反向代理地址；空列表表示不信任任何代理。
	if err := engine.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatalf("可信代理配置无效: %v", err)
	}
	router.Register(
		engine,
		authController,
		configController,
		adminController,
		healthController,
		jwtManager,
		userRepo,
		cfg.ConfigCipherV2MigrationEnabled,
	)
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	worker.StartExpiredBanWorker(workerCtx, cfg, adminUserService)
	worker.StartAssetTrashCleanupWorker(workerCtx, cfg, assetDeletionPolicyManager)
	worker.StartExpiredSessionWorker(workerCtx, refreshSessionRepo)

	log.Printf("OrbitTerm-Server 启动成功，监听端口: %s", cfg.ServerPort)
	server := &http.Server{Addr: ":" + cfg.ServerPort, Handler: engine, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: 10 * time.Minute, IdleTimeout: 60 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		log.Print("收到退出信号，停止接收新请求")
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
		return
	}
	stopWorkers()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 优雅退出失败: %v", err)
	}
}
