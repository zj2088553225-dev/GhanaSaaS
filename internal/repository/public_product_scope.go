package repository

import (
	"fmt"

	"saas/internal/models"
)

type PublicProductScopeOptions struct {
	PlatformMallTenantID uint
}

func publicProductSourceCondition(spuAlias string, tenantAlias string, source ProductSource, platformMallTenantID uint) (string, []any) {
	platformCondition, platformArgs := platformMallTenantCondition(tenantAlias, platformMallTenantID)
	publicShopCondition := publicShopTenantCondition(spuAlias)

	switch source {
	case ProductSourceT6:
		return platformCondition, platformArgs
	case ProductSourceMerchant:
		args := append([]any{}, platformArgs...)
		args = append(args, true)
		return "NOT (" + platformCondition + ") AND " + publicShopCondition, args
	default:
		args := append([]any{}, platformArgs...)
		args = append(args, true)
		return "(" + platformCondition + " OR " + publicShopCondition + ")", args
	}
}

func platformMallTenantCondition(tenantAlias string, platformMallTenantID uint) (string, []any) {
	if platformMallTenantID > 0 {
		return fmt.Sprintf("%s.id = ?", tenantAlias), []any{platformMallTenantID}
	}
	return fmt.Sprintf("(%s.type = ? AND %s.name = ?)", tenantAlias, tenantAlias), []any{models.TenantTypeSelfOperated, models.PlatformMallTenantName}
}

func publicShopTenantCondition(spuAlias string) string {
	return fmt.Sprintf("EXISTS (SELECT 1 FROM shop_profiles WHERE shop_profiles.tenant_id = %s.tenant_id AND shop_profiles.is_public = ?)", spuAlias)
}
