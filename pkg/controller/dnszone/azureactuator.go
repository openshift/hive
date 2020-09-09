package dnszone

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
	if err := a.modifyStatus(); err != nil {
		logger.WithError(err).Error("failed to modify DNSZone status")
		return err
	}
	return nil
}

// Delete implements the Delete call of the actuator interface
func (a *AzureActuator) Delete() error {
	if a.managedZone == nil {
		return errors.New("managedZone is unpopulated")
	}

	resourceGroupName := a.dnsZone.Spec.Azure.ResourceGroupName
	logger := a.logger.WithField("zone", a.dnsZone.Spec.Zone)

	logger.Info("Deleting recordsets in managedzone")
	if err := DeleteAzureRecordSets(a.azureClient, a.dnsZone, logger); err != nil {
		return err
	}

	logger.Info("Deleting managed zone")
	err := a.azureClient.DeleteZone(context.TODO(), resourceGroupName, a.dnsZone.Spec.Zone)
	if err != nil {
		log.WithError(err).Error("Cannot delete managed zone")
	}

	return err
}

// DeleteAzureRecordSets will remove all non-essential records from the DNSZone provided.
func DeleteAzureRecordSets(azureClient azureclient.Client, dnsZone *hivev1.DNSZone, logger log.FieldLogger) error {
	resourceGroupName := dnsZone.Spec.Azure.ResourceGroupName
	zoneName := dnsZone.Spec.Zone
	recordSetsPage, err := azureClient.ListRecordSetsByZone(context.Background(), resourceGroupName, zoneName, "")
	if err != nil {
		return err
	}
	for recordSetsPage.NotDone() {
		for _, recordSet := range recordSetsPage.Values() {
			if recordSet.Name == nil || recordSet.Type == nil {
				logger.Warn("found recordset with missing name or type")
				continue
			}
			name := *recordSet.Name
			// The type comes in as, for example, "Microsoft.Network/dnszones/NS". We need just the last part of that,
			// in this case "NS".
			typeParts := strings.Split(*recordSet.Type, "/")
			recordType := dns.RecordType(typeParts[len(typeParts)-1])
			// Ignore the 2 recordsets that are created with the managed zone and that cannot be deleted
			if name == "@" && (recordType == dns.NS || recordType == dns.SOA) {
				continue
			}
			logger.WithField("name", name).WithField("type", recordType).Info("deleting recordset")
			if err := azureClient.DeleteRecordSet(context.Background(), resourceGroupName, zoneName, name, recordType); err != nil {
				return err
			}
		}
		if err := recordSetsPage.NextWithContext(context.Background()); err != nil {
			return err
		}
	}
	return nil
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

// modifyStatus updates the DnsZone's status with Azure specific information.
func (a *AzureActuator) modifyStatus() error {
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
	logger := a.logger.WithField("zone", zoneName)
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
	if err := a.modifyStatus(); err != nil {
		logger.WithError(err).Error("failed to modify DNSZone status")
		return err
	}
	return nil
}

// UpdateMetadata implements the UpdateMetadata call of the actuator interface
func (a *AzureActuator) UpdateMetadata() error {
	return nil
}

// SetConditionsForError sets conditions on the dnszone given a specific error. Returns true if conditions changed.
func (a *AzureActuator) SetConditionsForError(err error) bool {
	return false // Not implemented for Azure yet.
}
