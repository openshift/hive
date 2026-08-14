package hive

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
)

func TestStatefulSetTemplateIncludesDefaultableFields(t *testing.T) {
	for _, controllerName := range []string{"clustersync", "machinepool"} {
		t.Run(controllerName, func(t *testing.T) {
			templateValues := map[string]string{
				"ControllerName": controllerName,
			}
			processed, err := controllerutils.ProcessAssetTemplate(
				assets.MustAsset("config/sharded_controllers/statefulset.yaml"),
				templateValues,
			)
			if err != nil {
				t.Fatalf("failed to process template: %v", err)
			}

			ss := readRuntimeObjectOrDie[*appsv1.StatefulSet](appsv1.SchemeGroupVersion, processed)

			if ss.Spec.PodManagementPolicy != appsv1.OrderedReadyPodManagement {
				t.Errorf("expected PodManagementPolicy=%q, got %q",
					appsv1.OrderedReadyPodManagement, ss.Spec.PodManagementPolicy)
			}

			if ss.Spec.RevisionHistoryLimit == nil || *ss.Spec.RevisionHistoryLimit != 10 {
				t.Errorf("expected RevisionHistoryLimit=10, got %v", ss.Spec.RevisionHistoryLimit)
			}

			if ss.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
				t.Errorf("expected UpdateStrategy.Type=%q, got %q",
					appsv1.RollingUpdateStatefulSetStrategyType, ss.Spec.UpdateStrategy.Type)
			}

			if ss.Spec.UpdateStrategy.RollingUpdate == nil ||
				ss.Spec.UpdateStrategy.RollingUpdate.Partition == nil ||
				*ss.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
				t.Error("expected UpdateStrategy.RollingUpdate.Partition=0")
			}
		})
	}
}
