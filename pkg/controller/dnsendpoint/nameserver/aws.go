package nameserver

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/awsclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// NewAWSQuery creates a new name server query for AWS.
func NewAWSQuery(c client.Client, credsSecretName string, region string) Query {
	return &awsQuery{
		getAWSClient: func() (awsclient.Client, error) {
			awsClient, err := awsclient.NewClient(c, credsSecretName, controllerutils.GetHiveNamespace(), region)
			return awsClient, errors.Wrap(err, "error creating AWS client")
		},
	}
}

type awsQuery struct {
	getAWSClient func() (awsclient.Client, error)
}

var _ Query = (*awsQuery)(nil)

// Get implements Query.Get.
func (q *awsQuery) Get(domain string) (map[string]sets.Set[string], error) {
	awsClient, err := q.getAWSClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get AWS client")
	}
	// TODO: cache the zone ID in rootDomainsInfo (how to make this generic across clouds?)
	zoneID, err := q.queryZoneID(awsClient, domain)
	if err != nil {
		return nil, errors.Wrap(err, "error querying zone ID")
	}
	if zoneID == nil {
		return nil, nil
	}
	currentNameServers, err := q.queryNameServers(awsClient, *zoneID)
	return currentNameServers, errors.Wrap(err, "error querying name servers")
}

// CreateOrUpdate implements Query.CreateOrUpdate.
func (q *awsQuery) CreateOrUpdate(rootDomain string, domain string, values sets.Set[string]) error {
	awsClient, err := q.getAWSClient()
	if err != nil {
		return errors.Wrap(err, "failed to get AWS client")
	}
	zoneID, err := q.queryZoneID(awsClient, rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying zone ID")
	}
	if zoneID == nil {
		return errors.New("no public hosted zone found for domain")
	}
	return errors.Wrap(
		q.changeNameServers(awsClient, *zoneID, domain, values, route53types.ChangeActionUpsert),
		"error creating the name server",
	)
}

// Delete implements Query.Delete.
func (q *awsQuery) Delete(rootDomain string, domain string, values sets.Set[string]) error {
	awsClient, err := q.getAWSClient()
	if err != nil {
		return errors.Wrap(err, "failed to get AWS client")
	}
	zoneID, err := q.queryZoneID(awsClient, rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying zone ID")
	}
	if zoneID == nil {
		return nil
	}
	if len(values) != 0 {
		// If values were provided for the name servers, attempt to perform a
		// delete using those values.
		err = q.changeNameServers(awsClient, *zoneID, domain, values, route53types.ChangeActionDelete)
		if err != nil {
			erricb := route53types.InvalidChangeBatch{}
			if !errors.As(err, erricb) {
				return errors.Wrap(err, "error deleting the name server")
			}
			if strings.HasSuffix(*erricb.Message, "not found]") {
				return nil
			}
			if !strings.HasSuffix(*erricb.Message, "not match the current values]") {
				return errors.Wrap(err, "error deleting the name server")
			}
		}
	}
	// Since we do not have up-to-date values for the name servers, we need
	// to query AWS for the current values to use them in the delete.
	values, err = q.queryNameServer(awsClient, *zoneID, domain)
	if err != nil {
		return errors.Wrap(err, "error querying the current values of the name server")
	}
	if len(values) == 0 {
		return nil
	}
	return errors.Wrap(
		q.changeNameServers(awsClient, *zoneID, domain, values, route53types.ChangeActionDelete),
		"error deleting the name server with recently read values",
	)
}

// queryZoneID queries AWS for the public hosted zone for the specified domain.
func (q *awsQuery) queryZoneID(awsClient awsclient.Client, domain string) (*string, error) {
	domain = controllerutils.Dotted(domain)
	listInput := &route53.ListHostedZonesByNameInput{
		DNSName:  &domain,
		MaxItems: aws.Int32(5),
	}
	for {
		listOutput, err := awsClient.ListHostedZonesByName(listInput)
		if err != nil {
			return nil, err
		}
		for _, zone := range listOutput.HostedZones {
			if zone.Name == nil || *zone.Name != domain {
				return nil, nil
			}
			if zone.Config == nil || zone.Config.PrivateZone {
				continue
			}
			return zone.Id, nil
		}
		if !listOutput.IsTruncated {
			return nil, nil
		}
		listInput.DNSName = listOutput.NextDNSName
		listInput.HostedZoneId = listOutput.NextHostedZoneId
	}
}

// queryNameServers queries AWS for the name servers in the specified hosted zone.
func (q *awsQuery) queryNameServers(awsClient awsclient.Client, hostedZoneID string) (map[string]sets.Set[string], error) {
	nameServers := map[string]sets.Set[string]{}
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId: &hostedZoneID,
		MaxItems:     aws.Int32(100),
	}
	for {
		listOutput, err := awsClient.ListResourceRecordSets(listInput)
		if err != nil {
			return nil, err
		}
		for _, recordSet := range listOutput.ResourceRecordSets {
			if recordSet.Name == nil {
				continue
			}
			if recordSet.Type != route53types.RRTypeNs {
				continue
			}
			values := sets.Set[string]{}
			for _, record := range recordSet.ResourceRecords {
				values.Insert(*record.Value)
			}
			nameServers[controllerutils.Undotted(*recordSet.Name)] = values
		}
		if !listOutput.IsTruncated {
			return nameServers, nil
		}
		listInput.StartRecordIdentifier = listOutput.NextRecordIdentifier
		listInput.StartRecordName = listOutput.NextRecordName
		listInput.StartRecordType = listOutput.NextRecordType
	}
}

// queryNameServer queries AWS for the name servers in the specified hosted zone for the specified domain.
func (q *awsQuery) queryNameServer(awsClient awsclient.Client, hostedZoneID string, domain string) (sets.Set[string], error) {
	recordType := route53types.RRTypeNs
	listOutput, err := awsClient.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:    &hostedZoneID,
		MaxItems:        aws.Int32(1),
		StartRecordName: &domain,
		StartRecordType: recordType,
	})
	if err != nil {
		return nil, err
	}
	if len(listOutput.ResourceRecordSets) == 0 {
		return nil, nil
	}
	recordSet := listOutput.ResourceRecordSets[0]
	if recordSet.Name == nil {
		return nil, nil
	}
	if controllerutils.Undotted(*recordSet.Name) != domain {
		return nil, nil
	}
	if recordSet.Type != route53types.RRTypeNs {
		return nil, nil
	}
	values := sets.Set[string]{}
	for _, record := range recordSet.ResourceRecords {
		values.Insert(*record.Value)
	}
	return values, nil
}

// changeNameServers changes the name servers for the specified domain in the specified hosted zone.
func (q *awsQuery) changeNameServers(awsClient awsclient.Client, hostedZoneID string, domain string, values sets.Set[string], action route53types.ChangeAction) error {
	recordType := route53types.RRTypeNs
	ttl := int64(60)
	records := make([]route53types.ResourceRecord, 0, len(values))
	for v := range values {
		value := v
		records = append(records, route53types.ResourceRecord{Value: &value})
	}
	changeInput := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: &hostedZoneID,
		ChangeBatch: &route53types.ChangeBatch{
			Changes: []route53types.Change{{
				Action: action,
				ResourceRecordSet: &route53types.ResourceRecordSet{
					Name:            &domain,
					Type:            recordType,
					TTL:             &ttl,
					ResourceRecords: records,
				},
			}},
		},
	}
	_, err := awsClient.ChangeResourceRecordSets(changeInput)
	return err
}
