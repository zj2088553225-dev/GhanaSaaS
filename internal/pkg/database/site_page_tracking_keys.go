package database

import (
	"bytes"
	"encoding/json"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/trackingkey"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	sitePageTranslationEntityType = "site_page"
	sitePageDraftContentField     = "draft_content"
	sitePagePublishedContentField = "published_content"
)

// BackfillSitePageTrackingKeys gives historical CMS nodes persistent UUIDs in
// one transaction. Published content inherits matching draft keys when it has
// none, and localized JSON always inherits the corresponding English keys.
func BackfillSitePageTrackingKeys(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var pages []models.SitePage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Find(&pages).Error; err != nil {
			return err
		}

		for index := range pages {
			page := &pages[index]
			draft, draftChanged, err := backfillSitePageContentKeys(page.DraftContent, nil, false)
			if err != nil {
				return err
			}
			published, publishedChanged, err := backfillSitePageContentKeys(page.PublishedContent, draft, false)
			if err != nil {
				return err
			}
			updates := map[string]any{}
			if draftChanged {
				updates[sitePageDraftContentField] = draft
			}
			if publishedChanged {
				updates[sitePagePublishedContentField] = published
			}
			if len(updates) > 0 {
				if err := tx.Model(&models.SitePage{}).Where("id = ?", page.ID).UpdateColumns(updates).Error; err != nil {
					return err
				}
			}

			var translations []models.ContentTranslation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("entity_type = ? AND entity_id = ? AND field_key IN ?", sitePageTranslationEntityType, page.ID, []string{sitePageDraftContentField, sitePagePublishedContentField}).
				Find(&translations).Error; err != nil {
				return err
			}
			for translationIndex := range translations {
				translation := &translations[translationIndex]
				base := draft
				if translation.FieldKey == sitePagePublishedContentField {
					base = published
				}
				repaired, changed, err := backfillSitePageContentKeys(translation.ValueJSON, base, true)
				if err != nil {
					return err
				}
				if changed {
					if err := tx.Model(&models.ContentTranslation{}).
						Where("id = ?", translation.ID).
						UpdateColumn("value_json", repaired).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func backfillSitePageContentKeys(data datatypes.JSON, preferred datatypes.JSON, forcePreferred bool) (datatypes.JSON, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return data, false, nil
	}

	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		// Existing malformed CMS JSON retains the pre-existing behavior: it is
		// handled as unavailable by the service rather than blocking startup.
		return data, false, nil
	}
	blocks, ok := content["blocks"].([]any)
	if !ok {
		return data, false, nil
	}

	preferredBlocks := sitePageTrackingNodeList(nil)
	if len(preferred) > 0 {
		var preferredContent map[string]any
		if json.Unmarshal(preferred, &preferredContent) == nil {
			preferredBlocks = sitePageTrackingNodeList(preferredContent["blocks"])
		}
	}

	changed := false
	seen := make(map[string]struct{})
	for index, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		var preferredBlock map[string]any
		if index < len(preferredBlocks) && sitePageTrackingNodesMatch(block, preferredBlocks[index]) {
			preferredBlock = preferredBlocks[index]
		}
		nodeChanged, err := backfillSitePageNodeKey(block, preferredBlock, forcePreferred, seen)
		if err != nil {
			return nil, false, err
		}
		changed = changed || nodeChanged
	}
	if !changed {
		return data, false, nil
	}
	repaired, err := json.Marshal(content)
	return datatypes.JSON(repaired), true, err
}

func backfillSitePageNodeKey(
	node map[string]any,
	preferred map[string]any,
	forcePreferred bool,
	seen map[string]struct{},
) (bool, error) {
	changed := false
	rawCurrent, currentIsString := node["tracking_key"].(string)
	current := trackingkey.Normalize(rawCurrent)
	preferredKey := ""
	if preferred != nil {
		preferredKey, _ = preferred["tracking_key"].(string)
		preferredKey = trackingkey.Normalize(preferredKey)
	}

	nextKey := current
	if trackingkey.IsValid(preferredKey) && (forcePreferred || !trackingkey.IsValid(current)) {
		nextKey = preferredKey
	}
	if !trackingkey.IsValid(nextKey) {
		generated, err := trackingkey.New()
		if err != nil {
			return false, err
		}
		nextKey = generated
	}
	if _, duplicate := seen[nextKey]; duplicate {
		if _, preferredAlreadyUsed := seen[preferredKey]; trackingkey.IsValid(preferredKey) && !preferredAlreadyUsed {
			nextKey = preferredKey
		} else {
			generated, err := trackingkey.New()
			if err != nil {
				return false, err
			}
			nextKey = generated
		}
	}
	if !currentIsString || rawCurrent != nextKey {
		node["tracking_key"] = nextKey
		changed = true
	}
	seen[nextKey] = struct{}{}

	for _, field := range []string{"slides", "items", "links"} {
		children := sitePageTrackingNodeList(node[field])
		preferredChildren := sitePageTrackingNodeList(nil)
		if preferred != nil {
			preferredChildren = sitePageTrackingNodeList(preferred[field])
		}
		for index, child := range children {
			var preferredChild map[string]any
			if index < len(preferredChildren) && sitePageTrackingNodesMatch(child, preferredChildren[index]) {
				preferredChild = preferredChildren[index]
			}
			childChanged, err := backfillSitePageNodeKey(child, preferredChild, forcePreferred, seen)
			if err != nil {
				return false, err
			}
			changed = changed || childChanged
		}
	}
	return changed, nil
}

func sitePageTrackingNodeList(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	nodes := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if node, ok := item.(map[string]any); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func sitePageTrackingNodesMatch(left map[string]any, right map[string]any) bool {
	leftType, _ := left["type"].(string)
	rightType, _ := right["type"].(string)
	if strings.TrimSpace(leftType) == "" && strings.TrimSpace(rightType) == "" {
		return true
	}
	return strings.TrimSpace(leftType) == strings.TrimSpace(rightType)
}
