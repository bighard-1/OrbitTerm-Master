package config

import (
	"fmt"
	"log"
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
	ServerPort                       string
	DatabaseURL                      string
	JWTSecret                        string
	JWTIssuer                        string
	JWTExpireHours                   int
	JWTAccessExpireMinutes           int
	JWTRefreshExpireDays             int
	AdminBootstrapToken              string
	AdminAutoUnbanEnabled            bool
	AdminAutoUnbanIntervalMinutes    int
	AdminAutoUnbanBatchLimit         int
	AssetTrashCleanupIntervalMinutes int
	DatabaseLogLevel                 string
	TrustedProxies                   []string
}

// Load 从环境变量读取配置并设置默认值。
// 注意：生产环境请务必通过安全方式注入 JWT_SECRET 与 DATABASE_URL。
func Load() Config {
	return Config{
		ServerPort:                       getEnv("SERVER_PORT", "8080"),
		DatabaseURL:                      getEnv("DATABASE_URL", "host=127.0.0.1 user=postgres password=postgres dbname=orbitterm port=5432 sslmode=disable TimeZone=UTC"),
		JWTSecret:                        getEnv("JWT_SECRET", "replace-this-with-a-strong-secret"),
		JWTIssuer:                        getEnv("JWT_ISSUER", "orbitterm-server"),
		JWTExpireHours:                   getEnvAsInt("JWT_EXPIRE_HOURS", 24),
		JWTAccessExpireMinutes:           getEnvAsInt("JWT_ACCESS_EXPIRE_MINUTES", 15),
		JWTRefreshExpireDays:             getEnvAsInt("JWT_REFRESH_EXPIRE_DAYS", 30),
		AdminBootstrapToken:              getEnv("ADMIN_BOOTSTRAP_TOKEN", ""),
		AdminAutoUnbanEnabled:            getEnvAsBool("ADMIN_AUTO_UNBAN_ENABLED", true),
		AdminAutoUnbanIntervalMinutes:    getEnvAsInt("ADMIN_AUTO_UNBAN_INTERVAL_MINUTES", 10),
		AdminAutoUnbanBatchLimit:         getEnvAsInt("ADMIN_AUTO_UNBAN_BATCH_LIMIT", 100),
		AssetTrashCleanupIntervalMinutes: getEnvAsInt("ASSET_TRASH_CLEANUP_INTERVAL_MINUTES", 60),
		DatabaseLogLevel:                 getEnv("DB_LOG_LEVEL", "warn"),
		TrustedProxies:                   getEnvAsCSV("TRUSTED_PROXIES", []string{"127.0.0.1", "::1"}),
	}
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
