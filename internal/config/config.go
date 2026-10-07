package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config 统一管理服务运行时配置。
// 这些配置通常来自环境变量，便于在本地、测试、生产之间切换。
type Config struct {
	AppEnv                           string
	ServerPort                       string
	DatabaseURL                      string
	JWTSecret                        string
	JWTIssuer                        string
	JWTExpireHours                   int
	JWTAccessExpireMinutes           int
	JWTRefreshExpireDays             int
	AllowLegacyRefresh               bool
	BackupLastSuccessAt              string
	RestoreDrillVerifiedAt           string
	AdminBootstrapToken              string
	AdminAutoUnbanEnabled            bool
	AdminAutoUnbanIntervalMinutes    int
	AdminAutoUnbanBatchLimit         int
	AssetTrashCleanupIntervalMinutes int
	// ConfigCipherV2MigrationEnabled is deliberately opt-in. Clients that know
	// how to read V2 may otherwise begin their controlled migration as soon as
	// the route is deployed, which is too broad for a production rollout.
	ConfigCipherV2MigrationEnabled bool
	DatabaseLogLevel               string
	TrustedProxies                 []string
}

// Load 从环境变量读取配置并设置默认值。
// 注意：生产环境请务必通过安全方式注入 JWT_SECRET 与 DATABASE_URL。
func Load() Config {
	return Config{
		AppEnv:                           getEnv("APP_ENV", "production"),
		ServerPort:                       getEnv("SERVER_PORT", "8080"),
		DatabaseURL:                      getEnv("DATABASE_URL", "host=127.0.0.1 user=postgres password=postgres dbname=orbitterm port=5432 sslmode=disable TimeZone=UTC"),
		JWTSecret:                        getEnv("JWT_SECRET", "replace-this-with-a-strong-secret"),
		JWTIssuer:                        getEnv("JWT_ISSUER", "orbitterm-server"),
		JWTExpireHours:                   getEnvAsInt("JWT_EXPIRE_HOURS", 24),
		JWTAccessExpireMinutes:           getEnvAsInt("JWT_ACCESS_EXPIRE_MINUTES", 15),
		JWTRefreshExpireDays:             getEnvAsInt("JWT_REFRESH_EXPIRE_DAYS", 30),
		AllowLegacyRefresh:               getEnvAsBool("ALLOW_LEGACY_REFRESH_TOKENS", true),
		BackupLastSuccessAt:              getEnv("BACKUP_LAST_SUCCESS_AT", ""),
		RestoreDrillVerifiedAt:           getEnv("RESTORE_DRILL_VERIFIED_AT", ""),
		AdminBootstrapToken:              getEnv("ADMIN_BOOTSTRAP_TOKEN", ""),
		AdminAutoUnbanEnabled:            getEnvAsBool("ADMIN_AUTO_UNBAN_ENABLED", true),
		AdminAutoUnbanIntervalMinutes:    getEnvAsInt("ADMIN_AUTO_UNBAN_INTERVAL_MINUTES", 10),
		AdminAutoUnbanBatchLimit:         getEnvAsInt("ADMIN_AUTO_UNBAN_BATCH_LIMIT", 100),
		AssetTrashCleanupIntervalMinutes: getEnvAsInt("ASSET_TRASH_CLEANUP_INTERVAL_MINUTES", 60),
		ConfigCipherV2MigrationEnabled:   getEnvAsBool("CONFIG_CIPHER_V2_MIGRATION_ENABLED", false),
		DatabaseLogLevel:                 getEnv("DB_LOG_LEVEL", "warn"),
		TrustedProxies:                   getEnvAsCSV("TRUSTED_PROXIES", []string{"127.0.0.1", "::1"}),
	}
}

// Validate rejects demo credentials before production can connect to a database.
// Development must be selected explicitly in deployment templates; an unknown
// environment is never silently treated as development.
func (c Config) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.AppEnv)) {
	case "development", "test":
		return nil
	case "production":
	default:
		return fmt.Errorf("APP_ENV must be development, test, or production")
	}
	secret := strings.TrimSpace(c.JWTSecret)
	if len(secret) < 32 || uniqueCharacters(secret) < 16 || weakCredential(secret) {
		return fmt.Errorf("production JWT_SECRET must be a non-demo secret of at least 32 bytes")
	}
	password := databasePassword(c.DatabaseURL)
	lower := strings.ToLower(password)
	if len(password) < 16 || uniqueCharacters(password) < 8 || weakCredential(lower) {
		return fmt.Errorf("production DATABASE_URL requires a non-demo database password of at least 16 bytes")
	}
	if c.AdminBootstrapToken != "" && (len(c.AdminBootstrapToken) < 32 || uniqueCharacters(c.AdminBootstrapToken) < 16 || weakCredential(c.AdminBootstrapToken)) {
		return fmt.Errorf("production ADMIN_BOOTSTRAP_TOKEN must be empty or a non-demo token of at least 32 bytes")
	}
	return nil
}

func uniqueCharacters(value string) int {
	seen := make(map[rune]struct{})
	for _, char := range value {
		seen[char] = struct{}{}
	}
	return len(seen)
}

func weakCredential(value string) bool {
	lower := strings.ToLower(value)
	for _, fragment := range []string{"replace", "example", "password", "orbitterm_pass", "qwerty", "123456", "postgres"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func databasePassword(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		if u.User != nil {
			password, _ := u.User.Password()
			return password
		}
		return ""
	}
	for _, part := range strings.Fields(dsn) {
		if strings.HasPrefix(part, "password=") {
			return strings.Trim(strings.TrimPrefix(part, "password="), "'\"")
		}
	}
	return ""
}

// NewDatabase 创建 PostgreSQL 连接。
// Gorm 默认连接池已可满足多数场景，后续可根据压测结果进行细调。
func NewDatabase(cfg Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		// 生产环境默认不输出常规 SQL；即使临时开启 info，参数也会被占位符隐藏。
		// 需要临时排障时可显式设置 DB_LOG_LEVEL=info，排障结束后应立即恢复。
		Logger: newDatabaseLogger(cfg.DatabaseLogLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open postgres failed: %w", err)
	}
	return db, nil
}

func newDatabaseLogger(level string) logger.Interface {
	return logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  databaseLogLevel(level),
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  false,
		},
	)
}

func databaseLogLevel(value string) logger.LogLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "info":
		return logger.Info
	default:
		return logger.Warn
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := getEnv(key, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvAsBool(key string, fallback bool) bool {
	value := getEnv(key, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvAsCSV(key string, fallback []string) []string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return append([]string(nil), fallback...)
	}

	items := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
