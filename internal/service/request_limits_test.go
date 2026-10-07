package service

import (
	"errors"
	"strings"
	"testing"
)

func TestUploadRejectsOversizeBeforeRepositoryWrite(t *testing.T) {
	_, err := normalizeConfigUploadInput(1, nil, "", "", make([]byte, MaxEncryptedBlobBytes+1), `{"device":1}`)
	if !errors.Is(err, ErrConfigTooLarge) {
		t.Fatalf("oversize ciphertext: %v", err)
	}
	_, err = normalizeConfigUploadInput(1, nil, "", "", []byte("cipher"), strings.Repeat("x", MaxVectorClockBytes+1))
	if !errors.Is(err, ErrConfigTooLarge) {
		t.Fatalf("oversize vector clock: %v", err)
	}
}
