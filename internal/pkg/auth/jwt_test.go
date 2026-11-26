package auth

import (
	"testing"
	"time"

	"saas/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndParseTokenRoundTrip(t *testing.T) {
	tenantID := uint(42)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 7},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}

	token, err := GenerateToken("test-secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	claims, err := ParseToken("test-secret", token)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}
	if claims.UserID != user.ID {
		t.Fatalf("UserID = %d, want %d", claims.UserID, user.ID)
	}
	if claims.TenantID == nil || *claims.TenantID != tenantID {
		t.Fatalf("TenantID = %v, want %d", claims.TenantID, tenantID)
	}
	if claims.Role != models.UserRoleTenantAdmin {
		t.Fatalf("Role = %q, want %q", claims.Role, models.UserRoleTenantAdmin)
	}
	if claims.TokenType != TokenTypeAdmin {
		t.Fatalf("TokenType = %q, want %q", claims.TokenType, TokenTypeAdmin)
	}
}

func TestGenerateTokenRejectsInvalidInput(t *testing.T) {
	user := &models.User{BaseModel: models.BaseModel{ID: 1}}

	cases := []struct {
		name     string
		secret   string
		duration time.Duration
		user     *models.User
	}{
		{name: "empty secret", secret: "", duration: time.Hour, user: user},
		{name: "non-positive duration", secret: "secret", duration: 0, user: user},
		{name: "nil user", secret: "secret", duration: time.Hour, user: nil},
		{name: "zero user id", secret: "secret", duration: time.Hour, user: &models.User{}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := GenerateToken(tt.secret, tt.duration, tt.user); err == nil {
				t.Fatal("GenerateToken returned nil error")
			}
		})
	}
}

func TestParseTokenRejectsInvalidToken(t *testing.T) {
	user := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := GenerateToken("correct-secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	cases := []struct {
		name   string
		secret string
		token  string
	}{
		{name: "empty secret", secret: "", token: token},
		{name: "empty token", secret: "correct-secret", token: ""},
		{name: "wrong secret", secret: "wrong-secret", token: token},
		{name: "malformed token", secret: "correct-secret", token: "not-a-token"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseToken(tt.secret, tt.token); err == nil {
				t.Fatal("ParseToken returned nil error")
			}
		})
	}
}

func TestGenerateAndParseCustomerTokenRoundTrip(t *testing.T) {
	token, err := GenerateCustomerToken("test-secret", time.Hour, 99)
	if err != nil {
		t.Fatalf("GenerateCustomerToken returned error: %v", err)
	}

	claims, err := ParseCustomerToken("test-secret", token)
	if err != nil {
		t.Fatalf("ParseCustomerToken returned error: %v", err)
	}
	if claims.CustomerID != 99 {
		t.Fatalf("CustomerID = %d, want 99", claims.CustomerID)
	}
	if claims.UserID != 99 {
		t.Fatalf("UserID = %d, want 99", claims.UserID)
	}
	if claims.Role != models.UserRoleCustomer {
		t.Fatalf("Role = %q, want %q", claims.Role, models.UserRoleCustomer)
	}
	if claims.TokenType != TokenTypeCustomer {
		t.Fatalf("TokenType = %q, want %q", claims.TokenType, TokenTypeCustomer)
	}
}

func TestCustomerTokenIncludesEmailUserIDAndRole(t *testing.T) {
	customer := &models.Customer{
		BaseModel: models.BaseModel{ID: 23},
		Email:     "buyer@example.com",
	}
	token, err := GenerateCustomerTokenForCustomer("test-secret", time.Hour, customer)
	if err != nil {
		t.Fatalf("GenerateCustomerTokenForCustomer returned error: %v", err)
	}
	claims, err := ParseCustomerToken("test-secret", token)
	if err != nil {
		t.Fatalf("ParseCustomerToken returned error: %v", err)
	}
	if claims.UserID != customer.ID ||
		claims.Email != customer.Email ||
		claims.Role != models.UserRoleCustomer {
		t.Fatalf("unexpected customer claims: %+v", claims)
	}
}

func TestAdminAndCustomerTokensAreNotInterchangeable(t *testing.T) {
	user := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	adminToken, err := GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	customerToken, err := GenerateCustomerToken("secret", time.Hour, 2)
	if err != nil {
		t.Fatalf("GenerateCustomerToken returned error: %v", err)
	}

	if _, err := ParseCustomerToken("secret", adminToken); err == nil {
		t.Fatal("ParseCustomerToken accepted an admin token")
	}
	if _, err := ParseToken("secret", customerToken); err == nil {
		t.Fatal("ParseToken accepted a customer token")
	}
}

func TestTokenParsersRejectNonHS256HMACAlgorithms(t *testing.T) {
	adminToken, err := jwt.NewWithClaims(jwt.SigningMethodHS512, Claims{
		UserID:    1,
		Role:      models.UserRoleSuperAdmin,
		TokenType: TokenTypeAdmin,
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign admin HS512 token: %v", err)
	}
	if _, err := ParseToken("secret", adminToken); err == nil {
		t.Fatal("ParseToken accepted HS512")
	}

	customerToken, err := jwt.NewWithClaims(jwt.SigningMethodHS512, CustomerClaims{
		UserID:     2,
		CustomerID: 2,
		Role:       models.UserRoleCustomer,
		TokenType:  TokenTypeCustomer,
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign customer HS512 token: %v", err)
	}
	if _, err := ParseCustomerToken("secret", customerToken); err == nil {
		t.Fatal("ParseCustomerToken accepted HS512")
	}
}

func TestHashPasswordAndCheckPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "correct-password" {
		t.Fatal("HashPassword returned the plain text password")
	}
	if !CheckPasswordHash("correct-password", hash) {
		t.Fatal("CheckPasswordHash rejected the correct password")
	}
	if CheckPasswordHash("wrong-password", hash) {
		t.Fatal("CheckPasswordHash accepted the wrong password")
	}
}

func TestHashPasswordRejectsEmptyPassword(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("HashPassword returned nil error")
	}
}
