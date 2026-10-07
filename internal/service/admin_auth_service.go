package service

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"orbitterm-server/internal/identity"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

var (
	ErrAdminAlreadyInitialized = errors.New("管理端已初始化")
	ErrAdminPermissionDenied   = errors.New("缺少管理端权限")
)

var adminRoles = []string{
	model.UserRoleSuperAdmin,
	model.UserRoleAdmin,
	model.UserRoleSupport,
}

type AdminBootstrapStatus struct {
	NeedsSetup bool  `json:"needs_setup"`
	AdminCount int64 `json:"admin_count"`
}

type AdminAuthService interface {
	BootstrapStatus() (*AdminBootstrapStatus, error)
	BootstrapSuperAdmin(username, password string, meta AdminRequestMeta) (*model.User, error)
	Login(username, password string, meta AdminRequestMeta) (*utils.TokenPair, error)
}

type adminAuthService struct {
	db             *gorm.DB
	userRepo       repository.UserRepository
	jwtManager     *utils.JWTManager
	auditService   AdminAuditService
	securityPolicy SecurityPolicyProvider
	now            func() time.Time
}

func NewAdminAuthService(userRepo repository.UserRepository, jwtManager *utils.JWTManager, auditService AdminAuditService) AdminAuthService {
	return NewAdminAuthServiceWithPolicy(userRepo, jwtManager, auditService, nil)
}

func NewAdminAuthServiceWithPolicy(userRepo repository.UserRepository, jwtManager *utils.JWTManager, auditService AdminAuditService, security SecurityPolicyProvider, dbs ...*gorm.DB) AdminAuthService {
	s := &adminAuthService{
		userRepo:       userRepo,
		jwtManager:     jwtManager,
		auditService:   auditService,
		securityPolicy: security,
		now:            func() time.Time { return time.Now().UTC() },
	}
	if len(dbs) > 0 {
		s.db = dbs[0]
	}
	return s
}

func (s *adminAuthService) BootstrapStatus() (*AdminBootstrapStatus, error) {
	count, err := s.userRepo.CountByRoles(adminRoles)
	if err != nil {
		return nil, err
	}
	return &AdminBootstrapStatus{
		NeedsSetup: count == 0,
		AdminCount: count,
	}, nil
}

func (s *adminAuthService) BootstrapSuperAdmin(username, password string, meta AdminRequestMeta) (*model.User, error) {
	if s.db != nil {
		var result *model.User
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if tx.Dialector.Name() == "postgres" {
				if err := tx.Exec("LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
					return err
				}
			}
			copy := *s
			copy.db = nil
			copy.userRepo = repository.NewUserRepository(tx)
			copy.auditService = NewAdminAuditService(repository.NewAdminAuditRepository(tx))
			var runErr error
			result, runErr = copy.BootstrapSuperAdmin(username, password, meta)
			return runErr
		})
		return result, err
	}
	username = identity.CanonicalUsername(username)
	if len(username) < 3 {
		return nil, ErrInvalidInput
	}
	policy := model.DefaultSecurityPolicy()
	if s.securityPolicy != nil {
		configured, err := s.securityPolicy.GetSecurityPolicy()
		if err != nil {
			return nil, err
		}
		policy = configured
	}
	policy.Normalize()
	if !ValidateStrongPassword(password, policy.MinPasswordLength) {
		return nil, ErrWeakPassword
	}

	status, err := s.BootstrapStatus()
	if err != nil {
		return nil, err
	}
	if !status.NeedsSetup {
		return nil, ErrAdminAlreadyInitialized
	}

	existed, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return nil, err
	}
	if existed != nil {
		return nil, ErrUserAlreadyExists
	}

	hashed, err := utils.HashPasswordArgon2ID(password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     username,
		PasswordHash: hashed,
		Role:         model.UserRoleSuperAdmin,
		Status:       model.UserStatusNormal,
	}
	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	if err := s.auditService.Record(AdminAuditEntry{
		AdminUserID:  user.ID,
		TargetUserID: &user.ID,
		Action:       model.AuditActionAdminBootstrap,
		ResourceType: "admin",
		ResourceID:   strconv.FormatUint(uint64(user.ID), 10),
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
		Reason:       "first super admin initialized",
	}); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *adminAuthService) Login(username, password string, meta AdminRequestMeta) (*utils.TokenPair, error) {
	username = identity.CanonicalUsername(username)
	if username == "" || password == "" {
		return nil, ErrInvalidInput
	}

	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredential
	}
	if !isAdminRole(user.Role) {
		return nil, ErrAdminPermissionDenied
	}
	if err := ensureUserCanAuthenticateForAdmin(user, s.userRepo, s.now()); err != nil {
		return nil, err
	}

	matched, err := utils.VerifyPasswordArgon2ID(password, user.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, ErrInvalidCredential
	}

	now := s.now()
	user.LastLoginAt = &now
	user.LastLoginIP = meta.IPAddress
	user.LastLoginUserAgent = meta.UserAgent
	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		return nil, err
	}

	_ = s.auditService.Record(AdminAuditEntry{
		AdminUserID:  user.ID,
		Action:       model.AuditActionAdminLogin,
		ResourceType: "admin",
		ResourceID:   strconv.FormatUint(uint64(user.ID), 10),
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	})
	return pair, nil
}

func isAdminRole(role string) bool {
	for _, candidate := range adminRoles {
		if role == candidate {
			return true
		}
	}
	return false
}

func ensureUserCanAuthenticateForAdmin(user *model.User, repo repository.UserRepository, now time.Time) error {
	if user.ClearExpiredBan(now) {
		if err := repo.Save(user); err != nil {
			return err
		}
	}
	if user.IsDeleted {
		return ErrAccountDeleted
	}
	if user.IsBanned {
		return ErrAccountBanned
	}
	return nil
}
