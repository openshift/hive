package generated

import (
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"k8s.io/api/admission/v1.AdmissionRequest":                  schema_k8sio_api_admission_v1_AdmissionRequest(ref),
		"k8s.io/api/admission/v1.AdmissionResponse":                 schema_k8sio_api_admission_v1_AdmissionResponse(ref),
		"k8s.io/api/admission/v1.AdmissionReview":                   schema_k8sio_api_admission_v1_AdmissionReview(ref),
		"k8s.io/api/admission/v1beta1.AdmissionRequest":             schema_k8sio_api_admission_v1beta1_AdmissionRequest(ref),
		"k8s.io/api/admission/v1beta1.AdmissionResponse":            schema_k8sio_api_admission_v1beta1_AdmissionResponse(ref),
		"k8s.io/api/admission/v1beta1.AdmissionReview":              schema_k8sio_api_admission_v1beta1_AdmissionReview(ref),
		"k8s.io/api/authentication/v1.UserInfo":                     schema_k8sio_api_authentication_v1_UserInfo(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind":     schema_pkg_apis_meta_v1_GroupVersionKind(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource": schema_pkg_apis_meta_v1_GroupVersionResource(ref),
		"k8s.io/apimachinery/pkg/runtime.RawExtension":              schema_k8sio_apimachinery_pkg_runtime_RawExtension(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.Status":               schema_pkg_apis_meta_v1_Status(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta":             schema_pkg_apis_meta_v1_ListMeta(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.StatusDetails":        schema_pkg_apis_meta_v1_StatusDetails(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.StatusCause":          schema_pkg_apis_meta_v1_StatusCause(ref),
	}
}

func schema_k8sio_api_admission_v1_AdmissionRequest(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "AdmissionRequest describes the admission.Attributes for the admission request.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID is an identifier for the individual request/response. It allows us to distinguish instances of requests which are otherwise identical (parallel requests, requests when earlier requests did not modify etc) The UID is meant to track the round trip (request/response) between the KAS and the WebHook, not the user request. It is suitable for correlating log entries between the webhook and apiserver, for either auditing or debugging.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is the fully-qualified type of object being submitted (for example, v1.Pod or autoscaling.v1.Scale)",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind"),
						},
					},
					"resource": {
						SchemaProps: spec.SchemaProps{
							Description: "Resource is the fully-qualified resource being requested (for example, v1.pods)",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource"),
						},
					},
					"subResource": {
						SchemaProps: spec.SchemaProps{
							Description: "SubResource is the subresource being requested, if any (for example, \"status\" or \"scale\")",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"requestKind": {
						SchemaProps: spec.SchemaProps{
							Description: "RequestKind is the fully-qualified type of the original API request (for example, v1.Pod or autoscaling.v1.Scale). If this is specified and differs from the value in \"kind\", an equivalent match and conversion was performed.\n\nFor example, if deployments can be modified via apps/v1 and apps/v1beta1, and a webhook registered a rule of `apiGroups:[\"apps\"], apiVersions:[\"v1\"], resources: [\"deployments\"]` and `matchPolicy: Equivalent`, an API request to apps/v1beta1 deployments would be converted and sent to the webhook with `kind: {group:\"apps\", version:\"v1\", kind:\"Deployment\"}` (matching the rule the webhook registered for), and `requestKind: {group:\"apps\", version:\"v1beta1\", kind:\"Deployment\"}` (indicating the kind of the original API request).\n\nSee documentation for the \"matchPolicy\" field in the webhook configuration type for more details.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind"),
						},
					},
					"requestResource": {
						SchemaProps: spec.SchemaProps{
							Description: "RequestResource is the fully-qualified resource of the original API request (for example, v1.pods). If this is specified and differs from the value in \"resource\", an equivalent match and conversion was performed.\n\nFor example, if deployments can be modified via apps/v1 and apps/v1beta1, and a webhook registered a rule of `apiGroups:[\"apps\"], apiVersions:[\"v1\"], resources: [\"deployments\"]` and `matchPolicy: Equivalent`, an API request to apps/v1beta1 deployments would be converted and sent to the webhook with `resource: {group:\"apps\", version:\"v1\", resource:\"deployments\"}` (matching the resource the webhook registered for), and `requestResource: {group:\"apps\", version:\"v1beta1\", resource:\"deployments\"}` (indicating the resource of the original API request).\n\nSee documentation for the \"matchPolicy\" field in the webhook configuration type.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource"),
						},
					},
					"requestSubResource": {
						SchemaProps: spec.SchemaProps{
							Description: "RequestSubResource is the name of the subresource of the original API request, if any (for example, \"status\" or \"scale\") If this is specified and differs from the value in \"subResource\", an equivalent match and conversion was performed. See documentation for the \"matchPolicy\" field in the webhook configuration type.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Name is the name of the object as presented in the request.  On a CREATE operation, the client may omit name and rely on the server to generate the name.  If that is the case, this field will contain an empty string.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"namespace": {
						SchemaProps: spec.SchemaProps{
							Description: "Namespace is the namespace associated with the request (if any).",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"operation": {
						SchemaProps: spec.SchemaProps{
							Description: "Operation is the operation being performed. This may be different than the operation requested. e.g. a patch can result in either a CREATE or UPDATE Operation.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"userInfo": {
						SchemaProps: spec.SchemaProps{
							Description: "UserInfo is information about the requesting user",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/api/authentication/v1.UserInfo"),
						},
					},
					"object": {
						SchemaProps: spec.SchemaProps{
							Description: "Object is the object from the incoming request.",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/runtime.RawExtension"),
						},
					},
					"oldObject": {
						SchemaProps: spec.SchemaProps{
							Description: "OldObject is the existing object. Only populated for DELETE and UPDATE requests.",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/runtime.RawExtension"),
						},
					},
					"dryRun": {
						SchemaProps: spec.SchemaProps{
							Description: "DryRun indicates that modifications will definitely not be persisted for this request. Defaults to false.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"options": {
						SchemaProps: spec.SchemaProps{
							Description: "Options is the operation option structure of the operation being performed. e.g. `meta.k8s.io/v1.DeleteOptions` or `meta.k8s.io/v1.CreateOptions`. This may be different than the options the caller provided. e.g. for a patch request the performed Operation might be a CREATE, in which case the Options will a `meta.k8s.io/v1.CreateOptions` even though the caller provided `meta.k8s.io/v1.PatchOptions`.",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/runtime.RawExtension"),
						},
					},
				},
				Required: []string{"uid", "kind", "resource", "operation", "userInfo"},
			},
		},
		Dependencies: []string{
			"k8s.io/api/authentication/v1.UserInfo", "k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind", "k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource", "k8s.io/apimachinery/pkg/runtime.RawExtension"},
	}
}

func schema_k8sio_api_admission_v1_AdmissionResponse(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "AdmissionResponse describes an admission response.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID is an identifier for the individual request/response. This must be copied over from the corresponding AdmissionRequest.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"allowed": {
						SchemaProps: spec.SchemaProps{
							Description: "Allowed indicates whether or not the admission request was permitted.",
							Default:     false,
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Description: "Result contains extra details into why an admission request was denied. This field IS NOT consulted in any way if \"Allowed\" is \"true\".",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Status"),
						},
					},
					"patch": {
						SchemaProps: spec.SchemaProps{
							Description: "The patch body. Currently we only support \"JSONPatch\" which implements RFC 6902.",
							Type:        []string{"string"},
							Format:      "byte",
						},
					},
					"patchType": {
						SchemaProps: spec.SchemaProps{
							Description: "The type of Patch. Currently we only allow \"JSONPatch\".",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"auditAnnotations": {
						SchemaProps: spec.SchemaProps{
							Description: "AuditAnnotations is an unstructured key value map set by remote admission controller (e.g. error=image-blacklisted). MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission controller will prefix the keys with admission webhook name (e.g. imagepolicy.example.com/error=image-blacklisted). AuditAnnotations will be provided by the admission webhook to add additional context to the audit log for this request.",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"warnings": {
						SchemaProps: spec.SchemaProps{
							Description: "warnings is a list of warning messages to return to the requesting API client. Warning messages describe a problem the client making the API request should correct or be aware of. Limit warnings to 120 characters if possible. Warnings over 256 characters and large numbers of warnings may be truncated.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
				},
				Required: []string{"uid", "allowed"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.Status"},
	}
}

func schema_k8sio_api_admission_v1_AdmissionReview(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "AdmissionReview describes an admission review request/response.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"request": {
						SchemaProps: spec.SchemaProps{
							Description: "Request describes the attributes for the admission request.",
							Ref:         ref("k8s.io/api/admission/v1.AdmissionRequest"),
						},
					},
					"response": {
						SchemaProps: spec.SchemaProps{
							Description: "Response describes the attributes for the admission response.",
							Ref:         ref("k8s.io/api/admission/v1.AdmissionResponse"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"k8s.io/api/admission/v1.AdmissionRequest", "k8s.io/api/admission/v1.AdmissionResponse"},
	}
}

func schema_k8sio_api_admission_v1beta1_AdmissionRequest(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "AdmissionRequest describes the admission.Attributes for the admission request.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID is an identifier for the individual request/response. It allows us to distinguish instances of requests which are otherwise identical (parallel requests, requests when earlier requests did not modify etc) The UID is meant to track the round trip (request/response) between the KAS and the WebHook, not the user request. It is suitable for correlating log entries between the webhook and apiserver, for either auditing or debugging.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is the fully-qualified type of object being submitted (for example, v1.Pod or autoscaling.v1.Scale)",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind"),
						},
					},
					"resource": {
						SchemaProps: spec.SchemaProps{
							Description: "Resource is the fully-qualified resource being requested (for example, v1.pods)",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource"),
						},
					},
					"subResource": {
						SchemaProps: spec.SchemaProps{
							Description: "SubResource is the subresource being requested, if any (for example, \"status\" or \"scale\")",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"requestKind": {
						SchemaProps: spec.SchemaProps{
							Description: "RequestKind is the fully-qualified type of the original API request (for example, v1.Pod or autoscaling.v1.Scale). If this is specified and differs from the value in \"kind\", an equivalent match and conversion was performed.\n\nFor example, if deployments can be modified via apps/v1 and apps/v1beta1, and a webhook registered a rule of `apiGroups:[\"apps\"], apiVersions:[\"v1\"], resources: [\"deployments\"]` and `matchPolicy: Equivalent`, an API request to apps/v1beta1 deployments would be converted and sent to the webhook with `kind: {group:\"apps\", version:\"v1\", kind:\"Deployment\"}` (matching the rule the webhook registered for), and `requestKind: {group:\"apps\", version:\"v1beta1\", kind:\"Deployment\"}` (indicating the kind of the original API request).\n\nSee documentation for the \"matchPolicy\" field in the webhook configuration type for more details.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind"),
						},
					},
					"requestResource": {
						SchemaProps: spec.SchemaProps{
							Description: "RequestResource is the fully-qualified resource of the original API request (for example, v1.pods). If this is specified and differs from the value in \"resource\", an equivalent match and conversion was performed.\n\nFor example, if deployments can be modified via apps/v1 and apps/v1beta1, and a webhook registered a rule of `apiGroups:[\"apps\"], apiVersions:[\"v1\"], resources: [\"deployments\"]` and `matchPolicy: Equivalent`, an API request to apps/v1beta1 deployments would be converted and sent to the webhook with `resource: {group:\"apps\", version:\"v1\", resource:\"deployments\"}` (matching the resource the webhook registered for), and `requestResource: {group:\"apps\", version:\"v1beta1\", resource:\"deployments\"}` (indicating the resource of the original API request).\n\nSee documentation for the \"matchPolicy\" field in the webhook configuration type.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource"),
						},
					},
					"requestSubResource": {
						SchemaProps: spec.SchemaProps{
							Description: "RequestSubResource is the name of the subresource of the original API request, if any (for example, \"status\" or \"scale\") If this is specified and differs from the value in \"subResource\", an equivalent match and conversion was performed. See documentation for the \"matchPolicy\" field in the webhook configuration type.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Name is the name of the object as presented in the request.  On a CREATE operation, the client may omit name and rely on the server to generate the name.  If that is the case, this field will contain an empty string.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"namespace": {
						SchemaProps: spec.SchemaProps{
							Description: "Namespace is the namespace associated with the request (if any).",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"operation": {
						SchemaProps: spec.SchemaProps{
							Description: "Operation is the operation being performed. This may be different than the operation requested. e.g. a patch can result in either a CREATE or UPDATE Operation.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"userInfo": {
						SchemaProps: spec.SchemaProps{
							Description: "UserInfo is information about the requesting user",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/api/authentication/v1.UserInfo"),
						},
					},
					"object": {
						SchemaProps: spec.SchemaProps{
							Description: "Object is the object from the incoming request.",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/runtime.RawExtension"),
						},
					},
					"oldObject": {
						SchemaProps: spec.SchemaProps{
							Description: "OldObject is the existing object. Only populated for DELETE and UPDATE requests.",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/runtime.RawExtension"),
						},
					},
					"dryRun": {
						SchemaProps: spec.SchemaProps{
							Description: "DryRun indicates that modifications will definitely not be persisted for this request. Defaults to false.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"options": {
						SchemaProps: spec.SchemaProps{
							Description: "Options is the operation option structure of the operation being performed. e.g. `meta.k8s.io/v1.DeleteOptions` or `meta.k8s.io/v1.CreateOptions`. This may be different than the options the caller provided. e.g. for a patch request the performed Operation might be a CREATE, in which case the Options will a `meta.k8s.io/v1.CreateOptions` even though the caller provided `meta.k8s.io/v1.PatchOptions`.",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/runtime.RawExtension"),
						},
					},
				},
				Required: []string{"uid", "kind", "resource", "operation", "userInfo"},
			},
		},
		Dependencies: []string{
			"k8s.io/api/authentication/v1.UserInfo", "k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionKind", "k8s.io/apimachinery/pkg/apis/meta/v1.GroupVersionResource", "k8s.io/apimachinery/pkg/runtime.RawExtension"},
	}
}

func schema_k8sio_api_admission_v1beta1_AdmissionResponse(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "AdmissionResponse describes an admission response.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID is an identifier for the individual request/response. This should be copied over from the corresponding AdmissionRequest.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"allowed": {
						SchemaProps: spec.SchemaProps{
							Description: "Allowed indicates whether or not the admission request was permitted.",
							Default:     false,
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Description: "Result contains extra details into why an admission request was denied. This field IS NOT consulted in any way if \"Allowed\" is \"true\".",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Status"),
						},
					},
					"patch": {
						SchemaProps: spec.SchemaProps{
							Description: "The patch body. Currently we only support \"JSONPatch\" which implements RFC 6902.",
							Type:        []string{"string"},
							Format:      "byte",
						},
					},
					"patchType": {
						SchemaProps: spec.SchemaProps{
							Description: "The type of Patch. Currently we only allow \"JSONPatch\".",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"auditAnnotations": {
						SchemaProps: spec.SchemaProps{
							Description: "AuditAnnotations is an unstructured key value map set by remote admission controller (e.g. error=image-blacklisted). MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission controller will prefix the keys with admission webhook name (e.g. imagepolicy.example.com/error=image-blacklisted). AuditAnnotations will be provided by the admission webhook to add additional context to the audit log for this request.",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"warnings": {
						SchemaProps: spec.SchemaProps{
							Description: "warnings is a list of warning messages to return to the requesting API client. Warning messages describe a problem the client making the API request should correct or be aware of. Limit warnings to 120 characters if possible. Warnings over 256 characters and large numbers of warnings may be truncated.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
				},
				Required: []string{"uid", "allowed"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.Status"},
	}
}

func schema_k8sio_api_admission_v1beta1_AdmissionReview(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "AdmissionReview describes an admission review request/response.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"request": {
						SchemaProps: spec.SchemaProps{
							Description: "Request describes the attributes for the admission request.",
							Ref:         ref("k8s.io/api/admission/v1beta1.AdmissionRequest"),
						},
					},
					"response": {
						SchemaProps: spec.SchemaProps{
							Description: "Response describes the attributes for the admission response.",
							Ref:         ref("k8s.io/api/admission/v1beta1.AdmissionResponse"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"k8s.io/api/admission/v1beta1.AdmissionRequest", "k8s.io/api/admission/v1beta1.AdmissionResponse"},
	}
}

func schema_k8sio_api_authentication_v1_UserInfo(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "UserInfo holds the information about the user needed to implement the user.Info interface.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"username": {
						SchemaProps: spec.SchemaProps{
							Description: "The name that uniquely identifies this user among all active users.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "A unique value that identifies this user across time. If this user is deleted and another user by the same name is added, they will have different UIDs.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"groups": {
						SchemaProps: spec.SchemaProps{
							Description: "The names of groups this user is a part of.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"extra": {
						SchemaProps: spec.SchemaProps{
							Description: "Any additional information provided by the authenticator.",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Type: []string{"array"},
										Items: &spec.SchemaOrArray{
											Schema: &spec.Schema{
												SchemaProps: spec.SchemaProps{
													Default: "",
													Type:    []string{"string"},
													Format:  "",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func schema_pkg_apis_meta_v1_GroupVersionKind(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "GroupVersionKind unambiguously identifies a kind.  It doesn't anonymously include GroupVersion to avoid automatic coercion.  It doesn't use a GroupVersion to avoid custom marshalling",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"group": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
					"version": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
					"kind": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
				},
				Required: []string{"group", "version", "kind"},
			},
		},
	}
}

func schema_pkg_apis_meta_v1_GroupVersionResource(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "GroupVersionResource unambiguously identifies a resource.  It doesn't anonymously include GroupVersion to avoid automatic coercion.  It doesn't use a GroupVersion to avoid custom marshalling",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"group": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
					"version": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
					"resource": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
				},
				Required: []string{"group", "version", "resource"},
			},
		},
	}
}

func schema_k8sio_apimachinery_pkg_runtime_RawExtension(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "RawExtension is used to hold extensions in external versions.\n\nTo use this, make a field which has RawExtension as its type in your external, versioned struct, and Object in your internal struct. You also need to register your various plugin types.\n\n// Internal package:\n\n\ttype MyAPIObject struct {\n\t\truntime.TypeMeta `json:\",inline\"`\n\t\tMyPlugin runtime.Object `json:\"myPlugin\"`\n\t}\n\n\ttype PluginA struct {\n\t\tAOption string `json:\"aOption\"`\n\t}\n\n// External package:\n\n\ttype MyAPIObject struct {\n\t\truntime.TypeMeta `json:\",inline\"`\n\t\tMyPlugin runtime.RawExtension `json:\"myPlugin\"`\n\t}\n\n\ttype PluginA struct {\n\t\tAOption string `json:\"aOption\"`\n\t}\n\n// On the wire, the JSON will look something like this:\n\n\t{\n\t\t\"kind\":\"MyAPIObject\",\n\t\t\"apiVersion\":\"v1\",\n\t\t\"myPlugin\": {\n\t\t\t\"kind\":\"PluginA\",\n\t\t\t\"aOption\":\"foo\",\n\t\t},\n\t}\n\nSo what happens? Decode first uses json or yaml to unmarshal the serialized data into your external MyAPIObject. That causes the raw JSON to be stored, but not unpacked. The next step is to copy (using pkg/conversion) into the internal struct. The runtime package's DefaultScheme has conversion functions installed which will unpack the JSON stored in RawExtension, turning it into the correct object type, and storing it in the Object. (TODO: In the case where the object is of an unknown type, a runtime.Unknown object will be created and stored.)",
				Type:        []string{"object"},
			},
		},
	}
}

func schema_pkg_apis_meta_v1_Status(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "Status is a return value for calls that don't return other objects.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Description: "Standard list metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Description: "Status of the operation. One of: \"Success\" or \"Failure\". More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"message": {
						SchemaProps: spec.SchemaProps{
							Description: "A human-readable description of the status of this operation.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"reason": {
						SchemaProps: spec.SchemaProps{
							Description: "A machine-readable description of why this operation is in the \"Failure\" status. If this value is empty there is no information available. A Reason clarifies an HTTP status code but does not override it.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"details": {
						SchemaProps: spec.SchemaProps{
							Description: "Extended data associated with the reason.  Each reason may define its own extended details. This field is optional and the data returned is not guaranteed to conform to any schema except that defined by the reason type.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.StatusDetails"),
						},
					},
					"code": {
						SchemaProps: spec.SchemaProps{
							Description: "Suggested HTTP return code for this status, 0 if not set.",
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
				},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta", "k8s.io/apimachinery/pkg/apis/meta/v1.StatusDetails"},
	}
}

func schema_pkg_apis_meta_v1_ListMeta(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ListMeta describes metadata that synthetic resources must have, including lists and various status objects. A resource may have only one of {ObjectMeta, ListMeta}.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"selfLink": {
						SchemaProps: spec.SchemaProps{
							Description: "Deprecated: selfLink is a legacy read-only field that is no longer populated by the system.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"resourceVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "String that identifies the server's internal version of this object that can be used by clients to determine when objects have changed. Value must be treated as opaque by clients and passed unmodified back to the server. Populated by the system. Read-only. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"continue": {
						SchemaProps: spec.SchemaProps{
							Description: "continue may be set if the user set a limit on the number of items returned, and indicates that the server has more data available. The value is opaque and may be used to issue another request to the endpoint that served this list to retrieve the next set of available objects. Continuing a consistent list may not be possible if the server configuration has changed or more than a few minutes have passed. The resourceVersion field returned when using this continue value will be identical to the value in the first response, unless you have received this token from an error message.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"remainingItemCount": {
						SchemaProps: spec.SchemaProps{
							Description: "remainingItemCount is the number of subsequent items in the list which are not included in this list response. If the list request contained label or field selectors, then the number of remaining items is unknown and the field will be left unset and omitted during serialization. If the list is complete (either because it is not chunking or because this is the last chunk), then there are no more remaining items and this field will be left unset and omitted during serialization. Servers older than v1.15 do not set this field. The intended use of the remainingItemCount is *estimating* the size of a collection. Clients should not rely on the remainingItemCount to be set or to be exact.",
							Type:        []string{"integer"},
							Format:      "int64",
						},
					},
				},
			},
		},
	}
}

func schema_pkg_apis_meta_v1_StatusDetails(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "StatusDetails is a set of additional properties that MAY be set by the server to provide additional information about a response. The Reason field of a Status object defines what attributes will be set. Clients must ignore fields that do not match the defined type of each attribute, and should assume that any attribute may be empty, invalid, or under defined.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "The name attribute of the resource associated with the status StatusReason (when there is a single name which can be described).",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"group": {
						SchemaProps: spec.SchemaProps{
							Description: "The group attribute of the resource associated with the status StatusReason.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "The kind attribute of the resource associated with the status StatusReason. On some operations may differ from the requested resource Kind. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID of the resource. (when there is a single resource which can be described). More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names#uids",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"causes": {
						SchemaProps: spec.SchemaProps{
							Description: "The Causes array includes more details associated with the StatusReason failure. Not all StatusReasons may provide detailed causes.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.StatusCause"),
									},
								},
							},
						},
					},
					"retryAfterSeconds": {
						SchemaProps: spec.SchemaProps{
							Description: "If specified, the time in seconds before the operation should be retried. Some errors may indicate the client must take an alternate action - for those errors this field may indicate how long to wait before taking the alternate action.",
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
				},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.StatusCause"},
	}
}

func schema_pkg_apis_meta_v1_StatusCause(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "StatusCause provides more information about an api.Status failure, including cases when multiple errors are encountered.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"reason": {
						SchemaProps: spec.SchemaProps{
							Description: "A machine-readable description of the cause of the error. If this value is empty there is no information available.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"message": {
						SchemaProps: spec.SchemaProps{
							Description: "A human-readable description of the cause of the error.  This field may be presented as-is to a reader.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"field": {
						SchemaProps: spec.SchemaProps{
							Description: "The field of the resource that has caused this error, as named by its JSON serialization. May include dot and postfix notation for nested attributes. Arrays are zero-indexed.  Fields may appear more than once in an array of causes due to fields having multiple errors. Optional.\n\nExamples:\n  \"name\" - the field \"name\" on the current resource\n  \"items[0].name\" - the field \"name\" on the first array entry in \"items\"",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
			},
		},
	}
}
