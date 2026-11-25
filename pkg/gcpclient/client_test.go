package gcpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func TestCreateServiceAttachmentError(t *testing.T) {
	const (
		insertPath = "/projects/test-project/regions/test-region/serviceAttachments"
		waitPath   = "/projects/test-project/regions/test-region/operations/test-op/wait"

		message       = "Forwarding rule test-rule is already in use"
		operationFail = `{"status":"DONE","error":{"errors":[{"code":"TEST_CONFLICT","message":"` + message + `"}]}}`
	)

	cases := []struct {
		name         string
		insertStatus int
		insertBody   string
		waitBody     string
		wantError    string
	}{
		{
			name:         "HTTP error from insert",
			insertStatus: http.StatusBadRequest,
			insertBody:   `{"error":{"code":400,"message":"` + message + `"}}`,
			wantError:    "googleapi: Error 400: " + message,
		},
		{
			name:         "insert returns a failed operation",
			insertStatus: http.StatusOK,
			insertBody:   operationFail,
			wantError:    "operation completed with error: TEST_CONFLICT: " + message,
		},
		{
			name:         "operation fails after wait",
			insertStatus: http.StatusOK,
			insertBody:   `{"name":"test-op","status":"RUNNING"}`,
			waitBody:     operationFail,
			wantError:    "operation completed with error: TEST_CONFLICT: " + message,
		},
		{
			name:         "operation error without a message",
			insertStatus: http.StatusOK,
			insertBody:   `{"status":"DONE","error":{"errors":[{"code":"TEST_CONFLICT"}]}}`,
			wantError:    "operation completed with error: TEST_CONFLICT",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == insertPath:
					w.WriteHeader(tc.insertStatus)
					_, _ = io.WriteString(w, tc.insertBody)
				case r.Method == http.MethodPost && r.URL.Path == waitPath && tc.waitBody != "":
					_, _ = io.WriteString(w, tc.waitBody)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			service, err := compute.NewService(
				context.Background(),
				option.WithEndpoint(server.URL+"/"),
				option.WithoutAuthentication(),
			)
			require.NoError(t, err)

			client := &gcpClient{projectName: "test-project", computeClient: service}
			attachment, err := client.CreateServiceAttachment(
				"test-attachment", "test-region", "test-rule", []string{"test-subnet"}, nil,
			)

			require.EqualError(t, err, tc.wantError)
			require.Nil(t, attachment)
		})
	}
}
