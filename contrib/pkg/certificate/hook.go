package certificate

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"

	awsclient "github.com/openshift/hive/pkg/awsclient"
)

// HookOptions is the set of options to create/delete DNS authentication entries
type HookOptions struct {
	HostedZoneID string
	Domain       string
	Value        string
	Region       string
	WaitTime     time.Duration
}

// NewAuthHookCommand returns a command that will create a DNS authentication entry
// in the given DNS hosted zone
func NewAuthHookCommand() *cobra.Command {
	opt := &HookOptions{}
	cmd := &cobra.Command{
		Use:   "auth-hook REGION HOSTEDZONEID",
		Short: "Creates authentication entries on a given AWS hosted zone",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			log.SetLevel(log.InfoLevel)
			err := opt.Create()
			if err != nil {
				log.WithError(err).Error("Error")
				os.Exit(1)
			}
		},
	}
	cmd.Flags().DurationVar(&opt.WaitTime, "wait-time", 30*time.Second, "Specify the amount of time to wait after creating record to allow for propagation")

	return cmd
}

// NewCleanupHookCommand returns a command that will cleanup a DNS authentication entry
// in the given DNS hosted zone
func NewCleanupHookCommand() *cobra.Command {
	opt := &HookOptions{}
	cmd := &cobra.Command{
		Use:   "cleanup-hook REGION HOSTEDZONEID",
		Short: "Deletes authentication entries on a given AWS hosted zone",
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			log.SetLevel(log.InfoLevel)
			err := opt.Delete()
			if err != nil {
				log.WithError(err).Error("Error")
				os.Exit(1)
			}
		},
	}
	return cmd
}

// Complete finalizes options by using arguments and environment variables
func (o *HookOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Region = args[0]
	o.HostedZoneID = args[1]

	o.Domain = os.Getenv("CERTBOT_DOMAIN")
	o.Value = os.Getenv("CERTBOT_VALIDATION")

	if o.Domain == "" || o.Value == "" {
		log.Infof("Required environment variables CERTBOT_DOMAIN and CERTBOT_VALIDATION must be present")
		return errors.New("invalid")
	}
	return nil
}

// Create creates an authentication DNS record in the specified zone
func (o *HookOptions) Create() error {
	err := o.modifyDNSRecord(false)
	if err != nil {
		return err
	}
	time.Sleep(o.WaitTime)
	return nil
}

// Delete removes the authentication DNS record from the specified zone
func (o *HookOptions) Delete() error {
	return o.modifyDNSRecord(true)
}

func (o *HookOptions) modifyDNSRecord(remove bool) error {
	client, err := awsclient.NewClient(nil, "", "", o.Region)
	if err != nil {
		return errors.Wrap(err, "cannot create AWS client")
	}
	action := "CREATE"
	if remove {
		action = "DELETE"
	}
	change := &route53.Change{
		Action: aws.String(action),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name: aws.String(fmt.Sprintf("_acme-challenge.%s.", o.Domain)),
			ResourceRecords: []*route53.ResourceRecord{
				{
					Value: aws.String(fmt.Sprintf("%q", o.Value)),
				},
			},
			Type: aws.String("TXT"),
			TTL:  aws.Int64(30),
		},
	}
	_, err = client.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(o.HostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{change},
		},
	})
	if err != nil {
		return errors.Wrap(err, "cannot make recordset change")
	}
	return nil
}
