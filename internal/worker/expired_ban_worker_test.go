package worker

import (
	"testing"

	"orbitterm-server/internal/service"
)

type fakeExpiredBanScanner struct {
	limit  int
	reason string
}

func (s *fakeExpiredBanScanner) ScanExpiredBansBySystem(limit int, reason string) (*service.AdminExpiredBanScanResult, error) {
	s.limit = limit
	s.reason = reason
	return &service.AdminExpiredBanScanResult{ScannedCount: 1, UnbannedCount: 1}, nil
}

func TestRunExpiredBanScan(t *testing.T) {
	scanner := &fakeExpiredBanScanner{}
	runExpiredBanScan(scanner, 88)
	if scanner.limit != 88 {
		t.Fatalf("expected limit 88, got %d", scanner.limit)
	}
	if scanner.reason == "" {
		t.Fatal("expected system reason")
	}
}
