package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestReservationPhoneNormalizationHashEncryptionAndMasking(t *testing.T) {
	phone, err := NormalizeReservationPhone("024 123 4567", "GH")
	if err != nil {
		t.Fatalf("NormalizeReservationPhone: %v", err)
	}
	if phone.E164 != "+233241234567" || phone.CountryCode != "+233" || phone.Last4 != "4567" {
		t.Fatalf("normalized phone = %+v", phone)
	}
	if _, err := NormalizeReservationPhone("123", "GH"); err == nil {
		t.Fatal("invalid phone was accepted")
	}
	if _, err := NormalizeReservationPhone("0241234567 ext 12", "GH"); err == nil {
		t.Fatal("phone extension was accepted")
	}
	for _, testCase := range []struct {
		number  string
		country string
	}{
		{number: "+1 415 555 2671", country: "GH"},
		{number: "＋233241234567", country: "GH"},
		{number: "024ABC4567", country: "GH"},
		{number: "++233241234567", country: "GH"},
		{number: "0241234567", country: "ZZ"},
		{number: "+19002345678", country: "US"},
		{number: "+18005551234", country: "US"},
		{number: "+14155552671", country: ""},
	} {
		if _, err := NormalizeReservationPhone(testCase.number, testCase.country); err == nil {
			t.Fatalf("unsafe phone input was accepted: %+v", testCase)
		}
	}
	usPhone, err := NormalizeReservationPhone("(415) 555-2671", "US")
	if err != nil || usPhone.E164 != "+14155552671" {
		t.Fatalf("US phone normalization = %+v/%v", usPhone, err)
	}

	key := []byte("abcdef0123456789abcdef0123456789")
	protector, err := NewReservationContactProtector("contact-hash-secret-0123456789abcdef", key)
	if err != nil {
		t.Fatalf("NewReservationContactProtector: %v", err)
	}
	hash := protector.Hash(phone.E164)
	if len(hash) != 64 || hash != protector.Hash(phone.E164) || strings.Contains(hash, "233241234567") {
		t.Fatalf("contact hash = %q", hash)
	}
	other, _ := NewReservationContactProtector("different-hash-secret-0123456789abc", key)
	if other.Hash(phone.E164) == hash {
		t.Fatal("different HMAC secrets produced the same contact hash")
	}
	plainMAC := hmac.New(sha256.New, []byte("contact-hash-secret-0123456789abcdef"))
	_, _ = plainMAC.Write([]byte(phone.E164))
	if hash == hex.EncodeToString(plainMAC.Sum(nil)) || protector.Hash("not-e164") != "" {
		t.Fatal("contact hash omitted its domain separator or accepted noncanonical input")
	}
	sealedOne, version, err := protector.Encrypt(phone.E164)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	sealedTwo, _, err := protector.Encrypt(phone.E164)
	if err != nil {
		t.Fatalf("second Encrypt: %v", err)
	}
	if bytes.Equal(sealedOne, sealedTwo) || bytes.Contains(sealedOne, []byte(phone.E164)) {
		t.Fatalf("AES-GCM ciphertext is deterministic or contains plaintext")
	}
	decrypted, err := protector.Decrypt(sealedOne, version)
	if err != nil || decrypted != phone.E164 {
		t.Fatalf("Decrypt = %q/%v", decrypted, err)
	}
	tamperedCiphertext := append([]byte(nil), sealedOne...)
	tamperedCiphertext[len(tamperedCiphertext)-1] ^= 1
	if _, err := protector.Decrypt(tamperedCiphertext, version); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
	tamperedNonce := append([]byte(nil), sealedOne...)
	tamperedNonce[0] ^= 1
	if _, err := protector.Decrypt(tamperedNonce, version); err == nil {
		t.Fatal("tampered nonce was accepted")
	}
	wrongKeyProtector, _ := NewReservationContactProtector("contact-hash-secret-0123456789abcdef", []byte("0123456789abcdef0123456789abcdef"))
	if _, err := wrongKeyProtector.Decrypt(sealedOne, version); err == nil {
		t.Fatal("wrong AES key decrypted the contact")
	}
	if _, _, err := protector.Encrypt(""); err == nil {
		t.Fatal("empty plaintext was encrypted")
	}
	if _, err := protector.Decrypt(make([]byte, maxReservationContactCiphertextBytes+1), version); err == nil {
		t.Fatal("oversized ciphertext was accepted")
	}
	legacy := sealLegacyReservationContact(t, key, phone.E164)
	if plaintext, err := protector.Decrypt(legacy, 1); err != nil || plaintext != phone.E164 {
		t.Fatalf("legacy v1 decrypt = %q/%v", plaintext, err)
	}
	if masked := MaskReservationPhone(phone.CountryCode, phone.Last4); masked != "+233 *** *** 4567" || strings.Contains(masked, "24123") {
		t.Fatalf("masked phone = %q", masked)
	}
}

func sealLegacyReservationContact(t *testing.T, key []byte, plaintext string) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{7}, gcm.NonceSize())
	return append(nonce, gcm.Seal(nil, nonce, []byte(plaintext), nil)...)
}

func TestReservationManageTokenAndIPRiskKey(t *testing.T) {
	token, hash, err := NewReservationManageToken()
	if err != nil {
		t.Fatalf("NewReservationManageToken: %v", err)
	}
	if token == "" || len(hash) != 64 || HashReservationManageToken(token) != hash || strings.Contains(hash, token) {
		t.Fatalf("manage token/hash lengths = %d/%d", len(token), len(hash))
	}
	protector, err := NewReservationContactProtector(
		"contact-hash-secret-0123456789abcdef",
		[]byte("abcdef0123456789abcdef0123456789"),
	)
	if err != nil {
		t.Fatalf("NewReservationContactProtector: %v", err)
	}
	idempotentOne, idempotentHashOne, err := protector.IdempotentManageToken("challenge", "request-one")
	if err != nil {
		t.Fatalf("IdempotentManageToken: %v", err)
	}
	idempotentRetry, idempotentHashRetry, _ := protector.IdempotentManageToken("challenge", "request-one")
	idempotentOther, _, _ := protector.IdempotentManageToken("challenge", "request-two")
	if len(idempotentOne) != 43 || idempotentOne != idempotentRetry || idempotentHashOne != idempotentHashRetry || idempotentOther == idempotentOne {
		t.Fatalf("idempotent manage-token semantics are unstable")
	}
	turnstileID := ReservationTurnstileRequestID(7, strings.Repeat("a", 64), "request-one")
	if len(turnstileID) != 36 || turnstileID != ReservationTurnstileRequestID(7, strings.Repeat("a", 64), "request-one") || turnstileID == ReservationTurnstileRequestID(8, strings.Repeat("a", 64), "request-one") || turnstileID == ReservationTurnstileRequestID(7, strings.Repeat("b", 64), "request-one") {
		t.Fatalf("Turnstile request identity binding is unstable: %q", turnstileID)
	}
	security, err := NewReservationRequestSecurity("ip-risk-hash-secret-0123456789abcdef")
	if err != nil {
		t.Fatalf("NewReservationRequestSecurity: %v", err)
	}
	if remote := security.RemoteIP("203.0.113.8:43210"); remote != "203.0.113.8" {
		t.Fatalf("RemoteIP = %q", remote)
	}
	riskKey := security.IPRiskKey("203.0.113.8:43210")
	if len(riskKey) != 64 || strings.Contains(riskKey, "203.0.113.8") || riskKey != security.IPRiskKey("203.0.113.8:9999") {
		t.Fatalf("IP risk key = %q", riskKey)
	}
	if security.IPRiskKey("not-an-ip") != "" {
		t.Fatal("invalid RemoteAddr produced a risk key")
	}
	rawMAC := hmac.New(sha256.New, []byte("ip-risk-hash-secret-0123456789abcdef"))
	_, _ = rawMAC.Write([]byte("203.0.113.8"))
	if riskKey == hex.EncodeToString(rawMAC.Sum(nil)) {
		t.Fatal("IP risk hash omitted its domain separator")
	}

	trusted, err := NewReservationRequestSecurity(
		"ip-risk-hash-secret-0123456789abcdef",
		ReservationRequestSecurityOptions{TrustedProxyCIDRs: []string{"10.0.0.0/8", "172.16.0.0/12"}},
	)
	if err != nil {
		t.Fatalf("trusted proxy security: %v", err)
	}
	if got := trusted.RemoteIP("172.18.0.4:8080", "192.0.2.99, 198.51.100.4, 10.0.0.8"); got != "198.51.100.4" {
		t.Fatalf("right-to-left trusted chain = %q", got)
	}
	if got := trusted.RemoteIP("203.0.113.9:8080", "192.0.2.99"); got != "203.0.113.9" {
		t.Fatalf("untrusted peer accepted forwarding header: %q", got)
	}
	if got := trusted.RemoteIP("172.18.0.4:8080", "not-an-ip"); got != "172.18.0.4" {
		t.Fatalf("malformed forwarding fallback = %q", got)
	}
	if got := trusted.RemoteIP("[::ffff:203.0.113.8]:8080"); got != "203.0.113.8" {
		t.Fatalf("IPv4-mapped IPv6 normalization = %q", got)
	}
	if _, err := NewReservationRequestSecurity("ip-risk-hash-secret-0123456789abcdef", ReservationRequestSecurityOptions{TrustedProxyCIDRs: []string{"0.0.0.0/0"}}); err == nil {
		t.Fatal("unsafe trust-all CIDR was accepted")
	}
	current := time.Date(2026, time.July, 17, 23, 59, 0, 0, time.UTC)
	rotating, err := NewReservationRequestSecurity(
		"ip-risk-hash-secret-0123456789abcdef",
		ReservationRequestSecurityOptions{Now: func() time.Time { return current }},
	)
	if err != nil {
		t.Fatal(err)
	}
	firstDay := rotating.IPRiskKey("203.0.113.8:43210")
	current = current.Add(2 * time.Minute)
	if nextDay := rotating.IPRiskKey("203.0.113.8:43210"); nextDay == firstDay {
		t.Fatal("IP risk key did not rotate at the UTC day boundary")
	}
}

func TestAnonymousReservationBrowserIdentityAndTrustedClientIP(t *testing.T) {
	identity, err := NewReservationBrowserIdentity("browser-hash-secret-0123456789abcdef")
	if err != nil {
		t.Fatalf("NewReservationBrowserIdentity: %v", err)
	}
	token, err := identity.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) < 32 || !identity.ValidToken(token) {
		t.Fatalf("browser token is not at least 32 random bytes: %d/%v", len(decoded), err)
	}
	browserHash := identity.HashToken(token)
	if len(browserHash) != sha256.Size*2 || strings.Contains(browserHash, token) ||
		browserHash != identity.HashToken(token) {
		t.Fatalf("browser hash is invalid: %q", browserHash)
	}
	expectedBrowserMAC := hmac.New(sha256.New, []byte("browser-hash-secret-0123456789abcdef"))
	_, _ = expectedBrowserMAC.Write([]byte(token))
	if browserHash != hex.EncodeToString(expectedBrowserMAC.Sum(nil)) {
		t.Fatal("browser hash is not HMAC-SHA256(secret, cookie_token)")
	}
	otherIdentity, _ := NewReservationBrowserIdentity("other-browser-secret-0123456789abcdef")
	if otherIdentity.HashToken(token) == browserHash {
		t.Fatal("different browser HMAC secrets produced the same hash")
	}
	if identity.ValidToken("short") || identity.HashToken("short") != "" {
		t.Fatal("invalid browser token was accepted")
	}

	security, err := NewReservationRequestSecurity(
		"stable-ip-hash-secret-0123456789abcdef",
		ReservationRequestSecurityOptions{TrustedProxyCIDRs: []string{"172.16.0.0/12", "10.0.0.0/8"}},
	)
	if err != nil {
		t.Fatalf("NewReservationRequestSecurity: %v", err)
	}
	if got := security.ClientIP("198.51.100.8:443", "203.0.113.99", "192.0.2.9"); got != "198.51.100.8" {
		t.Fatalf("untrusted peer forged proxy headers: %q", got)
	}
	if got := security.ClientIP("172.18.0.4:8080", "2001:db8::5", "192.0.2.9, 10.0.0.8"); got != "2001:db8::5" {
		t.Fatalf("trusted CF-Connecting-IP = %q", got)
	}
	if got := security.ClientIP("172.18.0.4:8080", "invalid", "192.0.2.9, 10.0.0.8"); got != "192.0.2.9" {
		t.Fatalf("trusted X-Forwarded-For chain = %q", got)
	}
	mapped := security.IPHash("172.18.0.4:8080", "::ffff:203.0.113.10")
	canonical := security.IPHash("172.18.0.4:8080", "203.0.113.10")
	if len(mapped) != sha256.Size*2 || mapped != canonical || strings.Contains(mapped, "203.0.113.10") {
		t.Fatalf("stable normalized IP hash = %q/%q", mapped, canonical)
	}
	expectedIPMAC := hmac.New(sha256.New, []byte("stable-ip-hash-secret-0123456789abcdef"))
	_, _ = expectedIPMAC.Write([]byte("203.0.113.10"))
	if canonical != hex.EncodeToString(expectedIPMAC.Sum(nil)) {
		t.Fatal("IP hash is not HMAC-SHA256(secret, normalized_ip)")
	}
}
