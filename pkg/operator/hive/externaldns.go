/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hive

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func (r *ReconcileHiveConfig) deployExternalDNS(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder) error {
	if instance.Spec.ExternalDNS == nil {
		// TODO: Add code to remove if already deployed
		hLog.Debug("external DNS is not configured in HiveConfig, it will not be deployed")
		return nil
	}

	// For now, we only support AWS
	if instance.Spec.ExternalDNS.AWS == nil {
		return fmt.Errorf("only AWS supported, AWS-specific external DNS configuration must be specified")
	}

	if len(instance.Spec.ExternalDNS.AWS.Credentials.Name) == 0 {
		return fmt.Errorf("a secret reference must be specified for AWS credentials")
	}

	asset := assets.MustAsset("config/external-dns/deployment.yaml")
	hLog.Debug("reading external-dns deployment")
	deployment := resourceread.ReadDeploymentV1OrDie(asset)

	// Make AWS-specific changes to external-dns deployment
	args := deployment.Spec.Template.Spec.Containers[0].Args
	args = append(args, "--provider=aws")
	for _, domain := range instance.Spec.ManagedDomains {
		args = append(args, fmt.Sprintf("--domain-filter=%s", domain))
	}
	deployment.Spec.Template.Spec.Containers[0].Args = args

	if len(instance.Spec.ExternalDNS.Image) > 0 {
		deployment.Spec.Template.Spec.Containers[0].Image = instance.Spec.ExternalDNS.Image
	}

	authEnvVars := []corev1.EnvVar{
		{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: instance.Spec.ExternalDNS.AWS.Credentials,
					Key:                  "aws_access_key_id",
				},
			},
		},
		{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: instance.Spec.ExternalDNS.AWS.Credentials,
					Key:                  "aws_secret_access_key",
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, authEnvVars...)

	// Apply service account
	err := util.ApplyAsset(h, "config/external-dns/service_account.yaml", hLog)
	if err != nil {
		hLog.WithError(err).Error("cannot apply asset external-dns service account")
		return err
	}

	// Apply deployment
	result, err := h.ApplyRuntimeObject(deployment, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying external-dns deployment")
		return err
	}
	hLog.Infof("external-dns deployment applied (%s)", result)

	return nil
}
