package certificate

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"

	awsclient "github.com/openshift/hive/pkg/awsclient"
)

// Options is the set of options to create a new server certificate
type Options struct {
	Name       string
	BaseDomain string
	Email      string
	Region     string
	OutputDir  string
	WaitTime   time.Duration
}

// NewCreateCertifcateCommand returns a command that will create a letsencrypt serving cert
// for OpenShift clusters on AWS.
func NewCreateCertifcateCommand() *cobra.Command {
	username := "user"
	homeDir := "/tmp"
	user, err := user.Current()
	if err == nil {
		username = user.Username
		homeDir = user.HomeDir
	}

	opt := &Options{}
	cmd := &cobra.Command{
		Use:   "create CLUSTER_NAME",
		Short: "Creates a new serving certificate for the given cluster",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			log.SetLevel(log.InfoLevel)
			err := opt.Run()
			if err != nil {
				log.WithError(err).Error("Error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.BaseDomain, "base-domain", "new-installer.openshift.com", "Base domain for the cluster")
	flags.StringVar(&opt.Email, "email", fmt.Sprintf("%s@redhat.com", username), "email address to use for certificate")
	flags.StringVar(&opt.Region, "region", "us-east-1", "AWS region for certificate hosted zone")
	flags.StringVar(&opt.OutputDir, "output-dir", homeDir, "Output directory for certs. Defaults to home directory")
	flags.DurationVar(&opt.WaitTime, "wait-time", 30*time.Second, "amount of time to wait after creating dns entries so that they can be authenticated")
	return cmd
}

// Complete finalizes command options
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.Name = args[0]
	return nil
}

// Run executes the command
func (o *Options) Run() error {
	certbotPath, err := exec.LookPath("certbot")
	if err != nil {
		return errors.New("certbot is required to create a certificate, install it by following instructions here: https://certbot.eff.org")
	}
	baseDomainID, err := o.getBaseDomainID()
	if err != nil {
		return err
	}
	certbotDir, err := ioutil.TempDir("", "")
	if err != nil {
		return errors.New("cannot create temporary directory for certbot")
	}
	log.Infof("Executing certbot command in directory %s", certbotDir)
	certbotCommand := exec.Command(certbotPath,
		"certonly",
		"--non-interactive",
		"--manual",
		"--manual-auth-hook", fmt.Sprintf("%s certificate auth-hook --wait-time %v %s %s", os.Args[0], o.WaitTime, o.Region, baseDomainID),
		"--manual-cleanup-hook", fmt.Sprintf("%s certificate cleanup-hook %s %s", os.Args[0], o.Region, baseDomainID),
		"--preferred-challenge", "dns",
		"--config-dir", certbotDir,
		"--work-dir", certbotDir,
		"--logs-dir", certbotDir,
		"--agree-tos",
		"--manual-public-ip-logging-ok",
		"--domains", fmt.Sprintf("api.%[1]s.%[2]s,*.apps.%[1]s.%[2]s", o.Name, o.BaseDomain),
		"--email", o.Email,
	)
	buf := &bytes.Buffer{}
	certbotCommand.Stdout = buf
	certbotCommand.Stderr = buf
	err = certbotCommand.Run()
	if err != nil {
		fmt.Printf("%s\n", buf.String())
		return errors.New("an error occurred creating certificate")
	}

	certFile := filepath.Join(o.OutputDir, fmt.Sprintf("%s.crt", o.Name))
	certKey := filepath.Join(o.OutputDir, fmt.Sprintf("%s.key", o.Name))
	caCert := filepath.Join(o.OutputDir, fmt.Sprintf("%s.ca", o.Name))

	err = copyFile(filepath.Join(certbotDir, "live", fmt.Sprintf("api.%s.%s", o.Name, o.BaseDomain), "fullchain.pem"), certFile)
	if err != nil {
		return err
	}
	err = copyFile(filepath.Join(certbotDir, "live", fmt.Sprintf("api.%s.%s", o.Name, o.BaseDomain), "privkey.pem"), certKey)
	if err != nil {
		return err
	}
	err = copyFile(filepath.Join(certbotDir, "live", fmt.Sprintf("api.%s.%s", o.Name, o.BaseDomain), "chain.pem"), caCert)
	if err != nil {
		return err
	}

	log.Infof("Your certificate has been created successfully.")
	log.Infof("Letsencrypt directory: %s", certbotDir)
	log.Infof("Certificate and key are at: %s  %s", certFile, certKey)
	log.Infof("Certificate authority cert is at: %s. It must be configured as an additional CA certificate in HiveConfig/hive", caCert)
	log.Infof("To configure the additional CA certificate:\n    hack/set-additional-ca.sh %s\n    (only needs to be done once per hive installation)", caCert)
	log.Infof("To create a cluster with this certificate:\n    %s create-cluster %s --serving-cert %s --serving-cert-key %s", os.Args[0], o.Name, certFile, certKey)
	return nil
}

func (o *Options) getBaseDomainID() (string, error) {
	client, err := awsclient.NewClient(nil, "", "", o.Region)
	if err != nil {
		return "", errors.Wrap(err, "cannot create AWS client; make sure your environment is setup to communicate with AWS")
	}
	result, err := client.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{
		DNSName: aws.String(o.BaseDomain),
	})
	if err != nil {
		return "", errors.Wrap(err, "cannot list hosted zones on AWS")
	}
	hostedZoneID := ""
	for _, zone := range result.HostedZones {
		if strings.TrimSuffix(aws.StringValue(zone.Name), ".") == o.BaseDomain {
			if zone.Config == nil || !aws.BoolValue(zone.Config.PrivateZone) {
				hostedZoneID = aws.StringValue(zone.Id)
				break
			}
			continue
		}
		break
	}
	if hostedZoneID == "" {
		return "", errors.Errorf("could not find public hosted zone with domain %s", o.BaseDomain)
	}
	return hostedZoneID, nil
}

func copyFile(src, dst string) error {
	b, err := ioutil.ReadFile(src)
	if err != nil {
		return errors.Wrapf(err, "cannot read %s", src)
	}
	err = ioutil.WriteFile(dst, b, 0644)
	if err != nil {
		return errors.Wrapf(err, "cannot write %s", dst)
	}
	return nil
}
