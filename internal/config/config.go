package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv            string            `mapstructure:"app_env"`
	Server            ServerConfig      `mapstructure:"server"`
	Database          DatabaseConfig    `mapstructure:"database"`
	JWT               JWTConfig         `mapstructure:"jwt"`
	OpenAPI           OpenAPIConfig     `mapstructure:"openapi"`
	Storefront        StorefrontConfig  `mapstructure:"storefront"`
	Reservation       ReservationConfig `mapstructure:"reservation"`
	TrustedProxyCIDRs string            `mapstructure:"trusted_proxy_cidrs"`
}

const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
	minimumJWTSecretBytes  = 32
)

var rejectedJWTSecrets = map[string]struct{}{
	"change-me-in-production": {},
	"development-secret":      {},
	"jwt-secret":              {},
	"secret":                  {},
	"super-secret-key":        {},
	"test-secret":             {},
	"your-jwt-secret":         {},
	"your-secret-key":         {},
}

var cloudflareTurnstileTestSiteKeys = map[string]struct{}{
	"1x00000000000000000000AA": {},
	"1x00000000000000000000BB": {},
	"2x00000000000000000000AB": {},
	"2x00000000000000000000BB": {},
	"3x00000000000000000000FF": {},
}

var cloudflareTurnstileTestSecretKeys = map[string]struct{}{
	"1x0000000000000000000000000000000AA": {},
	"2x0000000000000000000000000000000AA": {},
	"3x0000000000000000000000000000000AA": {},
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type JWTConfig struct {
	Secret         string `mapstructure:"secret"`
	ExpireDuration string `mapstructure:"expire_duration"`
}

type OpenAPIConfig struct {
	LegacySignatureEnabled bool `mapstructure:"legacy_signature_enabled"`
}

type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         string `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	TimeZone     string `mapstructure:"timezone"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
}

type StorefrontConfig struct {
	AuthEnabled                      bool `mapstructure:"auth_enabled"`
	PasswordResetEnabled             bool `mapstructure:"password_reset_enabled"`
	PriceVisible                     bool `mapstructure:"price_visible"`
	PlatformMallTenantID             uint `mapstructure:"platform_mall_tenant_id"`
	ProductPopularityCacheTTLSeconds int  `mapstructure:"product_popularity_cache_ttl_seconds"`
}

type ReservationConfig struct {
	Mode                   string                     `mapstructure:"mode"`
	ContactHashSecret      string                     `mapstructure:"contact_hash_secret"`
	ContactEncryptionKey   string                     `mapstructure:"contact_encryption_key"`
	BrowserHashSecret      string                     `mapstructure:"browser_hash_secret"`
	IPHashSecret           string                     `mapstructure:"ip_hash_secret"`
	TrustedProxyCIDRs      string                     `mapstructure:"trusted_proxy_cidrs"`
	AnonymousTTLHours      int                        `mapstructure:"anonymous_ttl_hours"`
	HeatPopularMin         int64                      `mapstructure:"heat_popular_min"`
	HeatTrendingMin        int64                      `mapstructure:"heat_trending_min"`
	HeatHighlyRequestedMin int64                      `mapstructure:"heat_highly_requested_min"`
	AttemptRetentionDays   int                        `mapstructure:"attempt_retention_days"`
	Turnstile              TurnstileConfig            `mapstructure:"turnstile"`
	OTP                    OTPConfig                  `mapstructure:"otp"`
	RateLimits             ReservationRateLimitConfig `mapstructure:"rate_limits"`
}

type TurnstileConfig struct {
	Enabled              bool   `mapstructure:"enabled"`
	SiteKey              string `mapstructure:"site_key"`
	SecretKey            string `mapstructure:"secret_key"`
	ExpectedHostname     string `mapstructure:"expected_hostname"`
	ExpectedAction       string `mapstructure:"expected_action"`
	VerifyTimeoutSeconds int    `mapstructure:"verify_timeout_seconds"`
}

type OTPConfig struct {
	Provider               string `mapstructure:"provider"`
	Channels               string `mapstructure:"channels"`
	ProviderTimeoutSeconds int    `mapstructure:"provider_timeout_seconds"`
	MinimumResponseMillis  int    `mapstructure:"minimum_response_milliseconds"`
	ChallengeTTLMinutes    int    `mapstructure:"challenge_ttl_minutes"`
	ResendCooldownSeconds  int    `mapstructure:"resend_cooldown_seconds"`
	MaxVerifyAttempts      int    `mapstructure:"max_verify_attempts"`
	TwilioAccountSID       string `mapstructure:"twilio_account_sid"`
	TwilioAuthToken        string `mapstructure:"twilio_auth_token"`
	TwilioVerifyServiceSID string `mapstructure:"twilio_verify_service_sid"`
}

type ReservationRateLimitConfig struct {
	BrowserPerHour            int `mapstructure:"browser_per_hour"`
	BrowserPerDay             int `mapstructure:"browser_per_day"`
	IPPerHour                 int `mapstructure:"ip_per_hour"`
	IPPerDay                  int `mapstructure:"ip_per_day"`
	IPProductPerDay           int `mapstructure:"ip_product_per_day"`
	InterestVisitorPerMinute  int `mapstructure:"interest_visitor_per_minute"`
	InterestIPPerMinute       int `mapstructure:"interest_ip_per_minute"`
	InterestGlobalPerMinute   int `mapstructure:"interest_global_per_minute"`
	OTPRequestContactPer30Min int `mapstructure:"otp_request_contact_per_30_min"`
	OTPRequestContactPerDay   int `mapstructure:"otp_request_contact_per_day"`
	OTPRequestVisitorPerHour  int `mapstructure:"otp_request_visitor_per_hour"`
	OTPRequestIPPerHour       int `mapstructure:"otp_request_ip_per_hour"`
	OTPRequestGlobalPerMinute int `mapstructure:"otp_request_global_per_minute"`
	OTPVerifyVisitorPer10Min  int `mapstructure:"otp_verify_visitor_per_10_min"`
	OTPVerifyIPPer10Min       int `mapstructure:"otp_verify_ip_per_10_min"`
	OTPVerifyGlobalPerMinute  int `mapstructure:"otp_verify_global_per_minute"`
	CancelIPPerMinute         int `mapstructure:"cancel_ip_per_minute"`
	CancelGlobalPerMinute     int `mapstructure:"cancel_global_per_minute"`
}

const (
	ReservationModeDisabled  = "disabled"
	ReservationModeAnonymous = "anonymous"
	ReservationModeOTP       = "otp"
)

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)
	bindEnvs(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c Config) Validate() error {
	if c.Storefront.PasswordResetEnabled {
		return fmt.Errorf("CUSTOMER_PASSWORD_RESET_ENABLED requires a configured email delivery provider")
	}
	if c.Storefront.ProductPopularityCacheTTLSeconds != 0 &&
		(c.Storefront.ProductPopularityCacheTTLSeconds < 1 ||
			c.Storefront.ProductPopularityCacheTTLSeconds > 3600) {
		return fmt.Errorf("PRODUCT_POPULARITY_CACHE_TTL_SECONDS must be between 1 and 3600")
	}
	reservation := c.Reservation
	if strings.TrimSpace(reservation.TrustedProxyCIDRs) == "" {
		reservation.TrustedProxyCIDRs = c.TrustedProxyCIDRs
	}
	if err := reservation.validateBasics(); err != nil {
		return err
	}
	if _, err := NewTrustedProxyCIDRList(c.TrustedProxyCIDRs, c.Reservation.TrustedProxyCIDRs); err != nil {
		return err
	}
	if !IsProductionEnvironment(c.AppEnv) {
		return nil
	}

	secret := strings.TrimSpace(c.JWT.Secret)
	if secret == "" {
		return fmt.Errorf("JWT_SECRET is required in production")
	}
	if _, rejected := rejectedJWTSecrets[strings.ToLower(secret)]; rejected {
		return fmt.Errorf("JWT_SECRET uses a known insecure default")
	}
	if len([]byte(secret)) < minimumJWTSecretBytes {
		return fmt.Errorf("JWT_SECRET must be at least %d bytes in production", minimumJWTSecretBytes)
	}
	if !hasSufficientSecretDiversity(secret) {
		return fmt.Errorf("JWT_SECRET must be a high-entropy generated value in production")
	}
	if reservation.NormalizedMode() != ReservationModeDisabled {
		if err := reservation.validateProduction(); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) TrustedProxyCIDRList() []string {
	values, _ := NewTrustedProxyCIDRList(c.TrustedProxyCIDRs, c.Reservation.TrustedProxyCIDRs)
	return values
}

func NewTrustedProxyCIDRList(primary string, legacy string) ([]string, error) {
	value := strings.TrimSpace(primary)
	if value == "" {
		value = strings.TrimSpace(legacy)
	}
	result := make([]string, 0)
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS contains an invalid or unsafe CIDR")
		}
		result = append(result, prefix.Masked().String())
	}
	return result, nil
}

func (c ReservationConfig) EnabledOTPChannels() map[string]bool {
	channels := make(map[string]bool)
	for _, value := range strings.Split(c.OTP.Channels, ",") {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			channels[value] = true
		}
	}
	return channels
}

func (c ReservationConfig) NormalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		mode = ReservationModeDisabled
	}
	if mode == ReservationModeAnonymous || mode == ReservationModeOTP {
		return mode
	}
	return ReservationModeDisabled
}

func (c ReservationConfig) TrustedProxyCIDRList() []string {
	result := make([]string, 0)
	for _, value := range strings.Split(c.TrustedProxyCIDRs, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func (c ReservationConfig) EncryptionKeyBytes() ([]byte, error) {
	value := strings.TrimSpace(c.ContactEncryptionKey)
	if value == "" {
		return nil, fmt.Errorf("RESERVATION_CONTACT_ENCRYPTION_KEY is required")
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("RESERVATION_CONTACT_ENCRYPTION_KEY must decode to exactly 32 bytes")
}

func (c ReservationConfig) validateBasics() error {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		mode = ReservationModeDisabled
	}
	switch mode {
	case ReservationModeDisabled:
		return nil
	case ReservationModeAnonymous, ReservationModeOTP:
	default:
		return fmt.Errorf("RESERVATION_MODE must be disabled, anonymous, or otp")
	}
	if c.HeatPopularMin <= 0 || c.HeatTrendingMin <= c.HeatPopularMin || c.HeatHighlyRequestedMin <= c.HeatTrendingMin {
		return fmt.Errorf("reservation heat thresholds must be positive and strictly increasing")
	}
	if c.AttemptRetentionDays <= 0 {
		return fmt.Errorf("RESERVATION_ATTEMPT_RETENTION_DAYS must be positive")
	}
	if c.Turnstile.VerifyTimeoutSeconds <= 0 || strings.TrimSpace(c.Turnstile.ExpectedAction) == "" {
		return fmt.Errorf("Turnstile timeout and expected action are required")
	}
	if mode == ReservationModeAnonymous {
		if !c.Turnstile.Enabled {
			return fmt.Errorf("TURNSTILE_ENABLED must be true when RESERVATION_MODE=anonymous")
		}
		if strings.TrimSpace(c.Turnstile.SiteKey) == "" || strings.TrimSpace(c.Turnstile.SecretKey) == "" {
			return fmt.Errorf("TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY are required when RESERVATION_MODE=anonymous")
		}
		if strings.TrimSpace(c.Turnstile.ExpectedAction) != "reservation_create" {
			return fmt.Errorf("TURNSTILE_EXPECTED_ACTION must be reservation_create when RESERVATION_MODE=anonymous")
		}
		if len([]byte(strings.TrimSpace(c.BrowserHashSecret))) < 32 {
			return fmt.Errorf("RESERVATION_BROWSER_HASH_SECRET must be at least 32 bytes when RESERVATION_MODE=anonymous")
		}
		if len([]byte(strings.TrimSpace(c.IPHashSecret))) < 32 {
			return fmt.Errorf("RESERVATION_IP_HASH_SECRET must be at least 32 bytes when RESERVATION_MODE=anonymous")
		}
		if c.AnonymousTTLHours <= 0 || c.AnonymousTTLHours > 24*30 {
			return fmt.Errorf("RESERVATION_ANONYMOUS_TTL_HOURS must be between 1 and 720")
		}
		for name, value := range map[string]int{
			"browser hour":   c.RateLimits.BrowserPerHour,
			"browser day":    c.RateLimits.BrowserPerDay,
			"IP hour":        c.RateLimits.IPPerHour,
			"IP day":         c.RateLimits.IPPerDay,
			"IP product day": c.RateLimits.IPProductPerDay,
		} {
			if value <= 0 || value > 1_000_000 {
				return fmt.Errorf("anonymous reservation rate limit %s must be between 1 and 1000000", name)
			}
		}
		return c.validateTrustedProxyCIDRs(true)
	}
	if strings.TrimSpace(c.Turnstile.ExpectedAction) != "reservation_otp" {
		return fmt.Errorf("TURNSTILE_EXPECTED_ACTION must be reservation_otp when RESERVATION_MODE=otp")
	}
	provider := strings.ToLower(strings.TrimSpace(c.OTP.Provider))
	if provider != "mock" && provider != "twilio_verify" {
		return fmt.Errorf("OTP_PROVIDER must be mock or twilio_verify")
	}
	channels := c.EnabledOTPChannels()
	if len(channels) == 0 {
		return fmt.Errorf("RESERVATION_OTP_CHANNELS must enable at least one channel")
	}
	for channel := range channels {
		if channel != "sms" && channel != "whatsapp" {
			return fmt.Errorf("RESERVATION_OTP_CHANNELS contains unsupported channel %q", channel)
		}
	}
	if c.OTP.ProviderTimeoutSeconds <= 0 || c.OTP.ChallengeTTLMinutes <= 0 || c.OTP.ResendCooldownSeconds <= 0 || c.OTP.MaxVerifyAttempts <= 0 {
		return fmt.Errorf("OTP timeout, challenge TTL, cooldown, and max attempts must be positive")
	}
	if c.OTP.MinimumResponseMillis < 0 || c.OTP.MinimumResponseMillis > 5_000 {
		return fmt.Errorf("RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS must be between 0 and 5000")
	}
	if _, err := c.EncryptionKeyBytes(); err != nil {
		return err
	}
	for name, value := range map[string]int{
		"interest visitor":  c.RateLimits.InterestVisitorPerMinute,
		"interest IP":       c.RateLimits.InterestIPPerMinute,
		"interest global":   c.RateLimits.InterestGlobalPerMinute,
		"OTP contact 30m":   c.RateLimits.OTPRequestContactPer30Min,
		"OTP contact day":   c.RateLimits.OTPRequestContactPerDay,
		"OTP visitor hour":  c.RateLimits.OTPRequestVisitorPerHour,
		"OTP IP hour":       c.RateLimits.OTPRequestIPPerHour,
		"OTP global minute": c.RateLimits.OTPRequestGlobalPerMinute,
		"verify visitor":    c.RateLimits.OTPVerifyVisitorPer10Min,
		"verify IP":         c.RateLimits.OTPVerifyIPPer10Min,
		"verify global":     c.RateLimits.OTPVerifyGlobalPerMinute,
		"cancel IP":         c.RateLimits.CancelIPPerMinute,
		"cancel global":     c.RateLimits.CancelGlobalPerMinute,
	} {
		if value <= 0 || value > 1_000_000 {
			return fmt.Errorf("reservation rate limit %s must be between 1 and 1000000", name)
		}
	}
	return c.validateTrustedProxyCIDRs(false)
}

func (c ReservationConfig) validateProduction() error {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if !c.Turnstile.Enabled {
		return fmt.Errorf("TURNSTILE_ENABLED must be true in production when reservations are enabled")
	}
	if strings.TrimSpace(c.Turnstile.SiteKey) == "" || strings.TrimSpace(c.Turnstile.SecretKey) == "" {
		return fmt.Errorf("TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY are required in production")
	}
	if len(strings.TrimSpace(c.Turnstile.SiteKey)) < 20 || len(strings.TrimSpace(c.Turnstile.SecretKey)) < 20 {
		return fmt.Errorf("TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY must not use placeholders in production")
	}
	if isCloudflareTurnstileTestKey(c.Turnstile.SiteKey, cloudflareTurnstileTestSiteKeys) ||
		isCloudflareTurnstileTestKey(c.Turnstile.SecretKey, cloudflareTurnstileTestSecretKeys) {
		return fmt.Errorf("Cloudflare Turnstile test keys are forbidden in production")
	}
	if strings.TrimSpace(c.Turnstile.ExpectedHostname) == "" {
		return fmt.Errorf("TURNSTILE_EXPECTED_HOSTNAME is required in production")
	}
	expectedAction := "reservation_otp"
	if mode == ReservationModeAnonymous {
		expectedAction = "reservation_create"
	}
	if strings.TrimSpace(c.Turnstile.ExpectedAction) != expectedAction {
		return fmt.Errorf("TURNSTILE_EXPECTED_ACTION must be %s in production", expectedAction)
	}
	if len(c.TrustedProxyCIDRList()) == 0 {
		return fmt.Errorf("RESERVATION_TRUSTED_PROXY_CIDRS is required in production for the documented proxy topology")
	}
	if mode == ReservationModeAnonymous {
		if len([]byte(strings.TrimSpace(c.BrowserHashSecret))) < 32 ||
			c.BrowserHashSecret == defaultReservationBrowserHashSecret ||
			!hasSufficientSecretDiversity(strings.TrimSpace(c.BrowserHashSecret)) {
			return fmt.Errorf("RESERVATION_BROWSER_HASH_SECRET must be a non-default high-entropy value of at least 32 bytes in production")
		}
		if len([]byte(strings.TrimSpace(c.IPHashSecret))) < 32 ||
			c.IPHashSecret == defaultReservationIPHashSecret ||
			!hasSufficientSecretDiversity(strings.TrimSpace(c.IPHashSecret)) {
			return fmt.Errorf("RESERVATION_IP_HASH_SECRET must be a non-default high-entropy value of at least 32 bytes in production")
		}
		return nil
	}
	if c.OTP.MinimumResponseMillis < 250 {
		return fmt.Errorf("RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS must be at least 250 in production")
	}
	if len([]byte(strings.TrimSpace(c.ContactHashSecret))) < 32 || c.ContactHashSecret == defaultReservationContactHashSecret || !hasSufficientSecretDiversity(strings.TrimSpace(c.ContactHashSecret)) {
		return fmt.Errorf("RESERVATION_CONTACT_HASH_SECRET must be a non-default value of at least 32 bytes in production")
	}
	if len([]byte(strings.TrimSpace(c.IPHashSecret))) < 32 || c.IPHashSecret == defaultReservationIPHashSecret || !hasSufficientSecretDiversity(strings.TrimSpace(c.IPHashSecret)) {
		return fmt.Errorf("RESERVATION_IP_HASH_SECRET must be a non-default value of at least 32 bytes in production")
	}
	if strings.TrimSpace(c.ContactEncryptionKey) == defaultReservationEncryptionKey {
		return fmt.Errorf("RESERVATION_CONTACT_ENCRYPTION_KEY must not use the development default in production")
	}
	key, err := c.EncryptionKeyBytes()
	if err != nil || !hasSufficientKeyDiversity(key) {
		return fmt.Errorf("RESERVATION_CONTACT_ENCRYPTION_KEY must be a high-entropy 32-byte key in production")
	}
	if strings.EqualFold(strings.TrimSpace(c.OTP.Provider), "mock") {
		return fmt.Errorf("OTP_PROVIDER=mock is forbidden in production")
	}
	if strings.TrimSpace(c.OTP.TwilioAccountSID) == "" || strings.TrimSpace(c.OTP.TwilioAuthToken) == "" || strings.TrimSpace(c.OTP.TwilioVerifyServiceSID) == "" {
		return fmt.Errorf("Twilio Verify credentials are required in production")
	}
	if !validTwilioSID(c.OTP.TwilioAccountSID, "AC") || !validTwilioSID(c.OTP.TwilioVerifyServiceSID, "VA") || len(strings.TrimSpace(c.OTP.TwilioAuthToken)) < 20 {
		return fmt.Errorf("Twilio Verify credentials use invalid or placeholder formats")
	}
	return nil
}

func (c ReservationConfig) validateTrustedProxyCIDRs(requireAtLeastOne bool) error {
	values := c.TrustedProxyCIDRList()
	if requireAtLeastOne && len(values) == 0 {
		return fmt.Errorf("RESERVATION_TRUSTED_PROXY_CIDRS is required when RESERVATION_MODE=anonymous")
	}
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() == 0 {
			return fmt.Errorf("RESERVATION_TRUSTED_PROXY_CIDRS contains an invalid or unsafe CIDR")
		}
	}
	return nil
}

func isCloudflareTurnstileTestKey(value string, known map[string]struct{}) bool {
	_, exists := known[strings.TrimSpace(value)]
	return exists
}

func validTwilioSID(value, prefix string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 34 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func hasSufficientKeyDiversity(value []byte) bool {
	unique := make(map[byte]struct{}, len(value))
	for _, item := range value {
		unique[item] = struct{}{}
	}
	return len(value) == 32 && len(unique) >= 8
}

func IsProductionEnvironment(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == EnvironmentProduction || value == "prod"
}

func hasSufficientSecretDiversity(secret string) bool {
	unique := make(map[rune]struct{})
	classes := 0
	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSymbol := false
	isHex := true
	for _, char := range secret {
		unique[char] = struct{}{}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			isHex = false
		}
		switch {
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= '0' && char <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	for _, present := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if present {
			classes++
		}
	}

	// Long hexadecimal secrets are valid high-entropy values despite using only
	// two character classes. Shorter secrets must use at least three classes.
	return len(unique) >= 12 && (classes >= 3 || (isHex && len([]byte(secret)) >= 64))
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		c.Host,
		c.User,
		c.Password,
		c.DBName,
		c.Port,
		c.TimeZone,
	)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app_env", EnvironmentDevelopment)
	v.SetDefault("server.port", "8080")
	v.SetDefault("jwt.secret", "change-me-in-production")
	v.SetDefault("jwt.expire_duration", "24h")
	v.SetDefault("openapi.legacy_signature_enabled", false)
	v.SetDefault("trusted_proxy_cidrs", "")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", "5432")
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.dbname", "saas_ecommerce")
	v.SetDefault("database.timezone", "UTC")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("storefront.auth_enabled", false)
	v.SetDefault("storefront.password_reset_enabled", false)
	v.SetDefault("storefront.price_visible", false)
	v.SetDefault("storefront.platform_mall_tenant_id", 0)
	v.SetDefault("storefront.product_popularity_cache_ttl_seconds", 120)
	v.SetDefault("reservation.mode", ReservationModeDisabled)
	v.SetDefault("reservation.contact_hash_secret", defaultReservationContactHashSecret)
	v.SetDefault("reservation.contact_encryption_key", defaultReservationEncryptionKey)
	v.SetDefault("reservation.browser_hash_secret", defaultReservationBrowserHashSecret)
	v.SetDefault("reservation.ip_hash_secret", defaultReservationIPHashSecret)
	v.SetDefault("reservation.trusted_proxy_cidrs", "")
	v.SetDefault("reservation.anonymous_ttl_hours", 24)
	v.SetDefault("reservation.heat_popular_min", 5)
	v.SetDefault("reservation.heat_trending_min", 20)
	v.SetDefault("reservation.heat_highly_requested_min", 50)
	v.SetDefault("reservation.attempt_retention_days", 30)
	v.SetDefault("reservation.turnstile.enabled", false)
	v.SetDefault("reservation.turnstile.site_key", "")
	v.SetDefault("reservation.turnstile.secret_key", "")
	v.SetDefault("reservation.turnstile.expected_hostname", "")
	v.SetDefault("reservation.turnstile.expected_action", "reservation_otp")
	v.SetDefault("reservation.turnstile.verify_timeout_seconds", 5)
	v.SetDefault("reservation.otp.provider", "mock")
	v.SetDefault("reservation.otp.channels", "sms")
	v.SetDefault("reservation.otp.provider_timeout_seconds", 5)
	v.SetDefault("reservation.otp.minimum_response_milliseconds", 750)
	v.SetDefault("reservation.otp.challenge_ttl_minutes", 10)
	v.SetDefault("reservation.otp.resend_cooldown_seconds", 60)
	v.SetDefault("reservation.otp.max_verify_attempts", 5)
	v.SetDefault("reservation.rate_limits.browser_per_hour", 5)
	v.SetDefault("reservation.rate_limits.browser_per_day", 20)
	v.SetDefault("reservation.rate_limits.ip_per_hour", 20)
	v.SetDefault("reservation.rate_limits.ip_per_day", 100)
	v.SetDefault("reservation.rate_limits.ip_product_per_day", 3)
	v.SetDefault("reservation.rate_limits.interest_visitor_per_minute", 20)
	v.SetDefault("reservation.rate_limits.interest_ip_per_minute", 60)
	v.SetDefault("reservation.rate_limits.interest_global_per_minute", 1000)
	v.SetDefault("reservation.rate_limits.otp_request_contact_per_30_min", 3)
	v.SetDefault("reservation.rate_limits.otp_request_contact_per_day", 5)
	v.SetDefault("reservation.rate_limits.otp_request_visitor_per_hour", 5)
	v.SetDefault("reservation.rate_limits.otp_request_ip_per_hour", 20)
	v.SetDefault("reservation.rate_limits.otp_request_global_per_minute", 100)
	v.SetDefault("reservation.rate_limits.otp_verify_visitor_per_10_min", 15)
	v.SetDefault("reservation.rate_limits.otp_verify_ip_per_10_min", 50)
	v.SetDefault("reservation.rate_limits.otp_verify_global_per_minute", 500)
	v.SetDefault("reservation.rate_limits.cancel_ip_per_minute", 60)
	v.SetDefault("reservation.rate_limits.cancel_global_per_minute", 1000)
}

const (
	defaultReservationContactHashSecret = "development-reservation-contact-hash-secret"
	defaultReservationEncryptionKey     = "0123456789abcdef0123456789abcdef"
	defaultReservationBrowserHashSecret = "development-reservation-browser-hash-secret"
	defaultReservationIPHashSecret      = "development-reservation-ip-risk-hash-secret"
)

func bindEnvs(v *viper.Viper) {
	_ = v.BindEnv("app_env", "APP_ENV")
	_ = v.BindEnv("server.port", "SERVER_PORT")
	_ = v.BindEnv("jwt.secret", "JWT_SECRET")
	_ = v.BindEnv("jwt.expire_duration", "JWT_EXPIRE_DURATION")
	_ = v.BindEnv("openapi.legacy_signature_enabled", "OPENAPI_LEGACY_SIGNATURE_ENABLED")
	_ = v.BindEnv("trusted_proxy_cidrs", "TRUSTED_PROXY_CIDRS")
	_ = v.BindEnv("database.host", "DB_HOST")
	_ = v.BindEnv("database.port", "DB_PORT")
	_ = v.BindEnv("database.user", "DB_USER")
	_ = v.BindEnv("database.password", "DB_PASSWORD")
	_ = v.BindEnv("database.dbname", "DB_NAME")
	_ = v.BindEnv("database.timezone", "DB_TIMEZONE")
	_ = v.BindEnv("database.max_idle_conns", "DB_MAX_IDLE_CONNS")
	_ = v.BindEnv("database.max_open_conns", "DB_MAX_OPEN_CONNS")
	_ = v.BindEnv("storefront.auth_enabled", "STOREFRONT_AUTH_ENABLED")
	_ = v.BindEnv("storefront.password_reset_enabled", "CUSTOMER_PASSWORD_RESET_ENABLED")
	_ = v.BindEnv("storefront.price_visible", "STOREFRONT_PRICE_VISIBLE")
	_ = v.BindEnv("storefront.platform_mall_tenant_id", "PLATFORM_MALL_TENANT_ID")
	_ = v.BindEnv("storefront.product_popularity_cache_ttl_seconds", "PRODUCT_POPULARITY_CACHE_TTL_SECONDS")
	_ = v.BindEnv("reservation.mode", "RESERVATION_MODE")
	_ = v.BindEnv("reservation.contact_hash_secret", "RESERVATION_CONTACT_HASH_SECRET")
	_ = v.BindEnv("reservation.contact_encryption_key", "RESERVATION_CONTACT_ENCRYPTION_KEY")
	_ = v.BindEnv("reservation.browser_hash_secret", "RESERVATION_BROWSER_HASH_SECRET")
	_ = v.BindEnv("reservation.ip_hash_secret", "RESERVATION_IP_HASH_SECRET")
	_ = v.BindEnv("reservation.trusted_proxy_cidrs", "RESERVATION_TRUSTED_PROXY_CIDRS")
	_ = v.BindEnv("reservation.anonymous_ttl_hours", "RESERVATION_ANONYMOUS_TTL_HOURS")
	_ = v.BindEnv("reservation.heat_popular_min", "RESERVATION_HEAT_POPULAR_MIN")
	_ = v.BindEnv("reservation.heat_trending_min", "RESERVATION_HEAT_TRENDING_MIN")
	_ = v.BindEnv("reservation.heat_highly_requested_min", "RESERVATION_HEAT_HIGHLY_REQUESTED_MIN")
	_ = v.BindEnv("reservation.attempt_retention_days", "RESERVATION_ATTEMPT_RETENTION_DAYS")
	_ = v.BindEnv("reservation.turnstile.enabled", "TURNSTILE_ENABLED")
	_ = v.BindEnv("reservation.turnstile.site_key", "TURNSTILE_SITE_KEY")
	_ = v.BindEnv("reservation.turnstile.secret_key", "TURNSTILE_SECRET_KEY")
	_ = v.BindEnv("reservation.turnstile.expected_hostname", "TURNSTILE_EXPECTED_HOSTNAME")
	_ = v.BindEnv("reservation.turnstile.expected_action", "TURNSTILE_EXPECTED_ACTION")
	_ = v.BindEnv("reservation.turnstile.verify_timeout_seconds", "TURNSTILE_VERIFY_TIMEOUT_SECONDS")
	_ = v.BindEnv("reservation.otp.provider", "OTP_PROVIDER")
	_ = v.BindEnv("reservation.otp.channels", "RESERVATION_OTP_CHANNELS")
	_ = v.BindEnv("reservation.otp.provider_timeout_seconds", "OTP_PROVIDER_TIMEOUT_SECONDS")
	_ = v.BindEnv("reservation.otp.minimum_response_milliseconds", "RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS")
	_ = v.BindEnv("reservation.otp.challenge_ttl_minutes", "RESERVATION_OTP_CHALLENGE_TTL_MINUTES")
	_ = v.BindEnv("reservation.otp.resend_cooldown_seconds", "RESERVATION_OTP_RESEND_COOLDOWN_SECONDS")
	_ = v.BindEnv("reservation.otp.max_verify_attempts", "RESERVATION_OTP_MAX_VERIFY_ATTEMPTS")
	_ = v.BindEnv("reservation.otp.twilio_account_sid", "TWILIO_ACCOUNT_SID")
	_ = v.BindEnv("reservation.otp.twilio_auth_token", "TWILIO_AUTH_TOKEN")
	_ = v.BindEnv("reservation.otp.twilio_verify_service_sid", "TWILIO_VERIFY_SERVICE_SID")
	_ = v.BindEnv("reservation.rate_limits.browser_per_hour", "RESERVATION_BROWSER_LIMIT_PER_HOUR")
	_ = v.BindEnv("reservation.rate_limits.browser_per_day", "RESERVATION_BROWSER_LIMIT_PER_DAY")
	_ = v.BindEnv("reservation.rate_limits.ip_per_hour", "RESERVATION_IP_LIMIT_PER_HOUR")
	_ = v.BindEnv("reservation.rate_limits.ip_per_day", "RESERVATION_IP_LIMIT_PER_DAY")
	_ = v.BindEnv("reservation.rate_limits.ip_product_per_day", "RESERVATION_IP_PRODUCT_LIMIT_PER_DAY")
	_ = v.BindEnv("reservation.rate_limits.interest_visitor_per_minute", "RESERVATION_INTEREST_VISITOR_PER_MINUTE")
	_ = v.BindEnv("reservation.rate_limits.interest_ip_per_minute", "RESERVATION_INTEREST_IP_PER_MINUTE")
	_ = v.BindEnv("reservation.rate_limits.interest_global_per_minute", "RESERVATION_INTEREST_GLOBAL_PER_MINUTE")
	_ = v.BindEnv("reservation.rate_limits.otp_request_contact_per_30_min", "RESERVATION_OTP_CONTACT_PER_30_MIN")
	_ = v.BindEnv("reservation.rate_limits.otp_request_contact_per_day", "RESERVATION_OTP_CONTACT_PER_DAY")
	_ = v.BindEnv("reservation.rate_limits.otp_request_visitor_per_hour", "RESERVATION_OTP_VISITOR_PER_HOUR")
	_ = v.BindEnv("reservation.rate_limits.otp_request_ip_per_hour", "RESERVATION_OTP_IP_PER_HOUR")
	_ = v.BindEnv("reservation.rate_limits.otp_request_global_per_minute", "RESERVATION_OTP_GLOBAL_PER_MINUTE")
	_ = v.BindEnv("reservation.rate_limits.otp_verify_visitor_per_10_min", "RESERVATION_VERIFY_VISITOR_PER_10_MIN")
	_ = v.BindEnv("reservation.rate_limits.otp_verify_ip_per_10_min", "RESERVATION_VERIFY_IP_PER_10_MIN")
	_ = v.BindEnv("reservation.rate_limits.otp_verify_global_per_minute", "RESERVATION_VERIFY_GLOBAL_PER_MINUTE")
	_ = v.BindEnv("reservation.rate_limits.cancel_ip_per_minute", "RESERVATION_CANCEL_IP_PER_MINUTE")
	_ = v.BindEnv("reservation.rate_limits.cancel_global_per_minute", "RESERVATION_CANCEL_GLOBAL_PER_MINUTE")
}
