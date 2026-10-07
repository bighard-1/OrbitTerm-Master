package repository

import (
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"orbitterm-server/internal/model"
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
	accepted, err = r.Rotate(900001, family, "old", "replay", 3, expires)
	if err != nil || accepted {
		t.Fatalf("replay: accepted=%v err=%v", accepted, err)
	}
	active, err := r.Active(900001, family, 3)
	if err != nil || active {
		t.Fatalf("replay did not revoke family: active=%v err=%v", active, err)
	}
}
