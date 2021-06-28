package utils

import (
	"regexp"
)

var (
	awsRequestIDRE = regexp.MustCompile(`(, )*(?i)(request id: )(?:[-[:xdigit:]]+)`)
	newlineTabRE   = regexp.MustCompile(`(\n\t)`)
)

// ErrorScrub scrubs cloud error messages destined for CRD status to remove things that
// change every attempt, such as request IDs, which subsequently cause an infinite update/reconcile loop.
func ErrorScrub(err error) string {
	s := newlineTabRE.ReplaceAllString(err.Error(), ", ")
	s = awsRequestIDRE.ReplaceAllString(s, "")
	return s
}
