package utils

import (
	"regexp"
)

var (
	newlineTabRE           = regexp.MustCompile(`\n\t`)
	certificateTimeErrorRE = regexp.MustCompile(`: current time \S+ is after \S+`)
	// aws
	awsRequestIDRE = regexp.MustCompile(`(, )*(?i)(request id: )(?:[-[:xdigit:]]+)`)
	// azure
	azureErrorDescriptionRE = regexp.MustCompile(`\"error_description\":\"(.*?)\\r\\n`)
)

// ErrorScrub scrubs cloud error messages destined for CRD status to remove things that
// change every attempt, such as request IDs, which subsequently cause an infinite update/reconcile loop.
func ErrorScrub(err error) string {
	if err == nil {
		return ""
	}
	s := newlineTabRE.ReplaceAllString(err.Error(), ", ")
	s = awsRequestIDRE.ReplaceAllString(s, "")
	s = certificateTimeErrorRE.ReplaceAllString(s, "")
	// if Azure error, return just the error description
	match := azureErrorDescriptionRE.FindStringSubmatch(s)
	if len(match) > 0 {
		return match[1]
	}
	return s
}
