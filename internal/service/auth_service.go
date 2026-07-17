package service

import (
	"errors"
	"strings"
	"time"

	"orbitterm-server/internal/identity"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

var (
	ErrInvalidInput       = errors.New("输入参数不合法")
	ErrUserAlreadyExists  = errors.New("用户名已存在")
	ErrInvalidCredential  = errors.New("用户名或密码错误")
	ErrAccountBanned      = errors.New("账号已被封禁")
	ErrAccountDeleted     = errors.New("账号已注销")
	ErrRegistrationClosed = errors.New("注册已关闭")
	ErrEmailDomainDenied  = errors.New("邮箱域名不允许注册")
	ErrWeakPassword       = errors.New("密码复杂度不足")
	ErrPasswordUnchanged  = errors.New("新密码不能与当前密码相同")
)

// AuthService 提供身份认证相关业务逻辑。
type AuthService interface {
	Register(username, password, inviteCode string) (*model.User, error)
	Login(username, password string) (*utils.TokenPair, error)
	Refresh(refreshToken string) (*utils.TokenPair, error)
	ChangePassword(userID uint, currentPassword, newPassword string) (*utils.TokenPair, error)
}

type authService struct {
	userRepo       repository.UserRepository
	jwtManager     *utils.JWTManager
	policyProvider SecurityPolicyProvider
	inviteService  RegistrationInviteConsumer
}

func NewAuthService(userRepo repository.UserRepository, jwtManager *utils.JWTManager, policyProvider SecurityPolicyProvider, inviteServices ...RegistrationInviteConsumer) AuthService {
	var inviteService RegistrationInviteConsumer
	if len(inviteServices) > 0 {
		inviteService = inviteServices[0]
	}
	return &authService{
		userRepo:       userRepo,
		jwtManager:     jwtManager,
		policyProvider: policyProvider,
		inviteService:  inviteService,
	}
}

// Register 注册新用户：
// 1) 参数校验；
// 2) 用户名唯一性检查；
// 3) Argon2id 密码哈希；
// 4) 创建用户记录。
func (s *authService) Register(username, password, inviteCode string) (*model.User, error) {
	policy, err := s.securityPolicy()
	if err != nil {
		return nil, err
	}
	if !policy.RegistrationEnabled {
		return nil, ErrRegistrationClosed
	}

	username = identity.CanonicalUsername(username)
	if !ValidateRegistrationEmail(username, policy.AllowedEmailDomains) {
		return nil, ErrEmailDomainDenied
	}
	if !ValidateStrongPassword(password, policy.MinPasswordLength) {
		return nil, ErrWeakPassword
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
	if policy.InvitationRequired {
		if s.inviteService == nil {
			return nil, ErrInviteRequired
		}
		if err := s.inviteService.Consume(inviteCode); err != nil {
			return nil, err
		}
	}

	user := &model.User{
		Username:     username,
		PasswordHash: hashed,
		Role:         policy.DefaultUserRole,
		Status:       policy.DefaultUserStatus,
	}
	if err := s.userRepo.Create(user); err != nil {
		if policy.InvitationRequired && s.inviteService != nil {
			_ = s.inviteService.Release(inviteCode)
		}
		return nil, err
	}
	return user, nil
}

func (s *authService) securityPolicy() (model.SecurityPolicy, error) {
	if s.policyProvider == nil {
		policy := model.DefaultSecurityPolicy()
		policy.Normalize()
		return policy, nil
	}
	policy, err := s.policyProvider.GetSecurityPolicy()
	if err != nil {
		return model.SecurityPolicy{}, err
	}
	policy.Normalize()
	return policy, nil
}

// Login 登录：
// 1) 根据用户名查询用户；
// 2) 校验 Argon2id 哈希；
// 3) 签发 JWT。
func (s *authService) Login(username, password string) (*utils.TokenPair, error) {
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
	if err := s.ensureUserCanAuthenticate(user); err != nil {
		return nil, err
	}

	matched, err := utils.VerifyPasswordArgon2ID(password, user.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, ErrInvalidCredential
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		return nil, err
	}
	return pair, nil
}

func (s *authService) Refresh(refreshToken string) (*utils.TokenPair, error) {
	token := strings.TrimSpace(refreshToken)
	if token == "" {
		return nil, ErrInvalidInput
	}

	claims, err := s.jwtManager.ParseAndVerifyRefreshToken(token)
	if err != nil {
		return nil, ErrInvalidCredential
	}

	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Username != claims.Username {
		return nil, ErrInvalidCredential
	}
	if claims.TokenVersion != user.TokenVersion {
		return nil, ErrInvalidCredential
	}
	if err := s.ensureUserCanAuthenticate(user); err != nil {
		return nil, err
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		return nil, err
	}
	return pair, nil
}

// ChangePassword verifies the current credential, applies the same policy as
// registration, and rotates TokenVersion. A freshly issued pair keeps only the
// caller signed in; every pre-change access and refresh token is invalidated.
func (s *authService) ChangePassword(userID uint, currentPassword, newPassword string) (*utils.TokenPair, error) {
	if userID == 0 || currentPassword == "" || newPassword == "" {
		return nil, ErrInvalidInput
	}

	policy, err := s.securityPolicy()
	if err != nil {
		return nil, err
	}
	if !ValidateStrongPassword(newPassword, policy.MinPasswordLength) {
		return nil, ErrWeakPassword
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredential
	}
	if err := s.ensureUserCanAuthenticate(user); err != nil {
		return nil, err
	}

	matched, err := utils.VerifyPasswordArgon2ID(currentPassword, user.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, ErrInvalidCredential
	}
	if currentPassword == newPassword {
		return nil, ErrPasswordUnchanged
	}

	hashed, err := utils.HashPasswordArgon2ID(newPassword)
	if err != nil {
		return nil, err
	}
	// Build the replacement before persisting the version change. Token signing
	// is side-effect free, so a failure cannot strand the caller without a token.
	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.TokenVersion+1)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = hashed
	user.TokenVersion++
	user.MustChangePassword = false
	if err := s.userRepo.Save(user); err != nil {
		return nil, err
	}
	return pair, nil
}

func (s *authService) ensureUserCanAuthenticate(user *model.User) error {
	now := time.Now().UTC()
	if user.ClearExpiredBan(now) {
		if err := s.userRepo.Save(user); err != nil {
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
