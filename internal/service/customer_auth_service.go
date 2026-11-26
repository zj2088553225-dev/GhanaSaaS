package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/pkg/auth"

	"gorm.io/gorm"
)

const (
	customerMinPasswordLength = 8
	customerMaxPasswordBytes  = 72
	passwordResetTTL          = 30 * time.Minute
)

type customerRepository interface {
	Create(ctx context.Context, customer *models.Customer) error
	FindByID(ctx context.Context, id uint) (*models.Customer, error)
	FindByEmail(ctx context.Context, email string) (*models.Customer, error)
	UpdateLastLoginAt(ctx context.Context, id uint, lastLoginAt time.Time) error
}

type passwordResetRepository interface {
	CreatePasswordResetToken(ctx context.Context, resetToken *models.PasswordResetToken) error
	ResetPassword(ctx context.Context, tokenHash string, passwordHash string, now time.Time) error
}

type AuthIdentifierType string

const (
	AuthIdentifierEmail AuthIdentifierType = "email"
	AuthIdentifierPhone AuthIdentifierType = "phone"
)

// AuthIdentifier keeps authentication flows independent from any one login
// identifier. The first release registers only EmailLoginProvider.
type AuthIdentifier struct {
	Type  AuthIdentifierType
	Value string
}

type LoginProvider interface {
	IdentifierType() AuthIdentifierType
	Authenticate(ctx context.Context, identifier AuthIdentifier, password string) (*models.Customer, error)
}

type EmailLoginProvider struct {
	repo              customerRepository
	dummyPasswordHash string
}

func NewEmailLoginProvider(repo customerRepository) *EmailLoginProvider {
	dummyPasswordHash, _ := auth.HashPassword("unused-customer-auth-password")
	return &EmailLoginProvider{repo: repo, dummyPasswordHash: dummyPasswordHash}
}

func (p *EmailLoginProvider) IdentifierType() AuthIdentifierType {
	return AuthIdentifierEmail
}

func (p *EmailLoginProvider) Authenticate(
	ctx context.Context,
	identifier AuthIdentifier,
	password string,
) (*models.Customer, error) {
	if identifier.Type != AuthIdentifierEmail || p.repo == nil {
		return nil, ErrInvalidCredentials
	}

	customer, err := p.repo.FindByEmail(ctx, identifier.Value)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		auth.CheckPasswordHash(password, p.dummyPasswordHash)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if customer.Status != models.CustomerStatusActive {
		auth.CheckPasswordHash(password, customer.PasswordHash)
		return nil, ErrInvalidCredentials
	}
	if !auth.CheckPasswordHash(password, customer.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	return customer, nil
}

type CustomerAuthService struct {
	repo           customerRepository
	resetRepo      passwordResetRepository
	loginProviders map[AuthIdentifierType]LoginProvider
	jwtSecret      string
	expireDuration time.Duration
	now            func() time.Time
}

type RegisterCustomerInput struct {
	Email    string
	Password string
}

type CustomerLoginInput struct {
	Identifier AuthIdentifier
	Password   string
}

type CustomerProfile struct {
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CustomerAuthResult struct {
	Token string          `json:"token"`
	User  CustomerProfile `json:"user"`
}

func NewCustomerAuthService(
	repo customerRepository,
	jwtSecret string,
	expireDuration time.Duration,
	resetRepos ...passwordResetRepository,
) *CustomerAuthService {
	var resetRepo passwordResetRepository
	if len(resetRepos) > 0 {
		resetRepo = resetRepos[0]
	} else if compatibleRepo, ok := repo.(passwordResetRepository); ok {
		resetRepo = compatibleRepo
	}

	emailProvider := NewEmailLoginProvider(repo)
	return &CustomerAuthService{
		repo:           repo,
		resetRepo:      resetRepo,
		loginProviders: map[AuthIdentifierType]LoginProvider{AuthIdentifierEmail: emailProvider},
		jwtSecret:      jwtSecret,
		expireDuration: expireDuration,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

func (s *CustomerAuthService) Register(ctx context.Context, input RegisterCustomerInput) (*CustomerProfile, error) {
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}
	if err := ValidateCustomerPassword(input.Password); err != nil {
		return nil, err
	}

	if _, err := s.repo.FindByEmail(ctx, email); err == nil {
		return nil, ErrEmailExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	customer := models.Customer{
		Email:         email,
		PasswordHash:  passwordHash,
		Status:        models.CustomerStatusActive,
		EmailVerified: false,
	}
	if err := s.repo.Create(ctx, &customer); err != nil {
		if _, findErr := s.repo.FindByEmail(ctx, email); findErr == nil {
			return nil, ErrEmailExists
		}
		return nil, err
	}

	profile := toCustomerProfile(&customer)
	return &profile, nil
}

func (s *CustomerAuthService) Login(ctx context.Context, input CustomerLoginInput) (*CustomerAuthResult, error) {
	identifier, err := NormalizeAuthIdentifier(input.Identifier)
	if err != nil || input.Password == "" {
		return nil, ErrInvalidCredentials
	}

	provider, ok := s.loginProviders[identifier.Type]
	if !ok {
		return nil, ErrInvalidCredentials
	}
	customer, err := provider.Authenticate(ctx, identifier, input.Password)
	if err != nil {
		return nil, err
	}

	now := s.now()
	if err := s.repo.UpdateLastLoginAt(ctx, customer.ID, now); err != nil {
		return nil, err
	}
	customer.LastLoginAt = &now

	token, err := auth.GenerateCustomerTokenForCustomer(s.jwtSecret, s.expireDuration, customer)
	if err != nil {
		return nil, err
	}
	return &CustomerAuthResult{Token: token, User: toCustomerProfile(customer)}, nil
}

func (s *CustomerAuthService) RequestPasswordReset(ctx context.Context, emailInput string) (string, error) {
	email, validationErr := NormalizeEmail(emailInput)
	rawToken, tokenHash, err := newPasswordResetToken()
	if err != nil {
		return "", err
	}
	if validationErr != nil {
		return "", nil
	}

	customer, err := s.repo.FindByEmail(ctx, email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if customer.Status != models.CustomerStatusActive {
		return "", nil
	}
	if s.resetRepo == nil {
		return "", errors.New("password reset repository is not configured")
	}

	resetToken := &models.PasswordResetToken{
		UserID:    customer.ID,
		TokenHash: tokenHash,
		ExpiresAt: s.now().Add(passwordResetTTL),
	}
	if err := s.resetRepo.CreatePasswordResetToken(ctx, resetToken); err != nil {
		return "", err
	}
	return rawToken, nil
}

func (s *CustomerAuthService) ResetPassword(ctx context.Context, rawToken string, newPassword string) error {
	if err := ValidateCustomerPassword(newPassword); err != nil {
		return err
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || s.resetRepo == nil {
		return ErrInvalidResetToken
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	err = s.resetRepo.ResetPassword(ctx, hashResetToken(rawToken), passwordHash, s.now())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrInvalidResetToken
	}
	return err
}

func (s *CustomerAuthService) Me(ctx context.Context, customerID uint) (*CustomerProfile, error) {
	if customerID == 0 {
		return nil, ErrUnauthorized
	}

	customer, err := s.repo.FindByID(ctx, customerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if customer.Status != models.CustomerStatusActive {
		return nil, ErrUnauthorized
	}

	profile := toCustomerProfile(customer)
	return &profile, nil
}

func NormalizeEmail(input string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(input))
	if email == "" || len(email) > 254 {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || strings.Count(email, "@") != 1 {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func NormalizeAuthIdentifier(identifier AuthIdentifier) (AuthIdentifier, error) {
	switch identifier.Type {
	case AuthIdentifierEmail:
		email, err := NormalizeEmail(identifier.Value)
		if err != nil {
			return AuthIdentifier{}, err
		}
		return AuthIdentifier{Type: AuthIdentifierEmail, Value: email}, nil
	default:
		return AuthIdentifier{}, ErrInvalidInput
	}
}

func ValidateCustomerPassword(password string) error {
	if strings.TrimSpace(password) == "" ||
		utf8.RuneCountInString(password) < customerMinPasswordLength ||
		len([]byte(password)) > customerMaxPasswordBytes {
		return ErrWeakPassword
	}
	return nil
}

func newPasswordResetToken() (string, string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)
	return rawToken, hashResetToken(rawToken), nil
}

func hashResetToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func toCustomerProfile(customer *models.Customer) CustomerProfile {
	if customer == nil {
		return CustomerProfile{}
	}

	profile := CustomerProfile{
		ID:        customer.ID,
		Email:     customer.Email,
		CreatedAt: customer.CreatedAt,
	}
	if customer.Phone != nil {
		profile.Phone = *customer.Phone
	}
	return profile
}
