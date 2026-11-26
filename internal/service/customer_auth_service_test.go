package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/auth"

	"gorm.io/gorm"
)

type fakeCustomerRepository struct {
	customers   map[uint]models.Customer
	resetTokens map[uint]models.PasswordResetToken
	nextID      uint
	nextResetID uint
}

func newFakeCustomerRepository() *fakeCustomerRepository {
	return &fakeCustomerRepository{
		customers:   make(map[uint]models.Customer),
		resetTokens: make(map[uint]models.PasswordResetToken),
		nextID:      1,
		nextResetID: 1,
	}
}

func (r *fakeCustomerRepository) Create(_ context.Context, customer *models.Customer) error {
	customer.ID = r.nextID
	r.nextID++
	if customer.CreatedAt.IsZero() {
		customer.CreatedAt = time.Now().UTC()
	}
	customer.UpdatedAt = customer.CreatedAt
	r.customers[customer.ID] = cloneCustomer(*customer)
	return nil
}

func (r *fakeCustomerRepository) FindByID(_ context.Context, id uint) (*models.Customer, error) {
	customer, ok := r.customers[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := cloneCustomer(customer)
	return &cloned, nil
}

func (r *fakeCustomerRepository) FindByEmail(_ context.Context, email string) (*models.Customer, error) {
	for _, customer := range r.customers {
		if customer.Email == email {
			cloned := cloneCustomer(customer)
			return &cloned, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeCustomerRepository) UpdateLastLoginAt(_ context.Context, id uint, lastLoginAt time.Time) error {
	customer, ok := r.customers[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	customer.LastLoginAt = &lastLoginAt
	customer.UpdatedAt = lastLoginAt
	r.customers[id] = cloneCustomer(customer)
	return nil
}

func (r *fakeCustomerRepository) CreatePasswordResetToken(
	_ context.Context,
	resetToken *models.PasswordResetToken,
) error {
	resetToken.ID = r.nextResetID
	r.nextResetID++
	r.resetTokens[resetToken.ID] = *resetToken
	return nil
}

func (r *fakeCustomerRepository) ResetPassword(
	_ context.Context,
	tokenHash string,
	passwordHash string,
	now time.Time,
) error {
	for id, resetToken := range r.resetTokens {
		if resetToken.TokenHash != tokenHash ||
			resetToken.UsedAt != nil ||
			!resetToken.ExpiresAt.After(now) {
			continue
		}
		customer, ok := r.customers[resetToken.UserID]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		customer.PasswordHash = passwordHash
		customer.UpdatedAt = now
		r.customers[customer.ID] = customer
		resetToken.UsedAt = &now
		r.resetTokens[id] = resetToken
		return nil
	}
	return gorm.ErrRecordNotFound
}

func cloneCustomer(customer models.Customer) models.Customer {
	if customer.Phone != nil {
		phone := *customer.Phone
		customer.Phone = &phone
	}
	if customer.LastLoginAt != nil {
		lastLoginAt := *customer.LastLoginAt
		customer.LastLoginAt = &lastLoginAt
	}
	return customer
}

func newCustomerAuthTestService(repo *fakeCustomerRepository) *CustomerAuthService {
	return NewCustomerAuthService(repo, "secret", time.Hour, repo)
}

func TestCustomerAuthRegisterSuccessWithoutAutomaticLogin(t *testing.T) {
	repo := newFakeCustomerRepository()
	authService := newCustomerAuthTestService(repo)

	profile, err := authService.Register(context.Background(), RegisterCustomerInput{
		Email:    " TOM@example.COM ",
		Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if profile.Email != "tom@example.com" {
		t.Fatalf("email = %q, want lowercase normalized email", profile.Email)
	}
	stored := repo.customers[profile.ID]
	if stored.PasswordHash == "strong-password" || !auth.CheckPasswordHash("strong-password", stored.PasswordHash) {
		t.Fatal("password was not stored as a bcrypt hash")
	}

	payload, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if strings.Contains(string(payload), "token") || strings.Contains(string(payload), "password") {
		t.Fatalf("registration profile contains authentication secrets: %s", payload)
	}
}

func TestCustomerAuthRegisterRejectsDuplicateEmailIgnoringCase(t *testing.T) {
	authService := newCustomerAuthTestService(newFakeCustomerRepository())
	ctx := context.Background()
	if _, err := authService.Register(ctx, RegisterCustomerInput{
		Email: "tom@example.com", Password: "strong-password",
	}); err != nil {
		t.Fatalf("seed Register returned error: %v", err)
	}

	_, err := authService.Register(ctx, RegisterCustomerInput{
		Email: " TOM@EXAMPLE.COM ", Password: "another-password",
	})
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("Register duplicate email error = %v, want ErrEmailExists", err)
	}
}

func TestCustomerAuthRegisterRejectsInvalidEmailAndWeakPassword(t *testing.T) {
	authService := newCustomerAuthTestService(newFakeCustomerRepository())

	if _, err := authService.Register(context.Background(), RegisterCustomerInput{
		Email: "not-an-email", Password: "strong-password",
	}); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid email error = %v, want ErrInvalidEmail", err)
	}
	if _, err := authService.Register(context.Background(), RegisterCustomerInput{
		Email: "tom@example.com", Password: "short",
	}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak password error = %v, want ErrWeakPassword", err)
	}
	if _, err := authService.Register(context.Background(), RegisterCustomerInput{
		Email: "tom@example.com", Password: "        ",
	}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("blank password error = %v, want ErrWeakPassword", err)
	}
}

func TestCustomerAuthLoginSuccess(t *testing.T) {
	repo := newFakeCustomerRepository()
	authService := newCustomerAuthTestService(repo)
	ctx := context.Background()
	profile, err := authService.Register(ctx, RegisterCustomerInput{
		Email: "tom@example.com", Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	result, err := authService.Login(ctx, CustomerLoginInput{
		Identifier: AuthIdentifier{Type: AuthIdentifierEmail, Value: " TOM@EXAMPLE.COM "},
		Password:   "strong-password",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if result.Token == "" || result.User.ID != profile.ID || repo.customers[profile.ID].LastLoginAt == nil {
		t.Fatalf("unexpected login result: %+v", result)
	}
	claims, err := auth.ParseCustomerToken("secret", result.Token)
	if err != nil {
		t.Fatalf("ParseCustomerToken returned error: %v", err)
	}
	if claims.UserID != profile.ID || claims.Email != profile.Email || claims.Role != models.UserRoleCustomer {
		t.Fatalf("unexpected customer claims: %+v", claims)
	}
}

func TestCustomerAuthLoginUsesUnifiedFailureForWrongPasswordAndDisabledUser(t *testing.T) {
	repo := newFakeCustomerRepository()
	authService := newCustomerAuthTestService(repo)
	ctx := context.Background()
	profile, err := authService.Register(ctx, RegisterCustomerInput{
		Email: "disabled@example.com", Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	for _, password := range []string{"wrong-password"} {
		_, err := authService.Login(ctx, CustomerLoginInput{
			Identifier: AuthIdentifier{Type: AuthIdentifierEmail, Value: profile.Email},
			Password:   password,
		})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
		}
	}

	customer := repo.customers[profile.ID]
	customer.Status = models.CustomerStatusDisabled
	repo.customers[profile.ID] = customer
	_, err = authService.Login(ctx, CustomerLoginInput{
		Identifier: AuthIdentifier{Type: AuthIdentifierEmail, Value: profile.Email},
		Password:   "strong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user error = %v, want ErrInvalidCredentials", err)
	}
}

func TestCustomerAuthMeReturnsOnlyPublicProfile(t *testing.T) {
	authService := newCustomerAuthTestService(newFakeCustomerRepository())
	ctx := context.Background()
	registered, err := authService.Register(ctx, RegisterCustomerInput{
		Email: "tom@example.com", Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	profile, err := authService.Me(ctx, registered.ID)
	if err != nil {
		t.Fatalf("Me returned error: %v", err)
	}
	payload, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, forbidden := range []string{"password", "token", "status", "updated_at", "last_login_at"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("profile payload contains %q: %s", forbidden, payload)
		}
	}
}

func TestCustomerAuthPasswordResetSuccessAndSingleUse(t *testing.T) {
	repo := newFakeCustomerRepository()
	authService := newCustomerAuthTestService(repo)
	fixedNow := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	authService.now = func() time.Time { return fixedNow }
	ctx := context.Background()
	profile, err := authService.Register(ctx, RegisterCustomerInput{
		Email: "tom@example.com", Password: "old-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	rawToken, err := authService.RequestPasswordReset(ctx, " TOM@EXAMPLE.COM ")
	if err != nil {
		t.Fatalf("RequestPasswordReset returned error: %v", err)
	}
	if rawToken == "" || len(repo.resetTokens) != 1 {
		t.Fatal("password reset token was not generated")
	}
	for _, resetToken := range repo.resetTokens {
		if resetToken.TokenHash == rawToken || len(resetToken.TokenHash) != 64 {
			t.Fatal("reset token was stored in plaintext")
		}
		if !resetToken.ExpiresAt.Equal(fixedNow.Add(30 * time.Minute)) {
			t.Fatalf("expires_at = %v, want 30 minute TTL", resetToken.ExpiresAt)
		}
	}

	if err := authService.ResetPassword(ctx, rawToken, "new-password"); err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}
	if !auth.CheckPasswordHash("new-password", repo.customers[profile.ID].PasswordHash) {
		t.Fatal("new password was not persisted as bcrypt")
	}
	if err := authService.ResetPassword(ctx, rawToken, "third-password"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("second ResetPassword error = %v, want ErrInvalidResetToken", err)
	}
}

func TestCustomerAuthPasswordResetRejectsExpiredToken(t *testing.T) {
	repo := newFakeCustomerRepository()
	authService := newCustomerAuthTestService(repo)
	fixedNow := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	authService.now = func() time.Time { return fixedNow }
	ctx := context.Background()
	if _, err := authService.Register(ctx, RegisterCustomerInput{
		Email: "tom@example.com", Password: "old-password",
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	rawToken, err := authService.RequestPasswordReset(ctx, "tom@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset returned error: %v", err)
	}

	authService.now = func() time.Time { return fixedNow.Add(31 * time.Minute) }
	if err := authService.ResetPassword(ctx, rawToken, "new-password"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("expired ResetPassword error = %v, want ErrInvalidResetToken", err)
	}
}

func TestPasswordResetRequestDoesNotCreateTokenForUnknownEmail(t *testing.T) {
	repo := newFakeCustomerRepository()
	authService := newCustomerAuthTestService(repo)

	rawToken, err := authService.RequestPasswordReset(context.Background(), "missing@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset returned error: %v", err)
	}
	if rawToken != "" || len(repo.resetTokens) != 0 {
		t.Fatal("unknown email created or exposed a reset token")
	}
}
