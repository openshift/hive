package installmanager

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/runtime"

	awsclient "github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestUploadLogs(t *testing.T) {
	tests := []struct {
		name                    string
		existing                []runtime.Object
		putObjectError          error
		setupPutObjectMock      bool
		setupEnvVars            bool
		expectedUploadLogsError bool
	}{
		{
			name:                    "missing env vars",
			existing:                []runtime.Object{},
			expectedUploadLogsError: true,
		},
		{
			name:               "successfully upload objects",
			existing:           []runtime.Object{},
			setupPutObjectMock: true,
			setupEnvVars:       true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, test.existing...)

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			// The aws actuator won't run without this set.
			if test.setupEnvVars {
				os.Setenv(constants.InstallLogsUploadProviderEnvVar, constants.InstallLogsUploadProviderAWS)
				os.Setenv(constants.InstallLogsCredentialsSecretRefEnvVar, "notarealsecret")
				os.Setenv(constants.InstallLogsAWSRegionEnvVar, "region1")
				os.Setenv(constants.InstallLogsAWSS3BucketEnvVar, "bucket1")
			}
			if test.setupPutObjectMock {
				mocks.mockAWSClient.EXPECT().
					Upload(gomock.Any()).
					Return(nil, test.putObjectError)
			}

			actuator := &s3LogUploaderActuator{awsClientFn: func(client.Client, string, string, string, log.FieldLogger) (awsclient.Client, error) {
				return mocks.mockAWSClient, nil
			}}
			provision := testClusterProvision()

			// Act
			err := actuator.UploadLogs("notarealcluster", provision, mocks.fakeKubeClient, log.New(), "/etc/issue")

			// Assert
			if test.expectedUploadLogsError {
				assert.Error(t, err, "Function didn't error as expected")
			} else {
				assert.NoError(t, err, "Function errored unexpectedly")
			}

			if test.setupEnvVars {
				os.Unsetenv(constants.InstallLogsUploadProviderEnvVar)
				os.Unsetenv(constants.InstallLogsCredentialsSecretRefEnvVar)
				os.Unsetenv(constants.InstallLogsAWSRegionEnvVar)
				os.Unsetenv(constants.InstallLogsAWSS3BucketEnvVar)
			}
		})
	}
}
