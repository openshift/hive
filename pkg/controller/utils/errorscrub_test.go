package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorScrubber(t *testing.T) {
	cases := []struct {
		name string

		input    error
		expected string
	}{
		{
			name:     "aws request id",
			input:    errors.New("failed to grant creds: error syncing creds in mint-mode: AWS Error: LimitExceeded - LimitExceeded: Cannot exceed quota for UsersPerAccount: 5000\n\tstatus code: 409, request id: 0604c1a4-0a68-4d1a-b8e6-cdcf68176d71"),
			expected: "failed to grant creds: error syncing creds in mint-mode: AWS Error: LimitExceeded - LimitExceeded: Cannot exceed quota for UsersPerAccount: 5000, status code: 409",
		},
		{
			name:     "request id mid",
			input:    errors.New("AWS Error: LimitExceeded - LimitExceeded: Cannot exceed quota for UsersPerAccount: 5000\n\trequest id: 0604c1a4-0a68-4d1a-b8e6-cdcf68176d71, something else"),
			expected: "AWS Error: LimitExceeded - LimitExceeded: Cannot exceed quota for UsersPerAccount: 5000, something else",
		},
		{
			name:     "request id start",
			input:    errors.New("Request id: 0604c1a4-0a68-4d1a-b8e6-cdcf68176d71, something else"), // shouldn't really happen
			expected: ", something else",                                                             // not pretty but what I want to verify happens
		},
		{
			name:     "handle nil",
			input:    nil,
			expected: "",
		},
		{
			name:     "azure error",
			input:    errors.New("azure.BearerAuthorizer#WithAuthorization: Failed to refresh the Token for request to https://management.azure.com/subscriptions/53b8f551-f0fc-4bea-8cba-6d1fefd54c8a/resourceGroups/os4-common/providers/Microsoft.Network/dnsZones/hive-managed.qe.azure.devcluster.openshift.com?api-version=2018-05-01: StatusCode=401 -- Original Error: adal: Refresh request failed. Status Code = '401'. Response body: {\"error\":\"invalid_client\",\"error_description\":\"AADSTS7000215: Invalid client secret is provided.\\r\\nTrace ID: 9a06fe2e-c2af-44eb-b3ec-036929a15701\\r\\nCorrelation ID: ae4130a3-7160-40d5-b67e-b6f4a0dd18a5\\r\\nTimestamp: 2021-07-02 09:28:53Z\",\"error_codes\":[7000215],\"timestamp\":\"2021-07-02 09:28:53Z\",\"trace_id\":\"9a06fe2e-c2af-44eb-b3ec-036929a15701\",\"correlation_id\":\"ae4130a3-7160-40d5-b67e-b6f4a0dd18a5\",\"error_uri\":\"https://login.microsoftonline.com/error?code=7000215"),
			expected: "AADSTS7000215: Invalid client secret is provided.",
		},
		{
			name:     "invalid certificate date",
			input:    errors.New(`[an error on the server ("") has prevented the request from succeeding, Get "https://api.foobar.openshiftapps.com:6443/api?timeout=32s": x509: certificate has expired or is not yet valid: current time 2021-08-06T11:54:02Z is after 2021-07-26T09:26:41Z]`),
			expected: `[an error on the server ("") has prevented the request from succeeding, Get "https://api.foobar.openshiftapps.com:6443/api?timeout=32s": x509: certificate has expired or is not yet valid`,
		},
		{
			name:     "aws not authorized arn",
			input:    errors.New("failed to describe load balancer for the cluster: AccessDenied: User: arn:aws:sts::999999999:assumed-role/RH-Managed-OpenShift-Installer/9999999999999999999 is not authorized to perform: sts:AssumeRole on resource: arn:aws:iam::999999999999:role/ManagedOpenShift-Installer-Role\n\tstatus code: 403, request id: be5fed4c-ba78-43a2-82b7-150bab2b10cc"),
			expected: `failed to describe load balancer for the cluster: AccessDenied: User: arn:aws:sts:: XXX :assumed-role/RH-Managed-OpenShift-Installer/ XXX  is not authorized to perform: sts:AssumeRole on resource: arn:aws:iam:: XXX :role/ManagedOpenShift-Installer-Role, status code: 403`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, ErrorScrub(test.input))
		})
	}
}
