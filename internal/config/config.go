package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config 统一管理服务运行时配置。
// 这些配置通常来自环境变量，便于在本地、测试、生产之间切换。
type Config struct {
	ServerPort             string
	DatabaseURL            string
	JWTSecret              string
	JWTIssuer              string
	JWTExpireHours         int
	JWTAccessExpireMinutes int
	JWTRefreshExpireDays   int
}

// Load 从环境变量读取配置并设置默认值。
// 注意：生产环境请务必通过安全方式注入 JWT_SECRET 与 DATABASE_URL。
func Load() Config {
	return Config{
		ServerPort:             getEnv("SERVER_PORT", "8080"),
		DatabaseURL:            getEnv("DATABASE_URL", "host=127.0.0.1 user=postgres password=postgres dbname=orbitterm port=5432 sslmode=disable TimeZone=UTC"),
		JWTSecret:              getEnv("JWT_SECRET", "replace-this-with-a-strong-secret"),
		JWTIssuer:              getEnv("JWT_ISSUER", "orbitterm-server"),
		JWTExpireHours:         getEnvAsInt("JWT_EXPIRE_HOURS", 24),
		JWTAccessExpireMinutes: getEnvAsInt("JWT_ACCESS_EXPIRE_MINUTES", 15),
		JWTRefreshExpireDays:   getEnvAsInt("JWT_REFRESH_EXPIRE_DAYS", 30),
	}
}

// NewDatabase 创建 PostgreSQL 连接。
// Gorm 默认连接池已可满足多数场景，后续可根据压测结果进行细调。
func NewDatabase(cfg Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open postgres failed: %w", err)
	}
	return db, nil
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
