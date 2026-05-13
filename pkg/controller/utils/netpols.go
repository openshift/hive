package utils

import (
	"context"

	"github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// EnsureNetworkPolicy makes sure a network policy exists (possibly creating it):
//   - In the cd's namespace
//   - Named after the cd and jobType
//   - Parented to the cd
//     (Note: Would rather parent to the Job, but then we have a chicken/egg race.)
//     (...or the Cluster{Dep|P}rovision, but then we would have one for each attempt.)
//   - With a PodSelector for the jobType's pod
//   - With the specified ingress and egress (note: We specify both PolicyTypes, so the
//     caller must be explicit about both)
//
// The bool return indicates whether we created it (true) or it already existed (false).
// If the error return is non-nil, the bool return is meaningless.
func EnsureNetworkPolicy(cl client.Client, scheme *runtime.Scheme, jobType string, cd *hivev1.ClusterDeployment, logger log.FieldLogger, ingress []v1.NetworkPolicyIngressRule, egress []v1.NetworkPolicyEgressRule) (bool, error) {
	// CD-specific name, scrubbed for length, with suffix intact
	name := helpers.GetResourceName(cd.Name, jobType)

	logger = logger.WithField("netpol", cd.Namespace+"/"+name)

	err := cl.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: name}, &v1.NetworkPolicy{})
	if err == nil {
		// TODO: Should we validate it?
		logger.Debug("network policy already exists")
		return false, nil
	}
	if !apierrors.IsNotFound(err) {
		return false, errors.Wrap(err, "failed to retrieve network policy")
	}

	// Not found; create.
	labels := map[string]string{
		constants.JobTypeLabel: jobType,
	}
	np := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: labels},
			Ingress:     ingress,
			Egress:      egress,
			PolicyTypes: []v1.PolicyType{
				v1.PolicyTypeEgress,
				v1.PolicyTypeIngress,
			},
		},
	}
	if err = controllerutil.SetControllerReference(cd, np, scheme); err != nil {
		return false, errors.Wrap(err, "failed to set controller reference on network policy")
	}

	logger.Info("creating network policy")
	err = cl.Create(context.Background(), np)
	if apierrors.IsAlreadyExists(err) {
		// We lost a race, but the important part is that the netpol exists
		logger.Info("network policy became extant while creating")
		return false, nil
	}
	// err is nil in the green path (create succeeded)
	return err == nil, err
}
