package installmanager

import (
	awsclient "github.com/openshift/hive/pkg/awsclient"

	log "github.com/sirupsen/logrus"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

// cleanupDNSZone queries the Route53 zone and deletes any A records found. Other record
// types may be added in the future, but right now this is the only one we're seeing
// leak and conflict.
// May no longer be necessary once https://jira.coreos.com/browse/CORS-1195 is fixed.
func cleanupDNSZone(dnsZoneID, region string, logger log.FieldLogger) error {
	zoneLogger := logger.WithField("dnsZoneID", dnsZoneID)
	zoneLogger.Info("cleaning up DNSZone")

	awsClient, err := awsclient.NewClient(nil, "", "", region)
	if err != nil {
		return err
	}
	recordsOutput, err := awsClient.ListResourceRecordSets(
		&route53.ListResourceRecordSetsInput{
			HostedZoneId: awssdk.String(dnsZoneID),
		},
	)
	if err != nil {
		return err
	}
	for _, r := range recordsOutput.ResourceRecordSets {
		// We're only experiencing problems with A records, so these are all we cleanup for now:
		if *r.Type == "A" {
			zoneLogger.WithFields(log.Fields{"name": *r.Name, "type": *r.Type}).Info("deleting A record")
			request := &route53.ChangeResourceRecordSetsInput{
				ChangeBatch: &route53.ChangeBatch{
					Changes: []*route53.Change{
						{
							Action:            awssdk.String("DELETE"),
							ResourceRecordSet: r,
						},
					},
				},
				HostedZoneId: awssdk.String(dnsZoneID),
			}
			_, err := awsClient.ChangeResourceRecordSets(request)
			if err != nil {
				logger.WithError(err).WithField("recordset", r.Name).Warn("error deleting recordset")
				return err
			}
		}
	}
	zoneLogger.Info("DNSZone A records deleted")
	return nil
}
