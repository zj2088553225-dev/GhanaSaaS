package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"saas/internal/analytics"
	"saas/internal/config"
	"saas/internal/controller"
	"saas/internal/middleware"
	"saas/internal/pkg/database"
	"saas/internal/repository"
	"saas/internal/router"
	"saas/internal/service"

	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.JWT.Secret == "" {
		log.Fatal("jwt secret is required")
	}
	jwtExpireDuration, err := time.ParseDuration(cfg.JWT.ExpireDuration)
	if err != nil {
		log.Fatalf("parse jwt expire duration: %v", err)
	}
	clientIPResolver, err := service.NewClientIPResolver(cfg.TrustedProxyCIDRList())
	if err != nil {
		log.Fatalf("initialize client IP resolver: %v", err)
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	tenantRepo := repository.NewTenantRepository(db)
	userRepo := repository.NewUserRepository(db)
	customerRepo := repository.NewCustomerRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	platformCategoryRepo := repository.NewPlatformCategoryRepository(db)
	publicProductScopeOptions := repository.PublicProductScopeOptions{
		PlatformMallTenantID: cfg.Storefront.PlatformMallTenantID,
	}
	storefrontRepo := repository.NewStorefrontRepository(db, publicProductScopeOptions)
	productRepo := repository.NewProductRepository(db)
	shopProfileRepo := repository.NewShopProfileRepository(db)
	sitePageRepo := repository.NewSitePageRepository(db)
	storefrontTrackingRepo := repository.NewStorefrontTrackingRepository(db)
	storefrontEventRepo := repository.NewStorefrontEventRepository(db, publicProductScopeOptions)
	storefrontAnalyticsRepo := repository.NewStorefrontAnalyticsRepository(db)
	productPopularityRepo := repository.NewProductPopularityRepository(db, publicProductScopeOptions)
	uploadAssetRepo := repository.NewUploadAssetRepository(db)
	orderRepo := repository.NewOrderRepository(db, publicProductScopeOptions)
	merchantLeadRepo := repository.NewMerchantLeadRepository(db)
	contentTranslationRepo := repository.NewContentTranslationRepository(db)
	contentTranslationRepo.SetMutationHook(repository.ContentTranslationMutationHook{
		Before: platformCategoryRepo.BeforeContentTranslationMutations,
		After:  platformCategoryRepo.AfterContentTranslationMutations,
	})
	publicHolidayRepo := repository.NewPublicHolidayRepository(db)

	contentTranslationService := service.NewContentTranslationService(contentTranslationRepo)
	tenantService := service.NewTenantService(tenantRepo)
	userService := service.NewUserService(userRepo)
	categoryService := service.NewCategoryService(categoryRepo)
	platformCategoryService := service.NewPlatformCategoryService(platformCategoryRepo, service.PlatformCategoryOptions{
		Translations: contentTranslationService,
	})
	storefrontService := service.NewStorefrontService(storefrontRepo, service.StorefrontOptions{
		PriceVisible:         cfg.Storefront.PriceVisible,
		PlatformMallTenantID: cfg.Storefront.PlatformMallTenantID,
		Translations:         contentTranslationService,
	})
	shopProfileService := service.NewShopProfileService(shopProfileRepo, service.ShopProfileOptions{
		Translations: contentTranslationService,
	})
	sitePageService := service.NewSitePageService(sitePageRepo, service.SitePageOptions{
		Translations: contentTranslationService,
	})
	storefrontTrackingService := service.NewStorefrontTrackingService(storefrontTrackingRepo)
	storefrontEventService := service.NewStorefrontEventService(storefrontEventRepo)
	marketingContentResolver := analytics.NewMarketingContentResolver(sitePageRepo)
	storefrontAnalyticsService := service.NewStorefrontAnalyticsService(storefrontAnalyticsRepo, service.StorefrontAnalyticsOptions{
		ContentResolver: marketingContentResolver,
	})
	productPopularityService := service.NewProductPopularityService(productPopularityRepo, service.ProductPopularityOptions{
		Translations: contentTranslationService,
		PriceVisible: cfg.Storefront.PriceVisible,
		CacheTTL:     time.Duration(cfg.Storefront.ProductPopularityCacheTTLSeconds) * time.Second,
	})
	authService := service.NewAuthService(userRepo, cfg.JWT.Secret, jwtExpireDuration)
	customerAuthService := service.NewCustomerAuthService(customerRepo, cfg.JWT.Secret, jwtExpireDuration)
	adminProductService := service.NewAdminProductService(productRepo, service.AdminProductOptions{
		Translations: contentTranslationService,
	})
	orderService := service.NewOrderService(orderRepo)
	dashboardService := service.NewDashboardService(db)
	superService := service.NewSuperService(db, cfg.JWT.Secret, jwtExpireDuration)
	merchantLeadService := service.NewMerchantLeadService(merchantLeadRepo)
	openAPIService := service.NewOpenAPIService(db)
	publicHolidayService := service.NewPublicHolidayService(publicHolidayRepo)
	reservationController, err := initializeReservationController(cfg.Reservation, db, clientIPResolver)
	if err != nil {
		log.Fatalf("initialize reservation feature: %v", err)
	}
	storageService := service.NewLocalStorage("uploads/images", uploadAssetRepo)
	videoStorageService := service.NewLocalVideoStorage("uploads/videos", uploadAssetRepo)
	if err := videoStorageService.EnsurePublicReadPermissions(); err != nil {
		log.Fatalf("prepare video upload permissions: %v", err)
	}

	tenantController := controller.NewTenantController(tenantService)
	userController := controller.NewUserController(userService)
	employeeController := controller.NewEmployeeController(userService)
	developerController := controller.NewDeveloperController(tenantService)
	categoryController := controller.NewCategoryController(categoryService)
	platformCategoryController := controller.NewPlatformCategoryController(platformCategoryService)
	storefrontController := controller.NewStorefrontController(storefrontService).
		WithPopularity(productPopularityService)
	shopProfileController := controller.NewShopProfileController(shopProfileService)
	sitePageController := controller.NewSitePageController(sitePageService)
	storefrontTrackingController := controller.NewStorefrontTrackingController(storefrontTrackingService)
	storefrontEventController := controller.NewStorefrontEventController(storefrontEventService).
		WithClientIPResolver(clientIPResolver)
	storefrontAnalyticsController := controller.NewStorefrontAnalyticsController(storefrontAnalyticsService).
		WithPopularity(productPopularityService)
	authController := controller.NewAuthController(authService, customerAuthService).
		WithCustomerAuthEnabled(cfg.Storefront.AuthEnabled).
		WithClientIPResolver(clientIPResolver)
	customerAuthController := controller.NewCustomerAuthController(
		customerAuthService,
		cfg.JWT.Secret,
		cfg.Storefront.AuthEnabled,
		cfg.Storefront.PasswordResetEnabled,
	).
		WithDatabase(db)
	adminProductController := controller.NewAdminProductController(adminProductService)
	adminOrderController := controller.NewAdminOrderController(orderService)
	dashboardController := controller.NewDashboardController(dashboardService)
	superController := controller.NewSuperController(superService)
	storefrontOrderController := controller.NewStorefrontOrderController(orderService, cfg.Storefront.AuthEnabled)
	openAPIController := controller.NewOpenAPIController(openAPIService)
	uploadController := controller.NewUploadController(storageService, videoStorageService)
	merchantLeadController := controller.NewMerchantLeadController(merchantLeadService)
	publicHolidayController := controller.NewPublicHolidayController(publicHolidayService)

	r := router.NewRouter(
		db,
		tenantController,
		userController,
		employeeController,
		developerController,
		categoryController,
		platformCategoryController,
		storefrontController,
		publicHolidayController,
		reservationController,
		shopProfileController,
		sitePageController,
		storefrontTrackingController,
		storefrontEventController,
		storefrontAnalyticsController,
		authController,
		customerAuthController,
		adminProductController,
		adminOrderController,
		dashboardController,
		superController,
		storefrontOrderController,
		openAPIController,
		uploadController,
		merchantLeadController,
		cfg.JWT.Secret,
		middleware.OpenAPIAuthOptions{
			AllowLegacySignatures: cfg.OpenAPI.LegacySignatureEnabled,
		},
	)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func initializeReservationController(
	cfg config.ReservationConfig,
	db *gorm.DB,
	clientIPResolver *service.ClientIPResolver,
) (*controller.ReservationController, error) {
	mode := cfg.NormalizedMode()
	if mode == config.ReservationModeDisabled {
		return controller.NewReservationController(nil, controller.ReservationControllerOptions{Mode: mode}), nil
	}
	if db == nil {
		return nil, fmt.Errorf("reservation database is required")
	}

	reservationRequestSecurity, err := service.NewReservationRequestSecurity(
		cfg.IPHashSecret,
		service.ReservationRequestSecurityOptions{ClientIPResolver: clientIPResolver},
	)
	if err != nil {
		return nil, fmt.Errorf("initialize reservation request security: %w", err)
	}

	reservationLimiter := service.NewReservationMemoryLimiter()
	reservationRateLimits := service.ReservationRateLimits{
		InterestVisitorPerMinute:  cfg.RateLimits.InterestVisitorPerMinute,
		InterestIPPerMinute:       cfg.RateLimits.InterestIPPerMinute,
		InterestGlobalPerMinute:   cfg.RateLimits.InterestGlobalPerMinute,
		OTPRequestContactPer30Min: cfg.RateLimits.OTPRequestContactPer30Min,
		OTPRequestContactPerDay:   cfg.RateLimits.OTPRequestContactPerDay,
		OTPRequestVisitorPerHour:  cfg.RateLimits.OTPRequestVisitorPerHour,
		OTPRequestIPPerHour:       cfg.RateLimits.OTPRequestIPPerHour,
		OTPRequestGlobalPerMinute: cfg.RateLimits.OTPRequestGlobalPerMinute,
		OTPVerifyVisitorPer10Min:  cfg.RateLimits.OTPVerifyVisitorPer10Min,
		OTPVerifyIPPer10Min:       cfg.RateLimits.OTPVerifyIPPer10Min,
		OTPVerifyGlobalPerMinute:  cfg.RateLimits.OTPVerifyGlobalPerMinute,
		CancelIPPerMinute:         cfg.RateLimits.CancelIPPerMinute,
		CancelGlobalPerMinute:     cfg.RateLimits.CancelGlobalPerMinute,
	}
	reservationService := service.NewReservationService(
		repository.NewReservationProductRepository(db),
		service.ReservationServiceOptions{
			HeatThresholds: service.ReservationHeatThresholds{
				PopularMin: cfg.HeatPopularMin, TrendingMin: cfg.HeatTrendingMin,
				HighlyRequestedMin: cfg.HeatHighlyRequestedMin,
			},
			Limiter: reservationLimiter, RateLimits: reservationRateLimits,
		},
	)

	var turnstileVerifier service.TurnstileVerifier
	if cfg.Turnstile.Enabled {
		turnstileVerifier, err = service.NewCloudflareTurnstileVerifier(service.CloudflareTurnstileOptions{
			Secret: cfg.Turnstile.SecretKey, ExpectedAction: cfg.Turnstile.ExpectedAction,
			ExpectedHostname: cfg.Turnstile.ExpectedHostname,
			Timeout:          time.Duration(cfg.Turnstile.VerifyTimeoutSeconds) * time.Second,
		})
	} else {
		turnstileVerifier = service.NewMockTurnstileVerifier(true)
	}
	if err != nil {
		return nil, fmt.Errorf("initialize Turnstile verifier: %w", err)
	}

	anonymousService, err := service.NewAnonymousReservationService(
		repository.NewAnonymousReservationRepository(db),
		service.AnonymousReservationOptions{
			Turnstile: turnstileVerifier,
			TTL:       time.Duration(cfg.AnonymousTTLHours) * time.Hour,
			RateLimits: repository.AnonymousReservationLimits{
				BrowserPerHour:  cfg.RateLimits.BrowserPerHour,
				BrowserPerDay:   cfg.RateLimits.BrowserPerDay,
				IPPerHour:       cfg.RateLimits.IPPerHour,
				IPPerDay:        cfg.RateLimits.IPPerDay,
				IPProductPerDay: cfg.RateLimits.IPProductPerDay,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("initialize anonymous reservation workflow: %w", err)
	}

	if mode == config.ReservationModeAnonymous {
		browserIdentity, identityErr := service.NewReservationBrowserIdentity(cfg.BrowserHashSecret)
		if identityErr != nil {
			return nil, fmt.Errorf("initialize anonymous browser identity: %w", identityErr)
		}
		if _, expireErr := anonymousService.ExpireDue(context.Background(), 1000); expireErr != nil {
			return nil, fmt.Errorf("expire anonymous reservations at startup: %w", expireErr)
		}
		startAnonymousReservationExpiryWorker(anonymousService)
		return controller.NewReservationController(reservationService, controller.ReservationControllerOptions{
			Mode: mode, Anonymous: anonymousService, BrowserIdentity: browserIdentity,
			Security: reservationRequestSecurity,
		}), nil
	}

	reservationEncryptionKey, err := cfg.EncryptionKeyBytes()
	if err != nil {
		return nil, fmt.Errorf("load reservation encryption key: %w", err)
	}
	reservationProtector, err := service.NewReservationContactProtector(cfg.ContactHashSecret, reservationEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("initialize reservation contact protection: %w", err)
	}

	var otpProvider service.OTPProvider
	switch strings.ToLower(strings.TrimSpace(cfg.OTP.Provider)) {
	case "mock":
		otpProvider = service.NewMockOTPProvider()
	case "twilio_verify":
		otpProvider, err = service.NewTwilioVerifyOTPProvider(service.TwilioVerifyOptions{
			AccountSID: cfg.OTP.TwilioAccountSID, AuthToken: cfg.OTP.TwilioAuthToken,
			ServiceSID: cfg.OTP.TwilioVerifyServiceSID,
			Timeout:    time.Duration(cfg.OTP.ProviderTimeoutSeconds) * time.Second,
		})
	default:
		err = service.ErrInvalidInput
	}
	if err != nil {
		return nil, fmt.Errorf("initialize OTP provider: %w", err)
	}

	reservationWorkflowService, err := service.NewReservationWorkflowService(
		repository.NewReservationWorkflowRepository(db),
		service.ReservationWorkflowOptions{
			Protector: reservationProtector, Turnstile: turnstileVerifier, OTPProvider: otpProvider,
			Limiter: reservationLimiter, RateLimits: reservationRateLimits, EnabledChannels: cfg.EnabledOTPChannels(),
			ChallengeTTL:        time.Duration(cfg.OTP.ChallengeTTLMinutes) * time.Minute,
			ResendCooldown:      time.Duration(cfg.OTP.ResendCooldownSeconds) * time.Second,
			MaxVerifyAttempts:   cfg.OTP.MaxVerifyAttempts,
			MinimumResponseTime: time.Duration(cfg.OTP.MinimumResponseMillis) * time.Millisecond,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("initialize reservation workflow: %w", err)
	}

	return controller.NewReservationController(reservationService, controller.ReservationControllerOptions{
		Mode: mode, Workflow: reservationWorkflowService, Anonymous: anonymousService,
		Security: reservationRequestSecurity,
	}), nil
}

func startAnonymousReservationExpiryWorker(workflow interface {
	ExpireDue(context.Context, int) (int64, error)
}) {
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := workflow.ExpireDue(context.Background(), 1000); err != nil {
				log.Printf("anonymous reservation expiry task failed")
			}
		}
	}()
}
