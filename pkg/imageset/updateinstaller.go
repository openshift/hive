package imageset

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var (
	releaseInfoCommandTimeout = 5 * time.Minute
	pollInterval              = 1 * time.Second
)

const (
	installerImageResolvedReason         = "InstallerImageResolved"
	installerImageResolvedMessage        = "InstallerImage is resolved."
	installerImageResolutionFailedReason = "InstallerImageResolutionFailed"
)

// UpdateInstallerImageOptions contains options for running the command
// to update the installer image
type UpdateInstallerImageOptions struct {
	ClusterDeploymentName      string
	ClusterDeploymentNamespace string
	LogLevel                   string
	WorkDir                    string
	log                        log.FieldLogger
	client                     client.Client
}

// NewUpdateInstallerImageCommand returns a command to update the installer image on
// a cluster deployment.
func NewUpdateInstallerImageCommand() *cobra.Command {
	opt := &UpdateInstallerImageOptions{}
	cmd := &cobra.Command{
		Use:   "update-installer-image OPTIONS",
		Short: "Updates the installer image and related status on a clusterdeployment based on results from 'oc adm release info'",
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(); err != nil {
				log.WithError(err).Error("cannot complete command")
				return
			}

			if err := opt.Validate(); err != nil {
				log.WithError(err).Error("invalid command options")
				return
			}

			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("runtime error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.LogLevel, "log-level", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.WorkDir, "work-dir", "/common", "directory to use for all input and output")
	flags.StringVar(&opt.ClusterDeploymentName, "cluster-deployment-name", "", "name of ClusterDeployment to update")
	flags.StringVar(&opt.ClusterDeploymentNamespace, "cluster-deployment-namespace", "", "namespace of ClusterDeployment to update")
	return cmd
}

// Complete sets remaining fields on the UpdateInstallerImageOptions based on command options and arguments.
func (o *UpdateInstallerImageOptions) Complete() error {
	// Set log level
	level, err := log.ParseLevel(o.LogLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
	}

	o.log = log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	if len(o.WorkDir) == 0 {
		log.Error("work directory not set")
	}
	absPath, err := filepath.Abs(o.WorkDir)
	if err != nil {
		o.log.WithError(err).Fatalf("error finding absolute workdir path")
	}
	o.WorkDir = absPath
	if _, err := os.Stat(o.WorkDir); os.IsNotExist(err) {
		o.log.WithField("workdir", o.WorkDir).Fatalf("workdir does not exist")
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		log.WithError(err).Error("Cannot obtain client config")
		return err
	}
	o.client, err = getClient(cfg)
	if err != nil {
		log.WithError(err).Error("Cannot obtain API client")
		return err
	}

	return nil
}

// Validate ensures the given options and arguments are valid.
func (o *UpdateInstallerImageOptions) Validate() error {
	if o.ClusterDeploymentName == "" {
		return fmt.Errorf("--cluster-deployment-name is required")
	}
	if o.ClusterDeploymentNamespace == "" {
		return fmt.Errorf("--cluster-deployment-namespace is required")
	}
	return nil
}

// Run updates the given ClusterImageSet based on the results of the 'oc adm release info'
// This command does the following:
// 1. Wait for the existence of ${workdir}/success
// 2. Determine based on the contents of that file whether the command was successful or not
// 3. If successful, obtain install image from ${workdir}/installer-image.txt
// 4. If failed, obtain error log from ${workdir}/error.log
// 3. Update the status of the ClusterImageSet based on those results
func (o *UpdateInstallerImageOptions) Run() error {
	successFileName := path.Join(o.WorkDir, "success")
	success := false
	o.log.Debug("starting to wait for success result")
	err := wait.PollImmediate(pollInterval, releaseInfoCommandTimeout, func() (bool, error) {
		_, err := os.Stat(successFileName)
		if err != nil {
			if !os.IsNotExist(err) {
				o.log.WithError(err).Warningf("unexpected error trying to stat %s", successFileName)
			}
			return false, nil
		}
		result, err := ioutil.ReadFile(successFileName)
		if err != nil {
			o.log.WithError(err).Warningf("unexpected error trying to read %s", successFileName)
			return false, nil
		}
		if len(string(result)) == 0 {
			// keep waiting for content
			return false, nil
		}
		o.log.Debugf("contents of success file: %s", strings.TrimSpace(string(result)))
		success = strings.TrimSpace(string(result)) == "1"
		return true, nil
	})
	if err != nil {
		o.log.WithError(err).Error("timed out waiting for result")
		return err
	}

	if success {
		o.log.Debugf("the oc release info command was successful")
		installerImageFileName := path.Join(o.WorkDir, "installer-image.txt")
		installerImageBytes, err := ioutil.ReadFile(installerImageFileName)
		if err != nil {
			o.log.WithError(err).Error("could not read install image file")
			return err
		}
		installerImage := strings.TrimSpace(string(installerImageBytes))
		o.log.Debugf("contents of installer-image.txt: %s", installerImage)

		cliImageFileName := path.Join(o.WorkDir, "cli-image.txt")
		cliImageBytes, err := ioutil.ReadFile(cliImageFileName)
		if err != nil {
			o.log.WithError(err).Error("could not read install image file")
			return err
		}
		cliImage := strings.TrimSpace(string(cliImageBytes))
		o.log.Debugf("contents of cli-image.txt: %s", cliImage)

		return o.updateInstallerImage(installerImage, cliImage)
	}

	o.log.Debugf("the oc release info command failed")
	errorLogFileName := path.Join(o.WorkDir, "error.log")
	errorLogBytes, err := ioutil.ReadFile(errorLogFileName)
	if err != nil {
		o.log.WithError(err).Error("could not read error log")
		return err
	}
	errorLog := string(errorLogBytes)
	o.log.Debugf("the contents of the error log: %s", errorLog)
	return o.setImageResolutionErrorCondition(errorLog)
}

func (o *UpdateInstallerImageOptions) updateInstallerImage(installerImage, cliImage string) error {
	cd := &hivev1.ClusterDeployment{}
	cdName := types.NamespacedName{Namespace: o.ClusterDeploymentNamespace, Name: o.ClusterDeploymentName}
	logger := o.log.WithField("clusterdeployment", cdName)
	logger.Debug("fetching clusterdeployment")
	err := o.client.Get(context.TODO(), cdName, cd)
	if err != nil {
		logger.WithError(err).Error("failed to get ClusterDeployment")
		return err
	}
	cd.Status.InstallerImage = &installerImage
	cd.Status.CLIImage = &cliImage
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.InstallerImageResolutionFailedCondition,
		corev1.ConditionFalse,
		installerImageResolvedReason,
		installerImageResolvedMessage,
		controllerutils.UpdateConditionNever)

	logger.Debug("updating clusterdeployment status")
	return o.client.Status().Update(context.TODO(), cd)
}

func (o *UpdateInstallerImageOptions) setImageResolutionErrorCondition(errorLog string) error {
	cd := &hivev1.ClusterDeployment{}
	cdName := types.NamespacedName{Namespace: o.ClusterDeploymentNamespace, Name: o.ClusterDeploymentName}
	logger := o.log.WithField("clusterdeployment", cdName)
	logger.Debug("fetching clusterdeployment")
	err := o.client.Get(context.TODO(), cdName, cd)
	if err != nil {
		logger.WithError(err).Errorf("failed to get ClusterDeployment")
		return err
	}
	message := fmt.Sprintf("Failed to resolve image: %s", errorLog)
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.InstallerImageResolutionFailedCondition,
		corev1.ConditionTrue,
		installerImageResolutionFailedReason,
		message,
		controllerutils.UpdateConditionAlways)

	logger.Debug("updating cluster deployment status")
	return o.client.Status().Update(context.TODO(), cd)
}

func getClient(kubeConfig *rest.Config) (client.Client, error) {
	clientScheme := scheme.Scheme
	apis.AddToScheme(clientScheme)

	managerOptions := manager.Options{
		Scheme:         clientScheme,
		MapperProvider: apiutil.NewDiscoveryRESTMapper,
	}

	mapper, err := managerOptions.MapperProvider(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get API Group-Resources")
	}
	kubeClient, err := client.New(kubeConfig, client.Options{
		Scheme: clientScheme,
		Mapper: mapper,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %v", err)
	}
	return kubeClient, nil
}
