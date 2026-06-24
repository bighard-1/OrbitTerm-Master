package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"

	"golang.org/x/crypto/scrypt"
	"gorm.io/gorm"
)

const (
	migrationBundleSchemaVersion = 1
	migrationBundleMagic         = "OTMB1"
	migrationBundleMaxBytes      = 256 << 20
)

var (
	ErrMigrationPassphrase = errors.New("migration passphrase does not meet policy")
	ErrMigrationBundle     = errors.New("migration bundle is invalid")
)

type MigrationBundleService interface {
	Export(adminID uint, passphrase string, meta AdminRequestMeta) ([]byte, MigrationBundleSummary, error)
	Restore(adminID uint, encrypted []byte, passphrase string, meta AdminRequestMeta) (MigrationRestoreResult, error)
}

type MigrationBundleSummary struct {
	SchemaVersion int            `json:"schema_version"`
	CreatedAt     time.Time      `json:"created_at"`
	TableCounts   map[string]int `json:"table_counts"`
	Environment   []string       `json:"environment_keys"`
}

type MigrationRestoreResult struct {
	SchemaVersion             int            `json:"schema_version"`
	RestoredAt                time.Time      `json:"restored_at"`
	TableCounts               map[string]int `json:"table_counts"`
	RuntimeEnvironmentApplied bool           `json:"runtime_environment_applied"`
	RestartRequired           bool           `json:"restart_required"`
	EnvironmentKeys           []string       `json:"environment_keys"`
	Message                   string         `json:"message"`
}

type migrationBundleService struct {
	db           *gorm.DB
	cfg          config.Config
	auditService AdminAuditService
	now          func() time.Time
}

func NewMigrationBundleService(db *gorm.DB, cfg config.Config, audit AdminAuditService) MigrationBundleService {
	return &migrationBundleService{db: db, cfg: cfg, auditService: audit, now: func() time.Time { return time.Now().UTC() }}
}

type migrationBundle struct {
	SchemaVersion int                  `json:"schema_version"`
	CreatedAt     time.Time            `json:"created_at"`
	Environment   migrationEnvironment `json:"environment"`
	Data          migrationData        `json:"data"`
}

type migrationEnvironment struct {
	ServerPort                       string   `json:"server_port"`
	DatabaseURL                      string   `json:"database_url"`
	JWTSecret                        string   `json:"jwt_secret"`
	JWTIssuer                        string   `json:"jwt_issuer"`
	JWTAccessExpireMinutes           int      `json:"jwt_access_expire_minutes"`
	JWTRefreshExpireDays             int      `json:"jwt_refresh_expire_days"`
	AdminBootstrapToken              string   `json:"admin_bootstrap_token"`
	AdminAutoUnbanEnabled            bool     `json:"admin_auto_unban_enabled"`
	AdminAutoUnbanIntervalMinutes    int      `json:"admin_auto_unban_interval_minutes"`
	AdminAutoUnbanBatchLimit         int      `json:"admin_auto_unban_batch_limit"`
	AssetTrashCleanupIntervalMinutes int      `json:"asset_trash_cleanup_interval_minutes"`
	DatabaseLogLevel                 string   `json:"database_log_level"`
	TrustedProxies                   []string `json:"trusted_proxies"`
}

type migrationData struct {
	Users             []migrationUser            `json:"users"`
	ServerConfigs     []model.ServerConfig       `json:"server_configs"`
	ConfigChanges     []model.ConfigSyncChange   `json:"config_sync_changes"`
	SyncDeviceStates  []model.SyncDeviceState    `json:"sync_device_states"`
	AdminAuditLogs    []model.AdminAuditLog      `json:"admin_audit_logs"`
	SystemSettings    []model.SystemSetting      `json:"system_settings"`
	RegistrationCodes []model.RegistrationInvite `json:"registration_invites"`
}

type migrationUser struct {
	ID                 uint       `json:"id"`
	Username           string     `json:"username"`
	PasswordHash       string     `json:"password_hash"`
	Role               string     `json:"role"`
	Status             string     `json:"status"`
	IsBanned           bool       `json:"is_banned"`
	BanUntil           *time.Time `json:"ban_until,omitempty"`
	BanReason          string     `json:"ban_reason,omitempty"`
	BannedAt           *time.Time `json:"banned_at,omitempty"`
	BannedBy           *uint      `json:"banned_by,omitempty"`
	IsDeleted          bool       `json:"is_deleted"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	MustChangePassword bool       `json:"must_change_password"`
	TokenVersion       int64      `json:"token_version"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP        string     `json:"last_login_ip,omitempty"`
	LastLoginUserAgent string     `json:"last_login_user_agent,omitempty"`
	CreatedBy          *uint      `json:"created_by,omitempty"`
	UpdatedBy          *uint      `json:"updated_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (s *migrationBundleService) Export(adminID uint, passphrase string, meta AdminRequestMeta) ([]byte, MigrationBundleSummary, error) {
	if adminID == 0 || !validMigrationPassphrase(passphrase) {
		return nil, MigrationBundleSummary{}, ErrMigrationPassphrase
	}
	bundle, err := s.snapshot()
	if err != nil {
		return nil, MigrationBundleSummary{}, err
	}
	plain, err := json.Marshal(bundle)
	if err != nil {
		return nil, MigrationBundleSummary{}, err
	}
	if len(plain) > migrationBundleMaxBytes {
		return nil, MigrationBundleSummary{}, fmt.Errorf("%w: snapshot exceeds size limit", ErrMigrationBundle)
	}
	encrypted, err := encryptMigrationBundle(plain, passphrase)
	if err != nil {
		return nil, MigrationBundleSummary{}, err
	}
	summary := summarizeMigrationBundle(bundle)
	_ = s.auditService.Record(AdminAuditEntry{AdminUserID: adminID, Action: model.AuditActionMigrationBundleExport, ResourceType: "migration_bundle", ResourceID: fmt.Sprintf("schema:%d", summary.SchemaVersion), IPAddress: meta.IPAddress, UserAgent: meta.UserAgent, Reason: "导出加密全量迁移包"})
	return encrypted, summary, nil
}

func (s *migrationBundleService) Restore(adminID uint, encrypted []byte, passphrase string, meta AdminRequestMeta) (MigrationRestoreResult, error) {
	if adminID == 0 || !validMigrationPassphrase(passphrase) || len(encrypted) > migrationBundleMaxBytes+(1<<20) {
		return MigrationRestoreResult{}, ErrMigrationBundle
	}
	plain, err := decryptMigrationBundle(encrypted, passphrase)
	if err != nil {
		return MigrationRestoreResult{}, ErrMigrationBundle
	}
	var bundle migrationBundle
	if err := json.Unmarshal(plain, &bundle); err != nil || !validMigrationBundle(bundle) {
		return MigrationRestoreResult{}, ErrMigrationBundle
	}
	if err := s.restoreData(bundle.Data); err != nil {
		return MigrationRestoreResult{}, err
	}
	summary := summarizeMigrationBundle(bundle)
	_ = s.auditService.Record(AdminAuditEntry{AdminUserID: adminID, Action: model.AuditActionMigrationBundleRestore, ResourceType: "migration_bundle", ResourceID: fmt.Sprintf("schema:%d", summary.SchemaVersion), IPAddress: meta.IPAddress, UserAgent: meta.UserAgent, Reason: "覆盖恢复加密全量迁移包"})
	return MigrationRestoreResult{
		SchemaVersion: summary.SchemaVersion, RestoredAt: s.now(), TableCounts: summary.TableCounts,
		RuntimeEnvironmentApplied: false, RestartRequired: true, EnvironmentKeys: summary.Environment,
		Message: "数据库已在单个事务内恢复。运行环境变量不能由容器内进程自行改写，请在部署平台核对迁移包对应参数并重启服务。",
	}, nil
}

func (s *migrationBundleService) snapshot() (migrationBundle, error) {
	data := migrationData{}
	var users []model.User
	queries := []struct {
		name string
		dest any
	}{
		{"users", &users}, {"server_configs", &data.ServerConfigs}, {"config_sync_changes", &data.ConfigChanges},
		{"sync_device_states", &data.SyncDeviceStates}, {"admin_audit_logs", &data.AdminAuditLogs},
		{"system_settings", &data.SystemSettings}, {"registration_invites", &data.RegistrationCodes},
	}
	for _, query := range queries {
		if err := s.db.Order("id ASC").Find(query.dest).Error; err != nil {
			return migrationBundle{}, fmt.Errorf("snapshot %s: %w", query.name, err)
		}
	}
	data.Users = make([]migrationUser, 0, len(users))
	for _, user := range users {
		data.Users = append(data.Users, migrationUserFromModel(user))
	}
	return migrationBundle{SchemaVersion: migrationBundleSchemaVersion, CreatedAt: s.now(), Environment: s.environmentSnapshot(), Data: data}, nil
}

func (s *migrationBundleService) restoreData(data migrationData) error {
	users := make([]model.User, 0, len(data.Users))
	for _, record := range data.Users {
		users = append(users, record.model())
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"sync_device_states", "config_sync_changes", "server_configs", "admin_audit_logs", "registration_invites", "system_settings", "users"} {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		if err := createBatch(tx, &users); err != nil {
			return err
		}
		if err := createBatch(tx.Omit("User"), &data.ServerConfigs); err != nil {
			return err
		}
		if err := createBatch(tx, &data.ConfigChanges); err != nil {
			return err
		}
		if err := createBatch(tx, &data.SyncDeviceStates); err != nil {
			return err
		}
		if err := createBatch(tx, &data.AdminAuditLogs); err != nil {
			return err
		}
		if err := createBatch(tx, &data.SystemSettings); err != nil {
			return err
		}
		if err := createBatch(tx, &data.RegistrationCodes); err != nil {
			return err
		}
		if tx.Dialector.Name() == "postgres" {
			for _, table := range []string{"users", "server_configs", "config_sync_changes", "sync_device_states", "admin_audit_logs", "system_settings", "registration_invites"} {
				statement := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s','id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM %s", table, table)
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("reset %s sequence: %w", table, err)
				}
			}
		}
		return nil
	})
}

func createBatch(tx *gorm.DB, value any) error {
	ref := reflect.ValueOf(value)
	if ref.Kind() == reflect.Ptr {
		ref = ref.Elem()
	}
	if ref.Kind() == reflect.Slice && ref.Len() == 0 {
		return nil
	}
	return tx.CreateInBatches(value, 200).Error
}

func validMigrationBundle(bundle migrationBundle) bool {
	if bundle.SchemaVersion != migrationBundleSchemaVersion || bundle.CreatedAt.IsZero() || len(bundle.Data.Users) == 0 {
		return false
	}
	for _, user := range bundle.Data.Users {
		if user.ID == 0 || strings.TrimSpace(user.Username) == "" || strings.TrimSpace(user.PasswordHash) == "" {
			return false
		}
		if user.Role == model.UserRoleSuperAdmin || user.Role == model.UserRoleAdmin {
			return true
		}
	}
	return false
}

func validMigrationPassphrase(passphrase string) bool {
	return utf8.RuneCountInString(passphrase) >= 16 && len(passphrase) <= 512 && strings.TrimSpace(passphrase) == passphrase
}

func encryptMigrationBundle(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	result := append([]byte(migrationBundleMagic), salt...)
	result = append(result, nonce...)
	return gcm.Seal(result, nonce, plain, []byte(migrationBundleMagic)), nil
}

func decryptMigrationBundle(encrypted []byte, passphrase string) ([]byte, error) {
	header := len(migrationBundleMagic) + 16 + 12
	if len(encrypted) < header || string(encrypted[:len(migrationBundleMagic)]) != migrationBundleMagic {
		return nil, ErrMigrationBundle
	}
	saltStart := len(migrationBundleMagic)
	salt := encrypted[saltStart : saltStart+16]
	nonce := encrypted[saltStart+16 : header]
	key, err := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, encrypted[header:], []byte(migrationBundleMagic))
}

func (s *migrationBundleService) environmentSnapshot() migrationEnvironment {
	return migrationEnvironment{ServerPort: s.cfg.ServerPort, DatabaseURL: s.cfg.DatabaseURL, JWTSecret: s.cfg.JWTSecret, JWTIssuer: s.cfg.JWTIssuer, JWTAccessExpireMinutes: s.cfg.JWTAccessExpireMinutes, JWTRefreshExpireDays: s.cfg.JWTRefreshExpireDays, AdminBootstrapToken: s.cfg.AdminBootstrapToken, AdminAutoUnbanEnabled: s.cfg.AdminAutoUnbanEnabled, AdminAutoUnbanIntervalMinutes: s.cfg.AdminAutoUnbanIntervalMinutes, AdminAutoUnbanBatchLimit: s.cfg.AdminAutoUnbanBatchLimit, AssetTrashCleanupIntervalMinutes: s.cfg.AssetTrashCleanupIntervalMinutes, DatabaseLogLevel: s.cfg.DatabaseLogLevel, TrustedProxies: append([]string(nil), s.cfg.TrustedProxies...)}
}

func summarizeMigrationBundle(bundle migrationBundle) MigrationBundleSummary {
	data := bundle.Data
	return MigrationBundleSummary{SchemaVersion: bundle.SchemaVersion, CreatedAt: bundle.CreatedAt, TableCounts: map[string]int{"users": len(data.Users), "server_configs": len(data.ServerConfigs), "config_sync_changes": len(data.ConfigChanges), "sync_device_states": len(data.SyncDeviceStates), "admin_audit_logs": len(data.AdminAuditLogs), "system_settings": len(data.SystemSettings), "registration_invites": len(data.RegistrationCodes)}, Environment: []string{"SERVER_PORT", "DATABASE_URL", "JWT_SECRET", "JWT_ISSUER", "JWT_ACCESS_EXPIRE_MINUTES", "JWT_REFRESH_EXPIRE_DAYS", "ADMIN_BOOTSTRAP_TOKEN", "ADMIN_AUTO_UNBAN_ENABLED", "ADMIN_AUTO_UNBAN_INTERVAL_MINUTES", "ADMIN_AUTO_UNBAN_BATCH_LIMIT", "ASSET_TRASH_CLEANUP_INTERVAL_MINUTES", "DB_LOG_LEVEL", "TRUSTED_PROXIES"}}
}

func migrationUserFromModel(user model.User) migrationUser {
	return migrationUser{ID: user.ID, Username: user.Username, PasswordHash: user.PasswordHash, Role: user.Role, Status: user.Status, IsBanned: user.IsBanned, BanUntil: user.BanUntil, BanReason: user.BanReason, BannedAt: user.BannedAt, BannedBy: user.BannedBy, IsDeleted: user.IsDeleted, DeletedAt: user.DeletedAt, MustChangePassword: user.MustChangePassword, TokenVersion: user.TokenVersion, LastLoginAt: user.LastLoginAt, LastLoginIP: user.LastLoginIP, LastLoginUserAgent: user.LastLoginUserAgent, CreatedBy: user.CreatedBy, UpdatedBy: user.UpdatedBy, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
}

func (u migrationUser) model() model.User {
	return model.User{ID: u.ID, Username: u.Username, PasswordHash: u.PasswordHash, Role: u.Role, Status: u.Status, IsBanned: u.IsBanned, BanUntil: u.BanUntil, BanReason: u.BanReason, BannedAt: u.BannedAt, BannedBy: u.BannedBy, IsDeleted: u.IsDeleted, DeletedAt: u.DeletedAt, MustChangePassword: u.MustChangePassword, TokenVersion: u.TokenVersion, LastLoginAt: u.LastLoginAt, LastLoginIP: u.LastLoginIP, LastLoginUserAgent: u.LastLoginUserAgent, CreatedBy: u.CreatedBy, UpdatedBy: u.UpdatedBy, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}
