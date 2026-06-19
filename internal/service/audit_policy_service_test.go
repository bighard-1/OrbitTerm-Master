package service

import (
	"testing"
	"time"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
)

func TestAuditPolicyUpdateNormalizesBounds(t *testing.T) {
	repo := newFakeSystemSettingRepo()
	audit := &fakeAdminAuditService{}
	svc := NewAuditPolicyService(repo, &fakeAdminAuditRepo{}, audit)

	retention := 1
	limit := 99999
	policy, err := svc.UpdateAuditPolicy(1, AuditPolicyUpdate{
		RetentionDays:     &retention,
		CleanupBatchLimit: &limit,
		Reason:            "更新审计策略",
	}, AdminRequestMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("update audit policy: %v", err)
	}
	if policy.RetentionDays != 30 {
		t.Fatalf("unexpected retention: %d", policy.RetentionDays)
	}
	if policy.CleanupBatchLimit != 5000 {
		t.Fatalf("unexpected cleanup limit: %d", policy.CleanupBatchLimit)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != model.AuditActionSystemAuditPolicyUpdate {
		t.Fatalf("missing policy audit entry: %+v", audit.entries)
	}
}

func TestAuditCleanupUsesPolicyCutoffAndRecordsAudit(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	repo := newFakeSystemSettingRepo()
	audit := &fakeAdminAuditService{}
	auditRepo := &fakeAdminAuditRepo{}
	svc := NewAuditPolicyService(repo, auditRepo, audit).(*auditPolicyService)
	svc.now = func() time.Time { return now }

	retention := 90
	limit := 250
	if _, err := svc.UpdateAuditPolicy(1, AuditPolicyUpdate{RetentionDays: &retention, CleanupBatchLimit: &limit, Reason: "设置保留周期"}, AdminRequestMeta{}); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	result, err := svc.CleanupExpiredAuditLogs(1, "例行清理", AdminRequestMeta{})
	if err != nil {
		t.Fatalf("cleanup audit logs: %v", err)
	}

	expectedCutoff := now.AddDate(0, 0, -90)
	if !auditRepo.cutoff.Equal(expectedCutoff) {
		t.Fatalf("unexpected cutoff: %s", auditRepo.cutoff)
	}
	if auditRepo.limit != 250 || result.BatchLimit != 250 {
		t.Fatalf("unexpected limit: repo=%d result=%d", auditRepo.limit, result.BatchLimit)
	}
	if result.DeletedCount != auditRepo.deleted {
		t.Fatalf("unexpected deleted count: %d", result.DeletedCount)
	}
	if len(audit.entries) != 2 || audit.entries[1].Action != model.AuditActionSystemAuditCleanup {
		t.Fatalf("missing cleanup audit entry: %+v", audit.entries)
	}
}

type fakeAdminAuditRepo struct {
	cutoff  time.Time
	limit   int
	deleted int64
}

func (r *fakeAdminAuditRepo) Create(_ *model.AdminAuditLog) error { return nil }

func (r *fakeAdminAuditRepo) List(_ int) ([]model.AdminAuditLog, error) { return nil, nil }

func (r *fakeAdminAuditRepo) ListWithFilter(_ repository.AdminAuditListFilter) ([]model.AdminAuditLog, int64, error) {
	return nil, 0, nil
}

func (r *fakeAdminAuditRepo) DeleteOlderThan(cutoff time.Time, limit int) (int64, error) {
	r.cutoff = cutoff
	r.limit = limit
	r.deleted = 7
	return r.deleted, nil
}
