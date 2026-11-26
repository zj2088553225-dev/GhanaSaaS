package repository

import (
	"fmt"
	"strings"

	"saas/internal/models"
	"saas/internal/pkg/categorysearch"

	"gorm.io/gorm"
)

const platformCategoryRepositorySearchMaxLimit = 100

type platformCategorySearchMatch struct {
	CategoryID uint `gorm:"column:category_id"`
}

func searchPlatformCategoryIDs(
	db *gorm.DB,
	query categorysearch.Query,
	limit int,
	activeOnly bool,
	locale string,
) ([]uint, error) {
	statement, arguments := buildPlatformCategorySearchMatchQuery(
		db.Dialector.Name(),
		query,
		limit,
		activeOnly,
		locale,
	)
	var matches []platformCategorySearchMatch
	if err := db.Raw(statement, arguments...).Scan(&matches).Error; err != nil {
		return nil, fmt.Errorf("search platform category index: %w", err)
	}
	categoryIDs := make([]uint, 0, len(matches))
	for _, match := range matches {
		categoryIDs = append(categoryIDs, match.CategoryID)
	}
	return categoryIDs, nil
}

func buildPlatformCategorySearchMatchQuery(
	dialect string,
	query categorysearch.Query,
	limit int,
	activeOnly bool,
	locale string,
) (string, []any) {
	if limit <= 0 || limit > platformCategoryRepositorySearchMaxLimit {
		limit = platformCategoryRepositorySearchMaxLimit
	}
	like := func(column string) string {
		if dialect == "postgres" {
			return column + ` ILIKE ? ESCAPE E'\\'`
		}
		return "LOWER(" + column + `) LIKE ? ESCAPE '\'`
	}
	containsPattern := func(value string) string {
		return "%" + categorysearch.EscapeLikePattern(value) + "%"
	}

	rankConditions := make([]string, 0, len(query.Tokens))
	arguments := make([]any, 0, len(query.Tokens)*2+8)
	arguments = append(arguments, query.Normalized, query.Tokens[0])
	arguments = append(arguments, categorysearch.EscapeLikePattern(query.Normalized)+"%")
	for _, token := range query.Tokens {
		rankConditions = append(rankConditions, like("search_index.name"))
		arguments = append(arguments, containsPattern(token))
	}

	where := make([]string, 0, len(query.Tokens)+2)
	switch locale {
	case "en", "zh":
		where = append(where, "search_index.locale = ?")
		arguments = append(arguments, locale)
	default:
		where = append(where, "search_index.locale IN (?, ?)")
		arguments = append(arguments, "en", "zh")
	}
	if activeOnly {
		where = append(where, "search_index.status = ?")
		arguments = append(arguments, models.PlatformCategoryStatusActive)
	}
	for _, token := range query.Tokens {
		where = append(where, like("search_index.search_text"))
		arguments = append(arguments, containsPattern(token))
	}
	arguments = append(arguments, limit)

	statement := `
		WITH category_matches AS (
			SELECT
				search_index.category_id,
				MIN(CASE
					WHEN LOWER(TRIM(search_index.name)) = ? THEN 0
					WHEN LOWER(TRIM(search_index.name)) = ? THEN 1
					WHEN ` + like("search_index.name") + ` THEN 2
					WHEN ` + strings.Join(rankConditions, " AND ") + ` THEN 3
					ELSE 4
				END) AS best_rank,
				MIN(search_index.level) AS level,
				MIN(search_index.sort_order) AS sort_order
			FROM platform_category_search_indexes AS search_index
			WHERE ` + strings.Join(where, " AND ") + `
			GROUP BY search_index.category_id
		)
		SELECT category_id
		FROM category_matches
		ORDER BY best_rank ASC, level ASC, sort_order ASC, category_id ASC
		LIMIT ?
	`
	return statement, arguments
}
