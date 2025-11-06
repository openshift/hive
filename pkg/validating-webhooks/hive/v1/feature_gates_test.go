package v1

import (
	"encoding/json"
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func Test_existsOnlyWhenFeatureGate(t *testing.T) {
	cases := []struct {
		name string

		obj          string
		enabledGates []string
		field        string
		err          string
	}{{
		name: "single level field, not exists",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      }
   }
}`,
		enabledGates: []string{},
		field:        `spec.allowedOnFeatureGate`,
	}, {
		name: "single level field, exists but gate not enabled",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      },
      "allowedOnFeatureGate": "value"
   }
}`,
		enabledGates: []string{},
		field:        `spec.allowedOnFeatureGate`,
		err:          `^spec\.allowedOnFeatureGate: Forbidden: should only be set when feature gate test_feature_gate is enabled$`,
	}, {
		name: "single level field, exists and gate enabled",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      },
      "allowedOnFeatureGate": "value"
   }
}`,
		enabledGates: []string{"test_feature_gate"},
		field:        `spec.allowedOnFeatureGate`,
	}, {
		name: "multi level field, does not exist",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      }
   }
}`,
		enabledGates: []string{},
		field:        `spec.anotherAlwaysAllowedKey.allowedOnFeatureGate`,
	}, {
		name: "multi level field, does not exist but some of the path does",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      },
      "anotherAlwaysAllowedKey": {
         "some": "value"
      }
   }
}`,
		enabledGates: []string{},
		field:        `spec.anotherAlwaysAllowedKey.allowedOnFeatureGate`,
	}, {
		name: "multi level field, exists but gate not enabled",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      },
      "anotherAlwaysAllowedKey": {
		 "some": "value",
		 "allowedOnFeatureGate": {
            "some": "value"
         }
      }
   }
}`,
		enabledGates: []string{},
		field:        `spec.anotherAlwaysAllowedKey.allowedOnFeatureGate`,
		err:          `^spec\.anotherAlwaysAllowedKey\.allowedOnFeatureGate: Forbidden: should only be set when feature gate test_feature_gate is enabled$`,
	}, {
		name: "multi level field, exists and gate enabled",

		obj: `{
   "apiVersion": "v1",
   "kind": "Test",
   "spec": {
      "alwaysAllowedKey": {
         "some": "value"
      },
      "anotherAlwaysAllowedKey": {
         "some": "value",
         "allowedOnFeatureGate": {
            "some": "value"
         }
      }
   }
}`,
		enabledGates: []string{"test_feature_gate"},
		field:        `spec.anotherAlwaysAllowedKey.allowedOnFeatureGate`,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fs := &featureSet{FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{Enabled: test.enabledGates}}
			obj := &unstructured.Unstructured{}
			err := json.Unmarshal([]byte(test.obj), &obj.Object)
			require.NoError(t, err)

			got := existsOnlyWhenFeatureGate(fs, obj, test.field, "test_feature_gate")
			if test.err == "" {
				assert.NoError(t, got.ToAggregate())
			} else {
				assert.Regexp(t, test.err, got.ToAggregate())
			}
		})
	}
}

func Test_equalOnlyWhenFeatureGate(t *testing.T) {
	cases := []struct {
		name string

		obj          string
		enabledGates []string
		field        string
		value        any

		err string
	}{{
		name: "single level field, not exists",

		obj: `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "alwaysAllowedKey": {
			 "some": "value"
		  }
	   }
	}`,
		enabledGates: []string{},
		field:        `spec.allowedOnFeatureGate`,
	}, {
		name: "single level field, exists and set to allowed value",

		obj: `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "alwaysAllowedKey": {
			 "some": "value"
		  },
		  "allowedOnFeatureGate": "allowed-value"
	   }
	}`,
		enabledGates: []string{},
		field:        `spec.allowedOnFeatureGate`,
		value:        "test-value",
	}, {
		name: "single level field, exists and set to value that requires feature gate (string)",

		obj: `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "alwaysAllowedKey": {
			 "some": "value"
		  },
		  "allowedOnFeatureGate": "test-value"
	   }
	}`,
		enabledGates: []string{},
		field:        `spec.allowedOnFeatureGate`,
		value:        "test-value",

		err: `^spec\.allowedOnFeatureGate: Unsupported value: "test-value"$`,
	}, {
		name: "single level field, exists and set to value that requires feature gate (int)",

		obj: `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "alwaysAllowedKey": {
			 "some": "value"
		  },
		  "allowedOnFeatureGate": 10
	   }
	}`,
		enabledGates: []string{},
		field:        `spec.allowedOnFeatureGate`,
		value:        10,

		err: `^spec.allowedOnFeatureGate: Unsupported value: 10$`,
	}, {
		name: "single level field, exists and set to value that requires feature gate and enabled",

		obj: `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "alwaysAllowedKey": {
			 "some": "value"
		  },
		  "allowedOnFeatureGate": "test-value"
	   }
	}`,
		enabledGates: []string{"test_feature_gate"},
		field:        `spec.allowedOnFeatureGate`,
		value:        "test-value",
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fs := &featureSet{FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{Enabled: test.enabledGates}}
			obj := &unstructured.Unstructured{}
			err := json.Unmarshal([]byte(test.obj), &obj.Object)
			require.NoError(t, err)

			got := equalOnlyWhenFeatureGate(fs, obj, test.field, "test_feature_gate", test.value)
			if test.err == "" {
				assert.NoError(t, got.ToAggregate())
			} else {
				assert.Regexp(t, test.err, got.ToAggregate())
			}
		})
	}
}

func TestUnionDiscrimatorSpecialPlatforms(t *testing.T) {
	validateF := func(fs *featureSet, obj *unstructured.Unstructured) error {
		errs := field.ErrorList{}

		// ensure platformA is only set when AlphaPlatformAEnabled
		errs = append(errs, equalOnlyWhenFeatureGate(fs, obj, "spec.platform.type", "AlphaPlatformAEnabled", "platformA")...)
		errs = append(errs, existsOnlyWhenFeatureGate(fs, obj, "spec.platform.platformA", "AlphaPlatformAEnabled")...)

		// ensure platformB is only set when AlphaPlatformBEnabled
		errs = append(errs, equalOnlyWhenFeatureGate(fs, obj, "spec.platform.type", "AlphaPlatformBEnabled", "platformB")...)
		errs = append(errs, existsOnlyWhenFeatureGate(fs, obj, "spec.platform.platformB", "AlphaPlatformBEnabled")...)

		return errs.ToAggregate()
	}

	// using supported platform
	fs := &featureSet{FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{Enabled: []string{}}}
	objR := `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "platform": {
			"type": "supportedPlatform",
		  	"supportedPlatform": {
			  "config-1": "value"
			}
		  }
	   }
	}`
	obj := &unstructured.Unstructured{}
	err := json.Unmarshal([]byte(objR), &obj.Object)
	require.NoError(t, err)
	err = validateF(fs, obj)
	require.NoError(t, err)

	// using unsupported platform without feature gate
	fs = &featureSet{FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{Enabled: []string{}}}
	objR = `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "platform": {
			"type": "platformA",
		  	"platformA": {
			  "config-1": "value"
			}
		  }
	   }
	}`
	obj = &unstructured.Unstructured{}
	err = json.Unmarshal([]byte(objR), &obj.Object)
	require.NoError(t, err)
	err = validateF(fs, obj)
	require.EqualError(t, err, "[spec.platform.type: Unsupported value: \"platformA\", spec.platform.platformA: Forbidden: should only be set when feature gate AlphaPlatformAEnabled is enabled]")

	// using unsupported platform with feature gate
	fs = &featureSet{FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{Enabled: []string{"AlphaPlatformBEnabled"}}}
	objR = `{
	   "apiVersion": "v1",
	   "kind": "Test",
	   "spec": {
		  "platform": {
			"type": "platformB",
		  	"platformB": {
			  "config-1": "value"
			}
		  }
	   }
	}`
	obj = &unstructured.Unstructured{}
	err = json.Unmarshal([]byte(objR), &obj.Object)
	require.NoError(t, err)
	err = validateF(fs, obj)
	require.NoError(t, err)
}
