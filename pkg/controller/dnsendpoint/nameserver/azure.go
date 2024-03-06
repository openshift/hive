package nameserver

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/azureclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	defaultCallTimeout = 30 * time.Second
)

func contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultCallTimeout)
}

// NewAzureQuery creates a new name server query for Azure.
func NewAzureQuery(c client.Client, credsSecretName, resourceGroupName, cloudName string) Query {
	return &azureQuery{
		getAzureClient: func() (azureclient.Client, error) {
			credsSecret := &corev1.Secret{}
			if err := c.Get(
				context.Background(),
				client.ObjectKey{Namespace: controllerutils.GetHiveNamespace(), Name: credsSecretName},
				credsSecret,
			); err != nil {
				return nil, errors.Wrap(err, "could not get the creds secret")
			}
			azureClient, err := azureclient.NewClientFromSecret(credsSecret, cloudName)
			return azureClient, errors.Wrap(err, "error creating Azure client")
		},
		resourceGroupName: resourceGroupName,
	}
}

type azureQuery struct {
	getAzureClient    func() (azureclient.Client, error)
	resourceGroupName string
}

var _ Query = (*azureQuery)(nil)

// Get implements Query.Get.
func (q *azureQuery) Get(domain string) (map[string]sets.Set[string], error) {
	azureClient, err := q.getAzureClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get azure client")
	}
	currentNameServers, err := q.queryNameServers(azureClient, domain)
	return currentNameServers, errors.Wrap(err, "error querying name servers")
}

// CreateOrUpdate implements Query.Create.
func (q *azureQuery) CreateOrUpdate(rootDomain string, domain string, values sets.Set[string]) error {
	azureClient, err := q.getAzureClient()
	if err != nil {
		return errors.Wrap(err, "failed to get Azure client")
	}

	return errors.Wrap(q.createNameServers(azureClient, rootDomain, domain, values), "error creating the name server")
}

// Delete implements Query.Delete.
func (q *azureQuery) Delete(rootDomain string, domain string, values sets.Set[string]) error {
	azureClient, err := q.getAzureClient()
	if err != nil {
		return errors.Wrap(err, "failed to get Azure client")
	}

	return errors.Wrap(q.deleteNameServers(azureClient, rootDomain, domain),
		"error deleting the name servers",
	)
}

// deleteNameServers deletes the name servers for the specified domain in the specified managed zone.
func (q *azureQuery) deleteNameServers(azureClient azureclient.Client, rootDomain string, domain string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return azureClient.DeleteRecordSet(ctx, q.resourceGroupName, rootDomain, q.getRelativeDomain(rootDomain, domain), dns.NS)
}

// createNameServers creates the name servers for the specified domain in the specified managed zone.
func (q *azureQuery) createNameServers(azureClient azureclient.Client, rootDomain string, domain string, values sets.Set[string]) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	_, err := azureClient.CreateOrUpdateRecordSet(ctx, q.resourceGroupName, rootDomain, q.getRelativeDomain(rootDomain, domain), dns.NS, q.recordSet(values))

	return errors.Wrap(err, "something went wrong when creating name servers")
}

func (q *azureQuery) recordSet(values sets.Set[string]) dns.RecordSet {
	nsRecords := make([]dns.NsRecord, len(values))

	for i, v := range sets.List(values) {
		nsRecords[i] = dns.NsRecord{
			Nsdname: to.StringPtr(v),
		}
	}

	return dns.RecordSet{
		RecordSetProperties: &dns.RecordSetProperties{
			NsRecords: &nsRecords,
			TTL:       to.Int64Ptr(60),
		},
	}
}

// queryNameServers queries Azure for the name servers in the specified managed zone.
func (q *azureQuery) queryNameServers(azureClient azureclient.Client, rootDomain string) (map[string]sets.Set[string], error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	nameServers := map[string]sets.Set[string]{}
	recordSetsPage, err := azureClient.ListRecordSetsByZone(ctx, q.resourceGroupName, rootDomain, "")
	if err != nil {
		return nil, err
	}

	for recordSetsPage.NotDone() {
		for _, recordSet := range recordSetsPage.Values() {
			if recordSet.RecordSetProperties.NsRecords == nil {
				continue
			}
			values := sets.Set[string]{}
			for _, v := range *recordSet.NsRecords {
				values.Insert(*v.Nsdname)
			}
			if *recordSet.Name == "@" {
				nameServers[rootDomain] = values
			} else {
				nameServers[q.getFQDN(*recordSet.Name, rootDomain)] = values
			}
		}

		if err := recordSetsPage.NextWithContext(ctx); err != nil {
			return nil, err
		}
	}

	return nameServers, nil
}

func (q *azureQuery) getRelativeDomain(rootDomain string, domain string) string {
	return controllerutils.Undotted(strings.TrimSuffix(domain, rootDomain))
}

func (q *azureQuery) getFQDN(relativeDomain string, rootDomain string) string {
	return relativeDomain + "." + rootDomain
}
