package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"saas/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	openAPITimestampSkew       = 5 * time.Minute
	openAPISignatureVersionV2  = "v2"
	openAPISignatureVersionV1  = "v1"
	openAPIV2PayloadIdentifier = "OPENAPI-HMAC-SHA256-V2"
	openAPIMinNonceLength      = 16
	openAPIMaxNonceLength      = 128
	openAPIMaxBodyBytes        = 2 << 20
)

type OpenAPIAuthOptions struct {
	AllowLegacySignatures bool
}

func RequireOpenAPIAuth(db *gorm.DB, options OpenAPIAuthOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		appKey := strings.TrimSpace(c.GetHeader("X-App-Key"))
		timestampValue := strings.TrimSpace(c.GetHeader("X-Timestamp"))
		signature := strings.TrimSpace(c.GetHeader("X-Signature"))
		version := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Signature-Version")))
		if db == nil || appKey == "" || timestampValue == "" || signature == "" {
			abortOpenAPIAuth(c, http.StatusUnauthorized)
			return
		}

		now := time.Now()
		timestamp, err := parseOpenAPITimestamp(timestampValue)
		if err != nil || timestamp.Before(now.Add(-openAPITimestampSkew)) || timestamp.After(now.Add(openAPITimestampSkew)) {
			abortOpenAPIAuth(c, http.StatusUnauthorized)
			return
		}

		var tenant models.Tenant
		result := db.WithContext(c.Request.Context()).
			Where("app_key = ? AND status = ?", appKey, models.TenantStatusActive).
			Limit(1).
			Find(&tenant)
		if result.Error != nil || result.RowsAffected != 1 || tenant.AppSecret == "" {
			abortOpenAPIAuth(c, http.StatusUnauthorized)
			return
		}

		switch version {
		case openAPISignatureVersionV2:
			if !authenticateOpenAPIV2(c, db, tenant, timestampValue, signature, now) {
				return
			}
		case "", openAPISignatureVersionV1:
			if !options.AllowLegacySignatures || !verifyLegacyOpenAPISignature(appKey, tenant.AppSecret, timestampValue, signature) {
				abortOpenAPIAuth(c, http.StatusUnauthorized)
				return
			}
		default:
			abortOpenAPIAuth(c, http.StatusUnauthorized)
			return
		}

		tenantID := tenant.ID
		c.Set(ContextTenantIDKey, &tenantID)
		c.Next()
	}
}

func authenticateOpenAPIV2(
	c *gin.Context,
	db *gorm.DB,
	tenant models.Tenant,
	timestamp string,
	signature string,
	now time.Time,
) bool {
	nonce := strings.TrimSpace(c.GetHeader("X-Nonce"))
	if !validOpenAPINonce(nonce) {
		abortOpenAPIAuth(c, http.StatusUnauthorized)
		return false
	}

	body, err := readAndRestoreRequestBody(c.Request)
	if err != nil {
		abortOpenAPIAuth(c, http.StatusUnauthorized)
		return false
	}
	canonicalQuery, err := canonicalOpenAPIQuery(c.Request.URL.RawQuery)
	if err != nil {
		abortOpenAPIAuth(c, http.StatusUnauthorized)
		return false
	}
	bodyHash := sha256.Sum256(body)
	expectedSignature := signOpenAPIRequestV2(
		tenant.AppSecret,
		tenant.AppKey,
		c.Request.Method,
		canonicalOpenAPIPath(c.Request.URL),
		canonicalQuery,
		hex.EncodeToString(bodyHash[:]),
		timestamp,
		nonce,
	)
	if !constantTimeHexEqual(expectedSignature, signature) {
		abortOpenAPIAuth(c, http.StatusUnauthorized)
		return false
	}

	reserved, err := reserveOpenAPINonce(c, db, tenant.AppKey, nonce, now)
	if err != nil {
		abortOpenAPIAuth(c, http.StatusServiceUnavailable)
		return false
	}
	if !reserved {
		abortOpenAPIAuth(c, http.StatusUnauthorized)
		return false
	}
	return true
}

func reserveOpenAPINonce(c *gin.Context, db *gorm.DB, appKey string, nonce string, now time.Time) (bool, error) {
	record := models.OpenAPIRequestNonce{
		AppKey:    appKey,
		Nonce:     nonce,
		ExpiresAt: now.Add(openAPITimestampSkew),
		CreatedAt: now,
	}
	result := db.WithContext(c.Request.Context()).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&record)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		cleanupExpiredOpenAPINonces(c, db, record.ID, now)
		return true, nil
	}

	result = db.WithContext(c.Request.Context()).
		Model(&models.OpenAPIRequestNonce{}).
		Where("app_key = ? AND nonce = ? AND expires_at <= ?", appKey, nonce, now).
		Updates(map[string]any{
			"expires_at": now.Add(openAPITimestampSkew),
			"created_at": now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func cleanupExpiredOpenAPINonces(c *gin.Context, db *gorm.DB, recordID uint64, now time.Time) {
	if recordID == 0 || recordID%100 != 0 {
		return
	}
	_ = db.WithContext(c.Request.Context()).
		Where("expires_at < ?", now.Add(-openAPITimestampSkew)).
		Delete(&models.OpenAPIRequestNonce{}).Error
}

func parseOpenAPITimestamp(value string) (time.Time, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	if parsed > 9999999999 {
		parsed /= 1000
	}
	return time.Unix(parsed, 0), nil
}

func readAndRestoreRequestBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return []byte{}, nil
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, openAPIMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > openAPIMaxBodyBytes {
		return nil, errors.New("openapi request body exceeds authentication limit")
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func canonicalOpenAPIPath(requestURL *url.URL) string {
	if requestURL == nil || requestURL.EscapedPath() == "" {
		return "/"
	}
	path := requestURL.EscapedPath()
	var canonical strings.Builder
	canonical.Grow(len(path))
	for index := 0; index < len(path); index++ {
		if path[index] == '%' && index+2 < len(path) {
			canonical.WriteByte('%')
			canonical.WriteString(strings.ToUpper(path[index+1 : index+3]))
			index += 2
			continue
		}
		canonical.WriteByte(path[index])
	}
	return canonical.String()
}

func canonicalOpenAPIQuery(rawQuery string) (string, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", err
	}
	pairs := make([]string, 0, len(values))
	for key, items := range values {
		if len(items) == 0 {
			items = []string{""}
		}
		for _, value := range items {
			pairs = append(pairs, encodeOpenAPIQueryComponent(key)+"="+encodeOpenAPIQueryComponent(value))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&"), nil
}

func encodeOpenAPIQueryComponent(value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var encoded strings.Builder
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("-._~", rune(char)) {
			encoded.WriteByte(char)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexadecimal[char>>4])
		encoded.WriteByte(hexadecimal[char&0x0f])
	}
	return encoded.String()
}

func signOpenAPIRequestV2(
	appSecret string,
	appKey string,
	method string,
	path string,
	canonicalQuery string,
	bodySHA256 string,
	timestamp string,
	nonce string,
) string {
	payload := strings.Join([]string{
		openAPIV2PayloadIdentifier,
		appKey,
		strings.ToUpper(method),
		path,
		canonicalQuery,
		strings.ToLower(bodySHA256),
		timestamp,
		nonce,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(appSecret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyLegacyOpenAPISignature(appKey string, appSecret string, timestamp string, signature string) bool {
	expected := sha256.Sum256([]byte(appKey + appSecret + timestamp))
	return constantTimeHexEqual(hex.EncodeToString(expected[:]), signature)
}

func constantTimeHexEqual(expected string, actual string) bool {
	expectedBytes, expectedErr := hex.DecodeString(expected)
	actualBytes, actualErr := hex.DecodeString(actual)
	if expectedErr != nil || actualErr != nil || len(expectedBytes) != len(actualBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(expectedBytes, actualBytes) == 1
}

func validOpenAPINonce(nonce string) bool {
	if len(nonce) < openAPIMinNonceLength || len(nonce) > openAPIMaxNonceLength {
		return false
	}
	for _, char := range nonce {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("-_.~", char) {
			continue
		}
		return false
	}
	return true
}

func abortOpenAPIAuth(c *gin.Context, status int) {
	message := "unauthorized"
	if status == http.StatusServiceUnavailable {
		message = "service unavailable"
	}
	c.JSON(status, gin.H{"error": message})
	c.Abort()
}
