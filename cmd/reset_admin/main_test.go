package main

import (
	"strings"
	"testing"
)

func TestLoadAdminCredentialsRequiresExplicitValues(t *testing.T) {
	_, _, err := loadAdminCredentials(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), resetAdminPhoneEnv) || !strings.Contains(err.Error(), resetAdminPasswordEnv) {
		t.Fatalf("loadAdminCredentials() error = %v, want required credentials error", err)
	}
}

func TestLoadAdminCredentialsRejectsWeakPassword(t *testing.T) {
	values := map[string]string{
		resetAdminPhoneEnv:    "18888888888",
		resetAdminPasswordEnv: "123456",
	}
	_, _, err := loadAdminCredentials(func(key string) string { return values[key] })
	if err == nil || !strings.Contains(err.Error(), "at least 12 bytes") {
		t.Fatalf("loadAdminCredentials() error = %v, want weak password error", err)
	}
}

func TestLoadAdminCredentialsRejectsKnownDefaultPassword(t *testing.T) {
	values := map[string]string{
		resetAdminPhoneEnv:    "18888888888",
		resetAdminPasswordEnv: "Admin@123456",
	}
	_, _, err := loadAdminCredentials(func(key string) string { return values[key] })
	if err == nil || !strings.Contains(err.Error(), "known default password") {
		t.Fatalf("loadAdminCredentials() error = %v, want known default password error", err)
	}
}

func TestLoadAdminCredentialsAcceptsExplicitValues(t *testing.T) {
	values := map[string]string{
		resetAdminPhoneEnv:    "18888888888",
		resetAdminPasswordEnv: "ResetPassword123!",
	}
	phone, password, err := loadAdminCredentials(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("loadAdminCredentials() returned error: %v", err)
	}
	if phone != values[resetAdminPhoneEnv] || password != values[resetAdminPasswordEnv] {
		t.Fatalf("credentials = (%q, %q), want configured values", phone, password)
	}
}
