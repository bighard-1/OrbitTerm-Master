package repository

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/session"
)

func TestPostgresRefreshSessionRotationAndReplay(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.RefreshSession{}); err != nil {
		t.Fatal(err)
	}
	r := NewRefreshSessionRepository(db)
	family := "integration-family-" + time.Now().UTC().Format("20060102150405.000000000")
	defer db.Where("family_id = ?", family).Delete(&model.RefreshSession{})
	expires := time.Now().Add(time.Hour)
	if err := r.Create(900001, family, "old", 3, expires); err != nil {
		t.Fatal(err)
	}
	accepted, err := r.Rotate(900001, family, "old", "new", 3, expires)
	if err != nil || !accepted {
		t.Fatalf("first rotation: accepted=%v err=%v", accepted, err)
	}
	if err := db.Model(&model.RefreshSession{}).Where("family_id = ? AND token_digest = ?", family, digestTokenID("old")).Update("used_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	accepted, err = r.Rotate(900001, family, "old", "replay", 3, expires)
	if err != nil || accepted {
		t.Fatalf("replay: accepted=%v err=%v", accepted, err)
	}
	active, err := r.Active(900001, family, 3)
	if err != nil || active {
		t.Fatalf("replay did not revoke family: active=%v err=%v", active, err)
	}
}

func TestPostgresConcurrentRefreshDoesNotRevokeWinner(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.RefreshSession{}); err != nil {
		t.Fatal(err)
	}
	r := NewRefreshSessionRepository(db)
	family := "concurrent-family-" + time.Now().UTC().Format("20060102150405.000000000")
	defer db.Where("family_id = ?", family).Delete(&model.RefreshSession{})
	expires := time.Now().Add(time.Hour)
	if err := r.Create(900002, family, "old", 3, expires); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type outcome struct {
		accepted bool
		err      error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"new-a", "new-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			ok, err := r.Rotate(900002, family, "old", id, 3, expires)
			results <- outcome{ok, err}
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded := 0
	conflicted := 0
	for result := range results {
		if result.accepted {
			succeeded++
		} else if errors.Is(result.err, session.ErrConcurrentRefresh) {
			conflicted++
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("expected one successful rotation and one concurrency rejection, got %d and %d", succeeded, conflicted)
	}
	active, err := r.Active(900002, family, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("concurrent duplicate refresh revoked the winning token family")
	}
}

func TestPostgresConcurrentRefreshAndLogoutEndsFamily(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.RefreshSession{}); err != nil {
		t.Fatal(err)
	}
	r := NewRefreshSessionRepository(db)
	family := "logout-race-" + time.Now().UTC().Format("20060102150405.000000000")
	defer db.Where("family_id = ?", family).Delete(&model.RefreshSession{})
	expires := time.Now().Add(time.Hour)
	if err := r.Create(900003, family, "old", 3, expires); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errors := make(chan error, 2)
	go func() {
		<-start
		_, err := r.Rotate(900003, family, "old", "new", 3, expires)
		errors <- err
	}()
	go func() {
		<-start
		errors <- r.RevokeFamily(900003, family)
	}()
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	active, err := r.Active(900003, family, 3)
	if err != nil || active {
		t.Fatalf("logout left refresh family active: active=%v err=%v", active, err)
	}
}
