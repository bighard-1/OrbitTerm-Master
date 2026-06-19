package service

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"

	"gorm.io/gorm"
)

type BackupReadinessService interface {
	GetReadiness(adminID uint, meta AdminRequestMeta) (BackupReadinessReport, error)
}

type BackupReadinessReport struct {
	GeneratedAt       time.Time            `json:"generated_at"`
	Ready             bool                 `json:"ready"`
	Database          DatabaseBackupStatus `json:"database"`
	Environment       []EnvironmentCheck   `json:"environment"`
	RecommendedItems  []BackupItem         `json:"recommended_items"`
	OperationalGuides []string             `json:"operational_guides"`
	Warnings          []string             `json:"warnings,omitempty"`
}

type DatabaseBackupStatus struct {
	Reachable    bool             `json:"reachable"`
	Dialect      string           `json:"dialect"`
	TableCounts  map[string]int64 `json:"table_counts"`
	BackupMethod string           `json:"backup_method"`
	Hint         string           `json:"hint"`
}

type EnvironmentCheck struct {
	Key         string `json:"key"`
	Configured  bool   `json:"configured"`
	Secure      bool   `json:"secure"`
	MaskedValue string `json:"masked_value,omitempty"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
}

type BackupItem struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type backupReadinessService struct {
	db           *gorm.DB
	cfg          config.Config
	auditService AdminAuditService
	now          func() time.Time
}

func NewBackupReadinessService(db *gorm.DB, cfg config.Config, auditService AdminAuditService) BackupReadinessService {
	return &backupReadinessService{
		db:           db,
		cfg:          cfg,
		auditService: auditService,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *backupReadinessService) GetReadiness(adminID uint, meta AdminRequestMeta) (BackupReadinessReport, error) {
	if adminID == 0 {
		return BackupReadinessReport{}, ErrInvalidInput
	}

	database, warnings, err := s.databaseStatus()
	if err != nil {
		return BackupReadinessReport{}, err
	}
	environment := s.environmentChecks()
	for _, check := range environment {
		if check.Severity != "ok" && check.Severity != "info" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", check.Key, check.Message))
		}
	}

	report := BackupReadinessReport{
		GeneratedAt:      s.now(),
		Ready:            database.Reachable && len(warnings) == 0,
		Database:         database,
		Environment:      environment,
		RecommendedItems: recommendedBackupItems(),
		OperationalGuides: []string{
			"在 1Panel 中优先使用 PostgreSQL 容器/数据库备份能力导出数据库快照。",
			"环境变量快照只应脱敏保存；JWT_SECRET、数据库密码、ADMIN_BOOTSTRAP_TOKEN 不应明文写入普通日志或工单。",
			"恢复时先还原数据库，再注入同一套 JWT_SECRET 与数据库连接变量，最后重启 orbit-api 容器。",
		},
		Warnings: warnings,
	}

	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:  adminID,
		Action:       model.AuditActionSystemBackupReadinessView,
		ResourceType: "system",
		ResourceID:   "backup_readiness",
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	})
	return report, nil
}

func (s *backupReadinessService) databaseStatus() (DatabaseBackupStatus, []string, error) {
	warnings := make([]string, 0)
	counts := map[string]int64{}
	tables := map[string]any{
		"users":            &model.User{},
		"server_configs":   &model.ServerConfig{},
		"admin_audit_logs": &model.AdminAuditLog{},
		"system_settings":  &model.SystemSetting{},
	}

	for table, modelRef := range tables {
		var count int64
		if err := s.db.Model(modelRef).Count(&count).Error; err != nil {
			warnings = append(warnings, fmt.Sprintf("表 %s 统计失败: %v", table, err))
			continue
		}
		counts[table] = count
	}

	reachable := len(warnings) == 0
	return DatabaseBackupStatus{
		Reachable:    reachable,
		Dialect:      s.db.Dialector.Name(),
		TableCounts:  counts,
		BackupMethod: "pg_dump / 1Panel PostgreSQL backup",
		Hint:         "建议在数据库容器内执行 pg_dump，或使用 1Panel 数据库备份功能；不要只备份 orbit-api 容器。",
	}, warnings, nil
}

func (s *backupReadinessService) environmentChecks() []EnvironmentCheck {
	return []EnvironmentCheck{
		checkSecret("JWT_SECRET", s.cfg.JWTSecret, "replace-this-with-a-strong-secret", 32),
		checkPlain("JWT_ISSUER", s.cfg.JWTIssuer, "JWT 签发者标识。"),
		checkNumber("JWT_ACCESS_EXPIRE_MINUTES", s.cfg.JWTAccessExpireMinutes, s.cfg.JWTAccessExpireMinutes > 0 && s.cfg.JWTAccessExpireMinutes <= 60, "Access Token 建议保持短周期。"),
		checkNumber("JWT_REFRESH_EXPIRE_DAYS", s.cfg.JWTRefreshExpireDays, s.cfg.JWTRefreshExpireDays > 0 && s.cfg.JWTRefreshExpireDays <= 90, "Refresh Token 建议设置为有限周期。"),
		checkDSN("DATABASE_URL", s.cfg.DatabaseURL),
		checkOptionalBootstrapToken(s.cfg.AdminBootstrapToken),
		checkPlain("ADMIN_AUTO_UNBAN_ENABLED", fmt.Sprintf("%t", s.cfg.AdminAutoUnbanEnabled), "到期封禁自动解封任务开关。"),
		checkNumber("ADMIN_AUTO_UNBAN_INTERVAL_MINUTES", s.cfg.AdminAutoUnbanIntervalMinutes, s.cfg.AdminAutoUnbanIntervalMinutes >= 1, "到期封禁自动解封扫描间隔，建议不低于 1 分钟。"),
		checkNumber("ADMIN_AUTO_UNBAN_BATCH_LIMIT", s.cfg.AdminAutoUnbanBatchLimit, s.cfg.AdminAutoUnbanBatchLimit > 0 && s.cfg.AdminAutoUnbanBatchLimit <= 500, "单次自动解封扫描的最大处理数量。"),
	}
}

func recommendedBackupItems() []BackupItem {
	return []BackupItem{
		{Name: "PostgreSQL 数据库快照", Required: true, Description: "包含用户、云端密文配置、审计日志与系统策略。"},
		{Name: "脱敏环境变量快照", Required: true, Description: "记录变量是否存在与脱敏形态，密钥原文需由管理员在安全密码库保存。"},
		{Name: "后端镜像版本号", Required: true, Description: "记录 GHCR 镜像 tag/digest，便于回滚到一致版本。"},
		{Name: "1Panel 反向代理与域名配置", Required: true, Description: "包含 HTTPS 证书来源、server.orbitterm.com 指向与容器端口映射。"},
	}
}

func checkPlain(key, value, message string) EnvironmentCheck {
	value = strings.TrimSpace(value)
	return EnvironmentCheck{
		Key:         key,
		Configured:  value != "",
		Secure:      value != "",
		MaskedValue: value,
		Severity:    severity(value != ""),
		Message:     message,
	}
}

func checkNumber(key string, value int, secure bool, message string) EnvironmentCheck {
	return EnvironmentCheck{
		Key:         key,
		Configured:  value > 0,
		Secure:      secure,
		MaskedValue: fmt.Sprintf("%d", value),
		Severity:    severity(value > 0 && secure),
		Message:     message,
	}
}

func checkSecret(key, value, insecureDefault string, minLength int) EnvironmentCheck {
	trimmed := strings.TrimSpace(value)
	configured := trimmed != ""
	secure := configured && len(trimmed) >= minLength && (insecureDefault == "" || trimmed != insecureDefault)
	message := "已配置。"
	if !configured {
		message = "未配置，生产环境必须设置。"
	} else if !secure {
		message = "已配置但强度不足或仍为默认演示值，请轮换为高强度随机值。"
	}
	return EnvironmentCheck{
		Key:         key,
		Configured:  configured,
		Secure:      secure,
		MaskedValue: maskSecret(trimmed),
		Severity:    severity(secure),
		Message:     message,
	}
}

func checkOptionalBootstrapToken(value string) EnvironmentCheck {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return EnvironmentCheck{
			Key:        "ADMIN_BOOTSTRAP_TOKEN",
			Configured: false,
			Secure:     true,
			Severity:   "info",
			Message:    "未配置。若首个管理员已创建，这是推荐状态；仅首次初始化前需要临时配置。",
		}
	}

	check := checkSecret("ADMIN_BOOTSTRAP_TOKEN", trimmed, "", 32)
	if check.Secure {
		check.Message = "已配置。若管理端已初始化，建议轮换或清空后重启。"
	}
	return check
}

func checkDSN(key, raw string) EnvironmentCheck {
	trimmed := strings.TrimSpace(raw)
	configured := trimmed != ""
	secure := configured && !strings.Contains(trimmed, "password=postgres") && !strings.Contains(trimmed, "password=orbitterm_pass")
	message := "数据库连接已配置。"
	if !configured {
		message = "未配置数据库连接。"
	} else if !secure {
		message = "数据库连接疑似使用默认密码，请更换。"
	}
	return EnvironmentCheck{
		Key:         key,
		Configured:  configured,
		Secure:      secure,
		MaskedValue: maskDatabaseURL(trimmed),
		Severity:    severity(secure),
		Message:     message,
	}
}

func severity(ok bool) string {
	if ok {
		return "ok"
	}
	return "warning"
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func maskDatabaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" {
		if parsed.User != nil {
			username := parsed.User.Username()
			if _, ok := parsed.User.Password(); ok {
				parsed.User = url.UserPassword(username, "******")
			}
		}
		return parsed.String()
	}

	parts := strings.Fields(raw)
	for i, part := range parts {
		if strings.HasPrefix(part, "password=") {
			parts[i] = "password=******"
		}
	}
	return strings.Join(parts, " ")
}
