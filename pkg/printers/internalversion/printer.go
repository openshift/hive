package internalversion

import (
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kprinters "k8s.io/kubernetes/pkg/printers"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
)

var (
	onlyNameColumns                  = []string{"NAME"}
	clusterDeploymentColumns         = []string{"NAME", "CLUSTERNAME", "CLUSTERTYPE", "BASEDOMAIN", "INSTALLED", "INFRAID", "AGE"}
	clusterDeprovisionRequestColumns = []string{"NAME", "INFRAID", "CLUSTERID", "COMPLETED", "AGE"}
	clusterImageSetColumns           = []string{"NAME", "INSTALLER", "RELEASE"}
	clusterProvisionColumns          = []string{"NAME", "CLUSTERDEPLOYMENT", "STAGE", "INFRAID"}
	syncSetInstanceColumns           = []string{"NAME", "APPLIED"}
)

// AddHandlers adds print handlers for Hive v1alpha1 API objects
func AddHandlers(p kprinters.PrintHandler) {
	p.Handler(onlyNameColumns, nil, printCheckpoint)
	p.Handler(onlyNameColumns, nil, printCheckpointList)
	p.Handler(clusterDeploymentColumns, nil, printClusterDeployment)
	p.Handler(clusterDeploymentColumns, nil, printClusterDeploymentList)
	p.Handler(clusterDeprovisionRequestColumns, nil, printClusterDeprovisionRequest)
	p.Handler(clusterDeprovisionRequestColumns, nil, printClusterDeprovisionRequestList)
	p.Handler(clusterImageSetColumns, nil, printClusterImageSet)
	p.Handler(clusterImageSetColumns, nil, printClusterImageSetList)
	p.Handler(clusterProvisionColumns, nil, printClusterProvision)
	p.Handler(clusterProvisionColumns, nil, printClusterProvisionList)
	p.Handler(onlyNameColumns, nil, printClusterState)
	p.Handler(onlyNameColumns, nil, printClusterStateList)
	p.Handler(onlyNameColumns, nil, printDNSZone)
	p.Handler(onlyNameColumns, nil, printDNSZoneList)
	p.Handler(onlyNameColumns, nil, printHiveConfig)
	p.Handler(onlyNameColumns, nil, printHiveConfigList)
	p.Handler(onlyNameColumns, nil, printSyncIdentityProvider)
	p.Handler(onlyNameColumns, nil, printSyncIdentityProviderList)
	p.Handler(onlyNameColumns, nil, printSelectorSyncIdentityProvider)
	p.Handler(onlyNameColumns, nil, printSelectorSyncIdentityProviderList)
	p.Handler(onlyNameColumns, nil, printSyncSet)
	p.Handler(onlyNameColumns, nil, printSyncSetList)
	p.Handler(onlyNameColumns, nil, printSelectorSyncSet)
	p.Handler(onlyNameColumns, nil, printSelectorSyncSetList)
	p.Handler(syncSetInstanceColumns, nil, printSyncSetInstance)
	p.Handler(syncSetInstanceColumns, nil, printSyncSetInstanceList)
}

// formatResourceName receives a resource kind, name, and boolean specifying
// whether or not to update the current name to "kind/name"
func formatResourceName(kind schema.GroupKind, name string, withKind bool) string {
	if !withKind || kind.Empty() {
		return name
	}
	return strings.ToLower(kind.String()) + "/" + name
}

func appendItemLabels(itemLabels map[string]string, w io.Writer, columnLabels []string, showLabels bool) error {
	if _, err := fmt.Fprint(w, kprinters.AppendLabels(itemLabels, columnLabels)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, kprinters.AppendAllLabels(showLabels, itemLabels)); err != nil {
		return err
	}
	return nil
}

func printOnlyName(obj metav1.Object, w io.Writer, opts kprinters.PrintOptions) error {
	if opts.WithNamespace {
		if _, err := fmt.Fprintf(w, "%v\t", obj.GetNamespace()); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, formatResourceName(opts.Kind, obj.GetName(), opts.WithKind)); err != nil {
		return err
	}
	if err := appendItemLabels(obj.GetLabels(), w, opts.ColumnLabels, opts.ShowLabels); err != nil {
		return err
	}
	return nil
}

func printOnlyNameList(list runtime.Object, w io.Writer, opts kprinters.PrintOptions) error {
	return meta.EachListItem(list, func(obj runtime.Object) error {
		return printOnlyName(obj.(metav1.Object), w, opts)
	})
}

func printCheckpoint(checkpoint *hiveapi.Checkpoint, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(checkpoint, w, opts)
}

func printCheckpointList(checkpointList *hiveapi.CheckpointList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(checkpointList, w, opts)
}

func printClusterDeployment(clusterDeployment *hiveapi.ClusterDeployment, w io.Writer, opts kprinters.PrintOptions) error {
	if opts.WithNamespace {
		if _, err := fmt.Fprintf(w, "%v\t", clusterDeployment.Namespace); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v",
		formatResourceName(opts.Kind, clusterDeployment.Name, opts.WithKind),
		clusterDeployment.Spec.ClusterName,
		clusterDeployment.Labels[hiveapi.HiveClusterTypeLabel],
		clusterDeployment.Spec.BaseDomain,
		clusterDeployment.Spec.Installed,
		clusterDeployment.Status.InfraID,
		fmt.Sprintf("%v ago", formatRelativeTime(clusterDeployment.CreationTimestamp.Time)),
	); err != nil {
		return err
	}
	if err := appendItemLabels(clusterDeployment.Labels, w, opts.ColumnLabels, opts.ShowLabels); err != nil {
		return err
	}
	return nil
}

func printClusterDeploymentList(clusterDeploymentList *hiveapi.ClusterDeploymentList, w io.Writer, opts kprinters.PrintOptions) error {
	for _, cd := range clusterDeploymentList.Items {
		if err := printClusterDeployment(&cd, w, opts); err != nil {
			return err
		}
	}
	return nil
}

func printClusterDeprovisionRequest(deprovisionRequest *hiveapi.ClusterDeprovisionRequest, w io.Writer, opts kprinters.PrintOptions) error {
	if opts.WithNamespace {
		if _, err := fmt.Fprintf(w, "%v\t", deprovisionRequest.Namespace); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v",
		formatResourceName(opts.Kind, deprovisionRequest.Name, opts.WithKind),
		deprovisionRequest.Spec.InfraID,
		deprovisionRequest.Spec.ClusterID,
		deprovisionRequest.Status.Completed,
		fmt.Sprintf("%v ago", formatRelativeTime(deprovisionRequest.CreationTimestamp.Time)),
	); err != nil {
		return err
	}
	if err := appendItemLabels(deprovisionRequest.Labels, w, opts.ColumnLabels, opts.ShowLabels); err != nil {
		return err
	}
	return nil
}

func printClusterDeprovisionRequestList(deprovisionRequestList *hiveapi.ClusterDeprovisionRequestList, w io.Writer, opts kprinters.PrintOptions) error {
	for _, deprovisionRequest := range deprovisionRequestList.Items {
		if err := printClusterDeprovisionRequest(&deprovisionRequest, w, opts); err != nil {
			return err
		}
	}
	return nil
}

func printClusterImageSet(imageSet *hiveapi.ClusterImageSet, w io.Writer, opts kprinters.PrintOptions) error {
	if _, err := fmt.Fprintf(w, "%v\t%v\t%v",
		formatResourceName(opts.Kind, imageSet.Name, opts.WithKind),
		emptyIfNil(imageSet.Spec.InstallerImage),
		emptyIfNil(imageSet.Spec.ReleaseImage),
	); err != nil {
		return err
	}
	if err := appendItemLabels(imageSet.Labels, w, opts.ColumnLabels, opts.ShowLabels); err != nil {
		return err
	}
	return nil
}

func printClusterImageSetList(imageSetList *hiveapi.ClusterImageSetList, w io.Writer, opts kprinters.PrintOptions) error {
	for _, imageSet := range imageSetList.Items {
		if err := printClusterImageSet(&imageSet, w, opts); err != nil {
			return err
		}
	}
	return nil
}

func printClusterProvision(clusterProvision *hiveapi.ClusterProvision, w io.Writer, opts kprinters.PrintOptions) error {
	if opts.WithNamespace {
		if _, err := fmt.Fprintf(w, "%v\t", clusterProvision.Namespace); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%v\t%v\t%v\t%v",
		formatResourceName(opts.Kind, clusterProvision.Name, opts.WithKind),
		clusterProvision.Spec.ClusterDeployment.Name,
		clusterProvision.Spec.Stage,
		emptyIfNil(clusterProvision.Spec.InfraID),
	); err != nil {
		return err
	}
	if err := appendItemLabels(clusterProvision.Labels, w, opts.ColumnLabels, opts.ShowLabels); err != nil {
		return err
	}
	return nil
}

func printClusterProvisionList(clusterProvisionList *hiveapi.ClusterProvisionList, w io.Writer, opts kprinters.PrintOptions) error {
	for _, clusterProvision := range clusterProvisionList.Items {
		if err := printClusterProvision(&clusterProvision, w, opts); err != nil {
			return err
		}
	}
	return nil
}

func printClusterState(clusterState *hiveapi.ClusterState, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(clusterState, w, opts)
}

func printClusterStateList(clusterStateList *hiveapi.ClusterStateList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(clusterStateList, w, opts)
}

func printDNSZone(dnsZone *hiveapi.DNSZone, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(dnsZone, w, opts)
}

func printDNSZoneList(dnsZoneList *hiveapi.DNSZoneList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(dnsZoneList, w, opts)
}

func printHiveConfig(hiveConfig *hiveapi.HiveConfig, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(hiveConfig, w, opts)
}

func printHiveConfigList(hiveConfigList *hiveapi.HiveConfigList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(hiveConfigList, w, opts)
}

func printSyncIdentityProvider(syncIdentityProvider *hiveapi.SyncIdentityProvider, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(syncIdentityProvider, w, opts)
}

func printSyncIdentityProviderList(syncIdentityProviderList *hiveapi.SyncIdentityProviderList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(syncIdentityProviderList, w, opts)
}

func printSelectorSyncIdentityProvider(selectorSyncIdentityProvider *hiveapi.SelectorSyncIdentityProvider, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(selectorSyncIdentityProvider, w, opts)
}

func printSelectorSyncIdentityProviderList(selectorSyncIdentityProviderList *hiveapi.SelectorSyncIdentityProviderList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(selectorSyncIdentityProviderList, w, opts)
}

func printSyncSet(syncSet *hiveapi.SyncSet, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(syncSet, w, opts)
}

func printSyncSetList(syncSetList *hiveapi.SyncSetList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(syncSetList, w, opts)
}

func printSelectorSyncSet(selectorSyncSet *hiveapi.SelectorSyncSet, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyName(selectorSyncSet, w, opts)
}

func printSelectorSyncSetList(selectorSyncSetList *hiveapi.SelectorSyncSetList, w io.Writer, opts kprinters.PrintOptions) error {
	return printOnlyNameList(selectorSyncSetList, w, opts)
}

func printSyncSetInstance(syncSetInstance *hiveapi.SyncSetInstance, w io.Writer, opts kprinters.PrintOptions) error {
	if opts.WithNamespace {
		if _, err := fmt.Fprintf(w, "%v\t", syncSetInstance.Namespace); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%v\t%v",
		formatResourceName(opts.Kind, syncSetInstance.Name, opts.WithKind),
		syncSetInstance.Status.Applied,
	); err != nil {
		return err
	}
	if err := appendItemLabels(syncSetInstance.Labels, w, opts.ColumnLabels, opts.ShowLabels); err != nil {
		return err
	}
	return nil
}

func printSyncSetInstanceList(syncSetInstanceList *hiveapi.SyncSetInstanceList, w io.Writer, opts kprinters.PrintOptions) error {
	for _, syncSetInstance := range syncSetInstanceList.Items {
		if err := printSyncSetInstance(&syncSetInstance, w, opts); err != nil {
			return err
		}
	}
	return nil
}
