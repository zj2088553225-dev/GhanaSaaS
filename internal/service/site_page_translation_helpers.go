package service

import (
	"strings"

	"saas/internal/models"
)

var sitePageTranslatableTextFields = map[string]struct{}{
	"title":       {},
	"subtitle":    {},
	"description": {},
	"content":     {},
	"buttonText":  {},
	"button_text": {},
	"label":       {},
	"text":        {},
	"name":        {},
	"alt":         {},
	"address":     {},
}

func sitePageContentFromTranslations(rows map[string]models.ContentTranslation, fieldKey string) (SitePageContent, bool) {
	if len(rows) == 0 {
		return SitePageContent{}, false
	}
	row, ok := rows[fieldKey]
	if !ok {
		return SitePageContent{}, false
	}
	content, err := unmarshalSitePageContent(row.ValueJSON)
	if err != nil || !hasSitePageTranslationText(content) {
		return SitePageContent{}, false
	}
	return content, true
}

func mergeLocalizedSitePageContent(base SitePageContent, localized SitePageContent) SitePageContent {
	return sitePageTranslationContentFromBase(base, localized, true)
}

func sitePageTranslationContentFromBase(base SitePageContent, localized SitePageContent, fallbackToBase bool) SitePageContent {
	blocks := make([]map[string]any, 0, len(base.Blocks))
	for index, baseBlock := range base.Blocks {
		var localizedBlock map[string]any
		if index < len(localized.Blocks) {
			localizedBlock = localized.Blocks[index]
			if localizedBlock["type"] != baseBlock["type"] {
				localizedBlock = nil
			}
		}
		blocks = append(blocks, mergeSitePageTranslationNode(baseBlock, localizedBlock, fallbackToBase))
	}
	return SitePageContent{Blocks: blocks}
}

func mergeSitePageTranslationNode(base map[string]any, localized map[string]any, fallbackToBase bool) map[string]any {
	dst := make(map[string]any, len(base))
	for key, baseValue := range base {
		if _, ok := sitePageTranslatableTextFields[key]; ok {
			if text, ok := sitePageNonEmptyString(localized[key]); ok {
				dst[key] = text
			} else if fallbackToBase {
				dst[key] = cloneSitePageTranslationValue(baseValue)
			} else {
				dst[key] = ""
			}
			continue
		}

		switch key {
		case "slides", "items", "links":
			dst[key] = mergeSitePageTranslationNodeList(baseValue, localized[key], fallbackToBase)
		default:
			dst[key] = cloneSitePageTranslationValue(baseValue)
		}
	}
	return dst
}

func mergeSitePageTranslationNodeList(baseValue any, localizedValue any, fallbackToBase bool) []map[string]any {
	baseItems := sitePageMapList(baseValue)
	localizedItems := sitePageMapList(localizedValue)
	items := make([]map[string]any, 0, len(baseItems))
	for index, baseItem := range baseItems {
		var localizedItem map[string]any
		if index < len(localizedItems) {
			localizedItem = localizedItems[index]
		}
		items = append(items, mergeSitePageTranslationNode(baseItem, localizedItem, fallbackToBase))
	}
	return items
}

func hasSitePageTranslationText(content SitePageContent) bool {
	for _, block := range content.Blocks {
		if hasSitePageTranslationTextInNode(block) {
			return true
		}
	}
	return false
}

func hasSitePageTranslationTextInNode(node map[string]any) bool {
	for key, value := range node {
		if _, ok := sitePageTranslatableTextFields[key]; ok {
			if _, hasText := sitePageNonEmptyString(value); hasText {
				return true
			}
			continue
		}
		switch key {
		case "slides", "items", "links":
			for _, child := range sitePageMapList(value) {
				if hasSitePageTranslationTextInNode(child) {
					return true
				}
			}
		}
	}
	return false
}

func sitePageNonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func sitePageMapList(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, mapped)
			}
		}
		return items
	default:
		return []map[string]any{}
	}
}

func cloneSitePageTranslationValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, nested := range typed {
			next[key] = cloneSitePageTranslationValue(nested)
		}
		return next
	case []map[string]any:
		next := make([]map[string]any, 0, len(typed))
		for _, nested := range typed {
			next = append(next, cloneSitePageTranslationValue(nested).(map[string]any))
		}
		return next
	case []any:
		next := make([]any, 0, len(typed))
		for _, nested := range typed {
			next = append(next, cloneSitePageTranslationValue(nested))
		}
		return next
	default:
		return value
	}
}
