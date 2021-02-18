package constants

import (
	"math/rand"
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	testNamespace = "default"
)

func TestGetMergedPullSecretName(t *testing.T) {
	shortName := randSeq(rand.Intn(validation.DNS1123SubdomainMaxLength-1) + 1)
	longName := randSeq(validation.DNS1123SubdomainMaxLength + rand.Intn(100))
	tests := []struct {
		name string
		cd   *hivev1.ClusterDeployment
	}{
		{
			name: "GetMergedPullSecretName should return same name everytime with long name",
			cd: func() *hivev1.ClusterDeployment {
				cd := getClusterDeployment(longName)
				return cd
			}(),
		},
		{
			name: "GetMergedPullSecretName should return same name everytime with short name",
			cd: func() *hivev1.ClusterDeployment {
				cd := getClusterDeployment(shortName)
				return cd
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			genratedName1 := GetMergedPullSecretName(test.cd)
			genratedName2 := GetMergedPullSecretName(test.cd)
			assert.Equal(t, genratedName1, genratedName2)
		})
	}
}

// From k8s.io/kubernetes/pkg/api/generator.go
var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789-")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func getClusterDeployment(testName string) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testName,
			Namespace:   testNamespace,
			Finalizers:  []string{hivev1.FinalizerDeprovision},
			UID:         types.UID("1234"),
			Annotations: map[string]string{},
		},
	}
	return cd
}
