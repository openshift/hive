package utils

import (
	"regexp"
)

var (
	newlineTabRE           = regexp.MustCompile(`\n\t`)
	certificateTimeErrorRE = regexp.MustCompile(`: current time \S+ is after \S+`)
	// aws
	awsRequestIDRE             = regexp.MustCompile(`(, )*(?i)(request ?id: )(?:[-[:xdigit:]]+)`)
	awsNotAuthorized           = regexp.MustCompile(`(User: arn:aws:sts::)\S+(:assumed-role/[^/]+/)\S+( is not authorized to perform: \S+ on resource: arn:aws:iam::)[^:]+(:\S+)`)
	awsNotAuthorizedNoResource = regexp.MustCompile(`(User: arn:aws:sts::)\S+(:assumed-role/[^/]+/)\S+( is not authorized to perform: \S)`)
	awsEncodedMessage          = regexp.MustCompile(`(Encoded authorization failure message: )[^,]+,`)
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
	s = awsNotAuthorized.ReplaceAllString(s, `${1}XXX${2}XXX${3}XXX${4}`)
	// Place the regex scrub for the AWS NotAuthorized error string without the resource below the NotAuthorized one,
	// because awsNotAuthorizedNoResource is essentially a substring of awsNotAuthorized
	s = awsNotAuthorizedNoResource.ReplaceAllString(s, `${1}XXX${2}XXX${3}`)
	s = awsEncodedMessage.ReplaceAllString(s, "${1}XXX,")
	s = certificateTimeErrorRE.ReplaceAllString(s, "")
	// if Azure error, return just the error description
	match := azureErrorDescriptionRE.FindStringSubmatch(s)
	if len(match) > 0 {
		return match[1]
	}
	return s
}
