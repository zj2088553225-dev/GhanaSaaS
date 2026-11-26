package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"saas/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	TokenTypeAdmin    = "admin"
	TokenTypeCustomer = "customer"
)

type Claims struct {
	UserID    uint            `json:"user_id"`
	TenantID  *uint           `json:"tenant_id,omitempty"`
	Role      models.UserRole `json:"role"`
	TokenType string          `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

type CustomerClaims struct {
	UserID     uint            `json:"user_id"`
	CustomerID uint            `json:"customer_id"`
	Email      string          `json:"email"`
	Role       models.UserRole `json:"role"`
	TokenType  string          `json:"token_type"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, expireDuration time.Duration, user *models.User) (string, error) {
	if secret == "" || expireDuration <= 0 || user == nil || user.ID == 0 {
		return "", errors.New("invalid token input")
	}

	now := time.Now()
	claims := Claims{
		UserID:    user.ID,
		TenantID:  user.TenantID,
		Role:      user.Role,
		TokenType: TokenTypeAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expireDuration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(secret string, tokenString string) (*Claims, error) {
	if secret == "" || tokenString == "" {
		return nil, errors.New("invalid token input")
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.TokenType != "" && claims.TokenType != TokenTypeAdmin {
		return nil, errors.New("invalid token type")
	}

	return claims, nil
}

func GenerateCustomerToken(secret string, expireDuration time.Duration, customerID uint) (string, error) {
	if secret == "" || expireDuration <= 0 || customerID == 0 {
		return "", errors.New("invalid token input")
	}

	return generateCustomerToken(secret, expireDuration, customerID, "")
}

func GenerateCustomerTokenForCustomer(
	secret string,
	expireDuration time.Duration,
	customer *models.Customer,
) (string, error) {
	if customer == nil || customer.ID == 0 || customer.Email == "" {
		return "", errors.New("invalid token input")
	}
	return generateCustomerToken(secret, expireDuration, customer.ID, customer.Email)
}

func generateCustomerToken(secret string, expireDuration time.Duration, customerID uint, email string) (string, error) {
	if secret == "" || expireDuration <= 0 || customerID == 0 {
		return "", errors.New("invalid token input")
	}

	now := time.Now()
	claims := CustomerClaims{
		UserID:     customerID,
		CustomerID: customerID,
		Email:      email,
		Role:       models.UserRoleCustomer,
		TokenType:  TokenTypeCustomer,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(customerID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expireDuration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseCustomerToken(secret string, tokenString string) (*CustomerClaims, error) {
	if secret == "" || tokenString == "" {
		return nil, errors.New("invalid token input")
	}

	claims := &CustomerClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.UserID == 0 {
		claims.UserID = claims.CustomerID
	}
	if claims.CustomerID == 0 {
		claims.CustomerID = claims.UserID
	}
	if claims.TokenType != TokenTypeCustomer ||
		claims.UserID == 0 ||
		claims.CustomerID != claims.UserID ||
		(claims.Role != "" && claims.Role != models.UserRoleCustomer) {
		return nil, errors.New("invalid token type")
	}
	if claims.Role == "" {
		claims.Role = models.UserRoleCustomer
	}

	return claims, nil
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPasswordHash(password string, passwordHash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}
