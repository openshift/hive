package awsprivatelink

import (
	"context"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/hive/contrib/pkg/awsprivatelink/common"
	hiveutils "github.com/openshift/hive/contrib/pkg/utils"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/pkg/util/scheme"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// privateLinkHubAcctCredsName is the name of the AWS PrivateLink Hub account credentials Secret
	// created by the "hiveutil awsprivatelink enable" command
	privateLinkHubAcctCredsName = "awsprivatelink-hub-acct-creds"

	// privateLinkHubAcctCredsLabel is added to the AWS PrivateLink Hub account credentials Secret
	// created by the "hiveutil awsprivatelink enable" command and
	// referenced by HiveConfig.spec.awsPrivateLink.credentialsSecretRef.
	privateLinkHubAcctCredsLabel = "hive.openshift.io/awsprivatelink-hub-acct-credentials"
)

var (
	logLevelDebug  bool
	credsSecretRef string
)

func NewAWSPrivateLinkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "awsprivatelink",
		Short: "AWS PrivateLink setup and tear down",
		Long: `AWS PrivateLink setup and tear down.
All subcommands require an active cluster.`,
		PersistentPreRun: persistentPreRun,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Usage()
		},
	}

	cmd.AddCommand(NewEnableAWSPrivateLinkCommand())
	cmd.AddCommand(NewDisableAWSPrivateLinkCommand())
	cmd.AddCommand(NewEndpointVPCCommand())

	pFlags := cmd.PersistentFlags()
	pFlags.BoolVarP(&logLevelDebug, "debug", "d", false, "Enable debug level logging")
	pFlags.StringVar(&credsSecretRef, "creds-secret", "",
		"Credentials Secret on the active cluster which takes priority over configuration of the environment "+
			"when building AWS clients and the Secret to be referenced in HiveConfig.spec.awsPrivateLink.credentialsSecretRef. "+
			"Example: --creds-secret kube-system/aws-creds.")
	return cmd
}

func persistentPreRun(cmd *cobra.Command, args []string) {
	setLogLevel()
	common.DynamicClient = getDynamicClient()

	if credsSecretRef != "" {
		log.Infof("Getting Secret %s", credsSecretRef)
		common.CredsSecret = getSecretFromRef(common.DynamicClient, credsSecretRef)
	}
}

func setLogLevel() {
	switch logLevelDebug {
	case true:
		log.SetLevel(log.DebugLevel)
		log.Debug("Setting log level to debug")
	default:
		log.SetLevel(log.InfoLevel)
	}
}

// Get controller-runtime dynamic client
func getDynamicClient() client.Client {
	if err := configv1.Install(scheme.GetScheme()); err != nil {
		log.WithError(err).Fatal("Failed to add Openshift configv1 types to the default scheme")
	}

	dynClient, err := hiveutils.GetClient("hiveutil-awsprivatelink")
	if err != nil {
		log.WithError(err).Fatal("Failed to create controller-runtime client")
	}
	return dynClient
}

// Get Secret referenced by ref in the namespace/name format.
func getSecretFromRef(client client.Client, ref string) *corev1.Secret {
	nsName := strings.Split(ref, "/")
	if len(nsName) != 2 {
		log.Errorf("secret ref %s is not in the `namespace/name` format. Proceed with environment configurations.", ref)
		return nil
	}
	ns := nsName[0]
	name := nsName[1]

	secret := &corev1.Secret{}
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, secret); err != nil {
		log.WithError(err).Errorf("Failed to get Secret %s in the %s namespace. Proceed with environment configurations.", name, ns)
		return nil
	}
	return secret
}
