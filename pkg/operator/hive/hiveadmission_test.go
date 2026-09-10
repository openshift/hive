package hive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTLSCurvePreferencesArg(t *testing.T) {
	log := testLogger()
	cases := []struct {
		name   string
		groups []string
		want   string
		ok     bool
	}{
		{
			name: "empty omits flag",
		},
		{
			name:   "named profile groups",
			groups: []string{"X25519MLKEM768", "X25519", "secp256r1", "secp384r1"},
			want:   "--tls-curve-preferences=4588,29,23,24",
			ok:     true,
		},
		{
			name:   "drops unrecognized",
			groups: []string{"not-a-group", "secp256r1"},
			want:   "--tls-curve-preferences=23",
			ok:     true,
		},
		{
			name:   "all unrecognized omits flag",
			groups: []string{"not-a-group"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tlsCurvePreferencesArg(tc.groups, log)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
