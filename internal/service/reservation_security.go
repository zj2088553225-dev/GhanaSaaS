package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nyaruka/phonenumbers"
)

const (
	reservationContactEncryptionVersion  = 2
	maxReservationContactCiphertextBytes = 128
	reservationContactHashDomain         = "reservation-contact:v1:"
	reservationManageTokenDomain         = "reservation-manage:v1:"
	reservationIPRiskHashDomain          = "reservation-ip-risk:v1:"
	reservationTurnstileRequestDomain    = "reservation-turnstile-request:v1:"
)

var reservationContactAAD = []byte("reservation-contact:v2")

const (
	ReservationDeviceCookieName = "gc_reservation_device"
	ReservationDeviceMaxAge     = 180 * 24 * 60 * 60
)

var (
	ErrInvalidPhoneNumber = &contextualError{message: "invalid phone number", target: ErrInvalidInput}
	ErrInvalidContactKey  = errors.New("invalid reservation contact encryption key")
)

type NormalizedReservationPhone struct {
	E164        string
	CountryCode string
	Last4       string
}

type ReservationContactProtector struct {
	hashSecret    []byte
	encryptionKey []byte
}

func NewReservationContactProtector(hashSecret string, encryptionKey []byte) (*ReservationContactProtector, error) {
	if len([]byte(hashSecret)) < 32 || len(encryptionKey) != 32 {
		return nil, ErrInvalidContactKey
	}
	return &ReservationContactProtector{
		hashSecret:    append([]byte(nil), []byte(hashSecret)...),
		encryptionKey: append([]byte(nil), encryptionKey...),
	}, nil
}

func NormalizeReservationPhone(number, country string) (NormalizedReservationPhone, error) {
	number = strings.TrimSpace(number)
	country = strings.ToUpper(strings.TrimSpace(country))
	if number == "" || country == "" || !validReservationPhoneInput(number) {
		return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
	}
	if country != "" && (len(country) != 2 || phonenumbers.GetCountryCodeForRegion(country) == 0) {
		return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
	}
	phone, err := phonenumbers.Parse(number, country)
	if err != nil || phone.GetExtension() != "" || !phonenumbers.IsValidNumber(phone) {
		return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
	}
	switch phonenumbers.GetNumberType(phone) {
	case phonenumbers.MOBILE, phonenumbers.FIXED_LINE_OR_MOBILE:
		// Verification channels require a mobile-capable number. Explicitly
		// reject premium, shared-cost, toll-free, service, pager, and VOIP
		// ranges even when libphonenumber considers their shape valid.
	default:
		return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
	}
	e164 := phonenumbers.Format(phone, phonenumbers.E164)
	if len(e164) < 5 || len(e164) > 16 || !strings.HasPrefix(e164, "+") {
		return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
	}
	if strings.HasPrefix(number, "+") && country != "" {
		parsedRegion := strings.ToUpper(phonenumbers.GetRegionCodeForNumber(phone))
		if parsedRegion == "" || parsedRegion == "001" || parsedRegion != country {
			return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
		}
	}
	digits := strings.TrimPrefix(e164, "+")
	for _, character := range digits {
		if character < '0' || character > '9' {
			return NormalizedReservationPhone{}, ErrInvalidPhoneNumber
		}
	}
	return NormalizedReservationPhone{
		E164:        e164,
		CountryCode: "+" + strconv.FormatInt(int64(phone.GetCountryCode()), 10),
		Last4:       digits[len(digits)-4:],
	}, nil
}

func validReservationPhoneInput(value string) bool {
	plusSeen := false
	seenNonSpace := false
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
			seenNonSpace = true
		case character == '+' && !seenNonSpace && !plusSeen:
			plusSeen = true
			seenNonSpace = true
		case character == ' ' || character == '\t' || character == '-' || character == '(' || character == ')' || character == '.':
			// Formatting separators are accepted; letters, extensions, Unicode
			// digits, and additional plus signs are not.
		default:
			return false
		}
	}
	return seenNonSpace
}

func (p *ReservationContactProtector) Hash(e164 string) string {
	if !validNormalizedReservationE164(e164) {
		return ""
	}
	mac := hmac.New(sha256.New, p.hashSecret)
	_, _ = mac.Write([]byte(reservationContactHashDomain))
	_, _ = mac.Write([]byte(e164))
	return hex.EncodeToString(mac.Sum(nil))
}

func (p *ReservationContactProtector) Encrypt(e164 string) ([]byte, int, error) {
	if !validNormalizedReservationE164(e164) {
		return nil, 0, ErrInvalidPhoneNumber
	}
	block, err := aes.NewCipher(p.encryptionKey)
	if err != nil {
		return nil, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, 0, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, 0, err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(e164), reservationContactAAD)
	sealed := make([]byte, 0, len(nonce)+len(ciphertext))
	sealed = append(sealed, nonce...)
	sealed = append(sealed, ciphertext...)
	return sealed, reservationContactEncryptionVersion, nil
}

func (p *ReservationContactProtector) Decrypt(sealed []byte, version int) (string, error) {
	if version != 1 && version != reservationContactEncryptionVersion {
		return "", ErrInvalidContactKey
	}
	block, err := aes.NewCipher(p.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) <= gcm.NonceSize() || len(sealed) > maxReservationContactCiphertextBytes {
		return "", ErrInvalidContactKey
	}
	var additionalData []byte
	if version == reservationContactEncryptionVersion {
		additionalData = reservationContactAAD
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], additionalData)
	if err != nil {
		return "", ErrInvalidContactKey
	}
	value := string(plaintext)
	if !validNormalizedReservationE164(value) {
		return "", ErrInvalidContactKey
	}
	return value, nil
}

func validNormalizedReservationE164(value string) bool {
	if len(value) < 5 || len(value) > 16 || value[0] != '+' {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func MaskReservationPhone(countryCode, last4 string) string {
	countryCode = strings.TrimSpace(countryCode)
	last4 = strings.TrimSpace(last4)
	if !validMaskedPhonePart(countryCode, true) || !validMaskedPhonePart(last4, false) {
		return "***"
	}
	return fmt.Sprintf("%s *** *** %s", countryCode, last4)
}

func validMaskedPhonePart(value string, requirePlus bool) bool {
	if requirePlus {
		if len(value) < 2 || len(value) > 8 || value[0] != '+' {
			return false
		}
		value = value[1:]
	} else if len(value) != 4 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func NewReservationManageToken() (plain string, hash string, err error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(value)
	digest := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(digest[:]), nil
}

func HashReservationManageToken(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

func (p *ReservationContactProtector) IdempotentManageToken(challengeID, idempotencyKey string) (plain string, hash string, err error) {
	challengeID = strings.TrimSpace(challengeID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if challengeID == "" || idempotencyKey == "" {
		return "", "", ErrInvalidInput
	}
	mac := hmac.New(sha256.New, p.hashSecret)
	_, _ = mac.Write([]byte(reservationManageTokenDomain))
	_, _ = mac.Write([]byte(challengeID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(idempotencyKey))
	plain = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return plain, HashReservationManageToken(plain), nil
}

func NewReservationChallengeID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// ReservationTurnstileRequestID binds Cloudflare's stable idempotency key to
// the complete normalized request identity. A client key reused for another
// product or contact therefore cannot reuse the same Turnstile token.
func ReservationTurnstileRequestID(productID uint64, contactHash, requestKey string) string {
	if productID == 0 || len(contactHash) != sha256.Size*2 || strings.TrimSpace(requestKey) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(
		reservationTurnstileRequestDomain + strconv.FormatUint(productID, 10) + "\x00" + contactHash + "\x00" + strings.TrimSpace(requestKey),
	))
	// Format the first 128 bits as an RFC 4122 UUID while retaining 122 bits of
	// digest entropy. This is an identifier, not an authentication secret.
	value := digest[:16]
	value[6] = (value[6] & 0x0f) | 0x50
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func NewReservationTurnstileRequestID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

type ReservationBrowserIdentity struct {
	hashSecret []byte
}

func NewReservationBrowserIdentity(secret string) (*ReservationBrowserIdentity, error) {
	if len([]byte(strings.TrimSpace(secret))) < 32 {
		return nil, errors.New("reservation browser hash secret must be at least 32 bytes")
	}
	return &ReservationBrowserIdentity{hashSecret: []byte(strings.TrimSpace(secret))}, nil
}

func (identity *ReservationBrowserIdentity) NewToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (identity *ReservationBrowserIdentity) ValidToken(token string) bool {
	value, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	return err == nil && len(value) >= 32
}

func (identity *ReservationBrowserIdentity) HashToken(token string) string {
	token = strings.TrimSpace(token)
	if !identity.ValidToken(token) {
		return ""
	}
	mac := hmac.New(sha256.New, identity.hashSecret)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

type ReservationRequestSecurity struct {
	ipHashSecret []byte
	ipResolver   *ClientIPResolver
	now          func() time.Time
}

type ReservationRequestSecurityOptions struct {
	TrustedProxyCIDRs []string
	ClientIPResolver  *ClientIPResolver
	Now               func() time.Time
}

func NewReservationRequestSecurity(secret string, options ...ReservationRequestSecurityOptions) (*ReservationRequestSecurity, error) {
	if len([]byte(secret)) < 32 {
		return nil, errors.New("reservation IP hash secret must be at least 32 bytes")
	}
	resolver, err := NewClientIPResolver(nil)
	if err != nil {
		return nil, err
	}
	security := &ReservationRequestSecurity{
		ipHashSecret: append([]byte(nil), []byte(secret)...),
		ipResolver:   resolver,
		now:          time.Now,
	}
	if len(options) > 0 {
		if options[0].Now != nil {
			security.now = options[0].Now
		}
		if options[0].ClientIPResolver != nil {
			security.ipResolver = options[0].ClientIPResolver
		} else {
			security.ipResolver, err = NewClientIPResolver(options[0].TrustedProxyCIDRs)
			if err != nil {
				return nil, errors.New("invalid reservation trusted proxy CIDR")
			}
		}
	}
	return security, nil
}

// RemoteIP ignores forwarding headers unless the immediate peer is trusted. It
// then walks X-Forwarded-For from right to left and stops at the first
// untrusted hop, so client-supplied leftmost values cannot override the proxy
// chain observed by a trusted edge.
func (s *ReservationRequestSecurity) RemoteIP(remoteAddr string, forwardedFor ...string) string {
	return s.ClientIP(remoteAddr, "", forwardedFor...)
}

// ClientIP accepts proxy headers only when the immediate TCP peer is in the
// explicit trusted proxy set. A valid CF-Connecting-IP takes precedence;
// otherwise the trusted X-Forwarded-For chain is evaluated right-to-left.
func (s *ReservationRequestSecurity) ClientIP(remoteAddr, cfConnectingIP string, forwardedFor ...string) string {
	return s.ClientIPWithHeaders(remoteAddr, cfConnectingIP, "", forwardedFor...)
}

func (s *ReservationRequestSecurity) ClientIPWithHeaders(
	remoteAddr string,
	cfConnectingIP string,
	xRealIP string,
	forwardedFor ...string,
) string {
	return s.ipResolver.Resolve(remoteAddr, cfConnectingIP, xRealIP, forwardedFor...)
}

func (s *ReservationRequestSecurity) IPRiskKey(remoteAddr string, forwardedFor ...string) string {
	ip := s.RemoteIP(remoteAddr, forwardedFor...)
	return s.ipRiskKey(ip)
}

func (s *ReservationRequestSecurity) IPRiskKeyForRequest(
	remoteAddr string,
	cfConnectingIP string,
	xRealIP string,
	forwardedFor ...string,
) string {
	return s.ipRiskKey(s.ClientIPWithHeaders(remoteAddr, cfConnectingIP, xRealIP, forwardedFor...))
}

func (s *ReservationRequestSecurity) ipRiskKey(ip string) string {
	if ip == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.ipHashSecret)
	_, _ = mac.Write([]byte(reservationIPRiskHashDomain))
	_, _ = mac.Write([]byte(s.now().UTC().Format("2006-01-02")))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *ReservationRequestSecurity) IPHash(remoteAddr, cfConnectingIP string, forwardedFor ...string) string {
	return s.IPHashWithHeaders(remoteAddr, cfConnectingIP, "", forwardedFor...)
}

func (s *ReservationRequestSecurity) IPHashWithHeaders(
	remoteAddr string,
	cfConnectingIP string,
	xRealIP string,
	forwardedFor ...string,
) string {
	ip := s.ClientIPWithHeaders(remoteAddr, cfConnectingIP, xRealIP, forwardedFor...)
	if ip == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.ipHashSecret)
	_, _ = mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}
