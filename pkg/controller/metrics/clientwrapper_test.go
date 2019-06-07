package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathParse(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "core pods list",
			path:     "/api/v1/pods",
			expected: "core/v1/pods",
		},
		{
			name:     "core global nodes update",
			path:     "/api/v1/nodes/nodename",
			expected: "core/v1/nodes",
		},
		{
			name:     "core configmaps update",
			path:     "/api/v1/namespaces/hive/configmaps/dgoodwin-del-install-log",
			expected: "core/v1/configmaps",
		},
		{
			name:     "batch job list",
			path:     "/apis/batch/v1/jobs",
			expected: "batch/v1/jobs",
		},
		{
			name:     "batch job create",
			path:     "/apis/batch/v1/namespaces/hive/jobs",
			expected: "batch/v1/jobs",
		},
		{
			name:     "batch job delete",
			path:     "/apis/batch/v1/namespaces/hive/jobs/dgoodwin-del-install",
			expected: "batch/v1/jobs",
		},
		{
			name:     "hive global crd list",
			path:     "/apis/hive.openshift.io/v1alpha1/selectorsyncidentityproviders",
			expected: "hive.openshift.io/v1alpha1/selectorsyncidentityproviders",
		},
		{
			name:     "hive global crd update",
			path:     "/apis/hive.openshift.io/v1alpha1/selectorsyncidentityproviders/ssname",
			expected: "hive.openshift.io/v1alpha1/selectorsyncidentityproviders",
		},
		{
			name:     "hive namespaced crd create",
			path:     "/apis/hive.openshift.io/v1alpha1/namespaces/hive/clusterdeprovisionrequests",
			expected: "hive.openshift.io/v1alpha1/clusterdeprovisionrequests",
		},
		{
			name:     "hive namespaced crd update",
			path:     "/apis/hive.openshift.io/v1alpha1/namespaces/hive/clusterdeployments/dgoodwin-del",
			expected: "hive.openshift.io/v1alpha1/clusterdeployments",
		},
		{
			name:     "hive namespaced crd update status",
			path:     "/apis/hive.openshift.io/v1alpha1/namespaces/hive/clusterdeployments/dgoodwin-del/status",
			expected: "hive.openshift.io/v1alpha1/clusterdeployments",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parsePath(test.path)
			assert.Equal(t, test.expected, result)
		})
	}

}
