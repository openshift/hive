package remoteclient

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

type dialerMock func(ctx context.Context, network, address string)

func (d dialerMock) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var c net.Conn
	d(ctx, network, address)
	return c, nil
}

func Test_createDialContext(t *testing.T) {
	for _, tc := range []struct {
		name                string
		apiServerIPOverride string
		dialNetwork         string
		dialAddress         string
		expectedAddress     string
		expectedErr         string
	}{
		{
			name:                "replace host ip",
			apiServerIPOverride: "10.0.4.6",
			dialAddress:         "1.1.1.1:6443",
			dialNetwork:         "tcp",
			expectedAddress:     "10.0.4.6:6443",
		},
		{
			name:                "invalid address",
			apiServerIPOverride: "10.0.4.6",
			dialAddress:         "1.1.1.1",
			dialNetwork:         "tcp",
			expectedErr:         "address 1.1.1.1: missing port in address",
		},
		{
			name:                "unsupported network",
			apiServerIPOverride: "10.0.4.6",
			dialAddress:         "1.1.1.1:6443",
			dialNetwork:         "udp",
			expectedErr:         "unimplemented network \"udp\"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testCtx := context.WithValue(context.Background(), struct{ nonEmptyCtx string }{}, 1)

			mockDialer := dialerMock(func(ctx context.Context, network, address string) {
				assert.Equal(t, testCtx, ctx, "unexpected context")
				assert.Equal(t, tc.dialNetwork, network, "network")
				assert.Equal(t, tc.expectedAddress, address, "unexpected address")
			})

			dial := createDialContext(mockDialer, tc.apiServerIPOverride)

			_, err := dial(testCtx, tc.dialNetwork, tc.dialAddress)
			if tc.expectedErr != "" && assert.Error(t, err, "expected error dialing") {
				assert.Equal(t, tc.expectedErr, err.Error(), "unexpected dial error")
			} else {
				assert.NoError(t, err, "")
			}
		})
	}
}
