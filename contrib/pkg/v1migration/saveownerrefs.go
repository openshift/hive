package v1migration

import (
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/imageset"
)

// SaveOwnerRefsOptions is the set of options for the saving owner references.
type SaveOwnerRefsOptions struct {
	workDir string
}

// NewSaveOwnerRefsCommand creates a command that executes the migration utility to save owner references to Hive resources.
func NewSaveOwnerRefsCommand() *cobra.Command {
	opt := &SaveOwnerRefsOptions{}
	cmd := &cobra.Command{
		Use:   "save-owner-refs",
		Short: "save to a file the owner references to Hive resources",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				log.WithError(err).Fatal("Error")
			}

			if err := opt.Validate(cmd); err != nil {
				log.WithError(err).Fatal("Error")
			}

			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.workDir, "work-dir", ".", "Directory in which store owner references file")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *SaveOwnerRefsOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *SaveOwnerRefsOptions) Validate(cmd *cobra.Command) error {
	return validateWorkDir(o.workDir)
}

// Run executes the command
func (o *SaveOwnerRefsOptions) Run() error {
	clientConfig, err := contributils.GetClientConfig()
	if err != nil {
		return errors.Wrap(err, "could not get the client config")
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could create a discovery client")
	}
	resources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return errors.Wrap(err, "could discovery the server resources")
	}
	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could not create kube client")
	}
	namespacesWithHiveResources, err := findNamespacesWithHiveResources(discoveryClient, client)
	if err != nil {
		return err
	}
	var refs []ownerRef
	for _, resourceList := range resources {
		for _, r := range resourceList.APIResources {
			logger := log.WithField("resource", r.Name)
			if !canList(r) {
				logger.Debug("skipping resource since it cannot be listed")
				continue
			}
			logger.Debugf("checking resource")
			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				return errors.Wrapf(err, "could not parse group version %q\n", resourceList.GroupVersion)
			}
			gvr := gv.WithResource(r.Name)
			if r.Namespaced {
				for _, ns := range namespacesWithHiveResources.List() {
					refs = addReferencesFromResource(refs, client.Resource(gvr).Namespace(ns), gvr, logger.WithField("namespace", ns))
				}
			} else {
				refs = addReferencesFromResource(refs, client.Resource(gvr), gvr, logger)
			}
		}
	}
	refsData, err := yaml.Marshal(refs)
	if err != nil {
		return errors.Wrap(err, "could not marshal the owner references")
	}
	if err := ioutil.WriteFile(filepath.Join(o.workDir, ownerRefsFilename), refsData, 0666); err != nil {
		return errors.Wrap(err, "could not write owner references file")
	}
	return nil
}

func findNamespacesWithHiveResources(discoveryClient *discovery.DiscoveryClient, client dynamic.Interface) (sets.String, error) {
	hiveResources, err := discoveryClient.ServerResourcesForGroupVersion(hivev1alpha1.SchemeGroupVersion.String())
	if err != nil {
		return nil, errors.Wrap(err, "could not discovery the Hive resources")
	}
	namespacesWithHiveResources := sets.NewString()
	for _, r := range hiveResources.APIResources {
		if !r.Namespaced {
			continue
		}
		if !canList(r) {
			continue
		}
		objs, err := client.Resource(hivev1alpha1.SchemeGroupVersion.WithResource(r.Name)).List(metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "could not list %s", r.Name)
		}
		for _, o := range objs.Items {
			namespacesWithHiveResources.Insert(o.GetNamespace())
		}
	}
	log.Info("Namespaces with Hive resources")
	for _, n := range namespacesWithHiveResources.List() {
		log.Infof("  %s", n)
	}
	return namespacesWithHiveResources, nil
}

func addReferencesFromResource(refs []ownerRef, resourceClient dynamic.ResourceInterface, gvr schema.GroupVersionResource, logger log.FieldLogger) []ownerRef {
	objs, err := resourceClient.List(metav1.ListOptions{})
	if err != nil {
		logger.WithError(err).Warn("could not list resource")
		return refs
	}
	for _, obj := range objs.Items {
		refs = addReferencesFromObject(refs, gvr, obj, logger)
	}
	return refs
}

func addReferencesFromObject(refs []ownerRef, gvr schema.GroupVersionResource, obj unstructured.Unstructured, logger log.FieldLogger) []ownerRef {
	if isInstallOrImagesetJob(gvr, obj) {
		logger.Debug("ignoring install or imageset job")
		return refs
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.APIVersion != hivev1alpha1.SchemeGroupVersion.String() {
			continue
		}
		logger = logger.WithField("name", obj.GetName()).
			WithField("referencedKind", ref.Kind).
			WithField("referencedName", ref.Name)
		if ns := obj.GetNamespace(); ns == "" {
			logger = logger.WithField("namespace", ns)
		}
		logger.Info("found reference to Hive resource")
		refs = append(refs,
			ownerRef{
				Group:     gvr.Group,
				Version:   gvr.Version,
				Resource:  gvr.Resource,
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
				Ref:       ref,
			})
	}
	return refs
}

func canList(r metav1.APIResource) bool {
	for _, v := range r.Verbs {
		if v == "list" {
			return true
		}
	}
	return false
}

func isInstallOrImagesetJob(gvr schema.GroupVersionResource, obj unstructured.Unstructured) bool {
	if gvr.Group != "batch" || gvr.Version != "v1" || gvr.Resource != "jobs" {
		return false
	}
	labels := obj.GetLabels()
	return labels[constants.InstallJobLabel] == "true" ||
		labels[imageset.ImagesetJobLabel] == "true"
}
