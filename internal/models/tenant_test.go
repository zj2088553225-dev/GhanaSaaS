package models

import "testing"

func TestIsPlatformMallTenantWithIDPrefersConfiguredID(t *testing.T) {
	configuredTenant := Tenant{
		BaseModel: BaseModel{ID: 99},
		Name:      "Renamed Production Mall",
		Type:      TenantTypeSelfOperated,
	}
	otherSelfOperated := Tenant{
		BaseModel: BaseModel{ID: 100},
		Name:      "Another Self Operated Store",
		Type:      TenantTypeSelfOperated,
	}

	if !IsPlatformMallTenantWithID(configuredTenant, 99) {
		t.Fatalf("configured tenant was not classified as platform mall")
	}
	if IsPlatformMallTenantWithID(otherSelfOperated, 99) {
		t.Fatalf("non-configured self-operated tenant was classified as platform mall")
	}
}

func TestIsPlatformMallTenantWithIDFallsBackToNameRule(t *testing.T) {
	tenant := Tenant{
		BaseModel: BaseModel{ID: 1},
		Name:      PlatformMallTenantName,
		Type:      TenantTypeSelfOperated,
	}

	if !IsPlatformMallTenantWithID(tenant, 0) {
		t.Fatalf("fallback platform mall tenant was not classified")
	}
}
