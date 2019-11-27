package utils

import (
	"strings"
)

const dot = "."

// Dotted adds a trailing dot to a domain if it doesn't exist.
func Dotted(domain string) string {
	if strings.HasSuffix(domain, dot) {
		return domain
	}
	return domain + dot
}

// Undotted removes the trailing dot from a domain if it exists.
func Undotted(domain string) string {
	if !strings.HasSuffix(domain, dot) {
		return domain
	}
	return domain[:len(domain)-1]
}
