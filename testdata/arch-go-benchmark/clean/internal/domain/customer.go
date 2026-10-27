package domain

import "strings"

func Normalize(name string) string {
	return strings.TrimSpace(name)
}
