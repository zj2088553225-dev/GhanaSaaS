package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type AuthService struct {
	userRepo       *repository.UserRepository
	jwtSecret      string
	expireDuration time.Duration
}

type LoginInput struct {
	Phone    string
	Password string
}

type AuthUser struct {
	ID       uint              `json:"id"`
	TenantID *uint             `json:"tenant_id,omitempty"`
	Role     models.UserRole   `json:"role"`
	Phone    string            `json:"phone"`
	Status   models.UserStatus `json:"status"`
}

type AuthResult struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

func NewAuthService(userRepo *repository.UserRepository, jwtSecret string, expireDuration time.Duration) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		jwtSecret:      jwtSecret,
		expireDuration: expireDuration,
	}
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*AuthResult, error) {
	input.Phone = strings.TrimSpace(input.Phone)
	if input.Phone == "" || input.Password == "" {
		return nil, ErrInvalidInput
	}

	user, err := s.userRepo.FindByPhoneWithRoles(
		ctx,
		input.Phone,
		models.UserRoleSuperAdmin,
		models.UserRoleTenantAdmin,
		models.UserRoleFinance,
		models.UserRolePicker,
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}

	return s.authenticateUser(input.Password, user)
}

func (s *AuthService) authenticateUser(password string, user *models.User) (*AuthResult, error) {
	if user.Status != models.UserStatusActive {
		return nil, ErrUnauthorized
	}
	if !auth.CheckPasswordHash(password, user.PasswordHash) {
		return nil, ErrUnauthorized
	}

	return s.authResult(user)
}

func (s *AuthService) authResult(user *models.User) (*AuthResult, error) {
	token, err := auth.GenerateToken(s.jwtSecret, s.expireDuration, user)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Token: token,
		User: AuthUser{
			ID:       user.ID,
			TenantID: user.TenantID,
			Role:     user.Role,
			Phone:    user.Phone,
			Status:   user.Status,
		},
	}, nil
}
