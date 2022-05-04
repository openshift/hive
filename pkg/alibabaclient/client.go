package alibabaclient

import (
	"fmt"
	"github.com/openshift/hive/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/endpoints"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/pkg/errors"
)

const (
	envCredentialFile = "ALIBABA_CLOUD_CREDENTIALS_FILE"
)

// Credential configuration file template.
const configurationTemplate = `
[default]              
enable = true                    
type = access_key                
access_key_id = %s              
access_key_secret = %s
`

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// API interface represent the calls made to the Alibaba Cloud API.
type API interface {
	DescribeAvailableZoneByInstanceType(string) (*ecs.DescribeAvailableResourceResponse, error)
	GetAvailableZonesByInstanceType(string) ([]string, error)
}

// Client makes calls to the Alibaba Cloud API.
type Client struct {
	sdk.Client
	RegionID        string
	AccessKeyID     string
	AccessKeySecret string
}

func NewClientFromSecret(secret *corev1.Secret, regionID string) (API, error) {
	accessKeyID, ok := secret.Data[constants.AlibabaCloudAccessKeyIDSecretKey]
	if !ok {
		return nil, errors.New("creds secret does not contain \"" + constants.AlibabaCloudAccessKeyIDSecretKey + "\" data")
	}

	accessKeySecret, ok := secret.Data[constants.AlibabaCloudAccessKeySecretSecretKey]
	if !ok {
		return nil, errors.New("creds secret does not contain \"" + constants.AlibabaCloudAccessKeySecretSecretKey + "\" data")
	}

	credentials := credentials.NewAccessKeyCredential(string(accessKeyID), string(accessKeySecret))
	config := sdk.NewConfig()

	return newClientWithOptions(regionID, config, credentials)
}

func newClientWithOptions(regionID string, config *sdk.Config, credential auth.Credential) (client *Client, err error) {
	client = &Client{
		RegionID: regionID,
	}
	err = client.InitWithOptions(regionID, config, credential)
	return
}

func (client *Client) doActionWithSetDomain(request requests.AcsRequest, response responses.AcsResponse) (err error) {
	endpoint := client.getEndpoint(request)
	request.SetDomain(endpoint)
	err = client.DoAction(request, response)
	return
}

// DescribeAvailableZoneByInstanceType queries available zone by instance type of ECS.
func (client *Client) DescribeAvailableZoneByInstanceType(instanceType string) (response *ecs.DescribeAvailableResourceResponse, err error) {
	request := ecs.CreateDescribeAvailableResourceRequest()
	request.RegionId = client.RegionID
	request.DestinationResource = "InstanceType"
	request.InstanceType = instanceType
	response = &ecs.DescribeAvailableResourceResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	err = client.doActionWithSetDomain(request, response)
	return
}

// GetAvailableZonesByInstanceType returns a list of available zones with the specified instance type is
// available and stock
func (client *Client) GetAvailableZonesByInstanceType(instanceType string) ([]string, error) {
	response, err := client.DescribeAvailableZoneByInstanceType(instanceType)
	if err != nil {
		return nil, err
	}

	var zones []string

	for _, zone := range response.AvailableZones.AvailableZone {
		if zone.Status == "Available" && zone.StatusCategory == "WithStock" {
			zones = append(zones, zone.ZoneId)
		}
	}
	return zones, nil
}

func (client *Client) getEndpoint(request requests.AcsRequest) string {
	productName := strings.ToLower(request.GetProduct())
	regionID := strings.ToLower(client.RegionID)

	if additionEndpoint, ok := additionEndpoint(productName, regionID); ok {
		return additionEndpoint
	}

	endpoint, err := endpoints.Resolve(&endpoints.ResolveParam{
		LocationProduct:      request.GetLocationServiceCode(),
		LocationEndpointType: request.GetLocationEndpointType(),
		Product:              productName,
		RegionId:             regionID,
		CommonApi:            client.ProcessCommonRequest,
	})

	if err != nil {
		endpoint = defaultEndpoint()[productName]
	}

	return endpoint
}

func defaultEndpoint() map[string]string {
	return map[string]string{
		"pvtz":            "pvtz.aliyuncs.com",
		"resourcemanager": "resourcemanager.aliyuncs.com",
		"ecs":             "ecs.aliyuncs.com",
	}
}

func additionEndpoint(productName string, regionID string) (string, bool) {
	endpoints := map[string]map[string]string{
		"ecs": {
			"cn-wulanchabu":  "ecs.cn-wulanchabu.aliyuncs.com",
			"cn-guangzhou":   "ecs.cn-guangzhou.aliyuncs.com",
			"ap-southeast-6": "ecs.ap-southeast-6.aliyuncs.com",
			"cn-heyuan":      "ecs.cn-heyuan.aliyuncs.com",
			"cn-chengdu":     "ecs.cn-chengdu.aliyuncs.com",
		},
	}
	if regionEndpoints, ok := endpoints[productName]; ok {
		if endpoint, ok := regionEndpoints[regionID]; ok {
			return endpoint, true
		}
	}
	return "", false
}

func storeCredentials(accessKeyID string, accessKeySecret string) (err error) {
	dirPath, ok := os.LookupEnv(envCredentialFile)
	if !ok || dirPath == "" {
		user, err := user.Current()
		if err != nil {
			return err
		}
		dirPath = user.HomeDir
	}

	dirPath = filepath.Join(dirPath, ".alibabacloud")
	err = os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dirPath, "credentials")

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer file.Close()
	file.WriteString(fmt.Sprintf(configurationTemplate, accessKeyID, accessKeySecret))

	return nil
}
