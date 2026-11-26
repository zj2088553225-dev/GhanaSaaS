package locale

import "strings"

const DefaultLocale = "en"

var SupportedLocales = []string{"en", "zh"}

func NormalizeLocale(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if IsSupportedLocale(normalized) {
		return normalized
	}
	return DefaultLocale
}

func IsSupportedLocale(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "en", "zh":
		return true
	default:
		return false
	}
}
