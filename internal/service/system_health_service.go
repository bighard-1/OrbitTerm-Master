package service

import (
	"time"

	"orbitterm-server/internal/config"

	"gorm.io/gorm"
)

type SystemHealthService interface {
	PublicHealth() SystemHealthReport
	RuntimeStatus() SystemRuntimeReport
}

type SystemHealthReport struct {
	GeneratedAt   time.Time             `json:"generated_at"`
	Status        string                `json:"status"`
	UptimeSeconds int64                 `json:"uptime_seconds"`
	Database      DatabaseRuntimeStatus `json:"database"`
}

type SystemRuntimeReport struct {
	GeneratedAt   time.Time             `json:"generated_at"`
	Status        string                `json:"status"`
	UptimeSeconds int64                 `json:"uptime_seconds"`
	Database      DatabaseRuntimeStatus `json:"database"`
	JWT           JWTRuntimeStatus      `json:"jwt"`
	AutoUnban     AutoUnbanStatus       `json:"auto_unban"`
}

type DatabaseRuntimeStatus struct {
	Reachable       bool   `json:"reachable"`
	Dialect         string `json:"dialect,omitempty"`
	OpenConnections int    `json:"open_connections,omitempty"`
	InUse           int    `json:"in_use,omitempty"`
	Idle            int    `json:"idle,omitempty"`
	Error           string `json:"error,omitempty"`
}

type JWTRuntimeStatus struct {
	Issuer               string `json:"issuer"`
	AccessExpireMinutes  int    `json:"access_expire_minutes"`
	RefreshExpireDays    int    `json:"refresh_expire_days"`
	SecretStrengthStatus string `json:"secret_strength_status"`
}

type AutoUnbanStatus struct {
	Enabled                   bool `json:"enabled"`
	ConfiguredIntervalMinutes int  `json:"configured_interval_minutes"`
	EffectiveIntervalMinutes  int  `json:"effective_interval_minutes"`
	ConfiguredBatchLimit      int  `json:"configured_batch_limit"`
	EffectiveBatchLimit       int  `json:"effective_batch_limit"`
}

type systemHealthService struct {
	db        *gorm.DB
	cfg       config.Config
	startedAt time.Time
	now       func() time.Time
}

func NewSystemHealthService(db *gorm.DB, cfg config.Config, startedAt time.Time) SystemHealthService {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return &systemHealthService{
		db:        db,
		cfg:       cfg,
		startedAt: startedAt.UTC(),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *systemHealthService) PublicHealth() SystemHealthReport {
	now := s.now()
	database := s.databaseStatus()
	status := "ok"
	if !database.Reachable {
		status = "degraded"
	}
	return SystemHealthReport{
		GeneratedAt:   now,
		Status:        status,
		UptimeSeconds: int64(now.Sub(s.startedAt).Seconds()),
		Database:      database,
	}
}

func (s *systemHealthService) RuntimeStatus() SystemRuntimeReport {
	now := s.now()
	database := s.databaseStatus()
	status := "ok"
	if !database.Reachable {
		status = "degraded"
	}
	return SystemRuntimeReport{
		GeneratedAt:   now,
		Status:        status,
		UptimeSeconds: int64(now.Sub(s.startedAt).Seconds()),
		Database:      database,
		JWT:           s.jwtStatus(),
		AutoUnban:     s.autoUnbanStatus(),
	}
}

func (s *systemHealthService) databaseStatus() DatabaseRuntimeStatus {
	if s.db == nil {
		return DatabaseRuntimeStatus{Reachable: false, Error: "database handle is nil"}
	}
	status := DatabaseRuntimeStatus{Dialect: s.db.Dialector.Name()}
	if err := s.db.Exec("SELECT 1").Error; err != nil {
		status.Error = err.Error()
		return status
	}
	status.Reachable = true
	if sqlDB, err := s.db.DB(); err == nil && sqlDB != nil {
		stats := sqlDB.Stats()
		status.OpenConnections = stats.OpenConnections
		status.InUse = stats.InUse
		status.Idle = stats.Idle
	}
	return status
}

func (s *systemHealthService) jwtStatus() JWTRuntimeStatus {
	strength := "strong"
	if len(s.cfg.JWTSecret) < 32 || s.cfg.JWTSecret == "replace-this-with-a-strong-secret" {
		strength = "weak_or_default"
	}
	return JWTRuntimeStatus{
		Issuer:               s.cfg.JWTIssuer,
		AccessExpireMinutes:  s.cfg.JWTAccessExpireMinutes,
		RefreshExpireDays:    s.cfg.JWTRefreshExpireDays,
		SecretStrengthStatus: strength,
	}
}

func (s *systemHealthService) autoUnbanStatus() AutoUnbanStatus {
	interval := s.cfg.AdminAutoUnbanIntervalMinutes
	if interval < 1 {
		interval = 10
	}
	limit := s.cfg.AdminAutoUnbanBatchLimit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return AutoUnbanStatus{
		Enabled:                   s.cfg.AdminAutoUnbanEnabled,
		ConfiguredIntervalMinutes: s.cfg.AdminAutoUnbanIntervalMinutes,
		EffectiveIntervalMinutes:  interval,
		ConfiguredBatchLimit:      s.cfg.AdminAutoUnbanBatchLimit,
		EffectiveBatchLimit:       limit,
	}
}
