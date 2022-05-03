package alibabacloud

import (
	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"
	"os"
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
