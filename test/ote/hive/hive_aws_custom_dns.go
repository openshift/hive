package hive

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	o "github.com/onsi/gomega"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	userProvisionedDNSEnabled = "Enabled"
	customDNSRecordTTL        = 60
)

type awsCustomDNSRecord struct {
	name   string
	target string
}

type awsCustomDNSManager struct {
	elbClient      *elb.Client
	elbv2Client    *elbv2.Client
	route53Client  *route53.Client
	hostedZoneID   string
	createdRecords map[string]awsCustomDNSRecord
}

func newAWSCustomDNSManager(cfg aws.Config, baseDomain string) *awsCustomDNSManager {
	route53Client := route53.NewFromConfig(cfg)
	output, err := route53Client.ListHostedZonesByName(context.Background(), &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(baseDomain),
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	var hostedZoneID string
	for _, zone := range output.HostedZones {
		if strings.TrimSuffix(aws.ToString(zone.Name), ".") == strings.TrimSuffix(baseDomain, ".") &&
			(zone.Config == nil || !zone.Config.PrivateZone) {
			hostedZoneID = aws.ToString(zone.Id)
			break
		}
	}
	o.Expect(hostedZoneID).NotTo(o.BeEmpty(), "public hosted zone for %s was not found", baseDomain)

	return &awsCustomDNSManager{
		elbClient:      elb.NewFromConfig(cfg),
		elbv2Client:    elbv2.NewFromConfig(cfg),
		route53Client:  route53Client,
		hostedZoneID:   hostedZoneID,
		createdRecords: map[string]awsCustomDNSRecord{},
	}
}

func (m *awsCustomDNSManager) publishAPIRecord(infraID, clusterName, baseDomain string) bool {
	output, err := m.elbv2Client.DescribeLoadBalancers(context.Background(), &elbv2.DescribeLoadBalancersInput{
		Names: []string{infraID + "-ext"},
	})
	if err != nil || len(output.LoadBalancers) != 1 {
		e2e.Logf("Waiting for the external API load balancer for infrastructure %s", infraID)
		return false
	}

	return m.upsertCNAME("api."+clusterName+"."+baseDomain, aws.ToString(output.LoadBalancers[0].DNSName))
}

func (m *awsCustomDNSManager) publishIngressRecord(infraID, clusterName, baseDomain string) bool {
	paginator := elb.NewDescribeLoadBalancersPaginator(m.elbClient, &elb.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(context.Background())
		if err != nil {
			e2e.Logf("Unable to list ingress load balancers yet: %v", err)
			return false
		}

		for start := 0; start < len(output.LoadBalancerDescriptions); start += 20 {
			end := start + 20
			if end > len(output.LoadBalancerDescriptions) {
				end = len(output.LoadBalancerDescriptions)
			}
			names := make([]string, 0, end-start)
			for _, description := range output.LoadBalancerDescriptions[start:end] {
				names = append(names, aws.ToString(description.LoadBalancerName))
			}
			tagOutput, err := m.elbClient.DescribeTags(context.Background(), &elb.DescribeTagsInput{
				LoadBalancerNames: names,
			})
			if err != nil {
				e2e.Logf("Unable to inspect ingress load balancer tags yet: %v", err)
				return false
			}
			for _, tagDescription := range tagOutput.TagDescriptions {
				if !hasAWSTag(tagDescription.Tags, "kubernetes.io/cluster/"+infraID, "owned") ||
					!hasAWSTag(tagDescription.Tags, "kubernetes.io/service-name", "openshift-ingress/router-default") {
					continue
				}
				for _, description := range output.LoadBalancerDescriptions[start:end] {
					if aws.ToString(description.LoadBalancerName) == aws.ToString(tagDescription.LoadBalancerName) {
						return m.upsertCNAME("*.apps."+clusterName+"."+baseDomain, aws.ToString(description.DNSName))
					}
				}
			}
		}
	}

	e2e.Logf("Waiting for the default ingress load balancer for infrastructure %s", infraID)
	return false
}

func hasAWSTag(tags []elbtypes.Tag, key, value string) bool {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key && aws.ToString(tag.Value) == value {
			return true
		}
	}
	return false
}

func (m *awsCustomDNSManager) upsertCNAME(name, target string) bool {
	name = strings.TrimSuffix(name, ".") + "."
	target = strings.TrimSuffix(target, ".") + "."
	_, err := m.route53Client.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(m.hostedZoneID),
		ChangeBatch: &route53types.ChangeBatch{
			Changes: []route53types.Change{{
				Action: route53types.ChangeActionUpsert,
				ResourceRecordSet: &route53types.ResourceRecordSet{
					Name:            aws.String(name),
					Type:            route53types.RRTypeCname,
					TTL:             aws.Int64(customDNSRecordTTL),
					ResourceRecords: []route53types.ResourceRecord{{Value: aws.String(target)}},
				},
			}},
		},
	})
	if err != nil {
		e2e.Logf("Unable to publish custom DNS record %s -> %s yet: %v", name, target, err)
		return false
	}
	m.createdRecords[name] = awsCustomDNSRecord{name: name, target: target}
	e2e.Logf("Published test-owned custom DNS record %s -> %s", name, target)
	return true
}

func (m *awsCustomDNSManager) cleanup() {
	for _, record := range m.createdRecords {
		_, err := m.route53Client.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(m.hostedZoneID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: []route53types.Change{{
					Action: route53types.ChangeActionDelete,
					ResourceRecordSet: &route53types.ResourceRecordSet{
						Name:            aws.String(record.name),
						Type:            route53types.RRTypeCname,
						TTL:             aws.Int64(customDNSRecordTTL),
						ResourceRecords: []route53types.ResourceRecord{{Value: aws.String(record.target)}},
					},
				}},
			},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}
