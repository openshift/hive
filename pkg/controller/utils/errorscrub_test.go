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
			expected: `failed to describe load balancer for the cluster: AccessDenied: User: arn:aws:sts::XXX:assumed-role/RH-Managed-OpenShift-Installer/XXX is not authorized to perform: sts:AssumeRole on resource: arn:aws:iam::XXX:role/ManagedOpenShift-Installer-Role, status code: 403`,
		},
		{
			name:     "aws not authorized arn no resource",
			input:    errors.New("failed to describe load balancer for the cluster: AccessDenied: User: arn:aws:sts::999999999:assumed-role/ManagedOpenShift-Installer-Role/9999999999999999999 is not authorized to perform: elasticloadbalancing:DescribeLoadBalancers with an explicit deny in an identity-based policy\n\tstatus code: 403, request id: be5fed4c-ba78-43a2-82b7-150bab2b10cc"),
			expected: `failed to describe load balancer for the cluster: AccessDenied: User: arn:aws:sts::XXX:assumed-role/ManagedOpenShift-Installer-Role/XXX is not authorized to perform: elasticloadbalancing:DescribeLoadBalancers with an explicit deny in an identity-based policy, status code: 403`,
		},
		{
			name:     "aws encoded msg",
			input:    errors.New("failed to reconcile the VPC Endpoint Service: UnauthorizedOperation: You are not authorized to perform this operation. Encoded authorization failure message: CnWuhDtDU8FQVg9qYvdTV1sflohKgfEbSt7r5FKOlWASj-sZd_ThxMgw1vWPaA03t0q-4YxC3ixYib1CpgUy5OxR9rvuTQ24D_CvTl9ZwqTqdG7OZ49nzPRBuYkL6WYrh_m40GVqa7dyZnhdsFADPpGbFngWx6whQqOU83hpGcYUtFuWmuHlMAz8sPsQHviCHZ9WgIWoTS8Rqjpf2BQWKknnF7IVfkAlyhZJGoPNZ5RUxgphGniUfk0G_2mEY8VmVLwzhqAzbJE_GYednXtMfLX9ctEVd1G2c-2bW4lrQpvjxVaZ92oKXhtfkT5zEI_7sw4fa8yqrxfnE6n4XG2M2CqpaT2HphDc8LJpg7lZXhpCCkunG7wtCAiQvF4NwWqENxlCd_k-E_d1FaIEMaUSAoOUXcjJx8W-ObZy0qst64dQPg-kPNHbrnwMh4DGfppWEPaQgLBOa9LN3xkD-5r6fjT5yJWGcTiKYAKjBiW0KAiwULI11_8Ty9l-QY1OMaudbv_0drhKs766YsbTlMTBWZ37trRzMTWmdCCntilfAVjVHWpdNhNlGwvWPB8DMs38I4zONqCUhw8WVbJ0B-b9BgmkD0rpz9-uQ9i3yh8In-YrFgcXy6CgJpKqYnQjy2QhLwJvDLVgf4eckchR7TkDWkjjrXcQ-15aWOnhtUnyRU6MaPRpCwFCj9xigNmVOQfmK63YeoxT9a1SF7XeJA\n\tstatus code: 403, request id: XXX"),
			expected: `failed to reconcile the VPC Endpoint Service: UnauthorizedOperation: You are not authorized to perform this operation. Encoded authorization failure message: XXX, status code: 403, request id: XXX`,
		},
		{
			name:     "suddenly it is spelled RequestID",
			input:    errors.New("operation error Resource Groups Tagging API: GetResources, https response error StatusCode: 400, RequestID: 032cc7f0-b1a6-4183-bdbb-a15a23a9e029, api error UnrecognizedClientException: The security token included in the request is invalid."),
			expected: `operation error Resource Groups Tagging API: GetResources, https response error StatusCode: 400, api error UnrecognizedClientException: The security token included in the request is invalid.`,
		},
		{
			name:     "two request IDs, spelled differently",
			input:    errors.New(`AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; Request ID: 42a5a4ce-9c1a-4916-a62a-72a2e6d9ae59; Proxy: null)\n\tstatus code: 403, request id: 9cc3b1f9-e161-402c-a942-d0ed7c7e5fd4`),
			expected: `AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; ; Proxy: null)\n\tstatus code: 403`,
		},
		{
			name: "embedded newline",
			input: errors.New(`AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; Request ID: 42a5a4ce-9c1a-4916-a62a-72a2e6d9ae59; Proxy: null)
		status code: 403, request id: 9cc3b1f9-e161-402c-a942-d0ed7c7e5fd4`),
			expected: `AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; ; Proxy: null), 	status code: 403`,
		},
		{
			name:     "request id is at the end",
			input:    errors.New(`AccessDenied: User: arn:aws:iam::12345:user/test-user is not authorized to perform: route53:ChangeResourceRecordSets on resource: arn:aws:route53:::hostedzone/12345\n\tstatus code: 403, request id: 22bc2e2e-9381-485f-8a46-c7ce8aad2a4d`),
			expected: `AccessDenied: User: arn:aws:iam::12345:user/test-user is not authorized to perform: route53:ChangeResourceRecordSets on resource: arn:aws:route53:::hostedzone/12345\n\tstatus code: 403`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, ErrorScrub(test.input))
		})
	}
}
