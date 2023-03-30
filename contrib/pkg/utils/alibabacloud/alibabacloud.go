package alibabacloud

import (
	"os"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetAlibabaCloud Creds reads Alibaba Cloud credentials either from either the specified credentials
// file, the standard environment variables, or a default credentials file. (~/.alibabacloud/credentials)
// The defaultCredsFile will only be used if credsFile is empty and the environment variables are not set.
func GetAlibabaCloud(credsFile, defaultCredsFile string) (string, string, error) {
	credsFilePath := defaultCredsFile
	switch {
	case credsFile != "":
		credsFilePath = credsFile
	default:
		accessKeyID := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
		accessKeySecret := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
		if len(accessKeySecret) > 0 && len(accessKeyID) > 0 {
			return accessKeyID, accessKeySecret, nil
		}
	}
	credFile, err := ini.Load(credsFilePath)
	if err != nil {
		log.Error("Cannot load Alibaba Cloud credentials")
		return "", "", err
	}
	defaultSection, err := credFile.GetSection("default")
	if err != nil {
		log.Error("Cannot get default section from Alibaba Cloud credentials file")
		return "", "", err
	}
	accessKeyIDValue := defaultSection.Key("access_key_id")
	accessKeySecretValue := defaultSection.Key("access_key_secret")
	if accessKeyIDValue == nil || accessKeySecretValue == nil {
		log.Error("Alibaba Cloud credentials file missing keys in default section")
	}
	return accessKeyIDValue.String(), accessKeySecretValue.String(), nil
}

// ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE
// and CREDS_SECRET_NAME and configures alibaba cloud credential environment variables accordingly.
func ConfigureCreds(c client.Client) {
	credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME")
	if credsSecret == nil {
		return
	}
	if id := string(credsSecret.Data[constants.AlibabaCloudAccessKeyIDSecretKey]); id != "" {
		os.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", id)
	}
	if secret := string(credsSecret.Data[constants.AlibabaCloudAccessKeySecretSecretKey]); secret != "" {
		os.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", secret)
	}
	utils.InstallClusterwideProxyCerts(c)
}
