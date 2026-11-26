package router

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"saas/internal/controller"
	"saas/internal/middleware"
	"saas/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(
	db *gorm.DB,
	tenantController *controller.TenantController,
	userController *controller.UserController,
	employeeController *controller.EmployeeController,
	developerController *controller.DeveloperController,
	categoryController *controller.CategoryController,
	platformCategoryController *controller.PlatformCategoryController,
	storefrontController *controller.StorefrontController,
	publicHolidayController *controller.PublicHolidayController,
	reservationController *controller.ReservationController,
	shopProfileController *controller.ShopProfileController,
	sitePageController *controller.SitePageController,
	storefrontTrackingController *controller.StorefrontTrackingController,
	storefrontEventController *controller.StorefrontEventController,
	storefrontAnalyticsController *controller.StorefrontAnalyticsController,
	authController *controller.AuthController,
	customerAuthController *controller.CustomerAuthController,
	adminProductController *controller.AdminProductController,
	adminOrderController *controller.AdminOrderController,
	dashboardController *controller.DashboardController,
	superController *controller.SuperController,
	storefrontOrderController *controller.StorefrontOrderController,
	openAPIController *controller.OpenAPIController,
	uploadController *controller.UploadController,
	merchantLeadController *controller.MerchantLeadController,
	jwtSecret string,
	openAPIOptions middleware.OpenAPIAuthOptions,
) *gin.Engine {
	r := gin.New()
	r.Use(newPrivacyLogger(gin.DefaultWriter))
	r.Use(newPrivacyRecovery())
	registerUploadMIMETypes()
	r.Static("/uploads", "./uploads")

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/sitemap.xml", storefrontController.Sitemap)

	v1 := r.Group("/api/v1")
	authController.RegisterRoutes(v1)
	customerAuthController.RegisterRoutes(v1)
	storefrontController.RegisterRoutes(v1)
	publicHolidayController.RegisterPublicRoutes(v1)
	reservationController.RegisterPublicRoutes(v1)
	merchantLeadController.RegisterPublicRoutes(v1)
	platformCategoryController.RegisterPublicRoutes(v1)
	sitePageController.RegisterPublicRoutes(v1)
	storefrontTrackingController.RegisterPublicRoutes(v1)
	storefrontEventController.RegisterRoutes(v1)

	openAPI := v1.Group("/openapi")
	openAPI.Use(middleware.RequireOpenAPIAuth(db, openAPIOptions))
	openAPIController.RegisterRoutes(openAPI)

	protected := v1.Group("")
	protected.Use(middleware.RequireAuth(db, jwtSecret))

	storefrontProtected := v1.Group("")
	storefrontProtected.Use(storefrontOrderController.RequireStorefrontAuthEnabled())
	storefrontProtected.Use(middleware.RequireCustomerAuth(jwtSecret, db))
	storefrontOrderController.RegisterRoutes(storefrontProtected)

	superAdmin := protected.Group("")
	superAdmin.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	tenantController.RegisterRoutes(superAdmin)
	sitePageController.RegisterAdminRoutes(superAdmin)

	super := protected.Group("/super")
	super.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	superController.RegisterRoutes(super)
	merchantLeadController.RegisterSuperRoutes(super)
	platformCategoryController.RegisterSuperRoutes(super)
	storefrontTrackingController.RegisterSuperRoutes(super)
	storefrontAnalyticsController.RegisterSuperRoutes(super)
	publicHolidayController.RegisterSuperRoutes(super)
	reservationController.RegisterSuperRoutes(super)

	userAdmin := protected.Group("")
	userAdmin.Use(middleware.RequireRole(models.UserRoleSuperAdmin, models.UserRoleTenantAdmin))
	userController.RegisterRoutes(userAdmin)

	tenantAdmin := protected.Group("")
	tenantAdmin.Use(middleware.RequireRole(models.UserRoleTenantAdmin))
	employeeController.RegisterRoutes(tenantAdmin)
	developerController.RegisterRoutes(tenantAdmin)
	shopProfileController.RegisterRoutes(tenantAdmin)
	storefrontAnalyticsController.RegisterAdminRoutes(tenantAdmin)

	sensitiveAdmin := protected.Group("")
	sensitiveAdmin.Use(middleware.RequireRole(models.UserRoleSuperAdmin, models.UserRoleTenantAdmin, models.UserRoleFinance))
	platformCategoryController.RegisterAdminRoutes(sensitiveAdmin)
	categoryController.RegisterRoutes(sensitiveAdmin)
	dashboardController.RegisterRoutes(sensitiveAdmin)
	adminProductController.RegisterRoutes(sensitiveAdmin)
	uploadController.RegisterRoutes(sensitiveAdmin)

	orderAdmin := protected.Group("")
	orderAdmin.Use(middleware.RequireRole(models.UserRoleTenantAdmin, models.UserRoleFinance, models.UserRolePicker))
	adminOrderController.RegisterRoutes(orderAdmin)

	return r
}

func newPrivacyLogger(output io.Writer) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "<unmatched>"
		}
		// Log the route template, not the request URL. This excludes IPs,
		// headers, query strings, bodies, path parameters, and Gin errors.
		_, _ = fmt.Fprintf(output, "%s method=%s path=%s status=%d latency=%s bytes=%d\n",
			time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			c.Request.Method, route, c.Writer.Status(), time.Since(startedAt), c.Writer.Size(),
		)
	}
}

func newPrivacyRecovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		route := c.FullPath()
		if route == "" {
			route = "<unmatched>"
		}
		log.Printf("panic recovered method=%s path=%s", c.Request.Method, route)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
	})
}

func registerUploadMIMETypes() {
	for extension, contentType := range map[string]string{
		".webp": "image/webp",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".mov":  "video/quicktime",
		".m4v":  "video/mp4",
	} {
		_ = mime.AddExtensionType(extension, contentType)
	}
}
