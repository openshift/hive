package nameserver

import (
	"strings"
)

const dot = "."

func dotted(domain string) string {
	if strings.HasSuffix(domain, dot) {
		return domain
	}
	return domain + dot
}

func undotted(domain string) string {
	if !strings.HasSuffix(domain, dot) {
		return domain
	}
	return domain[:len(domain)-1]
}
