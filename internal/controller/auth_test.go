package controller

import (
	"net/http"
	"strings"

	"saas/internal/middleware"
	"saas/internal/pkg/auth"

	"github.com/gin-gonic/gin"
)

// requireControllerTestAuth isolates controller tests from the database-backed
// production authentication middleware. Authentication behavior is covered in
// internal/middleware/auth_test.go.
func requireControllerTestAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		claims, err := auth.ParseToken(secret, parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Set(middleware.ContextUserIDKey, claims.UserID)
		c.Set(middleware.ContextTenantIDKey, claims.TenantID)
		c.Set(middleware.ContextRoleKey, claims.Role)
		c.Next()
	}
}
