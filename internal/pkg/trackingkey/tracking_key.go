package trackingkey

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func IsValid(value string) bool {
	return uuidPattern.MatchString(strings.TrimSpace(value))
}

func Normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func New() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return Format(value), nil
}

func Format(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
