package v1migration

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/apis"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// V1MigrationOptions is the set of options for the migration tooling.
type V1MigrationOptions struct {
}

// NewV1MigrationCommand creates a command that executes the migration utilities.
func NewV1MigrationCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "v1migration",
		Short: "Remove owner refs for secrets owned by Hive objects. (admin kubeconfig/password)",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewRemoveSecretOwnerRefsCommand())
	return cmd
}

// NewRemoveSecretOwnerRefsCommand creates a command that executes the migration utilities.
func NewRemoveSecretOwnerRefsCommand() *cobra.Command {

	opt := &V1MigrationOptions{}
	cmd := &cobra.Command{
		Use: "remove-secret-owner-refs",
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				return
			}

			if err := opt.Validate(cmd); err != nil {
				return
			}

			dynClient, err := contributils.GetClient()
			if err != nil {
				log.WithError(err).Fatal("error creating kube clients")
			}

			err = opt.Run(dynClient)
			if err != nil {
				log.WithError(err).Error("Error")
			}
		},
	}
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *V1MigrationOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *V1MigrationOptions) Validate(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (o *V1MigrationOptions) Run(dynClient client.Client) error {
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return err
	}

	provisions := &hivev1alpha1.ClusterProvisionList{}
	err := dynClient.List(context.Background(), provisions)
	if err != nil {
		log.WithError(err).Fatal("error listing cluster provisions")
	}
	fmt.Printf("Loaded %d total cluster provisions\n", len(provisions.Items))
	for _, p := range provisions.Items {
		removeSecretOwnerReference(dynClient, p.Namespace, p.Spec.AdminKubeconfigSecret)
		removeSecretOwnerReference(dynClient, p.Namespace, p.Spec.AdminPasswordSecret)
	}

	return nil
}

func removeSecretOwnerReference(dynClient client.Client, namespace string, lRef *corev1.LocalObjectReference) {
	if lRef != nil {
		s := &corev1.Secret{}
		nsn := types.NamespacedName{
			Namespace: namespace,
			Name:      lRef.Name,
		}
		sLog := log.WithField("secret", fmt.Sprintf("%s/%s", nsn.Namespace, nsn.Name))
		err := dynClient.Get(context.Background(), nsn, s)
		if err != nil {
			sLog.WithError(err).Fatal("error getting secret")
		}
		if len(s.OwnerReferences) == 0 {
			sLog.Info("no owner references to clear")
			return
		}
		sLog.Info("removing owner references")
		sCopy := s.DeepCopy()
		sCopy.OwnerReferences = []metav1.OwnerReference{}
		err = dynClient.Update(context.Background(), sCopy)
		if err != nil {
			sLog.WithError(err).Fatal("error getting secret")
		}
	}
}
