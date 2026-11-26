package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/trackingkey"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const marketingContentHomePageKey = "home"

type MarketingContentPageLoader interface {
	FindByKey(ctx context.Context, pageKey string) (*models.SitePage, error)
}

// MarketingContentResolver is a read-only current-dimension resolver. It reads
// the English home CMS exactly once per Resolve call and never normalizes,
// generates, or persists tracking keys.
type MarketingContentResolver struct {
	loader MarketingContentPageLoader
}

func NewMarketingContentResolver(loader MarketingContentPageLoader) *MarketingContentResolver {
	return &MarketingContentResolver{loader: loader}
}

func (r *MarketingContentResolver) Resolve(ctx context.Context) (map[string]MarketingContentMetadata, error) {
	metadata := make(map[string]MarketingContentMetadata)
	if r == nil || r.loader == nil {
		return metadata, nil
	}
	page, err := r.loader.FindByKey(ctx, marketingContentHomePageKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return metadata, nil
	}
	if err != nil {
		return nil, err
	}

	// Published metadata wins. Draft content only fills keys that are absent
	// from the current English published JSON.
	mergeMarketingContentMetadata(metadata, marketingContentMetadataFromJSON(page.PublishedContent, true))
	mergeMarketingContentMetadata(metadata, marketingContentMetadataFromJSON(page.DraftContent, false))
	return metadata, nil
}

func mergeMarketingContentMetadata(dst map[string]MarketingContentMetadata, values []MarketingContentMetadata) {
	for _, value := range values {
		if _, exists := dst[value.TrackingKey]; !exists {
			dst[value.TrackingKey] = value
		}
	}
}

func marketingContentMetadataFromJSON(data datatypes.JSON, published bool) []MarketingContentMetadata {
	var root map[string]any
	if len(data) == 0 || json.Unmarshal(data, &root) != nil {
		return nil
	}
	blocks, ok := root["blocks"].([]any)
	if !ok {
		return nil
	}
	result := make([]MarketingContentMetadata, 0, len(blocks))
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		blockType := normalizedMetadataString(block["type"])
		switch blockType {
		case "hero_carousel":
			for _, slide := range marketingContentNodeList(block["slides"]) {
				if value, ok := marketingContentMetadata(slide, ContentTypeHeroSlide, published, "Hero slide", []string{"image"}, []string{"link"}); ok {
					result = append(result, value)
				}
			}
		case "category_nav":
			for _, item := range marketingContentNodeList(block["items"]) {
				if value, ok := marketingContentMetadata(item, ContentTypeCategoryItem, published, "Category item", []string{"image"}, []string{"link"}); ok {
					result = append(result, value)
				}
			}
		case "footer_links":
			for _, group := range marketingContentNodeList(block["items"]) {
				for _, link := range marketingContentNodeList(group["links"]) {
					if value, ok := marketingContentMetadata(link, ContentTypeLink, published, "Link", nil, []string{"link"}); ok {
						result = append(result, value)
					}
				}
			}
		case "hero":
			if value, ok := marketingContentMetadata(block, ContentTypeHero, published, "Hero", []string{"backgroundImage"}, []string{"buttonLink"}); ok {
				result = append(result, value)
			}
		case "hero_banner":
			if value, ok := marketingContentMetadata(block, ContentTypeHeroBanner, published, "Hero banner", []string{"image_url"}, []string{"link_url"}); ok {
				result = append(result, value)
			}
		case "image_text":
			if value, ok := marketingContentMetadata(block, ContentTypeImageText, published, "Image text", []string{"image"}, []string{"buttonLink"}); ok {
				result = append(result, value)
			}
		case "feature_grid":
			if value, ok := marketingContentMetadata(block, ContentTypeFeatureGrid, published, "Feature grid", nil, nil); ok {
				result = append(result, value)
			}
		case "promo_banner":
			if value, ok := marketingContentMetadata(block, ContentTypePromoBanner, published, "Promo banner", []string{"image", "image_url"}, []string{"buttonLink", "link_url"}); ok {
				result = append(result, value)
			}
		case "contact":
			if value, ok := marketingContentMetadata(block, ContentTypeContact, published, "Contact block", nil, nil); ok {
				value.TargetType, value.TargetValue = marketingContactTarget(block)
				result = append(result, value)
			}
		case "image":
			if value, ok := marketingContentMetadata(block, ContentTypeImage, published, "Image", []string{"image"}, nil); ok {
				result = append(result, value)
			}
		case "product_floor", "product_grid", "shop_grid", "text":
			// These current blocks do not emit content_impression/content_click.
		default:
			// Preserve best-effort metadata for a future or historical clickable
			// block without allowing its dynamic type to become a new dimension.
			if value, ok := marketingContentMetadata(block, ContentTypeOther, published, "Marketing content", []string{"image", "image_url", "backgroundImage"}, []string{"link", "buttonLink", "link_url"}); ok {
				result = append(result, value)
			}
		}
	}
	return result
}

func marketingContentMetadata(
	node map[string]any,
	contentType ContentType,
	published bool,
	fallbackTitle string,
	imageFields []string,
	targetFields []string,
) (MarketingContentMetadata, bool) {
	trackingKey := trackingkey.Normalize(normalizedMetadataString(node["tracking_key"]))
	if !trackingkey.IsValid(trackingKey) {
		return MarketingContentMetadata{}, false
	}
	title := firstMetadataString(node, "title", "name", "label", "text", "alt")
	if title == "" {
		title = fallbackTitle
	}
	imageURL := firstMetadataString(node, imageFields...)
	targetType, targetValue := marketingLinkTarget(firstMetadataString(node, targetFields...))
	return MarketingContentMetadata{
		TrackingKey: trackingKey,
		ContentType: contentType,
		Title:       title,
		ImageURL:    imageURL,
		TargetType:  targetType,
		TargetValue: targetValue,
		Published:   published,
	}, true
}

func marketingContentNodeList(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		if node, ok := value.(map[string]any); ok {
			result = append(result, node)
		}
	}
	return result
}

func firstMetadataString(node map[string]any, fields ...string) string {
	for _, field := range fields {
		if value := normalizedMetadataString(node[field]); value != "" {
			return value
		}
	}
	return ""
}

func normalizedMetadataString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func marketingContactTarget(node map[string]any) (ContentTargetType, string) {
	if value := normalizedMetadataString(node["whatsapp"]); value != "" {
		return ContentTargetWhatsApp, value
	}
	if value := normalizedMetadataString(node["phone"]); value != "" {
		return ContentTargetPhone, value
	}
	return ContentTargetNone, ""
}

func marketingLinkTarget(value string) (ContentTargetType, string) {
	value = strings.TrimSpace(value)
	if value == "" || value == "#" {
		return ContentTargetNone, ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "tel:") {
		return ContentTargetPhone, strings.TrimSpace(value[len("tel:"):])
	}
	if strings.Contains(lower, "wa.me/") || strings.Contains(lower, "whatsapp.com/") {
		return ContentTargetWhatsApp, value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ContentTargetNone, ""
	}
	if parsed.IsAbs() || parsed.Host != "" {
		return ContentTargetExternal, value
	}
	path := parsed.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 && (parts[0] == "en" || parts[0] == "zh") {
		parts = parts[1:]
	}
	if len(parts) >= 2 && parts[0] == "products" {
		return ContentTargetProduct, parts[1]
	}
	if len(parts) >= 2 && parts[0] == "shops" {
		return ContentTargetShop, parts[1]
	}
	if categoryID := strings.TrimSpace(parsed.Query().Get("category_id")); categoryID != "" {
		return ContentTargetCategory, categoryID
	}
	if len(parts) >= 2 && (parts[0] == "category" || parts[0] == "categories") {
		return ContentTargetCategory, parts[1]
	}
	if strings.HasPrefix(path, "/") {
		return ContentTargetInternal, value
	}
	return ContentTargetNone, ""
}
