package internalversion

import (
	"time"

	"github.com/docker/go-units"
)

func emptyIfNil(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatRelativeTime(t time.Time) string {
	return units.HumanDuration(timeNowFn().Sub(t))
}

var timeNowFn = func() time.Time {
	return time.Now()
}
