package service

import (
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"orbitterm-server/internal/config"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

func TestPostgresAdminResetRollsBackWhenAuditFails(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("maturity_audit_%d", time.Now().UnixNano())
	if err := root.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer root.Exec("DROP SCHEMA " + schema + " CASCADE")
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.MigrateDatabase(db); err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "admin@example.com", PasswordHash: "admin-hash", Role: model.UserRoleSuperAdmin, Status: model.UserStatusNormal}
	user := model.User{Username: "user@example.com", PasswordHash: "original-hash", Role: model.UserRoleUser, Status: model.UserStatusNormal, TokenVersion: 5}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAdminUserServiceWithPolicies(repository.NewUserRepository(db), NewAdminAuditService(repository.NewAdminAuditRepository(db)), nil, nil, db)
	if err := db.Exec("DROP TABLE admin_audit_logs").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResetPassword(admin.ID, user.ID, "StrongNewCredential123!", "requested reset", AdminRequestMeta{}); err == nil {
		t.Fatal("expected audit failure")
	}
	var stored model.User
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != "original-hash" || stored.TokenVersion != 5 || stored.MustChangePassword {
		t.Fatalf("mutation was not rolled back: %+v", stored)
	}
}
