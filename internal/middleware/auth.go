package middleware

import (
	"net/http"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/auth"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ContextUserIDKey     = "userID"
	ContextTenantIDKey   = "tenantID"
	ContextRoleKey       = "role"
	ContextCustomerIDKey = "customerID"
)

func RequireAuth(db *gorm.DB, secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := bearerToken(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		claims, err := auth.ParseToken(secret, tokenString)
		if err != nil {
			abortAdminAuth(c, http.StatusUnauthorized)
			return
		}
		if db == nil || claims.UserID == 0 {
			abortAdminAuth(c, http.StatusUnauthorized)
			return
		}

		var user models.User
		if err := db.WithContext(c.Request.Context()).
			Select("id", "tenant_id", "role", "status").
			First(&user, claims.UserID).Error; err != nil {
			abortAdminAuth(c, http.StatusUnauthorized)
			return
		}
		if user.Status != models.UserStatusActive || user.Role != claims.Role || !sameTenantID(user.TenantID, claims.TenantID) {
			abortAdminAuth(c, http.StatusUnauthorized)
			return
		}

		switch user.Role {
		case models.UserRoleSuperAdmin:
			if user.TenantID != nil {
				abortAdminAuth(c, http.StatusUnauthorized)
				return
			}
		case models.UserRoleTenantAdmin, models.UserRoleFinance, models.UserRolePicker:
			if user.TenantID == nil || *user.TenantID == 0 {
				abortAdminAuth(c, http.StatusUnauthorized)
				return
			}
			var tenant models.Tenant
			if err := db.WithContext(c.Request.Context()).
				Select("id", "status").
				First(&tenant, *user.TenantID).Error; err != nil {
				abortAdminAuth(c, http.StatusForbidden)
				return
			}
			if tenant.Status != models.TenantStatusActive {
				abortAdminAuth(c, http.StatusForbidden)
				return
			}
		default:
			abortAdminAuth(c, http.StatusForbidden)
			return
		}

		c.Set(ContextUserIDKey, user.ID)
		c.Set(ContextTenantIDKey, user.TenantID)
		c.Set(ContextRoleKey, user.Role)
		c.Next()
	}
}

func abortAdminAuth(c *gin.Context, status int) {
	errorName := "unauthorized"
	if status == http.StatusForbidden {
		errorName = "forbidden"
	}
	c.JSON(status, gin.H{"error": errorName})
	c.Abort()
}

func sameTenantID(left *uint, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func RequireCustomerAuth(secret string, databases ...*gorm.DB) gin.HandlerFunc {
	var db *gorm.DB
	if len(databases) > 0 {
		db = databases[0]
	}
	return func(c *gin.Context) {
		tokenString, ok := bearerToken(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		claims, err := auth.ParseCustomerToken(secret, tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		if db != nil {
			var customer models.Customer
			if err := db.WithContext(c.Request.Context()).
				Select("id", "status").
				First(&customer, claims.CustomerID).Error; err != nil ||
				customer.Status != models.CustomerStatusActive {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				c.Abort()
				return
			}
		}

		c.Set(ContextCustomerIDKey, claims.CustomerID)
		c.Next()
	}
}

func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func RequireRole(roles ...models.UserRole) gin.HandlerFunc {
	allowed := make(map[models.UserRole]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		role, ok := RoleFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		if _, ok := allowed[role]; !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func UserIDFromContext(c *gin.Context) (uint, bool) {
	value, ok := c.Get(ContextUserIDKey)
	if !ok {
		return 0, false
	}
	userID, ok := value.(uint)
	return userID, ok
}

func TenantIDFromContext(c *gin.Context) (*uint, bool) {
	value, ok := c.Get(ContextTenantIDKey)
	if !ok {
		return nil, false
	}
	tenantID, ok := value.(*uint)
	return tenantID, ok
}

func RoleFromContext(c *gin.Context) (models.UserRole, bool) {
	value, ok := c.Get(ContextRoleKey)
	if !ok {
		return "", false
	}
	role, ok := value.(models.UserRole)
	return role, ok
}

func CustomerIDFromContext(c *gin.Context) (uint, bool) {
	value, ok := c.Get(ContextCustomerIDKey)
	if !ok {
		return 0, false
	}
	customerID, ok := value.(uint)
	return customerID, ok
}

func IsSuperAdmin(c *gin.Context) bool {
	role, ok := RoleFromContext(c)
	return ok && role == models.UserRoleSuperAdmin
}
