package installmanager

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"

	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// Ensure s3LogUploaderActuator implements the Actuator interface. This will fail at compile time when false.
var _ LogUploaderActuator = &s3LogUploaderActuator{}

// s3LogUploaderActuator manages getting the desired state, getting the current state and reconciling the two.
type s3LogUploaderActuator struct {
	// awsClientFn is the function to build an AWS client, here for lazy loading the client.
	awsClientFn func(client.Client, string, string, string, log.FieldLogger) (awsclient.Client, error)
}

// IsConfigured returns true if the actuator can handle a particular ClusterDeprovision
func (a *s3LogUploaderActuator) IsConfigured() bool {
	provider, foundProviderEnvVar := os.LookupEnv(constants.InstallLogsUploadProviderEnvVar)
	if !foundProviderEnvVar {
		log.Debug("Couldn't find install logs provider environment variable. Skipping.")
		return false
	}

	return provider == constants.InstallLogsUploadProviderAWS
}

// UploadLogs uploads installer logs to the provider's storage mechanism.
func (a *s3LogUploaderActuator) UploadLogs(clusterName string, clusterprovision *hivev1.ClusterProvision, c client.Client, log log.FieldLogger, filenames ...string) error {
	secretName, foundSecretName := os.LookupEnv(constants.InstallLogsCredentialsSecretRefEnvVar)
	if !foundSecretName {
		return errors.New("couldn't find secret name in environment variable. Skipping upload")
	}

	region, foundRegionEnvVar := os.LookupEnv(constants.InstallLogsAWSRegionEnvVar)
	if !foundRegionEnvVar {
		return errors.New("couldn't find region in environment variable. Skipping upload")
	}

	bucket, foundBucketEnvVar := os.LookupEnv(constants.InstallLogsAWSS3BucketEnvVar)
	if !foundBucketEnvVar {
		return errors.New("couldn't find bucket in environment variable. Skipping upload")
	}

	awsc, err := a.awsClientFn(c, secretName, clusterprovision.Namespace, region, log)
	if err != nil {
		return err
	}

	retvalErrs := []error{}

	folder := fmt.Sprintf("%v-%v", clusterName, clusterprovision.Namespace)

	log.Infof("Uploading log(s) to S3: s3://%v/%v/", bucket, folder)

	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			retvalErrs = append(retvalErrs, errors.Wrapf(err, "Failed opening log file: %v", filename))
			continue
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			retvalErrs = append(retvalErrs, errors.Wrapf(err, "Failed stat on log file: %v", filename))
			continue
		}

		logkey := fmt.Sprintf("%v/%v-%v", folder, clusterprovision.Name, stat.Name())

		_, err = awsc.Upload(&s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(logkey),
			Body:   file,
		})

		if err != nil {
			retvalErrs = append(retvalErrs, errors.Wrapf(err, "Failed uploading log file: %v", filename))
		}
	}

	return utilerrors.NewAggregate(retvalErrs)
}

func getAWSClient(c client.Client, secretName, namespace, region string, logger log.FieldLogger) (awsclient.Client, error) {
	awsClient, err := awsclient.NewClient(c, secretName, namespace, region)
	if err != nil {
		logger.WithError(err).Error("failed to get AWS client")
	}
	return awsClient, err
}
