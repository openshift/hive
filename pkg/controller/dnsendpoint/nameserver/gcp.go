package nameserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"
)

// NewGCPQuery creates a new name server query for GCP.
func NewGCPQuery(c client.Client, credsSecretName string) Query {
	return &gcpQuery{
		getGCPClient: func() (gcpclient.Client, error) {
			credsSecret := &corev1.Secret{}
			if err := c.Get(
				context.Background(),
				client.ObjectKey{Namespace: constants.HiveNamespace, Name: credsSecretName},
				credsSecret,
			); err != nil {
				return nil, errors.Wrap(err, "could not get the creds secret")
			}
			gcpClient, err := gcpclient.NewClientFromSecret(credsSecret)
			return gcpClient, errors.Wrap(err, "error creating GCP client")
		},
	}
}

type gcpQuery struct {
	getGCPClient func() (gcpclient.Client, error)
}

var _ Query = (*gcpQuery)(nil)

// Get implements Query.Get.
func (q *gcpQuery) Get(domain string) (map[string]sets.String, error) {
	gcpClient, err := q.getGCPClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get GCP client")
	}
	zoneName, err := q.queryZoneName(gcpClient, domain)
	if err != nil {
		return nil, errors.Wrap(err, "error querying zone name")
	}
	if zoneName == "" {
		return nil, nil
	}
	currentNameServers, err := q.queryNameServers(gcpClient, zoneName)
	return currentNameServers, errors.Wrap(err, "error querying name servers")
}

// Create implements Query.Create.
func (q *gcpQuery) Create(rootDomain string, domain string, values sets.String) error {
	gcpClient, err := q.getGCPClient()
	if err != nil {
		return errors.Wrap(err, "failed to get GCP client")
	}
	zoneName, err := q.queryZoneName(gcpClient, rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying zone name")
	}
	if zoneName == "" {
		return errors.New("no public managed zone found for domain")
	}
	return errors.Wrap(
		q.createNameServers(gcpClient, zoneName, domain, values),
		"error creating the name server",
	)
}

// Delete implements Query.Delete.
func (q *gcpQuery) Delete(rootDomain string, domain string, values sets.String) error {
	gcpClient, err := q.getGCPClient()
	if err != nil {
		return errors.Wrap(err, "failed to get GCP client")
	}
	zoneName, err := q.queryZoneName(gcpClient, rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying zone name")
	}
	if zoneName == "" {
		return errors.New("no public managed zone found for domain")
	}
	if len(values) != 0 {
		// If values were provided for the name servers, attempt to perform a
		// delete using those values.
		err = q.deleteNameServers(gcpClient, zoneName, domain, values)
		gcpErr, ok := err.(*googleapi.Error)
		if !ok {
			return errors.Wrap(err, "error deleting the name server")
		}
		if gcpErr.Code == http.StatusNotFound {
			return nil
		}
		if gcpErr.Code != http.StatusPreconditionFailed {
			return errors.Wrap(err, "error deleting the name server")
		}
	}
	// Since we do not have up-to-date values for the name servers, we need
	// to query GCP for the current values to use them in the delete.
	values, err = q.queryNameServer(gcpClient, zoneName, domain)
	if err != nil {
		return errors.Wrap(err, "error querying the current values of the name server")
	}
	if len(values) == 0 {
		return nil
	}
	return errors.Wrap(
		q.deleteNameServers(gcpClient, zoneName, domain, values),
		"error deleting the name server with recently read values",
	)
}

// queryZoneName queries GCP for the public managed zone for the specified domain.
func (q *gcpQuery) queryZoneName(gcpClient gcpclient.Client, domain string) (string, error) {
	listOpts := gcpclient.ListManagedZonesOptions{
		DNSName: controllerutils.Dotted(domain),
	}
	for {
		listOutput, err := gcpClient.ListManagedZones(listOpts)
		if err != nil {
			return "", err
		}
		for _, zone := range listOutput.ManagedZones {
			if zone.Visibility != "public" {
				continue
			}
			return zone.Name, nil
		}
		if listOutput.NextPageToken == "" {
			return "", nil
		}
		listOpts.PageToken = listOutput.NextPageToken
	}
}

// queryNameServers queries GCP for the name servers in the specified managed zone.
func (q *gcpQuery) queryNameServers(gcpClient gcpclient.Client, managedZone string) (map[string]sets.String, error) {
	nameServers := map[string]sets.String{}
	listOpts := gcpclient.ListResourceRecordSetsOptions{}
	for {
		listOutput, err := gcpClient.ListResourceRecordSets(managedZone, listOpts)
		if err != nil {
			return nil, err
		}
		for _, recordSet := range listOutput.Rrsets {
			if recordSet.Type != "NS" {
				continue
			}
			values := sets.NewString()
			for _, v := range recordSet.Rrdatas {
				values.Insert(controllerutils.Undotted(v))
			}
			nameServers[controllerutils.Undotted(recordSet.Name)] = values
		}
		if listOutput.NextPageToken == "" {
			return nameServers, nil
		}
		listOpts.PageToken = listOutput.NextPageToken
	}
}

// queryNameServer queries GCP for the name servers for the specified domain in the specified managed zone.
func (q *gcpQuery) queryNameServer(gcpClient gcpclient.Client, managedZone string, domain string) (sets.String, error) {
	listOutput, err := gcpClient.ListResourceRecordSets(
		managedZone,
		gcpclient.ListResourceRecordSetsOptions{
			MaxResults: 1,
			Name:       controllerutils.Dotted(domain),
			Type:       "NS",
		},
	)
	if err != nil {
		return nil, err
	}
	if len(listOutput.Rrsets) == 0 {
		return nil, nil
	}
	values := sets.NewString()
	for _, v := range listOutput.Rrsets[0].Rrdatas {
		values.Insert(controllerutils.Undotted(v))
	}
	return values, nil
}

// createNameServers creates the name servers for the specified domain in the specified managed zone.
func (q *gcpQuery) createNameServers(gcpClient gcpclient.Client, managedZone string, domain string, values sets.String) error {

	err := gcpClient.AddResourceRecordSet(managedZone, q.resourceRecordSet(domain, values))
	if gcpErr, ok := err.(*googleapi.Error); ok && gcpErr.Code == http.StatusConflict {
		// this means there is already an existing resource record, so we need
		// to fall through to the update path
	} else if err != nil {
		return errors.Wrap(err, "unexpected error creating NS record")
	} else {
		return nil
	}

	// An update using the GCP API involves listing the current set of records
	// for removal, and adding the new desired set of records as additions.
	response, err := gcpClient.ListResourceRecordSets(managedZone, gcpclient.ListResourceRecordSetsOptions{
		Name: controllerutils.Dotted(domain),
		Type: "NS",
	})
	if err != nil {
		errors.Wrap(err, "failed to list existing record sets")
	}
	switch len(response.Rrsets) {
	case 1:
		// Exactly one NS record that needs updating
		currentNSValues := sets.NewString(response.Rrsets[0].Rrdatas...)

		addRRSet := q.resourceRecordSet(domain, values)
		removeRRSet := q.resourceRecordSet(domain, currentNSValues)

		err := gcpClient.UpdateResourceRecordSet(managedZone, addRRSet, removeRRSet)
		return errors.Wrap(err, "failed to update existing NS entry")
	default:
		return fmt.Errorf("unexpected response when querying domain")
	}
}

// deleteNameServers deletes the name servers for the specified domain in the specified managed zone.
func (q *gcpQuery) deleteNameServers(gcpClient gcpclient.Client, managedZone string, domain string, values sets.String) error {
	return gcpClient.DeleteResourceRecordSet(managedZone, q.resourceRecordSet(domain, values))
}

func (q *gcpQuery) resourceRecordSet(domain string, values sets.String) *dns.ResourceRecordSet {
	dottedValues := make([]string, len(values))
	for i, v := range values.List() {
		dottedValues[i] = controllerutils.Dotted(v)
	}
	return &dns.ResourceRecordSet{
		Name:    controllerutils.Dotted(domain),
		Rrdatas: dottedValues,
		Ttl:     int64(60),
		Type:    "NS",
	}
}
