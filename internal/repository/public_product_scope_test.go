package repository

import (
	"reflect"
	"strings"
	"testing"

	"saas/internal/models"
)

func TestPublicProductSourceConditionUsesConfiguredPlatformMallTenantID(t *testing.T) {
	condition, args := publicProductSourceCondition("spus", "tenants", ProductSourceT6, 42)

	if condition != "tenants.id = ?" {
		t.Fatalf("condition = %q, want tenant ID condition", condition)
	}
	if !reflect.DeepEqual(args, []any{uint(42)}) {
		t.Fatalf("args = %#v, want configured tenant ID", args)
	}
}

func TestPublicProductSourceConditionFallsBackToPlatformNameRule(t *testing.T) {
	condition, args := publicProductSourceCondition("spus", "tenants", ProductSourceT6, 0)

	if !strings.Contains(condition, "tenants.type = ?") || !strings.Contains(condition, "tenants.name = ?") {
		t.Fatalf("condition = %q, want fallback self-operated name rule", condition)
	}
	wantArgs := []any{models.TenantTypeSelfOperated, models.PlatformMallTenantName}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestPublicProductSourceConditionExcludesConfiguredT6FromMerchantScope(t *testing.T) {
	condition, args := publicProductSourceCondition("spus", "tenants", ProductSourceMerchant, 7)

	if !strings.Contains(condition, "NOT (tenants.id = ?)") {
		t.Fatalf("condition = %q, want merchant scope to exclude configured T6 tenant", condition)
	}
	if !strings.Contains(condition, "shop_profiles.tenant_id = spus.tenant_id") {
		t.Fatalf("condition = %q, want merchant scope to require public shop profile", condition)
	}
	if !reflect.DeepEqual(args, []any{uint(7), true}) {
		t.Fatalf("args = %#v, want configured tenant ID and public shop flag", args)
	}
}
