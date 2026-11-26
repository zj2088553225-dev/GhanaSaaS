package service

import (
	"strconv"
	"strings"
)

func BuildProductSlug(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(name, "&", " and ")))
	var builder strings.Builder
	lastHyphen := false

	for _, char := range normalized {
		isAlpha := char >= 'a' && char <= 'z'
		isDigit := char >= '0' && char <= '9'
		if isAlpha || isDigit {
			if builder.Len() < 80 {
				builder.WriteRune(char)
				lastHyphen = false
			}
			continue
		}

		if builder.Len() > 0 && !lastHyphen && builder.Len() < 80 {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "product"
	}
	return slug
}

func BuildProductSEOPathSegment(name string, id uint) string {
	return BuildProductSlug(name) + "-" + strconv.FormatUint(uint64(id), 10)
}
