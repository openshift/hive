package dnszone

import (
	"context"
	"errors"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/azureclient"
)

// AzureActuator attempts to make the current state reflect the given desired state.
type AzureActuator struct {
	// logger is the logger used for this controller
	logger log.FieldLogger

	// azureClient is a utility for making it easy for controllers to interface with Azure
	azureClient azureclient.Client

	// dnsZone is the DNSZone that represents the desired state.
	dnsZone *hivev1.DNSZone

	// managedZone is the Azure DNS Managed zone object.
	managedZone *dns.Zone
}

type azureClientBuilderType func(secret *corev1.Secret) (azureclient.Client, error)

// NewAzureActuator creates a new NewAzureActuator object. A new NewAzureActuator is expected to be created for each controller sync.
func NewAzureActuator(
	logger log.FieldLogger,
	secret *corev1.Secret,
	dnsZone *hivev1.DNSZone,
	azureClientBuilder azureClientBuilderType,
) (*AzureActuator, error) {
	azureClient, err := azureClientBuilder(secret)
	if err != nil {
		logger.WithError(err).Error("Error creating AzureClient")
		return nil, err
	}

	azureActuator := &AzureActuator{
		logger:      logger,
		azureClient: azureClient,
		dnsZone:     dnsZone,
	}

	return azureActuator, nil
}

// Ensure AzureActuator implements the Actuator interface. This will fail at compile time when false.
var _ Actuator = &AzureActuator{}

// Create implements the Create call of the actuator interface
func (a *AzureActuator) Create() error {
	logger := a.logger.WithField("zone", a.dnsZone.Spec.Zone)
	logger.Info("Creating managed zone")

	resourceGroupName := a.dnsZone.Spec.Azure.ResourceGroupName

	zone := a.dnsZone.Spec.Zone
	managedZone, err := a.azureClient.CreateOrUpdateZone(context.TODO(), resourceGroupName, zone)
	if err != nil {
		logger.WithError(err).Error("Error creating managed zone")
		return err
	}

	logger.Debug("Managed zone successfully created")
	a.managedZone = &managedZone
	return nil
}

// Delete implements the Delete call of the actuator interface
func (a *AzureActuator) Delete() error {
	if a.managedZone == nil {
		return errors.New("managedZone is unpopulated")
	}

	resourceGroupName := a.dnsZone.Spec.Azure.ResourceGroupName
	logger := a.logger.WithField("zone", a.dnsZone.Spec.Zone).WithField("zoneName", a.managedZone.Name)
	logger.Info("Deleting managed zone")
	err := a.azureClient.DeleteZone(context.TODO(), resourceGroupName, *a.managedZone.Name)
	if err != nil {
		log.WithError(err).Error("Cannot delete managed zone")
	}

	return err
}

// Exists implements the Exists call of the actuator interface
func (a *AzureActuator) Exists() (bool, error) {
	return a.managedZone != nil, nil
}

// GetNameServers implements the GetNameServers call of the actuator interface
func (a *AzureActuator) GetNameServers() ([]string, error) {
	if a.managedZone == nil {
		return nil, errors.New("managedZone is unpopulated")
	}

	logger := a.logger.WithField("zone", a.dnsZone.Spec.Zone)
	result := a.managedZone.NameServers
	logger.WithField("nameservers", result).Debug("found managed zone name servers")
	return *result, nil
}

// ModifyStatus implements the ModifyStatus call of the actuator interface
func (a *AzureActuator) ModifyStatus() error {
	if a.managedZone == nil {
		return errors.New("managedZone is unpopulated")
	}

	return nil
}

// Refresh implements the Refresh call of the actuator interface
func (a *AzureActuator) Refresh() error {

	zoneName := a.dnsZone.Spec.Zone
	resourceGroupName := a.dnsZone.Spec.Azure.ResourceGroupName

	// Fetch the managed zone
	logger := a.logger.WithField("zoneName", zoneName)
	logger.Debug("Fetching managed zone by zone name")
	resp, err := a.azureClient.GetZone(context.TODO(), resourceGroupName, zoneName)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			logger.Debug("Zone not found, clearing out the cached object")
			a.managedZone = nil
			return nil
		}

		logger.WithError(err).Error("Cannot get managed zone")
		return err
	}

	logger.Debug("Found managed zone")
	a.managedZone = &resp
	return nil
}

// UpdateMetadata implements the UpdateMetadata call of the actuator interface
func (a *AzureActuator) UpdateMetadata() error {
	return nil
}
