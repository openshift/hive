package imageset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	imageapi "github.com/openshift/api/image/v1"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	installerImageResolvedReason         = "InstallerImageResolved"
	installerImageResolvedMessage        = "InstallerImage is resolved."
	installerImageResolutionFailedReason = "InstallerImageResolutionFailed"
	imageReferencesFilename              = "image-references"
	releaseMetadataFilename              = "release-metadata"
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
		Short: "Updates the installer image and related status on a clusterdeployment'",
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(); err != nil {
				log.WithError(err).Fatal("cannot complete command")
				return
			}

			if err := opt.Validate(); err != nil {
				log.WithError(err).Fatal("invalid command options")
				return
			}

			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("failed to set images")
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
	var err error
	o.log, err = utils.NewLogger(o.LogLevel)
	if err != nil {
		return err
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
	if len(o.WorkDir) == 0 {
		return errors.New("--workdir is required")
	}
	fi, err := os.Stat(o.WorkDir)
	if err != nil {
		return errors.New("could not access workdir")
	}
	if !fi.IsDir() {
		return errors.New("workdir is not a directory")
	}
	if _, err := os.Stat(filepath.Join(o.WorkDir, imageReferencesFilename)); err != nil {
		return errors.Errorf("could not get %s file in workdir", imageReferencesFilename)
	}
	return nil
}

// Run updates the given ClusterDeployment based on the image-references file.
func (o *UpdateInstallerImageOptions) Run() (returnErr error) {
	cd := &hivev1.ClusterDeployment{}
	cdName := types.NamespacedName{Namespace: o.ClusterDeploymentNamespace, Name: o.ClusterDeploymentName}
	logger := o.log.WithField("clusterdeployment", cdName)
	logger.Debug("fetching clusterdeployment")
	err := o.client.Get(context.TODO(), cdName, cd)
	if err != nil {
		return errors.Wrap(err, "failed to get ClusterDeployment")
	}

	defer func() {
		if returnErr == nil {
			return
		}
		o.setImageResolutionErrorCondition(cd, returnErr)
	}()

	imageStreamData, err := os.ReadFile(filepath.Join(o.WorkDir, imageReferencesFilename))
	if err != nil {
		return errors.Wrapf(err, "could not read %s file", imageReferencesFilename)
	}
	is := &imageapi.ImageStream{}
	if err := yaml.Unmarshal(imageStreamData, &is); err != nil {
		return errors.Wrap(err, "unable to load release image-references")
	}
	if is.Kind != "ImageStream" || is.APIVersion != "image.openshift.io/v1" {
		return errors.Wrap(err, "unrecognized image-references in release payload")
	}

	var installerImage string
	// There should be no way we get here with Provisioning == nil, but better safe.
	if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.InstallerImageOverride != "" {
		installerImage = cd.Spec.Provisioning.InstallerImageOverride
		o.log.WithField("installerImage", installerImage).Info("installer image overridden")
	} else {
		// Glean the installer image from the release metadata
		installerTagName := "installer"
		// If this is a bare metal install, we need to get the openshift-install binary from a different image with
		// bare metal functionality compiled in. The binary is named the same and in the same location, so after swapping
		// out what image to get it from, we can proceed with the code as we normally would.
		if cd.Spec.Platform.BareMetal != nil {
			installerTagName = "baremetal-installer"
		}
		installerImage, err = findImageSpec(is, installerTagName)
		if err != nil {
			return errors.Wrap(err, "could not get installer image")
		}
		o.log.WithField("installerImage", installerImage).Info("installer image found")
	}

	cliImage, err := findImageSpec(is, "cli")
	if err != nil {
		return errors.Wrap(err, "could not get cli image")
	}
	o.log.WithField("cliImage", cliImage).Info("cli image found")
	// If requested, use the installer image's host for the CLI image as well.
	if copyDomain, err := strconv.ParseBool(cd.Annotations[constants.CopyCLIImageDomainFromInstallerImage]); err == nil && copyDomain {
		if !strings.Contains(installerImage, "/") {
			return errors.New(fmt.Sprintf("invalid installer image %q: expected to find a '/'", installerImage))
		}
		if !strings.Contains(cliImage, "/") {
			return errors.New(fmt.Sprintf("invalid cli image %q: expected to find a '/'", cliImage))
		}
		installerDomain := strings.SplitN(installerImage, "/", 2)[0]
		cliPath := strings.SplitN(cliImage, "/", 2)[1]
		cliImage = installerDomain + "/" + cliPath
		o.log.WithField("cliImage", cliImage).Info("overrode cli image host")
	}

	releaseMetadataRaw, err := os.ReadFile(filepath.Join(o.WorkDir, releaseMetadataFilename))
	if err != nil {
		return errors.Wrapf(err, "could not read %s file", releaseMetadataFilename)
	}
	releaseMetadata := &cincinnatiMetadata{}
	if err := json.Unmarshal(releaseMetadataRaw, releaseMetadata); err != nil {
		return errors.Wrap(err, "unable to load release release-metadata")
	}
	if releaseMetadata.Kind != "cincinnati-metadata-v0" {
		return errors.Wrap(err, "unrecognized release-metadata in release payload")
	}

	releaseVersion := getReleaseVersion(releaseMetadata, is)
	if releaseVersion == "" {
		return errors.New("no release version set in the release payload")
	}

	cd.Status.InstallerImage = &installerImage
	cd.Status.CLIImage = &cliImage
	cd.Status.InstallVersion = &releaseVersion
	// Set InstallerImageResolutionFailedCondition to false
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.InstallerImageResolutionFailedCondition,
		corev1.ConditionFalse,
		installerImageResolvedReason,
		installerImageResolvedMessage,
		controllerutils.UpdateConditionNever)
	logger.Debug("updating clusterdeployment status")
	return errors.Wrap(
		o.client.Status().Update(context.TODO(), cd),
		"could not update clusterdeployment with images",
	)
}

func findImageSpec(image *imageapi.ImageStream, tagName string) (string, error) {
	for _, tag := range image.Spec.Tags {
		if tag.Name == tagName {
			if tag.From != nil && tag.From.Kind == "DockerImage" && len(tag.From.Name) > 0 {
				return tag.From.Name, nil
			}
		}
	}
	return "", fmt.Errorf("no image tag %q exists in the release image", tagName)
}

func (o *UpdateInstallerImageOptions) setImageResolutionErrorCondition(cd *hivev1.ClusterDeployment, err error) {
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.InstallerImageResolutionFailedCondition,
		corev1.ConditionTrue,
		installerImageResolutionFailedReason,
		err.Error(),
		controllerutils.UpdateConditionAlways)

	o.log.Debug("updating cluster deployment status")
	if err := o.client.Status().Update(context.TODO(), cd); err != nil {
		o.log.WithError(err).Error("could not update clusterdeployment with error condition")
	}
}

func getClient(kubeConfig *rest.Config) (client.Client, error) {
	clientScheme := scheme.Scheme
	apis.AddToScheme(clientScheme)

	managerOptions := manager.Options{
		Scheme:         clientScheme,
		MapperProvider: apiutil.NewDiscoveryRESTMapper,
	}
	httpClient, err := rest.HTTPClientFor(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create http client")
	}
	mapper, err := managerOptions.MapperProvider(kubeConfig, httpClient)
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

// cincinnatiMetadata is the compact version of the release metadata
// as stored by the release-image generator in [1].
//
// [1]: https://github.com/openshift/oc/blob/c66c03f3012a10f16eb86fdce6330433adf6c9ee/pkg/cli/admin/release/new.go#L320-L327
type cincinnatiMetadata struct {
	Kind string `json:"kind"`

	Version string `json:"version"`
}

func getReleaseVersion(releaseMetadata *cincinnatiMetadata, is *imageapi.ImageStream) string {
	if len(releaseMetadata.Version) != 0 {
		return releaseMetadata.Version
	}
	return is.Name
}
