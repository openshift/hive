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

package clusterdeployment

import (
	"context"
	"time"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/installer/pkg/rhcos"
)

const (
	hiveDefaultAMIAnnotation = "hive.openshift.io/default-AMI"
)

func isDefaultAMISet(cd *hivev1.ClusterDeployment) bool {
	defaultAMI, ok := cd.Annotations[hiveDefaultAMIAnnotation]
	if ok && defaultAMI != "" {
		return true
	}
	return false
}

func setDefaultAMI(cd *hivev1.ClusterDeployment, ami string) {
	if cd.Annotations == nil {
		cd.Annotations = map[string]string{}
	}
	cd.Annotations[hiveDefaultAMIAnnotation] = ami
}

// lookupAMI uses installer code to lookup the latest AMI from the RHCOS webapp.
func lookupAMI(cd *hivev1.ClusterDeployment) (string, error) {
	// In future this will hopefully be a part of the release image, but for now we have
	// to do a lookup via the RHCOS pipeline API. The installer does this already so we re-use their
	// function to do so.
	ctx, cancel := context.WithTimeout(context.TODO(), 60*time.Second)
	defer cancel()
	return rhcos.AMI(ctx, rhcos.DefaultChannel, cd.Spec.AWS.Region)
}
