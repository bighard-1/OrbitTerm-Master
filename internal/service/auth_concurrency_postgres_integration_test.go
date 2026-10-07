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
	"orbitterm-server/internal/utils"
)

type blockingRefreshCreate struct {
	utils.RefreshSessionStore
	started chan struct{}
	release chan struct{}
}

func (s *blockingRefreshCreate) Create(uint, string, string, int64, time.Time) error {
	close(s.started)
	<-s.release
	return nil
}
func (s *blockingRefreshCreate) RevokeFamily(uint, string) error { return nil }

func TestPostgresPasswordChangeCannotUndoLogoutAll(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("maturity_auth_race_%d", time.Now().UnixNano())
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
	oldPassword := "ExistingStrongCredential123!"
	hash, err := utils.HashPasswordArgon2ID(oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "race@example.com", PasswordHash: hash, Role: model.UserRoleUser, Status: model.UserStatusNormal, TokenVersion: 5}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	jwt := utils.NewJWTManager("a-long-random-test-key-for-race-0123456789", "test", 15, 30, 24)
	gate := &blockingRefreshCreate{started: make(chan struct{}), release: make(chan struct{})}
	jwt.SetRefreshSessionStore(gate, true)
	svc := NewAuthService(repository.NewUserRepository(db), jwt, nil)
	changed := make(chan error, 1)
	go func() { _, err := svc.ChangePassword(user.ID, oldPassword, "NewStrongCredential123!"); changed <- err }()
	select {
	case <-gate.started:
	case <-time.After(10 * time.Second):
		t.Fatal("password change did not reach token issuance")
	}
	if err := svc.LogoutAll(user.ID); err != nil {
		t.Fatal(err)
	}
	close(gate.release)
	changeErr := <-changed
	var stored model.User
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if changeErr == nil || stored.TokenVersion != 6 {
		t.Fatalf("concurrent password change should conflict without undoing logout-all: err=%v version=%d", changeErr, stored.TokenVersion)
	}
}
