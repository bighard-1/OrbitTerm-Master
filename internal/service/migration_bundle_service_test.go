package service

import (
	"bytes"
	"testing"
	"time"

	"orbitterm-server/internal/model"
)

func TestMigrationBundleEncryptionRoundTrip(t *testing.T) {
	plain := []byte(`{"schema_version":1,"secret":"not-plaintext"}`)
	encrypted, err := encryptMigrationBundle(plain, "Correct-Horse-123!")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(encrypted, []byte("not-plaintext")) {
		t.Fatal("encrypted bundle leaked plaintext")
	}
	decrypted, err := decryptMigrationBundle(encrypted, "Correct-Horse-123!")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("round trip mismatch: %q", decrypted)
	}
	if _, err := decryptMigrationBundle(encrypted, "Wrong-Passphrase-1!"); err == nil {
		t.Fatal("wrong passphrase must fail")
	}
}

func TestMigrationBundleValidationRequiresAdministrator(t *testing.T) {
	bundle := migrationBundle{SchemaVersion: migrationBundleSchemaVersion, CreatedAt: time.Now().UTC(), Data: migrationData{Users: []migrationUser{{ID: 1, Username: "user@gmail.com", PasswordHash: "$argon2id$hash", Role: model.UserRoleUser}}}}
	if validMigrationBundle(bundle) {
		t.Fatal("bundle without administrator must be rejected")
	}
	bundle.Data.Users[0].Role = model.UserRoleSuperAdmin
	if !validMigrationBundle(bundle) {
		t.Fatal("bundle with super administrator should be accepted")
	}
}

func TestMigrationPassphrasePolicy(t *testing.T) {
	for _, value := range []string{"short", " leading-passphrase-123!", "trailing-passphrase-123! "} {
		if validMigrationPassphrase(value) {
			t.Fatalf("expected passphrase rejection: %q", value)
		}
	}
	if !validMigrationPassphrase("Strong-Migration-123!") {
		t.Fatal("expected passphrase acceptance")
	}
}
