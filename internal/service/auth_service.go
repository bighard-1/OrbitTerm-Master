package service

import (
	"errors"
	"strings"

	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

var (
	ErrInvalidInput      = errors.New("输入参数不合法")
	ErrUserAlreadyExists = errors.New("用户名已存在")
	ErrInvalidCredential = errors.New("用户名或密码错误")
)

// AuthService 提供身份认证相关业务逻辑。
type AuthService interface {
	Register(username, password string) (*model.User, error)
	Login(username, password string) (*utils.TokenPair, error)
	Refresh(refreshToken string) (*utils.TokenPair, error)
}

type authService struct {
	userRepo   repository.UserRepository
	jwtManager *utils.JWTManager
}

func NewAuthService(userRepo repository.UserRepository, jwtManager *utils.JWTManager) AuthService {
	return &authService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
	}
}

// Register 注册新用户：
// 1) 参数校验；
// 2) 用户名唯一性检查；
// 3) Argon2id 密码哈希；
// 4) 创建用户记录。
func (s *authService) Register(username, password string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(password) < 8 {
		return nil, ErrInvalidInput
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
	}
	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login 登录：
// 1) 根据用户名查询用户；
// 2) 校验 Argon2id 哈希；
// 3) 签发 JWT。
func (s *authService) Login(username, password string) (*utils.TokenPair, error) {
	username = strings.TrimSpace(username)
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

	matched, err := utils.VerifyPasswordArgon2ID(password, user.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, ErrInvalidCredential
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username)
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

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username)
	if err != nil {
		return nil, err
	}
	return pair, nil
}
